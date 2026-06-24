package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

var (
	_ resource.Resource                = &KeySpecResource{}
	_ resource.ResourceWithConfigure   = &KeySpecResource{}
	_ resource.ResourceWithImportState = &KeySpecResource{}
)

// KeySpecResource manages a Microwave key specification. v0.1 covers the
// fields a Sandbar-style workspace needs to provision: name, description,
// format axis, permission set + signing key set bindings, the format-specific
// opaque/jwt config, and the expiry policy. Claims contract, override policy,
// and webhook config are deferred to v0.2.
type KeySpecResource struct {
	client *management.Client
}

type keySpecModel struct {
	ID              types.String         `tfsdk:"id"`
	Name            types.String         `tfsdk:"name"`
	Description     types.String         `tfsdk:"description"`
	Format          types.String         `tfsdk:"format"`
	PermissionSetID types.String         `tfsdk:"permission_set_id"`
	SigningKeySetID types.String         `tfsdk:"signing_key_set_id"`
	Opaque          *opaqueConfigModel   `tfsdk:"opaque"`
	JWT             *jwtConfigModel      `tfsdk:"jwt"`
	Expiry          *expiryPolicyModel   `tfsdk:"expiry"`
	CreatedAt       types.String         `tfsdk:"created_at"`
	UpdatedAt       types.String         `tfsdk:"updated_at"`
}

type opaqueConfigModel struct {
	Prefix types.String `tfsdk:"prefix"`
}

type jwtConfigModel struct {
	Algorithm types.String `tfsdk:"algorithm"`
	Issuer    types.String `tfsdk:"issuer"`
}

type expiryPolicyModel struct {
	DefaultTTL           types.String `tfsdk:"default_ttl"`
	MaxTTL               types.String `tfsdk:"max_ttl"`
	AllowNever           types.Bool   `tfsdk:"allow_never"`
	RotationReminderDays types.Int64  `tfsdk:"rotation_reminder_days"`
}

func NewKeySpecResource() resource.Resource { return &KeySpecResource{} }

func (r *KeySpecResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_key_spec"
}

func (r *KeySpecResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *management.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *KeySpecResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave key specification: the contract for keys issued from it — format (opaque vs JWT), permissions, signing material, expiry policy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Key spec name. Unique within the workspace.",
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"format": schema.StringAttribute{
				Required:    true,
				Description: "opaque (stateful, server-verified, revocable) or jwt (stateless, client-verified). Immutable after creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("opaque", "jwt"),
				},
			},
			"permission_set_id": schema.StringAttribute{
				Optional:    true,
				Description: "Permission set bound to keys issued from this spec.",
			},
			"signing_key_set_id": schema.StringAttribute{
				Optional:    true,
				Description: "Signing key set used when format=jwt. Ignored for opaque specs.",
			},
			"opaque": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Opaque-format config. Set when format=opaque.",
				Attributes: map[string]schema.Attribute{
					"prefix": schema.StringAttribute{
						Optional:    true,
						Description: "Visible prefix on issued keys (e.g. sbr_live_).",
					},
				},
			},
			"jwt": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "JWT-format config. Set when format=jwt.",
				Attributes: map[string]schema.Attribute{
					"algorithm": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "JWT signing algorithm. Server-derived from the signing key set; leave unset.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"issuer": schema.StringAttribute{
						Computed:    true,
						Description: "Server-derived issuer URL (https://{spec-id}.microwave.sh).",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
				},
			},
			"expiry": schema.SingleNestedAttribute{
				Required:    true,
				Description: "TTL and rotation policy for keys issued from this spec.",
				Attributes: map[string]schema.Attribute{
					"default_ttl": schema.StringAttribute{
						Required:    true,
						Description: "Go duration string (e.g. 24h, 720h). Use 0s with allow_never=true to mean never.",
					},
					"max_ttl": schema.StringAttribute{
						Required:    true,
						Description: "Maximum permitted TTL at issue time.",
					},
					"allow_never": schema.BoolAttribute{
						Required:    true,
						Description: "When true, issued keys may have no expiry.",
					},
					"rotation_reminder_days": schema.Int64Attribute{
						Required:    true,
						Description: "Notify subscribers this many days before a key's expiry. 0 disables.",
					},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *KeySpecResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan keySpecModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.KeySpecs.Create(ctx, keySpecToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Create key spec failed", err.Error())
		return
	}
	keySpecFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *KeySpecResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state keySpecModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.KeySpecs.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read key spec failed", err.Error())
		return
	}
	keySpecFromWire(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *KeySpecResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan keySpecModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.KeySpecs.Update(ctx, plan.ID.ValueString(), keySpecToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Update key spec failed", err.Error())
		return
	}
	keySpecFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *KeySpecResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state keySpecModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.KeySpecs.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete key spec failed", err.Error())
	}
}

func (r *KeySpecResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// keySpecToWire builds the API input from the TF model. expiry is required
// at the schema level so plan.Expiry is non-nil by the time we get here.
func keySpecToWire(m *keySpecModel) *management.KeySpecInput {
	in := &management.KeySpecInput{
		Name:            m.Name.ValueString(),
		Description:     m.Description.ValueString(),
		Format:          management.KeyFormat(m.Format.ValueString()),
		PermissionSetID: m.PermissionSetID.ValueString(),
		SigningKeySetID: m.SigningKeySetID.ValueString(),
		Expiry: management.ExpiryPolicy{
			DefaultTTL:           m.Expiry.DefaultTTL.ValueString(),
			MaxTTL:               m.Expiry.MaxTTL.ValueString(),
			AllowNever:           m.Expiry.AllowNever.ValueBool(),
			RotationReminderDays: int(m.Expiry.RotationReminderDays.ValueInt64()),
		},
	}
	if m.Opaque != nil {
		in.Opaque = management.OpaqueConfig{Prefix: m.Opaque.Prefix.ValueString()}
	}
	if m.JWT != nil {
		in.JWT = management.JWTConfig{Algorithm: m.JWT.Algorithm.ValueString()}
	}
	return in
}

func keySpecFromWire(m *keySpecModel, out *management.KeySpec) {
	m.ID = types.StringValue(out.ID)
	m.Name = types.StringValue(out.Name)
	if out.Description != "" {
		m.Description = types.StringValue(out.Description)
	}
	m.Format = types.StringValue(string(out.Format))
	if out.PermissionSetID != "" {
		m.PermissionSetID = types.StringValue(out.PermissionSetID)
	}
	if out.SigningKeySetID != "" {
		m.SigningKeySetID = types.StringValue(out.SigningKeySetID)
	}
	if out.Format == management.KeyFormatOpaque {
		m.Opaque = &opaqueConfigModel{Prefix: types.StringValue(out.Opaque.Prefix)}
	}
	if out.Format == management.KeyFormatJWT {
		m.JWT = &jwtConfigModel{
			Algorithm: stringOrNull(out.JWT.Algorithm),
			Issuer:    types.StringValue(out.JWT.Issuer),
		}
	}
	m.Expiry = &expiryPolicyModel{
		DefaultTTL:           types.StringValue(out.Expiry.DefaultTTL),
		MaxTTL:               types.StringValue(out.Expiry.MaxTTL),
		AllowNever:           types.BoolValue(out.Expiry.AllowNever),
		RotationReminderDays: types.Int64Value(int64(out.Expiry.RotationReminderDays)),
	}
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

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
	_ resource.Resource                = &TrustProviderResource{}
	_ resource.ResourceWithConfigure   = &TrustProviderResource{}
	_ resource.ResourceWithImportState = &TrustProviderResource{}
)

// TrustProviderResource manages the inverse of a Trust Exchange: an external
// party authenticates against ClientKeySpecID and mints a token under
// OutputKeySpecID, gated by a CEL policy. Use this when a Microwave-managed
// workspace wants to ISSUE OIDC tokens to downstream consumers rather than
// CONSUME OIDC tokens from upstream issuers.
type TrustProviderResource struct {
	client *management.Client
}

type trustProviderModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Type            types.String `tfsdk:"type"`
	ClientKeySpecID types.String `tfsdk:"client_key_spec_id"`
	OutputKeySpecID types.String `tfsdk:"output_key_spec_id"`
	Policy          types.String `tfsdk:"policy"`
	Active          types.Bool   `tfsdk:"active"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func NewTrustProviderResource() resource.Resource { return &TrustProviderResource{} }

func (r *TrustProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_provider"
}

func (r *TrustProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TrustProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave trust provider: lets an external party authenticate with a Microwave-issued client key spec and mint a token under the output key spec, gated by a CEL policy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Provider type. Only 'oidc' is supported today.",
				Validators:  []validator.String{stringvalidator.OneOf("oidc")},
			},
			"client_key_spec_id": schema.StringAttribute{
				Required:    true,
				Description: "Key spec the external party authenticates against. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"output_key_spec_id": schema.StringAttribute{
				Required:    true,
				Description: "Key spec that signs the minted output token. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy": schema.StringAttribute{
				Required:    true,
				Description: "CEL policy gating the mint. Has access to `client` (authenticated party's claims) and `output` (the token being minted).",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "When false, mint attempts are rejected immediately. Defaults to true.",
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

func (r *TrustProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trustProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustProviders.Create(ctx, trustProviderToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Create trust provider failed", err.Error())
		return
	}
	trustProviderFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trustProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustProviders.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read trust provider failed", err.Error())
		return
	}
	trustProviderFromWire(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrustProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trustProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustProviders.Update(ctx, plan.ID.ValueString(), trustProviderToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Update trust provider failed", err.Error())
		return
	}
	trustProviderFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trustProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.TrustProviders.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete trust provider failed", err.Error())
	}
}

func (r *TrustProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func trustProviderToWire(m *trustProviderModel) *management.TrustProviderInput {
	in := &management.TrustProviderInput{
		Name:            m.Name.ValueString(),
		Description:     m.Description.ValueString(),
		Type:            m.Type.ValueString(),
		ClientKeySpecID: m.ClientKeySpecID.ValueString(),
		OutputKeySpecID: m.OutputKeySpecID.ValueString(),
		Policy:          m.Policy.ValueString(),
	}
	if !m.Active.IsNull() && !m.Active.IsUnknown() {
		v := m.Active.ValueBool()
		in.Active = &v
	}
	return in
}

func trustProviderFromWire(m *trustProviderModel, out *management.TrustProvider) {
	m.ID = types.StringValue(out.ID)
	m.Name = types.StringValue(out.Name)
	if out.Description != "" {
		m.Description = types.StringValue(out.Description)
	}
	m.Type = types.StringValue(out.Type)
	m.ClientKeySpecID = types.StringValue(out.ClientKeySpecID)
	m.OutputKeySpecID = types.StringValue(out.OutputKeySpecID)
	m.Policy = types.StringValue(out.Policy)
	m.Active = types.BoolValue(out.Active)
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
}

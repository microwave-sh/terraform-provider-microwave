package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &SigningKeySetResource{}
	_ resource.ResourceWithConfigure   = &SigningKeySetResource{}
	_ resource.ResourceWithImportState = &SigningKeySetResource{}
)

// SigningKeySetResource manages a JWKS-rooted signing key set. Algorithm and
// kind are immutable server-side, so the schema marks them RequiresReplace —
// a change recreates the set rather than failing on PATCH.
type SigningKeySetResource struct {
	client *management.Client
}

type signingKeySetModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Kind      types.String `tfsdk:"kind"`
	Algorithm types.String `tfsdk:"algorithm"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func NewSigningKeySetResource() resource.Resource { return &SigningKeySetResource{} }

func (r *SigningKeySetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_signing_key_set"
}

func (r *SigningKeySetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SigningKeySetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave signing key set: the JWKS-managed signing material for one or more key specs. Individual keys auto-rotate server-side and are not managed by this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Signing key set name. Unique within (workspace, kind).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				Required:      true,
				Description:   "asymmetric or symmetric. Immutable after creation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("asymmetric", "symmetric"),
				},
			},
			"algorithm": schema.StringAttribute{
				Required:      true,
				Description:   "JWA algorithm name (e.g. ES256, RS256, EdDSA for asymmetric; HS256 for symmetric). Immutable after creation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *SigningKeySetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan signingKeySetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.SigningKeySets.Create(ctx, &management.SigningKeySetInput{
		Name:      plan.Name.ValueString(),
		Kind:      management.SigningKeySetKind(plan.Kind.ValueString()),
		Algorithm: plan.Algorithm.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create signing key set failed", err.Error())
		return
	}
	plan.ID = types.StringValue(out.ID)
	plan.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SigningKeySetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state signingKeySetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.SigningKeySets.Get(ctx, management.SigningKeySetKind(state.Kind.ValueString()), state.Name.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read signing key set failed", err.Error())
		return
	}
	state.ID = types.StringValue(out.Set.ID)
	state.Algorithm = types.StringValue(out.Set.Algorithm)
	state.CreatedAt = types.StringValue(out.Set.CreatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SigningKeySetResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All mutable fields are RequiresReplace, so a planned change recreates
	// the resource and Update is never actually called. Resist the temptation
	// to silently no-op — surface as an error so a regression that adds a
	// mutable field without wiring Update fails loudly in tests.
	resp.Diagnostics.AddError("Update not supported", "signing_key_set has no mutable fields; planned changes should trigger replacement, not in-place update")
}

func (r *SigningKeySetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state signingKeySetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.SigningKeySets.Delete(ctx, management.SigningKeySetKind(state.Kind.ValueString()), state.Name.ValueString())
	if err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete signing key set failed", err.Error())
	}
}

// ImportState takes a "kind/name" composite ID because the resource itself is
// keyed on the composite. `terraform import microwave_signing_key_set.x asymmetric/sandbar-cli`.
func (r *SigningKeySetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected 'kind/name' (e.g. asymmetric/sandbar-cli-sessions)")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kind"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

var (
	_ resource.Resource                = &TrustFederationBindingResource{}
	_ resource.ResourceWithConfigure   = &TrustFederationBindingResource{}
	_ resource.ResourceWithImportState = &TrustFederationBindingResource{}
)

// TrustFederationBindingResource manages a Trust Federation Binding. A binding
// maps a catalog-backed external identity tuple to customer-selected output
// claims that a Trust Exchange can stamp after lookupBinding resolves it.
//
// NOTE: The TF attribute was renamed from "binding_type" (used in the original
// microwave_trust_binding resource, server-side PR #58) to "federation_key" to
// match the renamed server-side and SDK model. Existing state using
// microwave_trust_binding must be migrated — see CHANGELOG.md for the command.
type TrustFederationBindingResource struct {
	client *management.Client
}

type trustFederationBindingModel struct {
	ID           types.String            `tfsdk:"id"`
	FederationKey types.String           `tfsdk:"federation_key"`
	Identity     map[string]types.String `tfsdk:"identity"`
	OutputClaims map[string]types.String `tfsdk:"output_claims"`
	CreatedAt    types.String            `tfsdk:"created_at"`
	UpdatedAt    types.String            `tfsdk:"updated_at"`
}

func NewTrustFederationBindingResource() resource.Resource {
	return &TrustFederationBindingResource{}
}

func (r *TrustFederationBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_federation_binding"
}

func (r *TrustFederationBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TrustFederationBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave Trust Federation Binding. Binds a catalog-backed external identity tuple to this workspace and optional output claims. Immutable after creation; changes force replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-assigned Trust Federation Binding ID (prefix: tfb_).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			// federation_key was "binding_type" in the old microwave_trust_binding resource
			// (renamed in server PR #58 / SDK PR #5). The catalog key identifies which
			// trust federation template applies — e.g. "terraform_cloud" or "github_actions".
			"federation_key": schema.StringAttribute{
				Required:    true,
				Description: "Trust Federation catalog key, for example terraform_cloud or github_actions. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"identity": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "External identity tuple keyed by the selected Trust Federation's required identity claims. Immutable.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"output_claims": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Claims returned by lookupBinding after this identity resolves. Immutable.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
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

func (r *TrustFederationBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trustFederationBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustFederationBindings.Create(ctx, trustFederationBindingToWire(&plan))
	if err != nil {
		addAPIError(&resp.Diagnostics, "Create Trust Federation Binding failed", err, trustFederationBindingFields)
		return
	}
	trustFederationBindingFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustFederationBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trustFederationBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustFederationBindings.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addAPIError(&resp.Diagnostics, "Read Trust Federation Binding failed", err, trustFederationBindingFields)
		return
	}
	trustFederationBindingFromWire(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrustFederationBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trustFederationBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Bindings are fully immutable — all mutable-looking attributes carry
	// RequiresReplace plan modifiers, so Update is only called for computed
	// fields (id, created_at, updated_at). Write the plan directly to state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustFederationBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trustFederationBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.TrustFederationBindings.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Delete Trust Federation Binding failed", err, trustFederationBindingFields)
	}
}

func (r *TrustFederationBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func trustFederationBindingToWire(m *trustFederationBindingModel) *management.TrustFederationBindingInput {
	return &management.TrustFederationBindingInput{
		FederationKey: management.FederationKey(m.FederationKey.ValueString()),
		Identity:      stringMapToAny(m.Identity),
		OutputClaims:  stringMapToAny(m.OutputClaims),
	}
}

func trustFederationBindingFromWire(m *trustFederationBindingModel, out *management.TrustFederationBinding) {
	m.ID = types.StringValue(out.ID)
	m.FederationKey = types.StringValue(string(out.FederationKey))
	m.Identity = anyMapToStringMap(out.Identity)
	m.OutputClaims = anyMapToStringMap(out.OutputClaims)
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
}

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
	_ resource.Resource                = &TrustBindingResource{}
	_ resource.ResourceWithConfigure   = &TrustBindingResource{}
	_ resource.ResourceWithImportState = &TrustBindingResource{}
)

// TrustBindingResource manages a Trust Binding. A binding maps a
// catalog-backed external identity tuple to customer-selected output claims
// that a Trust Exchange can stamp after lookupBinding resolves it.
type TrustBindingResource struct {
	client *management.Client
}

type trustBindingModel struct {
	ID           types.String            `tfsdk:"id"`
	BindingType  types.String            `tfsdk:"binding_type"`
	Identity     map[string]types.String `tfsdk:"identity"`
	OutputClaims map[string]types.String `tfsdk:"output_claims"`
	CreatedAt    types.String            `tfsdk:"created_at"`
	UpdatedAt    types.String            `tfsdk:"updated_at"`
}

func NewTrustBindingResource() resource.Resource { return &TrustBindingResource{} }

func (r *TrustBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_binding"
}

func (r *TrustBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TrustBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave Trust Binding. Binds a catalog-backed external identity tuple to this workspace and optional output claims. Immutable after creation; changes force replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-assigned Trust Binding ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"binding_type": schema.StringAttribute{
				Required:    true,
				Description: "Trust Binding Type catalog key, for example terraform_cloud or github_actions. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"identity": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "External identity tuple keyed by the selected Trust Binding Type's required identity claims. Immutable.",
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

func (r *TrustBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trustBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustBindings.Create(ctx, trustBindingToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Create Trust Binding failed", err.Error())
		return
	}
	trustBindingFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trustBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustBindings.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Trust Binding failed", err.Error())
		return
	}
	trustBindingFromWire(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrustBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trustBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trustBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.TrustBindings.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete Trust Binding failed", err.Error())
	}
}

func (r *TrustBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func trustBindingToWire(m *trustBindingModel) *management.TrustBindingInput {
	return &management.TrustBindingInput{
		BindingType:  management.TrustBindingType(m.BindingType.ValueString()),
		Identity:     stringMapToAny(m.Identity),
		OutputClaims: stringMapToAny(m.OutputClaims),
	}
}

func trustBindingFromWire(m *trustBindingModel, out *management.TrustBinding) {
	m.ID = types.StringValue(out.ID)
	m.BindingType = types.StringValue(string(out.BindingType))
	m.Identity = anyMapToStringMap(out.Identity)
	m.OutputClaims = anyMapToStringMap(out.OutputClaims)
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
}

func stringMapToAny(in map[string]types.String) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value.ValueString()
	}
	return out
}

func anyMapToStringMap(in map[string]any) map[string]types.String {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]types.String, len(in))
	for key, value := range in {
		if s, ok := value.(string); ok {
			out[key] = types.StringValue(s)
			continue
		}
		out[key] = types.StringValue(fmt.Sprint(value))
	}
	return out
}

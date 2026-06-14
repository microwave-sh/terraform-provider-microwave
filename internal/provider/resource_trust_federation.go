package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

var (
	_ resource.Resource                = &TrustFederationResource{}
	_ resource.ResourceWithConfigure   = &TrustFederationResource{}
	_ resource.ResourceWithImportState = &TrustFederationResource{}
)

// TrustFederationResource manages a Trust Federation catalog row. Each
// federation defines the OIDC issuer, audience, required identity fields, and
// optional CEL policy override used when evaluating federation redemption
// requests. Customers can define custom federations (e.g. for internal CI
// systems) alongside the Microwave-managed built-in ones.
type TrustFederationResource struct {
	client *management.Client
}

type trustFederationModel struct {
	ID              types.String `tfsdk:"id"`
	WorkspaceID     types.String `tfsdk:"workspace_id"`
	Key             types.String `tfsdk:"key"`
	Label           types.String `tfsdk:"label"`
	Description     types.String `tfsdk:"description"`
	LogoURL         types.String `tfsdk:"logo_url"`
	DocsURL         types.String `tfsdk:"docs_url"`
	Issuer          types.String `tfsdk:"issuer"`
	Audience        types.String `tfsdk:"audience"`
	IdentityFields  types.List   `tfsdk:"identity_fields"`
	OutputKeySpecID types.String `tfsdk:"output_key_spec_id"`
	PolicyOverride  types.String `tfsdk:"policy_override"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func NewTrustFederationResource() resource.Resource { return &TrustFederationResource{} }

func (r *TrustFederationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_federation"
}

func (r *TrustFederationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TrustFederationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave Trust Federation catalog row. Defines a custom OIDC federation template (issuer, audience, identity fields, optional CEL override) that Trust Federation Bindings reference by key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-assigned Trust Federation ID (prefix: tf_).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"workspace_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Workspace that owns this federation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				Required:    true,
				Description: "Unique catalog key for this federation (e.g. acme_circleci). Used as the federation_key in bindings. Immutable after creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable display name shown in the Microwave console and API responses.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Optional longer description for the federation.",
			},
			"logo_url": schema.StringAttribute{
				Optional:    true,
				Description: "Optional URL to a logo image shown in the Microwave console.",
			},
			"docs_url": schema.StringAttribute{
				Optional:    true,
				Description: "Optional URL to documentation for configuring this federation on the identity provider side.",
			},
			"issuer": schema.StringAttribute{
				Optional:    true,
				Description: "Expected OIDC issuer URL (iss claim). Required when the federation validates OIDC tokens.",
			},
			"audience": schema.StringAttribute{
				Optional:    true,
				Description: "Expected audience (aud claim) in tokens issued by this federation's identity provider.",
			},
			"identity_fields": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Ordered list of OIDC claim names that together uniquely identify an entity in this federation. At least one required.",
			},
			"output_key_spec_id": schema.StringAttribute{
				Optional:    true,
				Description: "Key spec ID used to sign tokens minted by this federation. When unset, the workspace default applies.",
			},
			"policy_override": schema.StringAttribute{
				Optional:    true,
				Description: "Optional CEL expression that overrides the default policy for this federation. Empty string clears the override.",
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

func (r *TrustFederationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trustFederationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := trustFederationToWire(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustFederations.Create(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create Trust Federation failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustFederationFromWire(ctx, &plan, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustFederationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trustFederationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustFederations.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Trust Federation failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustFederationFromWire(ctx, &state, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrustFederationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trustFederationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state trustFederationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch, diags := trustFederationUpdatePatch(ctx, &plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustFederations.Update(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Update Trust Federation failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustFederationFromWire(ctx, &plan, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustFederationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trustFederationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.TrustFederations.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete Trust Federation failed", err.Error())
	}
}

func (r *TrustFederationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// trustFederationToWire converts a full plan model to a Create input.
func trustFederationToWire(ctx context.Context, m *trustFederationModel) (*management.TrustFederationInput, diagnostics) {
	fields, diags := stringListToSlice(ctx, m.IdentityFields)
	if diags.HasError() {
		return nil, diags
	}
	return &management.TrustFederationInput{
		Key:             management.FederationKey(m.Key.ValueString()),
		Label:           m.Label.ValueString(),
		Description:     m.Description.ValueString(),
		LogoURL:         m.LogoURL.ValueString(),
		DocsURL:         m.DocsURL.ValueString(),
		Issuer:          m.Issuer.ValueString(),
		Audience:        m.Audience.ValueString(),
		IdentityFields:  fields,
		OutputKeySpecID: m.OutputKeySpecID.ValueString(),
		PolicyOverride:  m.PolicyOverride.ValueString(),
	}, diags
}

// trustFederationUpdatePatch builds a PATCH body from plan vs state.
// Pointer fields use nil to mean "no change" and &"" to mean "clear the field".
// Terraform Framework null values are treated as "clear"; unknown values skip.
func trustFederationUpdatePatch(ctx context.Context, plan, _ *trustFederationModel) (*management.TrustFederationUpdateInput, diagnostics) {
	patch := &management.TrustFederationUpdateInput{}

	// Label is required — always send it.
	label := plan.Label.ValueString()
	patch.Label = &label

	setOptionalString := func(v types.String) *string {
		if v.IsUnknown() {
			return nil
		}
		s := v.ValueString()
		return &s
	}

	patch.Description = setOptionalString(plan.Description)
	patch.LogoURL = setOptionalString(plan.LogoURL)
	patch.DocsURL = setOptionalString(plan.DocsURL)
	patch.Issuer = setOptionalString(plan.Issuer)
	patch.Audience = setOptionalString(plan.Audience)
	patch.OutputKeySpecID = setOptionalString(plan.OutputKeySpecID)
	patch.PolicyOverride = setOptionalString(plan.PolicyOverride)

	if !plan.IdentityFields.IsNull() && !plan.IdentityFields.IsUnknown() {
		fields, diags := stringListToSlice(ctx, plan.IdentityFields)
		if diags.HasError() {
			return nil, diags
		}
		patch.IdentityFields = fields
	}

	return patch, nil
}

// trustFederationFromWire writes a wire response back into the TF model.
func trustFederationFromWire(ctx context.Context, m *trustFederationModel, out *management.TrustFederation) diagnostics {
	m.ID = types.StringValue(out.ID)
	m.WorkspaceID = types.StringValue(out.WorkspaceID)
	m.Key = types.StringValue(string(out.Key))
	m.Label = types.StringValue(out.Label)
	if out.Description != "" {
		m.Description = types.StringValue(out.Description)
	}
	if out.LogoURL != "" {
		m.LogoURL = types.StringValue(out.LogoURL)
	}
	if out.DocsURL != "" {
		m.DocsURL = types.StringValue(out.DocsURL)
	}
	if out.Issuer != "" {
		m.Issuer = types.StringValue(out.Issuer)
	}
	if out.Audience != "" {
		m.Audience = types.StringValue(out.Audience)
	}
	fields, diags := stringSliceToList(ctx, out.IdentityFields)
	if diags.HasError() {
		return diags
	}
	m.IdentityFields = fields
	if out.OutputKeySpecID != "" {
		m.OutputKeySpecID = types.StringValue(out.OutputKeySpecID)
	}
	if out.PolicyOverride != "" {
		m.PolicyOverride = types.StringValue(out.PolicyOverride)
	}
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	return diags
}

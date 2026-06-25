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
	_ resource.Resource                = &TrustExchangeResource{}
	_ resource.ResourceWithConfigure   = &TrustExchangeResource{}
	_ resource.ResourceWithImportState = &TrustExchangeResource{}
)

// TrustExchangeResource manages an OIDC federation rule: which issuer +
// audience pair can mint tokens, gated by a CEL policy, producing tokens
// against the bound output key spec.
type TrustExchangeResource struct {
	client *management.Client
}

type trustExchangeModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	Type             types.String `tfsdk:"type"`
	OIDCProvider     types.String `tfsdk:"oidc_provider"`
	Issuer           types.String `tfsdk:"issuer"`
	DiscoveryURL     types.String `tfsdk:"discovery_url"`
	JWKSURL          types.String `tfsdk:"jwks_url"`
	AllowedAudiences types.List   `tfsdk:"allowed_audiences"`
	Policy               types.String `tfsdk:"policy"`
	OutputKeySpecID      types.String `tfsdk:"output_key_spec_id"`
	Active               types.Bool   `tfsdk:"active"`
	UpstreamClientID     types.String `tfsdk:"upstream_client_id"`
	UpstreamClientSecret types.String `tfsdk:"upstream_client_secret"`
	CreatedAt            types.String `tfsdk:"created_at"`
	UpdatedAt            types.String `tfsdk:"updated_at"`
}

func NewTrustExchangeResource() resource.Resource { return &TrustExchangeResource{} }

func (r *TrustExchangeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_exchange"
}

func (r *TrustExchangeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TrustExchangeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave trust exchange: an OIDC federation rule that gates token minting by issuer, audience, and a CEL policy.",
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
				Description: "Exchange type. Only 'oidc' is supported today.",
				Validators:  []validator.String{stringvalidator.OneOf("oidc")},
			},
			"oidc_provider": schema.StringAttribute{
				Required:    true,
				Description: "OIDC provider shape: github, google, auth0, clerk, custom_oidc. Immutable after creation. Named oidc_provider (not provider) because `provider` is a Terraform meta-argument reserved on resource blocks.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("github", "google", "auth0", "clerk", "custom_oidc"),
				},
			},
			"issuer": schema.StringAttribute{
				Required:    true,
				Description: "Expected `iss` claim value (e.g. https://token.actions.githubusercontent.com).",
			},
			"discovery_url": schema.StringAttribute{
				Optional:    true,
				Description: "OIDC discovery document URL. Optional — derived from issuer for well-known providers.",
			},
			"jwks_url": schema.StringAttribute{
				Optional:    true,
				Description: "JWKS URL. Optional — derived from discovery_url or issuer when omitted.",
			},
			"allowed_audiences": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Acceptable `aud` claim values on inbound assertions.",
			},
			"policy": schema.StringAttribute{
				Required:    true,
				Description: "CEL policy gating the mint. Has access to `assertion`, `output`, `workspace`.",
			},
			"output_key_spec_id": schema.StringAttribute{
				Required:    true,
				Description: "Key spec that signs the minted output token.",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "When false, inbound exchanges are rejected immediately. Defaults to true.",
			},
			"upstream_client_id": schema.StringAttribute{
				Optional:    true,
				Description: "OIDC relying-party client id Microwave uses to broker an interactive login (authorization-code / device) at the exchange's upstream issuer. Set together with upstream_client_secret to enable brokered CLI login.",
			},
			"upstream_client_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "OIDC relying-party client secret for the brokered interactive login. Write-only: the API never returns it, so it is not refreshed from the server and the configured value is retained in state. Leave unset to keep a previously-configured secret unchanged.",
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

func (r *TrustExchangeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trustExchangeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := trustExchangeToWire(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustExchanges.Create(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create trust exchange failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustExchangeFromWire(ctx, &plan, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustExchangeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trustExchangeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustExchanges.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read trust exchange failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustExchangeFromWire(ctx, &state, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrustExchangeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trustExchangeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := trustExchangeToWire(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.TrustExchanges.Update(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update trust exchange failed", err.Error())
		return
	}
	resp.Diagnostics.Append(trustExchangeFromWire(ctx, &plan, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrustExchangeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trustExchangeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.TrustExchanges.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete trust exchange failed", err.Error())
	}
}

func (r *TrustExchangeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func trustExchangeToWire(ctx context.Context, m *trustExchangeModel) (*management.TrustExchangeInput, diagnostics) {
	auds, diags := stringListToSlice(ctx, m.AllowedAudiences)
	if diags.HasError() {
		return nil, diags
	}
	in := &management.TrustExchangeInput{
		Name:             m.Name.ValueString(),
		Description:      m.Description.ValueString(),
		Type:             m.Type.ValueString(),
		Provider:         management.TrustExchangeProvider(m.OIDCProvider.ValueString()),
		Issuer:           m.Issuer.ValueString(),
		DiscoveryURL:     m.DiscoveryURL.ValueString(),
		JWKSURL:          m.JWKSURL.ValueString(),
		AllowedAudiences: auds,
		Policy:           m.Policy.ValueString(),
		OutputKeySpecID:  m.OutputKeySpecID.ValueString(),
	}
	if !m.Active.IsNull() && !m.Active.IsUnknown() {
		v := m.Active.ValueBool()
		in.Active = &v
	}
	if v := m.UpstreamClientID; !v.IsNull() && !v.IsUnknown() {
		in.UpstreamClientID = v.ValueString()
	}
	// Write-only: send the secret only when one is supplied. An unset/empty value
	// leaves any previously-configured secret unchanged (the API never returns it
	// to diff against, and sending "" would clear it).
	if v := m.UpstreamClientSecret; !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		in.UpstreamClientSecret = v.ValueString()
	}
	return in, diags
}

func trustExchangeFromWire(ctx context.Context, m *trustExchangeModel, out *management.TrustExchange) diagnostics {
	m.ID = types.StringValue(out.ID)
	m.Name = types.StringValue(out.Name)
	if out.Description != "" {
		m.Description = types.StringValue(out.Description)
	}
	m.Type = types.StringValue(out.Type)
	m.OIDCProvider = types.StringValue(string(out.Provider))
	m.Issuer = types.StringValue(out.Issuer)
	if out.DiscoveryURL != "" {
		m.DiscoveryURL = types.StringValue(out.DiscoveryURL)
	}
	if out.JWKSURL != "" {
		m.JWKSURL = types.StringValue(out.JWKSURL)
	}
	auds, diags := stringSliceToList(ctx, out.AllowedAudiences)
	if diags.HasError() {
		return diags
	}
	m.AllowedAudiences = auds
	m.Policy = types.StringValue(out.Policy)
	m.OutputKeySpecID = types.StringValue(out.OutputKeySpecID)
	m.Active = types.BoolValue(out.Active)
	if out.UpstreamClientID != "" {
		m.UpstreamClientID = types.StringValue(out.UpstreamClientID)
	}
	// upstream_client_secret is write-only: the API never returns it, so the
	// configured value in state is left untouched here.
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	return diags
}

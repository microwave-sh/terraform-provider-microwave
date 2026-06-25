// data_sources.go collects the four ID-based data source implementations.
// They share enough shape (Configure, simple Read against the SDK Get method)
// that splitting one per file would obscure the symmetry. v0.2 adds lookup-
// by-name discovery once the backend exposes the supporting endpoints.
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

// ─── permission_set ──────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = &PermissionSetDataSource{}
	_ datasource.DataSourceWithConfigure = &PermissionSetDataSource{}
)

type PermissionSetDataSource struct{ client *management.Client }

func NewPermissionSetDataSource() datasource.DataSource { return &PermissionSetDataSource{} }

func (d *PermissionSetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_set"
}

func (d *PermissionSetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *PermissionSetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing permission set by ID.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true},
			"name":        schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{Computed: true},
			"permissions": schema.SetNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true},
						"label":       schema.StringAttribute{Computed: true},
						"description": schema.StringAttribute{Computed: true},
						"dangerous":   schema.BoolAttribute{Computed: true},
					},
				},
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *PermissionSetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg permissionSetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.PermissionSets.Get(ctx, cfg.ID.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Lookup permission set failed", err, nil)
		return
	}
	cfg.Name = types.StringValue(out.Name)
	cfg.Description = types.StringValue(out.Description)
	cfg.Permissions = permissionsFromWire(out.Permissions)
	cfg.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	cfg.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ─── signing_key_set ─────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = &SigningKeySetDataSource{}
	_ datasource.DataSourceWithConfigure = &SigningKeySetDataSource{}
)

type SigningKeySetDataSource struct{ client *management.Client }

func NewSigningKeySetDataSource() datasource.DataSource { return &SigningKeySetDataSource{} }

func (d *SigningKeySetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_signing_key_set"
}

func (d *SigningKeySetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *SigningKeySetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing signing key set by (kind, name).",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf("asymmetric", "symmetric")},
			},
			"name":       schema.StringAttribute{Required: true},
			"id":         schema.StringAttribute{Computed: true},
			"algorithm":  schema.StringAttribute{Computed: true},
			"created_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *SigningKeySetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg signingKeySetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.SigningKeySets.Get(ctx, management.SigningKeySetKind(cfg.Kind.ValueString()), cfg.Name.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Lookup signing key set failed", err, nil)
		return
	}
	cfg.ID = types.StringValue(out.Set.ID)
	cfg.Algorithm = types.StringValue(out.Set.Algorithm)
	cfg.CreatedAt = types.StringValue(out.Set.CreatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ─── key_spec ────────────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = &KeySpecDataSource{}
	_ datasource.DataSourceWithConfigure = &KeySpecDataSource{}
)

type KeySpecDataSource struct{ client *management.Client }

func NewKeySpecDataSource() datasource.DataSource { return &KeySpecDataSource{} }

func (d *KeySpecDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_key_spec"
}

func (d *KeySpecDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *KeySpecDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing key spec by ID.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Required: true},
			"name":               schema.StringAttribute{Computed: true},
			"description":        schema.StringAttribute{Computed: true},
			"format":             schema.StringAttribute{Computed: true},
			"permission_set_id":  schema.StringAttribute{Computed: true},
			"signing_key_set_id": schema.StringAttribute{Computed: true},
			"opaque": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"prefix": schema.StringAttribute{Computed: true},
				},
			},
			"jwt": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"algorithm": schema.StringAttribute{Computed: true},
					"issuer":    schema.StringAttribute{Computed: true},
				},
			},
			"expiry": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"default_ttl":            schema.StringAttribute{Computed: true},
					"max_ttl":                schema.StringAttribute{Computed: true},
					"allow_never":            schema.BoolAttribute{Computed: true},
					"rotation_reminder_days": schema.Int64Attribute{Computed: true},
				},
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *KeySpecDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg keySpecModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.KeySpecs.Get(ctx, cfg.ID.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Lookup key spec failed", err, nil)
		return
	}
	keySpecFromWire(&cfg, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ─── trust_exchange ──────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = &TrustExchangeDataSource{}
	_ datasource.DataSourceWithConfigure = &TrustExchangeDataSource{}
)

type TrustExchangeDataSource struct{ client *management.Client }

func NewTrustExchangeDataSource() datasource.DataSource { return &TrustExchangeDataSource{} }

func (d *TrustExchangeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_exchange"
}

func (d *TrustExchangeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *TrustExchangeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing trust exchange by ID.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Required: true},
			"name":               schema.StringAttribute{Computed: true},
			"description":        schema.StringAttribute{Computed: true},
			"type":               schema.StringAttribute{Computed: true},
			"oidc_provider":      schema.StringAttribute{Computed: true},
			"issuer":             schema.StringAttribute{Computed: true},
			"discovery_url":      schema.StringAttribute{Computed: true},
			"jwks_url":           schema.StringAttribute{Computed: true},
			"allowed_audiences":  schema.ListAttribute{Computed: true, ElementType: types.StringType},
			"policy":             schema.StringAttribute{Computed: true},
			"output_key_spec_id": schema.StringAttribute{Computed: true},
			"active":             schema.BoolAttribute{Computed: true},
			"created_at":         schema.StringAttribute{Computed: true},
			"updated_at":         schema.StringAttribute{Computed: true},
		},
	}
}

func (d *TrustExchangeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg trustExchangeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.TrustExchanges.Get(ctx, cfg.ID.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Lookup trust exchange failed", err, nil)
		return
	}
	resp.Diagnostics.Append(trustExchangeFromWire(ctx, &cfg, out)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ─── trust_provider ──────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = &TrustProviderDataSource{}
	_ datasource.DataSourceWithConfigure = &TrustProviderDataSource{}
)

type TrustProviderDataSource struct{ client *management.Client }

func NewTrustProviderDataSource() datasource.DataSource { return &TrustProviderDataSource{} }

func (d *TrustProviderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trust_provider"
}

func (d *TrustProviderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *TrustProviderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing trust provider by ID.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Required: true},
			"name":               schema.StringAttribute{Computed: true},
			"description":        schema.StringAttribute{Computed: true},
			"type":               schema.StringAttribute{Computed: true},
			"client_key_spec_id": schema.StringAttribute{Computed: true},
			"output_key_spec_id": schema.StringAttribute{Computed: true},
			"policy":             schema.StringAttribute{Computed: true},
			"active":             schema.BoolAttribute{Computed: true},
			"created_at":         schema.StringAttribute{Computed: true},
			"updated_at":         schema.StringAttribute{Computed: true},
		},
	}
}

func (d *TrustProviderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg trustProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.TrustProviders.Get(ctx, cfg.ID.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Lookup trust provider failed", err, nil)
		return
	}
	trustProviderFromWire(&cfg, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// Package provider implements the Microwave Terraform provider — a thin
// terraform-plugin-framework shell around the Microwave Management API client
// (github.com/microwave-sh/microwave-go/management) plus an optional OIDC
// federation step (github.com/microwave-sh/microwave-go/auth) for TFC runs
// that want to ditch the static management_key in favour of a workload
// identity exchange.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/auth"
	"github.com/microwave-sh/microwave-go/management"
)

// MicrowaveProvider implements provider.Provider. The version field is wired
// in from main.go so panic stack traces and the registry-listed version match
// the binary that runs.
type MicrowaveProvider struct {
	version string
}

// ProviderModel mirrors the `provider "microwave" {}` HCL block. Every field
// is Optional — the constraints are enforced in Configure where we have
// access to env-var fallbacks and can emit useful diagnostics.
type ProviderModel struct {
	Endpoint         types.String `tfsdk:"endpoint"`
	WorkspaceID      types.String `tfsdk:"workspace_id"`
	ManagementKey    types.String `tfsdk:"management_key"`
	AuthEndpoint     types.String `tfsdk:"auth_endpoint"`
	TrustExchangeID  types.String `tfsdk:"trust_exchange_id"`
	WorkloadTokenEnv types.String `tfsdk:"workload_token_env"`
}

// New constructs the provider factory consumed by main and by acceptance
// tests via providerserver.NewProtocol6WithError.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MicrowaveProvider{version: version}
	}
}

func (p *MicrowaveProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "microwave"
	resp.Version = p.version
}

func (p *MicrowaveProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Microwave (Mataki Labs) workspace resources as infrastructure-as-code: permission sets, signing key sets, key specifications, and trust exchanges.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Microwave Management API base URL. Defaults to https://api.microwave.sh. Override via the MICROWAVE_ENDPOINT environment variable.",
			},
			"workspace_id": schema.StringAttribute{
				Optional:    true,
				Description: "Pin requests to a specific workspace. When unset, the management key's owning workspace is used. Override via MICROWAVE_WORKSPACE_ID.",
			},
			"management_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Static Microwave management key (mw_live_...). Set this OR (trust_exchange_id + a workload identity token in the env), not both. Override via MICROWAVE_MANAGEMENT_KEY.",
			},
			"auth_endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Microwave Auth plane base URL used to redeem OIDC tokens. Defaults to https://auth.microwave.sh. Override via MICROWAVE_AUTH_ENDPOINT.",
			},
			"trust_exchange_id": schema.StringAttribute{
				Optional:    true,
				Description: "Trust Exchange ID to redeem the workload-identity OIDC token against. Required for federated auth; mutually exclusive with management_key.",
			},
			"workload_token_env": schema.StringAttribute{
				Optional:    true,
				Description: "Environment variable name holding the inbound OIDC token. Defaults to TFC_WORKLOAD_IDENTITY_TOKEN (the HashiCorp Terraform Cloud convention).",
			},
		},
	}
}

// Configure resolves the auth mode and constructs a *management.Client. The
// resolved client is passed to every resource and data source via
// resp.{Resource,DataSource}Data; per-resource Configure pulls it out via a
// type assertion.
func (p *MicrowaveProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := firstNonEmpty(config.Endpoint.ValueString(), os.Getenv("MICROWAVE_ENDPOINT"))
	workspaceID := firstNonEmpty(config.WorkspaceID.ValueString(), os.Getenv("MICROWAVE_WORKSPACE_ID"))
	managementKey := firstNonEmpty(config.ManagementKey.ValueString(), os.Getenv("MICROWAVE_MANAGEMENT_KEY"))
	authEndpoint := firstNonEmpty(config.AuthEndpoint.ValueString(), os.Getenv("MICROWAVE_AUTH_ENDPOINT"))
	exchangeID := config.TrustExchangeID.ValueString()
	tokenEnv := firstNonEmpty(config.WorkloadTokenEnv.ValueString(), "TFC_WORKLOAD_IDENTITY_TOKEN")

	// Path A — static management key. Wins when present so dev workflows
	// (export MICROWAVE_MANAGEMENT_KEY=mw_live_...) keep working even inside a
	// TFC run that also has TFC_WORKLOAD_IDENTITY_TOKEN set.
	if managementKey != "" {
		if exchangeID != "" {
			resp.Diagnostics.AddError(
				"Conflicting auth configuration",
				"Both management_key (or MICROWAVE_MANAGEMENT_KEY) and trust_exchange_id are set. Pick exactly one auth mode per provider block.",
			)
			return
		}
		p.buildClient(ctx, endpoint, workspaceID, managementKey, resp)
		return
	}

	// Path B — OIDC federation. The provider holds no static credential;
	// each Terraform run mints a fresh session JWT against the configured
	// Trust Exchange using the workload-identity token TFC (or a CI workflow)
	// has injected into the runner.
	if exchangeID != "" {
		token := os.Getenv(tokenEnv)
		if token == "" {
			resp.Diagnostics.AddError(
				"Missing workload identity token",
				"trust_exchange_id is set but the environment variable "+tokenEnv+" is empty. In Terraform Cloud, confirm TFC_WORKLOAD_IDENTITY_AUDIENCE is configured on the workspace; locally, export the env var before running terraform.",
			)
			return
		}
		sessionJWT, err := redeemSessionJWT(ctx, authEndpoint, exchangeID, token)
		if err != nil {
			resp.Diagnostics.AddError("Token exchange failed", err.Error())
			return
		}
		p.buildClient(ctx, endpoint, workspaceID, sessionJWT, resp)
		return
	}

	resp.Diagnostics.AddError(
		"Missing auth configuration",
		"Provider needs either management_key (or MICROWAVE_MANAGEMENT_KEY env) OR trust_exchange_id with a workload identity token in "+tokenEnv+".",
	)
}

// buildClient assembles the management.Client and parks it on the response so
// every downstream Configure call (per resource, per data source) shares one
// HTTP client and connection pool.
func (p *MicrowaveProvider) buildClient(_ context.Context, endpoint, workspaceID, bearer string, resp *provider.ConfigureResponse) {
	opts := []management.Option{management.WithManagementKey(bearer)}
	if endpoint != "" {
		opts = append(opts, management.WithEndpoint(endpoint))
	}
	if workspaceID != "" {
		opts = append(opts, management.WithWorkspaceID(workspaceID))
	}
	client, err := management.NewClient(opts...)
	if err != nil {
		resp.Diagnostics.AddError("Failed to construct Microwave client", err.Error())
		return
	}
	resp.ResourceData = client
	resp.DataSourceData = client
}

// redeemSessionJWT runs the federation flow once. Re-redemption on a mid-run
// 401 is a future enhancement — the typical TFC session is short enough that
// a single redeem covers a plan + apply pair.
func redeemSessionJWT(ctx context.Context, authEndpoint, exchangeID, externalToken string) (string, error) {
	var opts []auth.Option
	if authEndpoint != "" {
		opts = append(opts, auth.WithEndpoint(authEndpoint))
	}
	authClient, err := auth.NewClient(opts...)
	if err != nil {
		return "", err
	}
	result, err := authClient.TokenExchange.Redeem(ctx, exchangeID, externalToken)
	if err != nil {
		return "", err
	}
	if !result.Valid {
		return "", &exchangeDeniedError{code: result.Code, ruleResults: result.RuleResults}
	}
	return result.JWT, nil
}

// exchangeDeniedError surfaces the CEL rule breakdown so operators can fix
// the policy without server-side log access. Microwave's RuleResults map
// names every clause that ran and whether it passed.
type exchangeDeniedError struct {
	code        string
	ruleResults map[string]bool
}

func (e *exchangeDeniedError) Error() string {
	msg := "trust exchange denied"
	if e.code != "" {
		msg += " (code=" + e.code + ")"
	}
	if len(e.ruleResults) > 0 {
		msg += "; rules:"
		for k, v := range e.ruleResults {
			if v {
				msg += " " + k + "=pass"
			} else {
				msg += " " + k + "=FAIL"
			}
		}
	}
	return msg
}

func (p *MicrowaveProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPermissionSetResource,
		NewSigningKeySetResource,
		NewKeySpecResource,
		NewTrustExchangeResource,
		NewTrustProviderResource,
	}
}

func (p *MicrowaveProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPermissionSetDataSource,
		NewSigningKeySetDataSource,
		NewKeySpecDataSource,
		NewTrustExchangeDataSource,
		NewTrustProviderDataSource,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

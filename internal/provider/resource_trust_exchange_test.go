package provider

import (
	"context"
	"testing"
	"time"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

func TestTrustExchangeResource_Metadata(t *testing.T) {
	r := NewTrustExchangeResource()
	var resp tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "microwave"}, &resp)
	if resp.TypeName != "microwave_trust_exchange" {
		t.Errorf("TypeName: got %q, want microwave_trust_exchange", resp.TypeName)
	}
}

func TestTrustExchangeResource_Schema(t *testing.T) {
	r := NewTrustExchangeResource()
	var resp tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"upstream_client_id", "upstream_client_secret"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	// The upstream client secret is write-only and must be marked sensitive.
	if !resp.Schema.Attributes["upstream_client_secret"].IsSensitive() {
		t.Error("upstream_client_secret must be Sensitive")
	}
}

func exchangeModelWithAudiences(t *testing.T, ctx context.Context) *trustExchangeModel {
	t.Helper()
	auds, diag := types.ListValueFrom(ctx, types.StringType, []string{"api://prod"})
	if diag.HasError() {
		t.Fatalf("build allowed_audiences: %v", diag)
	}
	return &trustExchangeModel{
		Name:             types.StringValue("cli-via-clerk"),
		Type:             types.StringValue("oidc"),
		OIDCProvider:     types.StringValue("clerk"),
		Issuer:           types.StringValue("https://clerk.sandbar.cloud"),
		AllowedAudiences: auds,
		Policy:           types.StringValue("assertion.subject != ''"),
		OutputKeySpecID:  types.StringValue("spec_cli"),
	}
}

func TestTrustExchangeResource_ToWire_UpstreamCredentials(t *testing.T) {
	ctx := context.Background()

	// Both supplied → both sent.
	model := exchangeModelWithAudiences(t, ctx)
	model.UpstreamClientID = types.StringValue("rp_client_123")
	model.UpstreamClientSecret = types.StringValue("rp_secret_xyz")
	in, diags := trustExchangeToWire(ctx, model)
	if diags.HasError() {
		t.Fatalf("trustExchangeToWire: %v", diags)
	}
	if in.UpstreamClientID != "rp_client_123" {
		t.Errorf("UpstreamClientID: got %q, want rp_client_123", in.UpstreamClientID)
	}
	if in.UpstreamClientSecret != "rp_secret_xyz" {
		t.Errorf("UpstreamClientSecret: got %q, want rp_secret_xyz", in.UpstreamClientSecret)
	}

	// Secret unset → not sent (an empty secret must never be transmitted, which the
	// server would treat as a clear).
	noSecret := exchangeModelWithAudiences(t, ctx)
	noSecret.UpstreamClientID = types.StringValue("rp_client_123")
	noSecret.UpstreamClientSecret = types.StringNull()
	in2, diags := trustExchangeToWire(ctx, noSecret)
	if diags.HasError() {
		t.Fatalf("trustExchangeToWire (no secret): %v", diags)
	}
	if in2.UpstreamClientSecret != "" {
		t.Errorf("UpstreamClientSecret should be omitted when unset, got %q", in2.UpstreamClientSecret)
	}
	if in2.UpstreamClientID != "rp_client_123" {
		t.Errorf("UpstreamClientID: got %q, want rp_client_123", in2.UpstreamClientID)
	}
}

func TestTrustExchangeResource_FromWire_SecretNotClobbered(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	// The read shape echoes the client id but never the secret.
	out := &management.TrustExchange{
		ID:               "ex_cli",
		Name:             "cli-via-clerk",
		Type:             "oidc",
		Provider:         management.TrustExchangeProviderClerk,
		Issuer:           "https://clerk.sandbar.cloud",
		AllowedAudiences: []string{"api://prod"},
		Policy:           "assertion.subject != ''",
		OutputKeySpecID:  "spec_cli",
		Active:           true,
		UpstreamClientID: "rp_client_123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// State already holds a configured secret; a read must not overwrite it.
	m := &trustExchangeModel{UpstreamClientSecret: types.StringValue("rp_secret_xyz")}
	if diags := trustExchangeFromWire(ctx, m, out); diags.HasError() {
		t.Fatalf("trustExchangeFromWire: %v", diags)
	}
	if m.UpstreamClientID.ValueString() != "rp_client_123" {
		t.Errorf("UpstreamClientID: got %q, want rp_client_123", m.UpstreamClientID.ValueString())
	}
	if m.UpstreamClientSecret.ValueString() != "rp_secret_xyz" {
		t.Errorf("write-only secret was clobbered on read: got %q, want rp_secret_xyz", m.UpstreamClientSecret.ValueString())
	}
}

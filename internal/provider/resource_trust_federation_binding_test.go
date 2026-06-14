package provider

import (
	"context"
	"os"
	"testing"
	"time"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

func TestTrustFederationBindingResource_Metadata(t *testing.T) {
	r := NewTrustFederationBindingResource()
	var resp tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "microwave"}, &resp)
	if resp.TypeName != "microwave_trust_federation_binding" {
		t.Errorf("TypeName: got %q, want microwave_trust_federation_binding", resp.TypeName)
	}
}

func TestTrustFederationBindingResource_Schema(t *testing.T) {
	r := NewTrustFederationBindingResource()
	var resp tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	wantAttrs := []string{
		"id", "federation_key",
		"identity", "output_claims",
		"created_at", "updated_at",
	}
	for _, name := range wantAttrs {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	// Ensure old attribute name is gone.
	for _, removed := range []string{
		"binding_type", "provider", "provider_type",
		"terraform_cloud", "github_actions", "workspace_id",
	} {
		if _, ok := resp.Schema.Attributes[removed]; ok {
			t.Errorf("schema must not expose old attribute %q", removed)
		}
	}
}

func TestTrustFederationBindingResource_ToWire(t *testing.T) {
	model := &trustFederationBindingModel{
		FederationKey: types.StringValue("terraform_cloud"),
		Identity: map[string]types.String{
			"terraform_organization_name": types.StringValue("mataki"),
			"terraform_workspace_name":    types.StringValue("sandbar-microwave"),
		},
		OutputClaims: map[string]types.String{
			"environment": types.StringValue("prod"),
		},
	}
	in := trustFederationBindingToWire(model)
	// The SDK no longer exports TrustBindingTypeTerraformCloud; compare against
	// the bare string "terraform_cloud" instead.
	if string(in.FederationKey) != "terraform_cloud" {
		t.Errorf("FederationKey: got %q, want terraform_cloud", in.FederationKey)
	}
	if in.Identity["terraform_organization_name"] != "mataki" {
		t.Errorf("Identity: got %+v", in.Identity)
	}
	if in.OutputClaims["environment"] != "prod" {
		t.Errorf("OutputClaims: got %+v", in.OutputClaims)
	}
}

func TestTrustFederationBindingResource_FromWire(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	out := &management.TrustFederationBinding{
		ID:            "tfb_123",
		WorkspaceID:   "ws_abc",
		FederationKey: management.FederationKey("terraform_cloud"),
		Identity: map[string]any{
			"terraform_organization_name": "mataki",
			"terraform_workspace_name":    "sandbar-microwave",
		},
		OutputClaims: map[string]any{
			"environment": "prod",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	m := &trustFederationBindingModel{}
	trustFederationBindingFromWire(m, out)

	if m.ID.ValueString() != "tfb_123" {
		t.Errorf("ID: got %q, want tfb_123", m.ID.ValueString())
	}
	if m.FederationKey.ValueString() != "terraform_cloud" {
		t.Errorf("FederationKey: got %q, want terraform_cloud", m.FederationKey.ValueString())
	}
	if m.Identity["terraform_workspace_name"].ValueString() != "sandbar-microwave" {
		t.Errorf("Identity: got %+v", m.Identity)
	}
	if m.OutputClaims["environment"].ValueString() != "prod" {
		t.Errorf("OutputClaims: got %+v", m.OutputClaims)
	}
	if m.CreatedAt.ValueString() == "" {
		t.Error("CreatedAt should be set")
	}
}

func TestTrustFederationBindingResource_RoundTrip(t *testing.T) {
	original := &trustFederationBindingModel{
		FederationKey: types.StringValue("terraform_cloud"),
		Identity: map[string]types.String{
			"terraform_organization_name": types.StringValue("mataki"),
			"terraform_workspace_name":    types.StringValue("sandbar-microwave"),
		},
		OutputClaims: map[string]types.String{
			"environment": types.StringValue("prod"),
		},
	}
	in := trustFederationBindingToWire(original)
	fake := &management.TrustFederationBinding{
		ID:            "tfb_round",
		FederationKey: in.FederationKey,
		Identity:      in.Identity,
		OutputClaims:  in.OutputClaims,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	roundtripped := &trustFederationBindingModel{}
	trustFederationBindingFromWire(roundtripped, fake)

	if roundtripped.FederationKey.ValueString() != original.FederationKey.ValueString() {
		t.Errorf("FederationKey drifted: %q -> %q",
			original.FederationKey.ValueString(),
			roundtripped.FederationKey.ValueString())
	}
	if roundtripped.Identity["terraform_organization_name"].ValueString() != original.Identity["terraform_organization_name"].ValueString() {
		t.Errorf("identity drifted: %+v -> %+v", original.Identity, roundtripped.Identity)
	}
	if roundtripped.OutputClaims["environment"].ValueString() != original.OutputClaims["environment"].ValueString() {
		t.Errorf("output_claims drifted: %+v -> %+v", original.OutputClaims, roundtripped.OutputClaims)
	}
}

func TestAccTrustFederationBinding(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; acceptance test deferred")
	}
	t.Skip("acceptance test rig not yet wired (see provider_test.go); deferred to v0.2")
}

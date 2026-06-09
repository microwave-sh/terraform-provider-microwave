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

func TestTrustBindingResource_Metadata(t *testing.T) {
	r := NewTrustBindingResource()
	var resp tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "microwave"}, &resp)
	if resp.TypeName != "microwave_trust_binding" {
		t.Errorf("TypeName: got %q, want microwave_trust_binding", resp.TypeName)
	}
}

func TestTrustBindingResource_Schema(t *testing.T) {
	r := NewTrustBindingResource()
	var resp tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	wantAttrs := []string{
		"id", "binding_type",
		"identity", "output_claims",
		"created_at", "updated_at",
	}
	for _, name := range wantAttrs {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	for _, removed := range []string{"provider", "provider_type", "terraform_cloud", "github_actions"} {
		if _, ok := resp.Schema.Attributes[removed]; ok {
			t.Errorf("schema must not expose old connector attribute %q", removed)
		}
	}
	if _, ok := resp.Schema.Attributes["workspace_id"]; ok {
		t.Errorf("schema must not expose workspace_id; workspace scope comes from provider/auth context")
	}
}

func TestTrustBindingResource_ToWire(t *testing.T) {
	model := &trustBindingModel{
		BindingType: types.StringValue("terraform_cloud"),
		Identity: map[string]types.String{
			"terraform_organization_name": types.StringValue("mataki"),
			"terraform_workspace_name":    types.StringValue("sandbar-microwave"),
		},
		OutputClaims: map[string]types.String{
			"environment": types.StringValue("prod"),
		},
	}
	in := trustBindingToWire(model)
	if in.BindingType != management.TrustBindingTypeTerraformCloud {
		t.Errorf("BindingType: got %q, want terraform_cloud", in.BindingType)
	}
	if in.Identity["terraform_organization_name"] != "mataki" {
		t.Errorf("Identity: got %+v", in.Identity)
	}
	if in.OutputClaims["environment"] != "prod" {
		t.Errorf("OutputClaims: got %+v", in.OutputClaims)
	}
}

func TestTrustBindingResource_FromWire(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	out := &management.TrustBinding{
		ID:          "tb_123",
		WorkspaceID: "ws_abc",
		BindingType: management.TrustBindingTypeTerraformCloud,
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

	m := &trustBindingModel{}
	trustBindingFromWire(m, out)

	if m.ID.ValueString() != "tb_123" {
		t.Errorf("ID: got %q, want tb_123", m.ID.ValueString())
	}
	if m.BindingType.ValueString() != "terraform_cloud" {
		t.Errorf("BindingType: got %q, want terraform_cloud", m.BindingType.ValueString())
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

func TestTrustBindingResource_RoundTrip(t *testing.T) {
	original := &trustBindingModel{
		BindingType: types.StringValue("terraform_cloud"),
		Identity: map[string]types.String{
			"terraform_organization_name": types.StringValue("mataki"),
			"terraform_workspace_name":    types.StringValue("sandbar-microwave"),
		},
		OutputClaims: map[string]types.String{
			"environment": types.StringValue("prod"),
		},
	}
	in := trustBindingToWire(original)
	fake := &management.TrustBinding{
		ID:           "tb_round",
		BindingType:  in.BindingType,
		Identity:     in.Identity,
		OutputClaims: in.OutputClaims,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	roundtripped := &trustBindingModel{}
	trustBindingFromWire(roundtripped, fake)

	if roundtripped.BindingType.ValueString() != original.BindingType.ValueString() {
		t.Errorf("BindingType drifted: %q -> %q",
			original.BindingType.ValueString(),
			roundtripped.BindingType.ValueString())
	}
	if roundtripped.Identity["terraform_organization_name"].ValueString() != original.Identity["terraform_organization_name"].ValueString() {
		t.Errorf("identity drifted: %+v -> %+v", original.Identity, roundtripped.Identity)
	}
	if roundtripped.OutputClaims["environment"].ValueString() != original.OutputClaims["environment"].ValueString() {
		t.Errorf("output_claims drifted: %+v -> %+v", original.OutputClaims, roundtripped.OutputClaims)
	}
}

func TestAccTrustBinding(t *testing.T) {
	if testingShortAcc() {
		t.Skip("TF_ACC not set; acceptance test deferred to v0.2 rig")
	}
	t.Skip("acceptance test rig not yet wired (see provider_test.go); deferred to v0.2")
}

func testingShortAcc() bool {
	return os.Getenv("TF_ACC") == ""
}

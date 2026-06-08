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

// TestConnectorResource_Metadata pins the resource type name; a typo here
// silently breaks every HCL config that references microwave_connector.
func TestConnectorResource_Metadata(t *testing.T) {
	r := NewConnectorResource()
	var resp tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "microwave"}, &resp)
	if resp.TypeName != "microwave_connector" {
		t.Errorf("TypeName: got %q, want microwave_connector", resp.TypeName)
	}
}

// TestConnectorResource_Schema verifies the schema shape: required + optional
// attributes, nested block attributes on each flavor. Catches accidental
// schema drift without standing up a full acceptance-test rig.
func TestConnectorResource_Schema(t *testing.T) {
	r := NewConnectorResource()
	var resp tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	wantAttrs := []string{
		"id", "workspace_id", "provider_type",
		"terraform_cloud", "github_actions",
		"created_at", "updated_at",
	}
	for _, name := range wantAttrs {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	if _, ok := resp.Schema.Attributes["provider"]; ok {
		t.Error("schema must not expose bare 'provider' attribute (collides with Terraform meta-argument)")
	}
}

// TestConnectorResource_ToWire verifies the model → SDK input conversion for
// both flavors. The flavor-mismatch case (provider_type=tfc but only gha set)
// is caught by ValidateConfig, not here — here we just confirm the wire
// payload faithfully reflects what's in state.
func TestConnectorResource_ToWire(t *testing.T) {
	tests := []struct {
		name   string
		model  *connectorModel
		assert func(t *testing.T, in *management.ConnectorInput)
	}{
		{
			name: "terraform_cloud flavor",
			model: &connectorModel{
				ProviderType: types.StringValue("terraform_cloud"),
				TerraformCloud: &terraformCloudClaimsModel{
					Organization: types.StringValue("mataki"),
					Workspace:    types.StringValue("sandbar-microwave"),
				},
			},
			assert: func(t *testing.T, in *management.ConnectorInput) {
				if in.Provider != management.ConnectorProviderTerraformCloud {
					t.Errorf("Provider: got %q, want terraform_cloud", in.Provider)
				}
				if in.TerraformCloud == nil {
					t.Fatal("TerraformCloud claims missing")
				}
				if in.TerraformCloud.Organization != "mataki" {
					t.Errorf("Organization: got %q, want mataki", in.TerraformCloud.Organization)
				}
				if in.TerraformCloud.Workspace != "sandbar-microwave" {
					t.Errorf("Workspace: got %q, want sandbar-microwave", in.TerraformCloud.Workspace)
				}
				if in.GitHubActions != nil {
					t.Error("GitHubActions should be nil")
				}
			},
		},
		{
			name: "github_actions flavor",
			model: &connectorModel{
				ProviderType: types.StringValue("github_actions"),
				GitHubActions: &gitHubActionsClaimsModel{
					Repository: types.StringValue("mataki/sandbar"),
					Workflow:   types.StringValue(".github/workflows/deploy.yml"),
				},
			},
			assert: func(t *testing.T, in *management.ConnectorInput) {
				if in.Provider != management.ConnectorProviderGitHubActions {
					t.Errorf("Provider: got %q, want github_actions", in.Provider)
				}
				if in.GitHubActions == nil {
					t.Fatal("GitHubActions claims missing")
				}
				if in.GitHubActions.Repository != "mataki/sandbar" {
					t.Errorf("Repository: got %q, want mataki/sandbar", in.GitHubActions.Repository)
				}
				if in.GitHubActions.Workflow != ".github/workflows/deploy.yml" {
					t.Errorf("Workflow: got %q, want .github/workflows/deploy.yml", in.GitHubActions.Workflow)
				}
				if in.TerraformCloud != nil {
					t.Error("TerraformCloud should be nil")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, connectorToWire(tc.model))
		})
	}
}

// TestConnectorResource_FromWire verifies the SDK response → state shape
// round-trip. Specifically: setting one flavor must clear the other so a
// post-Read state doesn't carry a stale block from a prior shape.
func TestConnectorResource_FromWire(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	out := &management.Connector{
		ID:          "con_123",
		WorkspaceID: "ws_abc",
		Provider:    management.ConnectorProviderTerraformCloud,
		TerraformCloud: &management.TerraformCloudClaims{
			Organization: "mataki",
			Workspace:    "sandbar-microwave",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Seed a stale github_actions block to confirm FromWire clears it.
	m := &connectorModel{
		GitHubActions: &gitHubActionsClaimsModel{
			Repository: types.StringValue("stale/repo"),
			Workflow:   types.StringValue("stale.yml"),
		},
	}
	connectorFromWire(m, out)

	if m.ID.ValueString() != "con_123" {
		t.Errorf("ID: got %q, want con_123", m.ID.ValueString())
	}
	if m.WorkspaceID.ValueString() != "ws_abc" {
		t.Errorf("WorkspaceID: got %q, want ws_abc", m.WorkspaceID.ValueString())
	}
	if m.ProviderType.ValueString() != "terraform_cloud" {
		t.Errorf("ProviderType: got %q, want terraform_cloud", m.ProviderType.ValueString())
	}
	if m.TerraformCloud == nil {
		t.Fatal("TerraformCloud should be populated")
	}
	if m.TerraformCloud.Organization.ValueString() != "mataki" {
		t.Errorf("Organization: got %q", m.TerraformCloud.Organization.ValueString())
	}
	if m.GitHubActions != nil {
		t.Error("FromWire must clear the unused flavor block; got stale GitHubActions")
	}
	if m.CreatedAt.ValueString() == "" {
		t.Error("CreatedAt should be set")
	}
}

// TestConnectorResource_FromWire_GitHubActions covers the inverse direction
// of TestConnectorResource_FromWire — switching from a TFC state to a GHA
// state must clear the TFC block.
func TestConnectorResource_FromWire_GitHubActions(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	out := &management.Connector{
		ID:          "con_456",
		WorkspaceID: "ws_xyz",
		Provider:    management.ConnectorProviderGitHubActions,
		GitHubActions: &management.GitHubActionsClaims{
			Repository: "mataki/sandbar",
			Workflow:   ".github/workflows/deploy.yml",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	m := &connectorModel{
		TerraformCloud: &terraformCloudClaimsModel{
			Organization: types.StringValue("stale-org"),
			Workspace:    types.StringValue("stale-ws"),
		},
	}
	connectorFromWire(m, out)
	if m.GitHubActions == nil || m.GitHubActions.Repository.ValueString() != "mataki/sandbar" {
		t.Errorf("GitHubActions Repository: got %v", m.GitHubActions)
	}
	if m.TerraformCloud != nil {
		t.Error("FromWire must clear stale TerraformCloud block")
	}
}

// TestConnectorResource_RoundTrip composes ToWire + FromWire to confirm a
// state value survives the model → SDK input → SDK output → model loop with
// no field loss. Skips timestamps (the SDK input has none — the server fills
// them on Create).
func TestConnectorResource_RoundTrip(t *testing.T) {
	original := &connectorModel{
		WorkspaceID:  types.StringValue("ws_abc"),
		ProviderType: types.StringValue("terraform_cloud"),
		TerraformCloud: &terraformCloudClaimsModel{
			Organization: types.StringValue("mataki"),
			Workspace:    types.StringValue("sandbar-microwave"),
		},
	}
	in := connectorToWire(original)
	// Synthesize what the server would return.
	fake := &management.Connector{
		ID:             "con_round",
		WorkspaceID:    original.WorkspaceID.ValueString(),
		Provider:       in.Provider,
		TerraformCloud: in.TerraformCloud,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	roundtripped := &connectorModel{
		WorkspaceID: original.WorkspaceID,
	}
	connectorFromWire(roundtripped, fake)

	if roundtripped.ProviderType.ValueString() != original.ProviderType.ValueString() {
		t.Errorf("ProviderType drifted: %q -> %q",
			original.ProviderType.ValueString(),
			roundtripped.ProviderType.ValueString())
	}
	if roundtripped.TerraformCloud.Organization.ValueString() != original.TerraformCloud.Organization.ValueString() {
		t.Errorf("Organization drifted: %q -> %q",
			original.TerraformCloud.Organization.ValueString(),
			roundtripped.TerraformCloud.Organization.ValueString())
	}
	if roundtripped.TerraformCloud.Workspace.ValueString() != original.TerraformCloud.Workspace.ValueString() {
		t.Errorf("Workspace drifted: %q -> %q",
			original.TerraformCloud.Workspace.ValueString(),
			roundtripped.TerraformCloud.Workspace.ValueString())
	}
}

// TestConnectorResource_splitOnce sanity-checks the import ID parser. A
// regression here breaks `terraform import microwave_connector.x ws/con`.
func TestConnectorResource_splitOnce(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"ws_abc/con_123", []string{"ws_abc", "con_123"}},
		{"ws_abc/con/extra", []string{"ws_abc", "con/extra"}},
		{"plain", []string{"plain"}},
		{"", []string{""}},
	}
	for _, tc := range tests {
		got := splitOnce(tc.in, "/")
		if len(got) != len(tc.want) {
			t.Errorf("splitOnce(%q): got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitOnce(%q)[%d]: got %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// TestAccConnector is a placeholder acceptance test. The full
// terraform-plugin-testing rig is deferred to provider v0.2 (same as the
// other resources in this provider — see provider_test.go comment).
// When that rig lands, this test will:
//
//  1. Provision a connector against a live Microwave instance.
//  2. Verify the connector is readable.
//  3. Verify a provider_type/flavor change forces replacement.
//  4. Verify deletion removes the row.
//
// For now we honour the TF_ACC convention so `go test ./...` stays fast.
func TestAccConnector(t *testing.T) {
	if testingShortAcc() {
		t.Skip("TF_ACC not set; acceptance test deferred to v0.2 rig")
	}
	t.Skip("acceptance test rig not yet wired (see provider_test.go); deferred to v0.2")
}

func testingShortAcc() bool {
	// Mirror the standard terraform-plugin-testing gate without pulling the
	// dependency just to read the env var.
	return os.Getenv("TF_ACC") == ""
}

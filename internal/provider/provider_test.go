package provider_test

import (
	"context"
	"testing"

	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"

	"github.com/microwave-sh/terraform-provider-microwave/internal/provider"
)

// TestProviderShape is a smoke test: New() returns a usable factory, the
// resulting provider answers Metadata + Schema without panicking, and the
// Resources + DataSources lists each include the expected count. Full
// resource-level acceptance tests (terraform-plugin-testing) land in v0.2;
// for v0.1 the wire-layer guarantees are covered by the SDK's own tests
// (microwave-go management/auth packages, ~13 tests).
func TestProviderShape(t *testing.T) {
	p := provider.New("test")()

	var metaResp tfprovider.MetadataResponse
	p.Metadata(context.Background(), tfprovider.MetadataRequest{}, &metaResp)
	if metaResp.TypeName != "microwave" {
		t.Errorf("TypeName: got %q, want microwave", metaResp.TypeName)
	}
	if metaResp.Version != "test" {
		t.Errorf("Version: got %q, want test", metaResp.Version)
	}

	var schemaResp tfprovider.SchemaResponse
	p.Schema(context.Background(), tfprovider.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", schemaResp.Diagnostics)
	}
	for _, want := range []string{"endpoint", "management_key", "trust_exchange_id", "workspace_id", "auth_endpoint", "workload_token_env"} {
		if _, ok := schemaResp.Schema.Attributes[want]; !ok {
			t.Errorf("provider schema missing attribute %q", want)
		}
	}

	resources := p.Resources(context.Background())
	if len(resources) != 6 {
		t.Errorf("Resources count: got %d, want 6", len(resources))
	}
	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 5 {
		t.Errorf("DataSources count: got %d, want 5", len(dataSources))
	}
}

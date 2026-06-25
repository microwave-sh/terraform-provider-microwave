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

func TestTrustFederationResource_Metadata(t *testing.T) {
	r := NewTrustFederationResource()
	var resp tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "microwave"}, &resp)
	if resp.TypeName != "microwave_trust_federation" {
		t.Errorf("TypeName: got %q, want microwave_trust_federation", resp.TypeName)
	}
}

func TestTrustFederationResource_Schema(t *testing.T) {
	r := NewTrustFederationResource()
	var resp tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	wantAttrs := []string{
		"id", "workspace_id", "key", "label",
		"description", "logo_url", "docs_url",
		"issuer", "audience", "identity_fields",
		"output_key_spec_id", "policy_override",
		"created_at", "updated_at",
	}
	for _, name := range wantAttrs {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
}

func TestTrustFederationResource_ToWire(t *testing.T) {
	ctx := context.Background()
	fields, diag := types.ListValueFrom(ctx, types.StringType, []string{"oidc.circleci.com/project-id"})
	if diag.HasError() {
		t.Fatalf("build identity_fields: %v", diag)
	}
	model := &trustFederationModel{
		Key:            types.StringValue("acme_circleci"),
		Label:          types.StringValue("Acme CircleCI"),
		Description:    types.StringValue("CircleCI workload identity"),
		Issuer:         types.StringValue("https://oidc.circleci.com/org/0193abcd"),
		Audience:       types.StringValue("https://api.acme.example.com"),
		IdentityFields: fields,
	}
	in, diags := trustFederationToWire(ctx, model)
	if diags.HasError() {
		t.Fatalf("trustFederationToWire: %v", diags)
	}
	if string(in.Key) != "acme_circleci" {
		t.Errorf("Key: got %q, want acme_circleci", in.Key)
	}
	if in.Label != "Acme CircleCI" {
		t.Errorf("Label: got %q", in.Label)
	}
	if len(in.IdentityFields) != 1 || in.IdentityFields[0] != "oidc.circleci.com/project-id" {
		t.Errorf("IdentityFields: got %+v", in.IdentityFields)
	}
}

func TestTrustFederationResource_FromWire(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	out := &management.TrustFederation{
		ID:             "tf_123",
		WorkspaceID:    "ws_abc",
		Key:            management.FederationKey("acme_circleci"),
		Label:          "Acme CircleCI",
		Description:    "CircleCI workload identity",
		Issuer:         "https://oidc.circleci.com/org/0193abcd",
		Audience:       "https://api.acme.example.com",
		IdentityFields: []string{"oidc.circleci.com/project-id"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	m := &trustFederationModel{}
	diags := trustFederationFromWire(ctx, m, out)
	if diags.HasError() {
		t.Fatalf("trustFederationFromWire: %v", diags)
	}
	if m.ID.ValueString() != "tf_123" {
		t.Errorf("ID: got %q, want tf_123", m.ID.ValueString())
	}
	if m.WorkspaceID.ValueString() != "ws_abc" {
		t.Errorf("WorkspaceID: got %q", m.WorkspaceID.ValueString())
	}
	if m.Key.ValueString() != "acme_circleci" {
		t.Errorf("Key: got %q", m.Key.ValueString())
	}
	if m.Label.ValueString() != "Acme CircleCI" {
		t.Errorf("Label: got %q", m.Label.ValueString())
	}
	if m.IdentityFields.IsNull() || m.IdentityFields.IsUnknown() {
		t.Error("IdentityFields should be set")
	}
	if m.CreatedAt.ValueString() == "" {
		t.Error("CreatedAt should be set")
	}
}

func TestTrustFederationResource_RoundTrip(t *testing.T) {
	ctx := context.Background()
	fields, _ := types.ListValueFrom(ctx, types.StringType, []string{"oidc.circleci.com/project-id", "oidc.circleci.com/context-id"})
	original := &trustFederationModel{
		Key:            types.StringValue("acme_circleci"),
		Label:          types.StringValue("Acme CircleCI"),
		Description:    types.StringValue("CircleCI workload identity"),
		Issuer:         types.StringValue("https://oidc.circleci.com/org/0193abcd"),
		Audience:       types.StringValue("https://api.acme.example.com"),
		IdentityFields: fields,
	}
	in, diags := trustFederationToWire(ctx, original)
	if diags.HasError() {
		t.Fatalf("toWire: %v", diags)
	}
	fake := &management.TrustFederation{
		ID:             "tf_round",
		WorkspaceID:    "ws_abc",
		Key:            in.Key,
		Label:          in.Label,
		Description:    in.Description,
		Issuer:         in.Issuer,
		Audience:       in.Audience,
		IdentityFields: in.IdentityFields,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	roundtripped := &trustFederationModel{}
	if diags = trustFederationFromWire(ctx, roundtripped, fake); diags.HasError() {
		t.Fatalf("fromWire: %v", diags)
	}
	if roundtripped.Key.ValueString() != original.Key.ValueString() {
		t.Errorf("Key drifted: %q -> %q", original.Key.ValueString(), roundtripped.Key.ValueString())
	}
	if roundtripped.Label.ValueString() != original.Label.ValueString() {
		t.Errorf("Label drifted: %q -> %q", original.Label.ValueString(), roundtripped.Label.ValueString())
	}
	origSlice, _ := stringListToSlice(ctx, original.IdentityFields)
	rtSlice, _ := stringListToSlice(ctx, roundtripped.IdentityFields)
	if len(origSlice) != len(rtSlice) {
		t.Errorf("IdentityFields length drifted: %d -> %d", len(origSlice), len(rtSlice))
	}
	for i := range origSlice {
		if origSlice[i] != rtSlice[i] {
			t.Errorf("IdentityFields[%d] drifted: %q -> %q", i, origSlice[i], rtSlice[i])
		}
	}
}

func TestTrustFederationResource_UpdatePatch(t *testing.T) {
	ctx := context.Background()
	fields, _ := types.ListValueFrom(ctx, types.StringType, []string{"oidc.circleci.com/project-id"})
	plan := &trustFederationModel{
		Key:            types.StringValue("acme_circleci"),
		Label:          types.StringValue("Updated Label"),
		Description:    types.StringValue(""),
		PolicyOverride: types.StringValue(""),
		IdentityFields: fields,
	}
	state := &trustFederationModel{}
	patch, diags := trustFederationUpdatePatch(ctx, plan, state)
	if diags.HasError() {
		t.Fatalf("trustFederationUpdatePatch: %v", diags)
	}
	if patch.Label != "Updated Label" {
		t.Errorf("Label patch: got %q", patch.Label)
	}
	// Empty string description should be sent as &"" (clear).
	if patch.Description == nil || *patch.Description != "" {
		t.Errorf("Description patch expected &\"\", got %v", patch.Description)
	}
	if patch.PolicyOverride == nil || *patch.PolicyOverride != "" {
		t.Errorf("PolicyOverride patch expected &\"\", got %v", patch.PolicyOverride)
	}
	if len(patch.IdentityFields) != 1 {
		t.Errorf("IdentityFields: expected 1, got %d", len(patch.IdentityFields))
	}
}

func TestAccTrustFederation(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; acceptance test deferred")
	}
	t.Skip("acceptance test rig not yet wired (see provider_test.go); deferred to v0.2")
}

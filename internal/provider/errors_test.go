package provider

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/microwave-sh/microwave-go/management"
)

func TestAddAPIError_NonAPIErrorFallsBack(t *testing.T) {
	var diags diag.Diagnostics
	addAPIError(&diags, "Create trust exchange failed", errors.New("dial tcp: connection refused"), trustExchangeFields)

	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Summary() != "Create trust exchange failed" {
		t.Errorf("summary = %q", diags[0].Summary())
	}
	if !strings.Contains(diags[0].Detail(), "connection refused") {
		t.Errorf("detail = %q, want the underlying error", diags[0].Detail())
	}
}

func TestAddAPIError_MapsFieldErrorToAttribute(t *testing.T) {
	apiErr := &management.Error{
		StatusCode: 400,
		Title:      "Bad Request",
		Errors: []management.ErrorDetail{
			{Message: "must be one of [github google auth0 clerk custom_oidc]", Location: "body.provider"},
		},
	}
	var diags diag.Diagnostics
	addAPIError(&diags, "Create trust exchange failed", apiErr, trustExchangeFields)

	// The only problem maps to an attribute and there's no extra detail, so we
	// expect a single attribute-scoped diagnostic and no generic top-level one.
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	withPath, ok := diags[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("diagnostic is not attribute-scoped: %T", diags[0])
	}
	// provider (server) must map to oidc_provider (Terraform).
	if got := withPath.Path().String(); got != "oidc_provider" {
		t.Errorf("attribute path = %q, want oidc_provider", got)
	}
	if !strings.Contains(diags[0].Detail(), "must be one of") {
		t.Errorf("detail = %q", diags[0].Detail())
	}
}

func TestAddAPIError_UnmappedFieldFoldsIntoDetailWithStatus(t *testing.T) {
	apiErr := &management.Error{
		StatusCode: 422,
		Detail:     "policy failed to compile",
		Errors: []management.ErrorDetail{
			{Message: "undeclared reference to 'foo'", Location: "body.some_unknown_field"},
		},
	}
	var diags diag.Diagnostics
	addAPIError(&diags, "Update trust exchange failed", apiErr, trustExchangeFields)

	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(diags))
	}
	if _, ok := diags[0].(diag.DiagnosticWithPath); ok {
		t.Fatalf("unmapped field should not produce an attribute diagnostic")
	}
	d := diags[0].Detail()
	if !strings.Contains(d, "HTTP 422") || !strings.Contains(d, "policy failed to compile") || !strings.Contains(d, "undeclared reference") {
		t.Errorf("detail missing status/detail/field error: %q", d)
	}
}

func TestAddAPIError_NilFieldsKeepsEverythingInDetail(t *testing.T) {
	apiErr := &management.Error{
		StatusCode: 404,
		Title:      "Not Found",
		Detail:     "permission set not found",
	}
	var diags diag.Diagnostics
	addAPIError(&diags, "Lookup permission set failed", apiErr, nil)

	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(diags))
	}
	if _, ok := diags[0].(diag.DiagnosticWithPath); ok {
		t.Fatal("data-source lookup should not attach to an attribute")
	}
	if !strings.Contains(diags[0].Detail(), "HTTP 404") {
		t.Errorf("detail = %q, want HTTP 404", diags[0].Detail())
	}
}

package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/microwave-sh/microwave-go/management"
)

// addAPIError turns an error from the Microwave SDK into Terraform diagnostics.
//
// For a *management.Error it emits a summary plus the server's HTTP status and
// human-readable detail, and — crucially — attaches each field-level validation
// error to the offending attribute (via AddAttributeError) when its location
// maps to a known attribute, so `terraform plan` highlights the exact field
// that's wrong. fields maps a server field name (e.g. "provider") to a Terraform
// attribute name (e.g. "oidc_provider"); pass nil to skip attribute mapping
// (e.g. for data-source lookups). Field errors that don't map fold into the
// top-level detail. Non-API errors fall back to a plain message.
func addAPIError(diags *diag.Diagnostics, summary string, err error, fields map[string]string) {
	var apiErr *management.Error
	if !errors.As(err, &apiErr) {
		diags.AddError(summary, err.Error())
		return
	}

	detail := apiErr.Detail
	if detail == "" {
		detail = apiErr.Message
	}
	if detail == "" {
		detail = strings.TrimSpace(apiErr.RawBody)
	}

	mapped := 0
	var unmapped []string
	for _, fe := range apiErr.Errors {
		field := errorFieldName(fe)
		if attr, ok := fields[field]; ok {
			diags.AddAttributeError(path.Root(attr), summary, fe.Message)
			mapped++
			continue
		}
		msg := fe.Message
		if field != "" {
			msg += " (" + field + ")"
		}
		unmapped = append(unmapped, msg)
	}

	// When every problem was attached to a specific attribute and the server
	// gave no extra detail, the attribute diagnostics stand on their own — a
	// generic top-level "HTTP 400" would just be noise.
	if mapped > 0 && detail == "" && len(unmapped) == 0 {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Microwave returned HTTP %d", apiErr.StatusCode)
	if detail != "" {
		fmt.Fprintf(&b, ": %s", detail)
	}
	for _, m := range unmapped {
		fmt.Fprintf(&b, "\n  - %s", m)
	}
	diags.AddError(summary, b.String())
}

// errorFieldName extracts the offending field from a server error detail. The
// Huma schema envelope uses a dotted `location` (e.g. "body.provider"); the
// Mataki domain envelope uses `field`. Both are reduced to the leading attribute
// name so it can be matched against the resource's attribute map.
func errorFieldName(fe management.ErrorDetail) string {
	loc := fe.Location
	if loc == "" {
		loc = fe.Field
	}
	loc = strings.TrimPrefix(loc, "body.")
	if i := strings.IndexAny(loc, ".["); i >= 0 {
		loc = loc[:i]
	}
	return loc
}

// Server-field → Terraform-attribute maps. Identity for matching names; the only
// rename is trust exchange's provider (Terraform reserves `provider` on resource
// blocks, so the attribute is `oidc_provider`). Computed attributes (id,
// created_at, …) are omitted: a validation error never targets them.
var trustExchangeFields = map[string]string{
	"name":                   "name",
	"description":            "description",
	"type":                   "type",
	"provider":               "oidc_provider",
	"issuer":                 "issuer",
	"discovery_url":          "discovery_url",
	"jwks_url":               "jwks_url",
	"allowed_audiences":      "allowed_audiences",
	"policy":                 "policy",
	"output_key_spec_id":     "output_key_spec_id",
	"active":                 "active",
	"upstream_client_id":     "upstream_client_id",
	"upstream_client_secret": "upstream_client_secret",
	"verification_uri":       "verification_uri",
}

var permissionSetFields = map[string]string{
	"name":        "name",
	"label":       "label",
	"description": "description",
	"permissions": "permissions",
	"dangerous":   "dangerous",
}

var trustFederationFields = map[string]string{
	"key":                "key",
	"label":              "label",
	"description":        "description",
	"issuer":             "issuer",
	"audience":           "audience",
	"identity_fields":    "identity_fields",
	"glob_fields":        "glob_fields",
	"policy_override":    "policy_override",
	"output_key_spec_id": "output_key_spec_id",
	"logo_url":           "logo_url",
	"docs_url":           "docs_url",
}

var trustFederationBindingFields = map[string]string{
	"federation_key": "federation_key",
	"identity":       "identity",
	"output_claims":  "output_claims",
}

var signingKeySetFields = map[string]string{
	"name":      "name",
	"kind":      "kind",
	"algorithm": "algorithm",
}

var trustProviderFields = map[string]string{
	"name":               "name",
	"description":        "description",
	"type":               "type",
	"client_key_spec_id": "client_key_spec_id",
	"output_key_spec_id": "output_key_spec_id",
	"policy":             "policy",
	"active":             "active",
}

var keySpecFields = map[string]string{
	"name":                   "name",
	"description":            "description",
	"format":                 "format",
	"type":                   "type",
	"mode":                   "mode",
	"algorithm":              "algorithm",
	"issuer":                 "issuer",
	"key":                    "key",
	"prefix":                 "prefix",
	"claims":                 "claims",
	"jwt":                    "jwt",
	"opaque":                 "opaque",
	"default_ttl":            "default_ttl",
	"max_ttl":                "max_ttl",
	"expiry":                 "expiry",
	"permission_set_id":      "permission_set_id",
	"signing_key_set_id":     "signing_key_set_id",
	"allow_unlisted_claims":  "allow_unlisted_claims",
	"allow_never":            "allow_never",
	"rotation_reminder_days": "rotation_reminder_days",
	"value":                  "value",
}

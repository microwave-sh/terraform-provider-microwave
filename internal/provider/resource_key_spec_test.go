package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

// TestKeySpecToWire_AllowUnlistedClaims pins that the allow_unlisted lever is
// sent while the per-claim rows are left nil, so the server seeds and manages
// the standard rows. This is what lets a trust-exchange policy stamp a
// workspace_id claim through to the minted token.
func TestKeySpecToWire_AllowUnlistedClaims(t *testing.T) {
	m := &keySpecModel{
		Name:                types.StringValue("sandbar-cli-session"),
		Format:              types.StringValue("jwt"),
		AllowUnlistedClaims: types.BoolValue(true),
		Expiry: &expiryPolicyModel{
			DefaultTTL:           types.StringValue("1h"),
			MaxTTL:               types.StringValue("1h"),
			AllowNever:           types.BoolValue(false),
			RotationReminderDays: types.Int64Value(0),
		},
	}
	in := keySpecToWire(m)
	if !in.Claims.AllowUnlisted {
		t.Error("Claims.AllowUnlisted = false, want true")
	}
	if in.Claims.Claims != nil {
		t.Errorf("Claims.Claims = %v, want nil (server seeds the rows)", in.Claims.Claims)
	}
}

// TestKeySpecFromWire_AllowUnlistedClaims pins that the lever round-trips on
// read so a server-reported value matches state without a perpetual diff.
func TestKeySpecFromWire_AllowUnlistedClaims(t *testing.T) {
	m := &keySpecModel{}
	keySpecFromWire(m, &management.KeySpec{
		ID:     "spec_1",
		Name:   "sandbar-cli-session",
		Format: management.KeyFormatJWT,
		Claims: management.ClaimsConfig{AllowUnlisted: true},
	})
	if !m.AllowUnlistedClaims.ValueBool() {
		t.Error("AllowUnlistedClaims = false, want true")
	}
}

// TestKeySpecToWire_ExplicitClaims pins that declared rows are sent verbatim so
// the workspace_id claim can be listed explicitly (allow_unlisted stays false),
// rather than blanket-allowing unlisted claims.
func TestKeySpecToWire_ExplicitClaims(t *testing.T) {
	m := &keySpecModel{
		Name:   types.StringValue("sandbar-cli-session"),
		Format: types.StringValue("jwt"),
		Claims: []claimPolicyModel{
			{Key: types.StringValue("sub"), Mode: types.StringValue("required"), Type: types.StringValue("string")},
			{Key: types.StringValue("workspace_id"), Mode: types.StringValue("allowed"), Type: types.StringValue("string")},
		},
		Expiry: &expiryPolicyModel{
			DefaultTTL:           types.StringValue("1h"),
			MaxTTL:               types.StringValue("1h"),
			AllowNever:           types.BoolValue(false),
			RotationReminderDays: types.Int64Value(0),
		},
	}
	in := keySpecToWire(m)
	if in.Claims.AllowUnlisted {
		t.Error("AllowUnlisted = true, want false when claims are listed explicitly")
	}
	if len(in.Claims.Claims) != 2 {
		t.Fatalf("Claims rows = %d, want 2", len(in.Claims.Claims))
	}
	if in.Claims.Claims[1].Key != "workspace_id" || in.Claims.Claims[1].Mode != "allowed" {
		t.Errorf("row[1] = %+v, want workspace_id/allowed", in.Claims.Claims[1])
	}
}

// TestClaimsToWire_NilWhenEmpty pins that an empty contract sends nil (not an
// empty slice), so the server falls back to seeding the standard rows.
func TestClaimsToWire_NilWhenEmpty(t *testing.T) {
	if claimsToWire(nil) != nil {
		t.Error("claimsToWire(nil) != nil — would suppress server seeding")
	}
	if claimsToWire([]claimPolicyModel{}) != nil {
		t.Error("claimsToWire(empty) != nil — would suppress server seeding")
	}
}

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

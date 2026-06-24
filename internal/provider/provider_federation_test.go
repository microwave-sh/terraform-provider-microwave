package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedeemFederationSessionJWT(t *testing.T) {
	var sawAuth string
	var sawPath string
	var sawToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawPath = r.URL.Path
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		sawToken = body.Token
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"mw_session_jwt","expires_at":"2026-06-24T00:00:00Z","scopes":["keys:read"],"claims":{}}`))
	}))
	defer server.Close()

	token, err := redeemFederationSessionJWT(context.Background(), server.URL, "tf_system", "tfc-oidc")
	if err != nil {
		t.Fatalf("redeemFederationSessionJWT: %v", err)
	}
	if token != "mw_session_jwt" {
		t.Fatalf("token = %q, want mw_session_jwt", token)
	}
	if sawPath != "/api/trust-federations/tf_system/redeem" {
		t.Fatalf("path = %q, want /api/trust-federations/tf_system/redeem", sawPath)
	}
	if sawAuth != "" {
		t.Fatalf("Authorization header = %q, want empty", sawAuth)
	}
	if sawToken != "tfc-oidc" {
		t.Fatalf("request token = %q, want tfc-oidc", sawToken)
	}
}

func TestRedeemFederationSessionJWTRejectsEmptyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":""}`))
	}))
	defer server.Close()

	if _, err := redeemFederationSessionJWT(context.Background(), server.URL, "tf_system", "tfc-oidc"); err == nil {
		t.Fatal("redeemFederationSessionJWT error = nil, want non-nil")
	}
}

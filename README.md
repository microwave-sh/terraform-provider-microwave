# terraform-provider-microwave

Terraform provider for [Microwave](https://microwave.sh) (Mataki Labs) — manage permission sets, signing key sets, key specifications, and trust exchanges as IaC.

## Usage

```hcl
terraform {
  required_providers {
    microwave = {
      source  = "microwave-sh/microwave"
      version = "~> 0.1"
    }
  }
}

provider "microwave" {
  # Option A — static management key (dev workflows, back-compat):
  management_key = var.microwave_management_key  # or MICROWAVE_MANAGEMENT_KEY env

  # Option B — OIDC workload-identity federation (Terraform Cloud, CI):
  # Omit management_key. The provider redeems the inbound OIDC token from
  # workload_token_env (default: TFC_WORKLOAD_IDENTITY_TOKEN) against the
  # configured Trust Exchange.
  trust_exchange_id  = "ex_tfc_admin"
  workload_token_env = "TFC_WORKLOAD_IDENTITY_TOKEN"  # default
  auth_endpoint      = "https://auth.microwave.sh"     # default
}
```

Pick exactly one auth path per provider block. The provider rejects "both set" at Configure time.

## Resources + data sources

| Type | Purpose |
|---|---|
| `microwave_permission_set` | RBAC bundles bound into key specs |
| `microwave_signing_key_set` | JWKS-managed signing material (asymmetric or symmetric) |
| `microwave_key_spec` | Key specifications — opaque + JWT formats |
| `microwave_trust_exchange` | OIDC federation rules with CEL policy gates |

Matching `data.microwave_*` data sources look up any of the above by ID.

## Quick example — a Sandbar-style setup

```hcl
resource "microwave_permission_set" "deployer" {
  name        = "deployer"
  description = "Deploy + upload, no destructive ops"
  permissions = [
    { resource = "deploys", action = "create" },
    { resource = "deploys", action = "activate" },
    { resource = "blobs", action = "upload" },
    { resource = "sites", action = "read" },
  ]
}

resource "microwave_signing_key_set" "cli_sessions" {
  name      = "sandbar-cli-sessions"
  kind      = "asymmetric"
  algorithm = "ES256"
}

resource "microwave_key_spec" "cli_session" {
  name              = "sandbar-cli-session"
  format            = "jwt"
  permission_set_id = microwave_permission_set.deployer.id
  signing_key_set_id = microwave_signing_key_set.cli_sessions.id
  jwt = {}
  expiry = {
    default_ttl            = "1h"
    max_ttl                = "1h"
    allow_never            = false
    rotation_reminder_days = 0
  }
}

resource "microwave_trust_exchange" "cli_via_clerk" {
  name              = "sandbar-cli-session-exchange"
  type              = "oidc"
  provider          = "clerk"
  issuer            = "https://clerk.sandbar.cloud"
  allowed_audiences = ["https://api.sandbar.cloud"]
  output_key_spec_id = microwave_key_spec.cli_session.id
  policy = <<-CEL
    assertion.permissions.exists(p, p == "session:approve") &&
    has(assertion.org_id) && assertion.org_id != "" &&
    output.workspace_id == assertion.org_id &&
    output.permission_set == "deployer"
  CEL
}
```

## Status

v0.x — surface stable for the four resources listed above. Future versions add `microwave_trust_provider`, lookup-by-name data sources for cross-workspace references, paginated list discovery, and auto-refresh of federated sessions on 401.

## License

Apache 2.0 — see [LICENSE](LICENSE).

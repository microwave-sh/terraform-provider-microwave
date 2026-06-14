# A custom Trust Federation catalog row for CircleCI workload identity.
# Once created, bindings can reference it via federation_key = "acme_circleci".
resource "microwave_trust_federation" "acme_circleci" {
  key             = "acme_circleci"
  label           = "Acme CircleCI"
  description     = "CircleCI workload identity federation for Acme"
  logo_url        = "https://acme.example.com/logo.svg"
  docs_url        = "https://docs.acme.example.com/circleci"
  issuer          = "https://oidc.circleci.com/org/0193abcd-1234-5678-abcd-ef0123456789"
  audience        = "https://api.acme.example.com"
  identity_fields = ["oidc.circleci.com/project-id"]

  output_key_spec_id = microwave_key_spec.acme_session_jwt.id
  policy_override    = "" # optional extra CEL gating
}

# A Terraform Cloud Trust Federation Binding. Trust Exchange CEL can resolve
# this row with lookupBinding("terraform_cloud", identity) and stamp the
# returned claims.
resource "microwave_trust_federation_binding" "tfc" {
  federation_key = "terraform_cloud"
  identity = {
    terraform_organization_name = "mataki"
    terraform_workspace_name    = "sandbar-microwave"
  }
  output_claims = {
    environment = "prod"
  }
}

# A GitHub Actions Trust Federation Binding.
resource "microwave_trust_federation_binding" "gha" {
  federation_key = "github_actions"
  identity = {
    repository = "mataki/sandbar"
    workflow   = ".github/workflows/deploy.yml"
  }
}

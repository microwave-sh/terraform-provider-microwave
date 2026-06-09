# A Terraform Cloud Trust Binding. Trust Exchange CEL can resolve this row with
# lookupBinding("terraform_cloud", identity) and stamp the returned claims.
resource "microwave_trust_binding" "tfc" {
  binding_type = "terraform_cloud"
  identity = {
    terraform_organization_name = "mataki"
    terraform_workspace_name    = "sandbar-microwave"
  }
  output_claims = {
    environment = "prod"
  }
}

# A GitHub Actions Trust Binding.
resource "microwave_trust_binding" "gha" {
  binding_type = "github_actions"
  identity = {
    repository = "mataki/sandbar"
    workflow   = ".github/workflows/deploy.yml"
  }
}

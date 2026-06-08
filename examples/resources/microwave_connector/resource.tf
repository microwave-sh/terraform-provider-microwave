# A Terraform Cloud federation connector. The matching SYSTEM TFC Trust
# Exchange resolves this row at policy time to bind the inbound OIDC token's
# organization+workspace claims to this Microwave workspace.
resource "microwave_connector" "tfc" {
  workspace_id  = "ws_abc123"
  provider_type = "terraform_cloud"
  terraform_cloud = {
    organization = "mataki"
    workspace    = "sandbar-microwave"
  }
}

# A GitHub Actions federation connector. The matching SYSTEM GHA Trust
# Exchange resolves this row to bind the inbound OIDC token's repository +
# workflow claims.
resource "microwave_connector" "gha" {
  workspace_id  = "ws_abc123"
  provider_type = "github_actions"
  github_actions = {
    repository = "mataki/sandbar"
    workflow   = ".github/workflows/deploy.yml"
  }
}

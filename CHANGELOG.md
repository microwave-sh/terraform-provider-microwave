# Changelog

All notable changes to this provider are documented here.

## [Unreleased]

### Breaking changes

#### `microwave_trust_binding` → `microwave_trust_federation_binding`

The `microwave_trust_binding` resource type has been renamed to
`microwave_trust_federation_binding`. This matches the server-side rename
introduced in microwave#58 and SDK rename in microwave-go#5.

The `binding_type` attribute is also renamed to `federation_key`.

**Migration — existing state:** run `terraform state mv` for each managed
binding, then update your HCL.

```sh
terraform state mv \
  microwave_trust_binding.tfc \
  microwave_trust_federation_binding.tfc
```

**Migration — HCL:**

Before:

```hcl
resource "microwave_trust_binding" "tfc" {
  binding_type = "terraform_cloud"
  identity     = { ... }
}
```

After:

```hcl
resource "microwave_trust_federation_binding" "tfc" {
  federation_key = "terraform_cloud"
  identity       = { ... }
}
```

Server-side ID prefix also changed from `tb_` to `tfb_`. If you reference
`.id` outputs anywhere, the values in state will update automatically after the
`terraform state mv` + `terraform apply` cycle.

### New resources

#### `microwave_trust_federation`

Manages a Trust Federation catalog row. Customers can now define custom
federation types (e.g. for an internal CI system) alongside Microwave-managed
built-in federations like `terraform_cloud` and `github_actions`.

```hcl
resource "microwave_trust_federation" "acme_circleci" {
  key             = "acme_circleci"
  label           = "Acme CircleCI"
  issuer          = "https://oidc.circleci.com/org/0193abcd-..."
  audience        = "https://api.acme.example.com"
  identity_fields = ["oidc.circleci.com/project-id"]
}
```

See `examples/resources/microwave_trust_federation/resource.tf` for a full
example.

### Internal

- Moved `stringMapToAny` / `anyMapToStringMap` helpers from the connector
  resource file into `helpers.go` so they are available to all resource
  implementations.
- SDK dependency (`github.com/microwave-sh/microwave-go`) continues to point at
  the local `connectors` worktree via a `replace` directive until the SDK PR
  (`microwave-go#5`, tip `399313d`) is published as a versioned release.

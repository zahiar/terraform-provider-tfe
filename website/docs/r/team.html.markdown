---
layout: "tfe"
page_title: "Terraform Enterprise: tfe_team"
sidebar_current: "docs-resource-tfe-team-x"
description: |-
  Manages teams.
---

# tfe_team

Manages teams.

## Example Usage

Basic usage:

```hcl
resource "tfe_team" "test" {
  name         = "my-team-name"
  organization = "my-org-name"
}
```

Organization Permission usage:

```hcl
resource "tfe_team" "test" {
  name         = "my-team-name"
  organization = "my-org-name"
  organization_access {
    manage_vcs_settings = true
  }
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) Name of the team.
* `organization` - (Required) Name of the organization.
* `visibility` - (Optional) The visibility of the team ("secret" or "organization"). Defaults to "secret".
* `organization_access` - (Optional) Settings for the team's [organization access](https://www.terraform.io/docs/cloud/users-teams-organizations/permissions.html#organization-level-permissions).
* `sso_team_id` - (Optional) Unique Identifier to control team membership via SAML. Defaults to `null`

The `organization_access` block supports:

* `manage_policies` - (Optional) Allows members to create, edit, and delete the organization's Sentinel policies.
* `manage_policy_overrides` - (Optional) Allows members to override soft-mandatory policy checks.
* `manage_workspaces` - (Optional) Allows members to create and administrate all workspaces within the organization.
* `manage_vcs_settings` - (Optional) Allows members to manage the organization's VCS Providers and SSH keys.
* `manage_providers` - (Optional) Allow members to publish and delete providers in the organization's private registry.
* `manage_modules` - (Optional) Allow members to publish and delete modules in the organization's private registry.

## Attributes Reference

* `id` The ID of the team.

## Import

Teams can be imported; use `<ORGANIZATION NAME>/<TEAM ID>` as the import ID. For
example:

```shell
terraform import tfe_team.test my-org-name/team-uomQZysH9ou42ZYY
```

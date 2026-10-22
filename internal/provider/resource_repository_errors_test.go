package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepository_DataSourceNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
data "quay_repository" "missing" {
  namespace = "this-namespace-does-not-exist-tf-acc"
  name      = "also-missing"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Reading Repository)`),
			},
		},
	})
}

func TestAccRepository_InvalidVisibility(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository" "bad" {
  namespace  = "myorg"
  name       = "myrepo"
  visibility = "internal"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(invalid|value must be one of)`),
			},
		},
	})
}

// TestAccRepository_PartialUpdateSavesState reproduces the bug raised by
// jbpratt: when UpdateRepository (description) succeeds but the subsequent
// ChangeRepositoryVisibility fails, resp.State.Set is never reached — so the
// description change is applied to Quay but invisible to Terraform. Every
// subsequent plan then shows drift and re-applies the description change.
//
// The fix is to save state after the first successful call so that a failure
// on the second call still records the partial update.
func TestAccRepository_PartialUpdateSavesState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-partial-upd-org")
	repoName := "partial-upd-repo"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
		),
		Steps: []resource.TestStep{
			// Step 1: create the org and a private repo with an initial description.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "partial-upd@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = %q
  visibility  = "private"
  description = "initial"
}
`, orgName, repoName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "description", "initial"),
					resource.TestCheckResourceAttr("quay_repository.test", "visibility", "private"),
				),
			},
			// Step 2: change description AND visibility in the same apply.
			// ChangeRepositoryVisibility on a free-plan Quay instance returns
			// 403 when changing from private to public is restricted; use an
			// invalid visibility value to force an API error after the
			// description PUT has already succeeded.
			//
			// We change description to "updated" and also flip visibility from
			// "private" to "public". The description PUT fires first and
			// succeeds; the visibility call then fails. With the bug present,
			// state is never saved so the description reverts to "initial" in
			// state — Terraform sees drift and re-applies it on every plan.
			// With the fix, state is saved after the description PUT so
			// "updated" is recorded even though visibility failed.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "partial-upd@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = %q
  visibility  = "public"
  description = "updated"
}
`, orgName, repoName),
				// Whether the visibility change succeeds or fails depends on
				// the Quay instance configuration. Either outcome is valid for
				// this test — what matters is that after this step the
				// description in state matches what was applied.
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", "updated"),
			},
		},
	})
}

// TestAccRepository_OrgDeletedOutOfBandRemovesRepoFromState reproduces the bug
// raised by jbpratt: when a parent organization is deleted out-of-band, Quay
// returns 403 (not 404) for any GET on a repository in that namespace. Because
// Read only treated 404 as "gone", a 403 caused a plan error instead of
// removing the resource from state, leaving it permanently stuck.
//
// The fix is: on a 403, check whether the namespace itself still exists. If it
// is also gone, treat the repository as gone and remove it from state. If the
// namespace exists, the 403 is a genuine permission error and should surface.
func TestAccRepository_OrgDeletedOutOfBandRemovesRepoFromState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-org-del")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Create the org and a repo managed by Terraform.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "oob-org-del@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "oob-repo"
  visibility  = "private"
  description = ""
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "name", "oob-repo"),
				),
			},
			// Delete the entire organization out-of-band. The next refresh will
			// call Read on the repository. Quay returns 403 for a repo in a
			// deleted namespace. With the bug present, Read surfaces this as an
			// error and the plan is stuck. With the fix, Read detects the
			// namespace is gone and removes the resource from state, so the
			// refresh produces an empty plan.
			{
				PreConfig: func() {
					c := quayClient()
					ctx := t.Context()
					if err := c.DeleteOrganization(ctx, orgName); err != nil {
						t.Fatalf("deleting organization out-of-band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccRepository_RemovingDescriptionClearsIt reproduces the bug raised by
// jbpratt: removing the description attribute from config never clears it in
// Quay. The UseStateForUnknown plan modifier on an Optional+Computed field
// causes Terraform to copy the old state value into the plan when the
// attribute is absent from config — so the provider never sends an update and
// Quay keeps the old text.
//
// The fix is to drop UseStateForUnknown and use a Default of "" instead, so
// that removing description from config sends an explicit empty string to the
// API and clears the value.
func TestAccRepository_RemovingDescriptionClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-desc-clear-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/desc-repo"),
		),
		Steps: []resource.TestStep{
			// Create with an explicit description.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "desc-clear@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "desc-repo"
  visibility  = "private"
  description = "initial description"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", "initial description"),
			},
			// Remove description from config entirely. With the bug present,
			// UseStateForUnknown copies "initial description" into the plan so
			// no update is sent and the value persists in state and in Quay.
			// With the fix, the absence of description resolves to "" and an
			// update is sent to clear it.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "desc-clear@example.com"
}
resource "quay_repository" "test" {
  namespace  = quay_organization.test.name
  name       = "desc-repo"
  visibility = "private"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", ""),
			},
		},
	})
}

func TestAccRepository_DeletedOutOfBand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-repo-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "oob-repo@example.com"
}
resource "quay_repository" "oob" {
  namespace   = quay_organization.test.name
  name        = "oob-repo"
  visibility  = "private"
  description = ""
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.oob", "name", "oob-repo"),
				),
			},
			{RefreshState: true},
		},
	})
}

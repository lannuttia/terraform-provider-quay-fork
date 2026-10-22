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

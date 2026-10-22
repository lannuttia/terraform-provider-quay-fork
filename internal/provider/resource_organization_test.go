package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOrganization_basic(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "email", "tf-acc@example.com"),
				),
			},
			// Update
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-updated@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "email", "tf-acc-updated@example.com"),
				),
			},
		},
	})
}

func TestAccOrganization_datasource(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-ds-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-ds@example.com"
}

data "quay_organization" "test" {
  name = quay_organization.test.name
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttrSet("data.quay_organization.test", "is_organization"),
				),
			},
		},
	})
}

func TestAccOrganization_DeleteNonEmpty(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-nodel-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			// Create an org with a child repository managed by Terraform.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-org@example.com"
}
resource "quay_repository" "child" {
  namespace  = quay_organization.test.name
  name       = "child-repo"
  visibility = "private"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
			},
			// Drop only the org block. The repository remains in config with a
			// hardcoded namespace string (no dependency on the org resource),
			// so Terraform has no reason to destroy it first. The org destroy
			// fires while the repository still exists in Quay, which is exactly
			// the condition the guard must catch.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_repository" "child" {
  namespace  = %q
  name       = "child-repo"
  visibility = "private"
}
`, orgName),
				ExpectError: regexp.MustCompile(`(?i)(non-empty|contains repositories|cannot delete)`),
			},
			// Clean up: delete the child repository out-of-band so the org is
			// empty, then let Terraform destroy the now-empty org cleanly.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-org@example.com"
}
`, orgName),
				PreConfig: func() {
					if err := quayClient().DeleteRepository(context.Background(), orgName+"/child-repo"); err != nil {
						t.Fatalf("deleting child repository for cleanup: %v", err)
					}
				},
			},
		},
	})
}

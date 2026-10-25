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

// TestAccOrganization_CreateWithInvoiceEmailTrue verifies that invoice_email
// is persisted correctly on create and can be toggled to false on update.
// Quay's POST endpoint does not accept this field, so it is applied via a
// follow-up PUT whose body must reflect the configured value, not the API default.
func TestAccOrganization_CreateWithInvoiceEmailTrue(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-inv-email-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name          = %q
  email         = "tf-acc@example.com"
  invoice_email = true
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "true"),
				),
			},
			// false must round-trip correctly; the Quay API would silently
			// ignore an omitted field and keep the old value.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name          = %q
  email         = "tf-acc@example.com"
  invoice_email = false
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "false"),
				),
			},
		},
	})
}

// TestAccOrganization_CreateWithTagExpirationS verifies that tag_expiration_s
// is persisted correctly on create and can be changed on update. Quay's POST
// endpoint does not accept this field, so it is applied via a follow-up PUT.
// 86400 is used as the initial value because it is unambiguously different from
// Quay's default of 1209600, making a regression immediately visible.
func TestAccOrganization_CreateWithTagExpirationS(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-tag-exp-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name             = %q
  email            = "tf-acc@example.com"
  tag_expiration_s = 86400
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "86400"),
				),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name             = %q
  email            = "tf-acc@example.com"
  tag_expiration_s = 604800
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "604800"),
				),
			},
		},
	})
}

// TestAccOrganization_CreateWithAllOptionalFields verifies that all fields
// excluded from Quay's POST endpoint are persisted correctly when configured
// simultaneously on create.
func TestAccOrganization_CreateWithAllOptionalFields(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-all-opts-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name                  = %q
  email                 = "tf-acc@example.com"
  invoice_email         = true
  invoice_email_address = "billing@example.com"
  tag_expiration_s      = 86400
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "true"),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", "billing@example.com"),
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "86400"),
				),
			},
		},
	})
}

// TestAccOrganization_RemovingEmailClearsIt verifies that removing the email
// attribute from config sends an explicit empty string to Quay and clears the
// value. UseStateForUnknown on an Optional+Computed field causes Terraform to
// copy the old state value into the plan when the attribute is absent, so no
// update is ever sent and Quay keeps the old value.
func TestAccOrganization_RemovingEmailClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-email-clear-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "initial@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "email", "initial@example.com"),
			},
			// Remove email from config. With the bug, UseStateForUnknown
			// copies "initial@example.com" into the plan and no update is sent.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name = %q
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "email", ""),
			},
		},
	})
}

// TestAccOrganization_RemovingInvoiceEmailAddressClearsIt verifies that removing
// invoice_email_address from config sends an explicit empty string to Quay and
// clears the value.
func TestAccOrganization_RemovingInvoiceEmailAddressClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-inv-addr-clear-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name                  = %q
  email                 = "tf-acc@example.com"
  invoice_email_address = "billing@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", "billing@example.com"),
			},
			// Remove invoice_email_address from config.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", ""),
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
			// The repository is kept in config with a hardcoded namespace so
			// Terraform has no reason to destroy it before the org, exercising
			// the non-empty delete guard.
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
			// Remove the child out-of-band so the org can be destroyed cleanly
			// by CheckDestroy.
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

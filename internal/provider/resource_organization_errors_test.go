package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/quay/terraform-provider-quay/internal/provider"
)

// TestAccOrganization_TagExpirationSInvalidValueRejected verifies that a
// tag_expiration_s value not in Quay's TAG_EXPIRATION_OPTIONS is rejected at
// plan time. Without a validator the provider accepts any integer and only
// fails at apply time with an opaque 400 from the Quay API.
func TestAccOrganization_TagExpirationSInvalidValueRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_organization" "test" {
  name             = "myorg"
  email            = "tf-acc@example.com"
  tag_expiration_s = 999
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// orgDataSourceSchema returns the schema of the quay_organization data source.
func orgDataSourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	ds := provider.NewOrganizationDataSource()
	var resp fwdatasource.SchemaResponse
	ds.Schema(context.Background(), fwdatasource.SchemaRequest{}, &resp)
	return resp.Schema
}

// TestOrgDataSource_IsOrganizationDropped verifies that is_organization is not
// present in the data source schema. Quay never returns this field in the GET
// response, so it always reads as false — a misleading constant that adds no
// information.
func TestOrgDataSource_IsOrganizationDropped(t *testing.T) {
	s := orgDataSourceSchema(t)
	if _, ok := s.Attributes["is_organization"]; ok {
		t.Error("is_organization should be removed from the org data source schema — " +
			"Quay never returns this field, so it is always false")
	}
}

// TestOrgDataSource_AdminFieldsAreNullable verifies that admin-only fields in
// the org data source use nullable types. These fields are absent from the API
// response for non-admin tokens, so they must be null rather than zero values
// to allow callers to distinguish "not returned" from "set to zero".
func TestOrgDataSource_AdminFieldsAreNullable(t *testing.T) {
	s := orgDataSourceSchema(t)

	for _, field := range []string{"invoice_email", "invoice_email_address", "tag_expiration_s", "is_free_account"} {
		attr, ok := s.Attributes[field]
		if !ok {
			t.Errorf("expected attribute %q to exist in org data source schema", field)
			continue
		}
		// Nullable attributes must not be Required — they need to tolerate
		// absence from the API response.
		switch a := attr.(type) {
		case schema.BoolAttribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		case schema.StringAttribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		case schema.Int64Attribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		}
	}
}

func TestAccOrganization_DuplicateName(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-dup-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Create the first instance.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "first" {
  name  = %q
  email = "first@example.com"
}
`, orgName),
			},
			// Attempt to create a second resource with the same name — should fail.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "first" {
  name  = %q
  email = "first@example.com"
}

resource "quay_organization" "second" {
  name  = %q
  email = "second@example.com"
}
`, orgName, orgName),
				ExpectError: regexp.MustCompile(`(?i)(already exists|conflict|409|Error Creating Organization)`),
			},
		},
	})
}

func TestAccOrganization_DataSourceNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
data "quay_organization" "missing" {
  name = "this-org-does-not-exist-tf-acc"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Reading Organization)`),
			},
		},
	})
}

// TestAccOrganization_PostCreateUpdateFailureOrphansOrg reproduces the bug
// described by jbpratt: when the post-create PUT fails (e.g. because
// tag_expiration_s holds a value Quay rejects), the org is created in Quay
// but state is never saved. The next apply then fails with "already exists"
// because the org exists in Quay but Terraform treats it as new.
//
// The fix is to save state immediately after the successful POST, so that even
// if the follow-up PUT fails the org is tracked and subsequent plans can
// reconcile or update it.
func TestAccOrganization_PostCreateUpdateFailureOrphansOrg(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-orphan-org")

	// Best-effort cleanup: delete the org even if it ended up orphaned in Quay
	// without any Terraform state. This runs after the test (and after any
	// CheckDestroy), so any state-managed destroy has already fired.
	t.Cleanup(func() {
		_ = quayClient().DeleteOrganization(context.Background(), orgName)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with an invalid tag_expiration_s value that Quay
			// will reject (e.g. 999 is not in TAG_EXPIRATION_OPTIONS). The POST
			// to create the org succeeds, but the follow-up PUT fails. With the
			// bug present, state is never saved and the org is orphaned.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "orphan" {
  name             = %q
  email            = "orphan@example.com"
  tag_expiration_s = 999
}
`, orgName),
				ExpectError: regexp.MustCompile(`(?i)(Error Configuring Organization|tag_expiration_s|400|invalid)`),
			},
			// Step 2: After the failed apply, re-apply with a valid config (no
			// tag_expiration_s). With the bug present the org already exists in
			// Quay but has no Terraform state, so the provider tries to re-create
			// it and Quay returns "already exists" / 400. With the fix applied,
			// the org is in state from Step 1 and this apply succeeds cleanly.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "orphan" {
  name  = %q
  email = "orphan@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.orphan", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.orphan", "email", "orphan@example.com"),
				),
			},
		},
	})
}

// TestAccOrganization_DeletedOutOfBand verifies that when an organization is
// deleted outside of Terraform, a subsequent refresh removes it from state
// and produces a non-empty plan (Terraform wants to recreate it).
func TestAccOrganization_DeletedOutOfBand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "oob" {
  name  = %q
  email = "oob@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.oob", "name", orgName),
				),
			},
			{
				// Delete the organization out-of-band, then refresh. Read gets
				// a 404 and removes the resource from state, so Terraform
				// reports a non-empty plan (it wants to recreate the org).
				PreConfig: func() {
					if err := quayClient().DeleteOrganization(t.Context(), orgName); err != nil {
						t.Fatalf("deleting organization out-of-band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

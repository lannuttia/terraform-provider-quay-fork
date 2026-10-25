package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepositoryNotification_NoMethodBlock(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository = "myorg/myrepo"
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_MultipleMethodBlocks(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository = "myorg/myrepo"
  webhook    = { url = "https://example.com" }
  slack      = { url = "https://hooks.slack.com/services/T/B/X" }
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_NoEventBlock(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository = "myorg/myrepo"
  webhook    = { url = "https://example.com" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_MultipleEventBlocks(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository    = "myorg/myrepo"
  webhook       = { url = "https://example.com" }
  repo_push     = {}
  build_success = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

// TestAccRepositoryNotification_VulnerabilityLevelUnknownStringRejected asserts
// that an unrecognised severity string is rejected at plan time by the OneOf
// validator. The level attribute now accepts the canonical Quay severity names
// ("Critical", "High", "Medium", "Low", "Negligible", "Unknown") — not
// arbitrary strings or the old integer index.
func TestAccRepositoryNotification_VulnerabilityLevelUnknownStringRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository          = "myorg/myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "critical" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestAccRepositoryNotification_VulnerabilityLevelIntegerRejected asserts that
// a bare integer is no longer accepted for vulnerability_found.level. HCL
// coerces it to a string (e.g. "6") which the OneOf validator then rejects as
// not being a valid severity name.
func TestAccRepositoryNotification_VulnerabilityLevelIntegerRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository          = "myorg/myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = 6 }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestAccRepositoryNotification_QuayNotificationMissingTarget asserts that
// supplying a quay_notification block without a target is rejected at plan
// time. Without a target the Quay API always returns 400 "Missing target",
// so the schema enforces it as Required to surface the error before any API
// call is made.
func TestAccRepositoryNotification_QuayNotificationMissingTarget(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository        = "myorg/myrepo"
  quay_notification = {}
  repo_push         = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(required|missing|target)`),
			},
		},
	})
}

// TestAccRepositoryNotification_DeletedOutOfBandRemovesFromState verifies that
// when a notification is deleted out-of-band, a subsequent refresh removes it
// from state rather than erroring. Read uses the repository list endpoint to
// find the notification by uuid within the correct repository context, so a
// missing notification is correctly detected even when Quay returns 400 (not
// 404) for a GET on a deleted notification uuid.
func TestAccRepositoryNotification_DeletedOutOfBandRemovesFromState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-notif-oob-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/oob-notif-repo"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-notif-oob@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "oob-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  repository = quay_repository.test.id
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
			// Delete the notification out-of-band. A subsequent refresh must
			// remove it from state without error.
			{
				PreConfig: func() {
					rs := resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid")
					_ = rs
					c := quayClient()
					ctx := context.Background()
					// Retrieve the uuid from the list endpoint to delete it.
					notifs, err := c.ListRepoNotifications(ctx, orgName+"/oob-notif-repo")
					if err != nil {
						t.Fatalf("listing notifications out-of-band: %v", err)
					}
					for _, n := range notifs {
						if err := c.DeleteRepoNotification(ctx, orgName+"/oob-notif-repo", n.UUID); err != nil {
							t.Fatalf("deleting notification out-of-band: %v", err)
						}
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccRepositoryNotification_RepositoryNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository = "this-org-does-not-exist-tf-acc/also-missing"
  webhook    = { url = "https://example.com" }
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Creating Repository Notification)`),
			},
		},
	})
}

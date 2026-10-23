package provider_test

import (
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

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

// TestAccRepositoryNotification_VulnerabilityLevelStringRejected asserts that
// passing a string (the old schema type) is now rejected, since level is an
// Int64 that maps directly to the Quay API integer index.
func TestAccRepositoryNotification_VulnerabilityLevelStringRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  repository          = "myorg/myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "High" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(inappropriate value|number|int)`),
			},
		},
	})
}

// TestAccRepositoryNotification_VulnerabilityLevelOutOfRange asserts that an
// integer outside the valid Quay index range (0–5) is rejected at plan time.
func TestAccRepositoryNotification_VulnerabilityLevelOutOfRange(t *testing.T) {
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
				ExpectError: regexp.MustCompile(`(?i)(between|must be at least|value must be)`),
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

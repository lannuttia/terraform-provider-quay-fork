package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepositoryNotification_webhook(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-notif-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-notif@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  repository = quay_repository.test.id
  title      = "push webhook"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "title", "push webhook"),
					resource.TestCheckResourceAttr("quay_repository_notification.test", "webhook.url", "https://httpbin.org/post"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_slack(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-slack-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/slack-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-slack@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "slack-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  repository    = quay_repository.test.id
  title         = "build failure slack"
  slack         = { url = "https://hooks.slack.com/services/T00000/B00000/XXXXXXXXXXXX" }
  build_failure = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "slack.url", "https://hooks.slack.com/services/T00000/B00000/XXXXXXXXXXXX"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_vulnerability(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-vuln-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/vuln-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-vuln@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "vuln-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  repository          = quay_repository.test.id
  title               = "vuln alert"
  webhook             = { url = "https://httpbin.org/post" }
  vulnerability_found = { level = "High" }
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "vulnerability_found.level", "High"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_buildWithRefRegex(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-build-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/build-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-build@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "build-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  repository    = quay_repository.test.id
  title         = "main branch build"
  webhook       = { url = "https://httpbin.org/post" }
  build_success = { ref_regex = "refs/heads/main" }
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "build_success.ref_regex", "refs/heads/main"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestProvider_TokenFromEnv verifies that the provider configures successfully
// when the token is supplied only via the QUAY_TOKEN environment variable and
// is omitted from the provider block entirely.
func TestProvider_TokenFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_TOKEN", testState.quayToken)
	t.Setenv("QUAY_URL", testState.quayURL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				// token and url are intentionally omitted — both come from env vars.
				Config: `provider "quay" {}`,
			},
		},
	})
}

// TestProvider_TokenInBlockURLFromEnv verifies that the provider configures
// successfully when the token is set in the provider block and the url comes
// from the QUAY_URL environment variable.
func TestProvider_TokenInBlockURLFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_URL", testState.quayURL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				// url is intentionally omitted — it comes from the QUAY_URL env var.
				Config: `
provider "quay" {
  token = "` + testState.quayToken + `"
}
`,
			},
		},
	})
}

// TestProvider_URLInBlockTokenFromEnv verifies that the provider configures
// successfully when the url is set in the provider block and the token comes
// from the QUAY_TOKEN environment variable.
func TestProvider_URLInBlockTokenFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_TOKEN", testState.quayToken)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				// token is intentionally omitted — it comes from the QUAY_TOKEN env var.
				Config: `
provider "quay" {
  url = "` + testState.quayURL + `"
}
`,
			},
		},
	})
}

// TestProvider_UnknownToken verifies that an unknown token value (e.g. from a
// resource not yet created) is rejected at configure time rather than silently
// falling back to QUAY_TOKEN or an empty string. A silent fallback could send
// the token to the wrong host or use no token at all, removing resources from
// state when the API returns 404 or 401.
func TestProvider_UnknownToken(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "env-token-should-not-be-used")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				// terraform_data.test.output is unknown at plan time, so
				// token is unknown when the provider is configured.
				Config: `
resource "terraform_data" "test" {
  input = "not-yet-known"
}

provider "quay" {
  url   = "https://quay.io"
  token = terraform_data.test.output
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(unknown|Invalid Quay API Token)`),
			},
		},
	})
}

// TestProvider_UnknownURL verifies that an unknown url value is rejected at
// configure time rather than silently falling back to QUAY_URL or https://quay.io.
func TestProvider_UnknownURL(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "test-token")
	t.Setenv("QUAY_URL", "https://should-not-be-used.example.com")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "test" {
  input = "https://not-yet-known.example.com"
}

provider "quay" {
  url   = terraform_data.test.output
  token = "test-token"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(unknown|Invalid Quay URL)`),
			},
		},
	})
}

// TestProvider_HTTPURLRejected verifies that an http:// URL is rejected at
// configure time, since the bearer token would be sent in plaintext.
func TestProvider_HTTPURLRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url   = "http://quay.example.com"
  token = "test-token"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(http.*plaintext|Invalid Quay URL)`),
			},
		},
	})
}

// TestProvider_MissingToken verifies the provider returns a clear error when
// no token is supplied (neither in config nor environment).
func TestProvider_MissingToken(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url = "https://quay.io"
  # token deliberately omitted
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`Missing Quay API Token`),
			},
		},
	})
}

// TestProvider_InvalidToken verifies that an invalid token results in a
// meaningful error from the API rather than a panic or empty state.
func TestProvider_InvalidToken(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url   = "` + testState.quayURL + `"
  token = "this-token-is-completely-invalid"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(401|Unauthorized|invalid_token|Error Reading Repository)`),
			},
		},
	})
}

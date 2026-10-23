package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	provider "github.com/quay/terraform-provider-quay/internal/provider"
)

// schemaForOrg returns the schema of the quay_organization resource.
func schemaForOrg(t *testing.T) resourceschema.Schema {
	t.Helper()
	r := provider.NewOrganizationResource()
	var resp fwresource.SchemaResponse
	r.Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	return resp.Schema
}

// schemaForOrgDS returns the schema of the quay_organization data source.
func schemaForOrgDS(t *testing.T) datasourceschema.Schema {
	t.Helper()
	d := provider.NewOrganizationDataSource()
	var resp fwdatasource.SchemaResponse
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, &resp)
	return resp.Schema
}

// TestOrganizationSchema_TagExpirationIsGoDuration asserts that the
// tag_expiration attribute on the resource is a timetypes.GoDurationType,
// not an Int64Attribute. Exposing raw seconds as the public API forces users
// to write tag_expiration_s = 86400 instead of tag_expiration = "24h".
func TestOrganizationSchema_TagExpirationIsGoDuration(t *testing.T) {
	s := schemaForOrg(t)

	attr, ok := s.Attributes["tag_expiration"]
	if !ok {
		t.Fatal("tag_expiration attribute not found in resource schema; " +
			"the attribute may still be named tag_expiration_s (old integer form)")
	}

	strAttr, ok := attr.(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("tag_expiration: expected resourceschema.StringAttribute (timetypes.GoDuration), got %T", attr)
	}

	if _, ok := strAttr.CustomType.(timetypes.GoDurationType); !ok {
		t.Errorf("tag_expiration: CustomType is %T, want timetypes.GoDurationType", strAttr.CustomType)
	}
}

// TestOrganizationSchema_TagExpirationDataSourceIsGoDuration asserts that the
// data source also exposes tag_expiration (not tag_expiration_s) with a
// GoDurationType custom type.
func TestOrganizationSchema_TagExpirationDataSourceIsGoDuration(t *testing.T) {
	s := schemaForOrgDS(t)

	attr, ok := s.Attributes["tag_expiration"]
	if !ok {
		t.Fatal("tag_expiration attribute not found in data source schema; " +
			"the attribute may still be named tag_expiration_s (old integer form)")
	}

	dsStrAttr, ok := attr.(datasourceschema.StringAttribute)
	if !ok {
		t.Fatalf("tag_expiration: expected datasourceschema.StringAttribute, got %T", attr)
	}
	if _, ok := dsStrAttr.CustomType.(timetypes.GoDurationType); !ok {
		t.Errorf("tag_expiration: CustomType is %T, want timetypes.GoDurationType", dsStrAttr.CustomType)
	}
}

// TestOrganizationSchema_TagExpirationRejectsInvalidDuration asserts that a
// syntactically invalid Go duration string is rejected at plan time.
func TestOrganizationSchema_TagExpirationRejectsInvalidDuration(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_organization" "bad" {
  name           = "myorg"
  tag_expiration = "not-a-duration"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(duration|invalid|must be)`),
			},
		},
	})
}

// TestOrganizationSchema_TagExpirationAcceptsValidDurations asserts that valid
// Go duration strings pass plan-time validation. Uses a mock HTTP server so the
// test never touches a real Quay instance and is not sensitive to Quay's
// server-side whitelist of allowed tag_expiration_s values.
func TestOrganizationSchema_TagExpirationAcceptsValidDurations(t *testing.T) {
	cases := []string{"0s", "24h", "168h", "1h30m", "30m"}
	for _, d := range cases {
		t.Run(d, func(t *testing.T) {
			dur, _ := time.ParseDuration(d)
			secs := int(dur.Seconds())

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPost:
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte("{}"))
				case http.MethodGet:
					_, _ = fmt.Fprintf(w, `{"name":"myorg","email":"","invoice_email":false,"invoice_email_address":"","tag_expiration_s":%d,"is_organization":true,"is_free_account":true}`, secs)
				case http.MethodPut:
					_, _ = fmt.Fprintf(w, `{"name":"myorg","email":"","invoice_email":false,"invoice_email_address":"","tag_expiration_s":%d,"is_organization":true,"is_free_account":true}`, secs)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{
					{
						Config: fmt.Sprintf(`
provider "quay" {
  url   = %q
  token = "test-token"
}
resource "quay_organization" "test" {
  name           = "myorg"
  tag_expiration = %q
}
`, srv.URL, d),
						// No ExpectError — both plan and apply should succeed
						// against the mock server.
					},
				},
			})
		})
	}
}

// TestOrganizationSchema_TagExpirationSecondsConversion verifies that Go
// duration strings convert to the expected number of seconds. This documents
// the conversion contract used by the provider when building the API request.
func TestOrganizationSchema_TagExpirationSecondsConversion(t *testing.T) {
	cases := []struct {
		duration string
		wantSecs int
	}{
		{"24h", 86400},
		{"168h", 604800},
		{"0s", 0},
		{"30m", 1800},
	}

	for _, tc := range cases {
		t.Run(tc.duration, func(t *testing.T) {
			dur, err := time.ParseDuration(tc.duration)
			if err != nil {
				t.Fatalf("invalid test case duration %q: %v", tc.duration, err)
			}
			got := int(dur.Seconds())
			if got != tc.wantSecs {
				t.Errorf("duration %q: want %d seconds, got %d", tc.duration, tc.wantSecs, got)
			}
		})
	}
}

// TestOrganizationSchema_TagExpirationAPIPayload asserts that the provider
// sends tag_expiration_s as an integer number of seconds in the PUT body when
// tag_expiration is set as a Go duration string.
func TestOrganizationSchema_TagExpirationAPIPayload(t *testing.T) {
	cases := []struct {
		duration string
		wantSecs float64 // JSON numbers decode as float64
	}{
		{"24h", 86400},
		{"0s", 0},
		{"30m", 1800},
	}

	for _, tc := range cases {
		t.Run(tc.duration, func(t *testing.T) {
			var capturedPUT map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPost:
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte("{}"))
				case http.MethodGet:
					_, _ = fmt.Fprintf(w, `{"name":"myorg","email":"","invoice_email":false,"invoice_email_address":"","tag_expiration_s":%d,"is_organization":true,"is_free_account":true}`, int(tc.wantSecs))
				case http.MethodPut:
					b, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(b, &capturedPUT)
					_, _ = fmt.Fprintf(w, `{"name":"myorg","email":"","invoice_email":false,"invoice_email_address":"","tag_expiration_s":%d,"is_organization":true,"is_free_account":true}`, int(tc.wantSecs))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{
					{
						Config: fmt.Sprintf(`
provider "quay" {
  url   = %q
  token = "test-token"
}
resource "quay_organization" "test" {
  name           = "myorg"
  tag_expiration = %q
}
`, srv.URL, tc.duration),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration", tc.duration),
							func(_ *terraform.State) error {
								if capturedPUT == nil {
									return fmt.Errorf("PUT was never called")
								}
								got, ok := capturedPUT["tag_expiration_s"]
								if !ok {
									return fmt.Errorf("tag_expiration_s missing from PUT body")
								}
								if got != tc.wantSecs {
									return fmt.Errorf("tag_expiration_s: want %v, got %v", tc.wantSecs, got)
								}
								return nil
							},
						),
					},
				},
			})
		})
	}
}

package provider_test

import (
	"context"
	"regexp"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	provider "github.com/quay/terraform-provider-quay/internal/provider"
)

// schemaFor returns the schema of a resource produced by the given factory.
func schemaFor(t *testing.T, factory func() fwresource.Resource) schema.Schema {
	t.Helper()
	r := factory()
	var resp fwresource.SchemaResponse
	r.Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	return resp.Schema
}

// nestedAttr walks a dot-separated path through a schema and returns the leaf
// attribute. It understands SingleNestedAttribute as an intermediate node.
func nestedAttr(t *testing.T, s schema.Schema, path ...string) schema.Attribute {
	t.Helper()
	var cur schema.Attribute
	attrs := s.Attributes
	for i, seg := range path {
		a, ok := attrs[seg]
		if !ok {
			t.Fatalf("attribute %q not found (path so far: %v)", seg, path[:i+1])
		}
		cur = a
		if i < len(path)-1 {
			nested, ok := a.(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("expected SingleNestedAttribute at %q, got %T", seg, a)
			}
			attrs = nested.Attributes
		}
	}
	return cur
}

// TestNotificationSchema_QuayNotificationTarget asserts that the
// quay_notification method block has a required nested "target" attribute
// containing "kind" and "name" child attributes.
//
// Without a target the Quay API always returns 400 "Missing target", so
// accepting a quay_notification block with no target means creates always fail
// with an opaque API error — not a clear plan-time validation message.
func TestNotificationSchema_QuayNotificationTarget(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)

	// quay_notification must exist and be a SingleNestedAttribute.
	qnRaw, ok := s.Attributes["quay_notification"]
	if !ok {
		t.Fatal("quay_notification attribute not found in schema")
	}
	qn, ok := qnRaw.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("quay_notification: expected schema.SingleNestedAttribute, got %T", qnRaw)
	}

	// It must contain a "target" child attribute.
	targetRaw, ok := qn.Attributes["target"]
	if !ok {
		t.Fatal("quay_notification.target attribute is missing — Quay returns 400 \"Missing target\" when no target is provided")
	}

	target, ok := targetRaw.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("quay_notification.target: expected schema.SingleNestedAttribute, got %T", targetRaw)
	}
	if !target.Required {
		t.Error("quay_notification.target must be Required")
	}

	// target must contain both "kind" and "name".
	for _, child := range []string{"kind", "name"} {
		childRaw, ok := target.Attributes[child]
		if !ok {
			t.Errorf("quay_notification.target.%s attribute is missing", child)
			continue
		}
		strAttr, ok := childRaw.(schema.StringAttribute)
		if !ok {
			t.Errorf("quay_notification.target.%s: expected schema.StringAttribute, got %T", child, childRaw)
			continue
		}
		if !strAttr.Required {
			t.Errorf("quay_notification.target.%s must be Required", child)
		}
	}
}

// TestNotificationSchema_SensitiveFields asserts that credential-bearing
// attributes in the notification schema are marked Sensitive so Terraform
// redacts them in plan/apply output and encrypts them in state.
//
// A Slack incoming-webhook URL is the credential itself; webhook URLs frequently
// embed tokens. Without Sensitive: true these values appear in every plan diff
// and CI log.
func TestNotificationSchema_SensitiveFields(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)

	cases := []struct {
		path []string
	}{
		{[]string{"webhook", "url"}},
		{[]string{"slack", "url"}},
	}

	for _, tc := range cases {
		attr := nestedAttr(t, s, tc.path...)
		strAttr, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("attribute %v: expected schema.StringAttribute, got %T", tc.path, attr)
			continue
		}
		if !strAttr.Sensitive {
			t.Errorf("attribute %v: Sensitive must be true — this attribute contains a credential that would otherwise appear in plain-text plan output and CI logs", tc.path)
		}
	}
}

// TestNotificationSchema_VulnerabilityLevelIsOptionalString asserts that the
// vulnerability_found.level attribute is an Optional StringAttribute. It must
// be Optional (not Required) because Quay treats a missing level as "any
// severity", and the Quay UI creates notifications without a level. Making it
// Required would prevent importing those notifications.
func TestNotificationSchema_VulnerabilityLevelIsOptionalString(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)
	attr := nestedAttr(t, s, "vulnerability_found", "level")

	strAttr, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("vulnerability_found.level: expected schema.StringAttribute, got %T — "+
			"the level must be a human-readable severity name, not a magic integer", attr)
	}
	if strAttr.Required {
		t.Error("vulnerability_found.level must be Optional, not Required — " +
			"Quay treats a missing level as 'any severity' and the UI omits it")
	}
}

// TestNotificationSchema_VulnerabilityLevelOmittedAccepted asserts that omitting
// level entirely from a vulnerability_found block is accepted at plan time.
func TestNotificationSchema_VulnerabilityLevelOmittedAccepted(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "test" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(404|not found|Error Creating)`),
			},
		},
	})
}

// TestNotificationSchema_VulnerabilityLevelRejectsInteger asserts that passing
// an integer for vulnerability_found.level is rejected at plan time. HCL
// coerces bare integers to strings for StringAttribute, so "1" passes the type
// check but is rejected by the OneOf validator as not being a valid severity name.
func TestNotificationSchema_VulnerabilityLevelRejectsInteger(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = 1 }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestNotificationSchema_VulnerabilityLevelRejectsUnknownString asserts that
// an unrecognised severity string is rejected at plan time.
func TestNotificationSchema_VulnerabilityLevelRejectsUnknownString(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "critical" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid)`),
			},
		},
	})
}

// TestNotificationSchema_QuayNotificationKindValidator asserts that
// quay_notification.target.kind is validated as either "user" or "team" at
// plan time. Without this, an invalid kind (e.g. "org") is accepted by
// Terraform but rejected by the Quay API with an opaque error at apply time.
func TestNotificationSchema_QuayNotificationKindValidator(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)
	attr := nestedAttr(t, s, "quay_notification", "target", "kind")

	strAttr, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("quay_notification.target.kind: expected schema.StringAttribute, got %T", attr)
	}
	if len(strAttr.Validators) == 0 {
		t.Error("quay_notification.target.kind: no validators defined — invalid values like \"org\" are silently accepted at plan time and only rejected by the Quay API at apply time")
	}
}

// TestNotificationSchema_QuayNotificationKindRejectsInvalidValue asserts that
// an invalid kind is rejected at plan time.
func TestNotificationSchema_QuayNotificationKindRejectsInvalidValue(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  quay_notification = { target = { kind = "org", name = "myorg" } }
  repo_push         = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestNotificationSchema_QuayNotificationKindAcceptsValidValues asserts that
// "user" and "team" are both accepted at plan time.
func TestNotificationSchema_QuayNotificationKindAcceptsValidValues(t *testing.T) {
	for _, kind := range []string{"user", "team"} {
		t.Run(kind, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{
					{
						Config: providerConfig() + `
resource "quay_repository_notification" "test" {
  namespace  = "myorg"

  name       = "myrepo"
  quay_notification = { target = { kind = "` + kind + `", name = "someone" } }
  repo_push         = {}
}
`,
						// Plan succeeds; apply fails with no live Quay.
						ExpectError: regexp.MustCompile(`(?i)(404|not found|Error Creating|403)`),
					},
				},
			})
		})
	}
}

// TestNotificationSchema_VulnerabilityLevelAcceptsValidStrings asserts that
// each canonical Quay severity name is accepted at plan time.
func TestNotificationSchema_VulnerabilityLevelAcceptsValidStrings(t *testing.T) {
	levels := []string{"Critical", "High", "Medium", "Low", "Negligible", "Unknown"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{
					{
						Config: providerConfig() + `
resource "quay_repository_notification" "test" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "` + level + `" }
}
`,
						// No ExpectError — plan should succeed. Apply will fail
						// (no live Quay), but we only check plan here.
						ExpectError: regexp.MustCompile(`(?i)(404|not found|Error Creating)`),
					},
				},
			})
		})
	}
}

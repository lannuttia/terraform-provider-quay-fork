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
// embed tokens; HipChat notification_token and Flowdock flow_api_token are
// explicit API secrets. Without Sensitive: true these values appear in every
// plan diff and CI log.
func TestNotificationSchema_SensitiveFields(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)

	cases := []struct {
		path []string
	}{
		{[]string{"webhook", "url"}},
		{[]string{"slack", "url"}},
		{[]string{"hipchat", "notification_token"}},
		{[]string{"flowdock", "flow_api_token"}},
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

// TestNotificationSchema_VulnerabilityLevelIsString asserts that the
// vulnerability_found.level attribute is a StringAttribute, not an integer.
// Exposing a numeric index as the public API is poor UX — users are expected
// to know that 2 means "Medium". The provider should accept the canonical Quay
// severity names ("Critical", "High", "Medium", "Low", "Negligible", "Unknown")
// and translate to the integer index internally.
func TestNotificationSchema_VulnerabilityLevelIsString(t *testing.T) {
	s := schemaFor(t, provider.NewRepositoryNotificationResource)
	attr := nestedAttr(t, s, "vulnerability_found", "level")

	if _, ok := attr.(schema.StringAttribute); !ok {
		t.Errorf("vulnerability_found.level: expected schema.StringAttribute, got %T — "+
			"the level must be a human-readable severity name, not a magic integer", attr)
	}
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
  repository          = "myorg/myrepo"
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
  repository          = "myorg/myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "critical" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid)`),
			},
		},
	})
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
  repository          = "myorg/myrepo"
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

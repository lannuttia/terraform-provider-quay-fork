package provider_test

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

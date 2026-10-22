package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	provider "github.com/lannuttia/terraform-provider-quay/internal/provider"
)

// schemaFor returns the schema of a resource produced by the given factory.
func schemaFor(t *testing.T, factory func() resource.Resource) schema.Schema {
	t.Helper()
	r := factory()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
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

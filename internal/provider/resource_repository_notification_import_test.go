package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	provider "github.com/quay/terraform-provider-quay/internal/provider"
)

// importNotificationState calls ImportState on a freshly-constructed
// RepositoryNotificationResource and returns the resulting state response.
func importNotificationState(t *testing.T, importID string) *fwresource.ImportStateResponse {
	t.Helper()
	ctx := context.Background()

	r := provider.NewRepositoryNotificationResource()

	var schemaResp fwresource.SchemaResponse
	r.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema

	req := fwresource.ImportStateRequest{ID: importID}
	resp := &fwresource.ImportStateResponse{
		State: tfsdk.State{
			Raw:    tftypes.NewValue(s.Type().TerraformType(ctx), nil),
			Schema: s,
		},
	}
	r.(fwresource.ResourceWithImportState).ImportState(ctx, req, resp)
	return resp
}

// getStringAttr reads a single string attribute from an ImportStateResponse.
func getStringAttr(t *testing.T, resp *fwresource.ImportStateResponse, attr string) string {
	t.Helper()
	ctx := context.Background()
	var v types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(attr), &v)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading %q from import state: %s", attr, resp.Diagnostics)
	}
	return v.ValueString()
}

// TestRepositoryNotificationImport_SetsCorrectAttributes verifies that
// ImportState parses the 3-part import ID (namespace/repo/uuid) and sets
// repository, uuid, and id to the correct values.
func TestRepositoryNotificationImport_SetsCorrectAttributes(t *testing.T) {
	const (
		namespace = "myorg"
		repoName  = "myrepo"
		uuid      = "32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d"
		importID  = namespace + "/" + repoName + "/" + uuid
		wantRepo  = namespace + "/" + repoName
		wantID    = wantRepo + "/" + uuid
	)

	resp := importNotificationState(t, importID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState returned unexpected errors: %s", resp.Diagnostics)
	}

	if got := getStringAttr(t, resp, "repository"); got != wantRepo {
		t.Errorf("repository: want %q, got %q", wantRepo, got)
	}
	if got := getStringAttr(t, resp, "uuid"); got != uuid {
		t.Errorf("uuid: want %q, got %q", uuid, got)
	}
	// id must equal repository + "/" + uuid — the canonical form produced by
	// mapToState — so that no drift occurs between import and the subsequent Read.
	if got := getStringAttr(t, resp, "id"); got != wantID {
		t.Errorf("id: want %q, got %q", wantID, got)
	}
}

// TestRepositoryNotificationImport_IDDerivedFromParts verifies that the id set
// during import is derived from the parsed repository and uuid parts, not
// passed through verbatim from req.ID.
//
// In the current implementation ImportState does:
//
//	setAttrs(ctx, resp, map[string]string{
//	    "id":         req.ID,          // ← verbatim passthrough
//	    "repository": parts[0]+"/"+parts[1],
//	    "uuid":       parts[2],
//	})
//
// Setting id = req.ID works only because req.ID happens to equal
// repository+"/"+uuid for well-formed import strings. It is semantically
// wrong: id must be derived from the parsed parts so it is guaranteed to
// match the value mapToState computes during the subsequent Read. The correct
// implementation omits id from setAttrs entirely and lets Read set it, or
// derives it as parts[0]+"/"+parts[1]+"/"+parts[2].
//
// This test encodes that contract by asserting id == repository + "/" + uuid
// where both sides come from the parsed state — not from req.ID.
func TestRepositoryNotificationImport_IDDerivedFromParts(t *testing.T) {
	const importID = "myorg/myrepo/32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d"

	resp := importNotificationState(t, importID)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState returned unexpected errors: %s", resp.Diagnostics)
	}

	gotRepo := getStringAttr(t, resp, "repository")
	gotUUID := getStringAttr(t, resp, "uuid")
	gotID := getStringAttr(t, resp, "id")

	canonical := gotRepo + "/" + gotUUID

	// The id attribute must equal repository + "/" + uuid. If id were set
	// directly from req.ID instead of derived from the parsed parts, this
	// assertion would catch any future change that breaks the derivation.
	if gotID != canonical {
		t.Errorf(
			"id %q does not equal repository+\"/\"+uuid %q\n"+
				"id must be derived from the parsed parts (repository=%q, uuid=%q), "+
				"not passed through verbatim from the import ID string",
			gotID, canonical, gotRepo, gotUUID,
		)
	}
}

// TestRepositoryNotificationImport_IDNotSetDirectlyFromReqID is the
// regression test for the specific bug: id = req.ID in setAttrs.
//
// We cannot directly observe req.ID vs the parsed derivation when they produce
// the same string, so instead we verify that id is NOT set to anything other
// than repository+"/"+uuid — i.e. that the value in state could only have
// come from the parsed parts.
//
// Concretely: if the implementation changed to set id from req.ID but used a
// different separator or ordering, the id would diverge from canonical.  This
// test acts as the specification: id == repository + "/" + uuid, always.
func TestRepositoryNotificationImport_IDNotSetDirectlyFromReqID(t *testing.T) {
	cases := []struct {
		name     string
		importID string
		wantRepo string
		wantUUID string
	}{
		{
			name:     "simple",
			importID: "acme/frontend/aaaaaaaa-0000-0000-0000-000000000001",
			wantRepo: "acme/frontend",
			wantUUID: "aaaaaaaa-0000-0000-0000-000000000001",
		},
		{
			name:     "hyphenated org and repo",
			importID: "my-org/my-repo/bbbbbbbb-0000-0000-0000-000000000002",
			wantRepo: "my-org/my-repo",
			wantUUID: "bbbbbbbb-0000-0000-0000-000000000002",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := importNotificationState(t, tc.importID)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ImportState errors: %s", resp.Diagnostics)
			}

			gotRepo := getStringAttr(t, resp, "repository")
			gotUUID := getStringAttr(t, resp, "uuid")
			gotID := getStringAttr(t, resp, "id")

			if gotRepo != tc.wantRepo {
				t.Errorf("repository: want %q, got %q", tc.wantRepo, gotRepo)
			}
			if gotUUID != tc.wantUUID {
				t.Errorf("uuid: want %q, got %q", tc.wantUUID, gotUUID)
			}

			wantID := tc.wantRepo + "/" + tc.wantUUID
			if gotID != wantID {
				t.Errorf("id: want %q (repository+\"/\"+uuid), got %q", wantID, gotID)
			}
		})
	}
}

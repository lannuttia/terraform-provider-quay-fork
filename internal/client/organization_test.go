package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

// TestOrganization_MarshalZeroValues ensures that zero/false values on the
// Organization response struct are preserved when the struct is marshalled to
// JSON. Fields tagged with omitempty silently drop false booleans, empty
// strings, and zero integers, which corrupts any downstream code that
// re-serialises the struct (e.g. caching, logging, or future client features).
func TestOrganization_MarshalZeroValues(t *testing.T) {
	org := client.Organization{
		Name:                "myorg",
		Email:               "",
		InvoiceEmail:        false,
		InvoiceEmailAddress: "",
		TagExpirationS:      0,
		IsOrganization:      false,
		IsFreeAccount:       false,
	}

	data, err := json.Marshal(org)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	requiredKeys := []string{
		"invoice_email",
		"invoice_email_address",
		"tag_expiration_s",
		"is_organization",
		"is_free_account",
	}
	for _, key := range requiredKeys {
		if _, exists := got[key]; !exists {
			t.Errorf("key %q was missing from marshalled Organization; omitempty is dropping the zero value", key)
		}
	}
}

func TestCreateOrganization(t *testing.T) {
	created := false
	srv, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organization/"):
			created = true
			writeJSON(w, http.StatusCreated, map[string]string{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/organization/myorg"):
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: "a@b.com"})
		default:
			http.NotFound(w, r)
		}
	})
	_ = srv

	org, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name:  "myorg",
		Email: "a@b.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("POST /organization/ was never called")
	}
	if org.Name != "myorg" {
		t.Errorf("expected name myorg, got %q", org.Name)
	}
}

func TestGetOrganization_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetOrganization(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestUpdateOrganization(t *testing.T) {
	updated := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			updated = true
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: "new@b.com"})
		}
	})
	org, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		Email: "new@b.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Error("PUT was never called")
	}
	if org.Email != "new@b.com" {
		t.Errorf("expected email new@b.com, got %q", org.Email)
	}
}

func TestDeleteOrganization(t *testing.T) {
	deleted := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if err := c.DeleteOrganization(context.Background(), "myorg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DELETE was never called")
	}
}

// TestUpdateOrganization_InvoiceEmailFalse ensures that false is included in
// the PUT request body when invoice_email is set to false. Previously omitempty
// on the bool field caused false to be silently dropped, so Quay kept the old
// value instead of disabling invoice emails.
func TestUpdateOrganization_InvoiceEmailFalse(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", InvoiceEmail: false})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		InvoiceEmail: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["invoice_email"]
	if !exists {
		t.Fatal("invoice_email was missing from the PUT request body")
	}
	if v != false {
		t.Errorf("expected invoice_email=false in request body, got %v", v)
	}
}

// TestUpdateOrganization_EmailEmpty ensures that an empty string is included in
// the PUT request body when email is explicitly set to "". The Quay API treats
// an empty email as a request to clear the field (it stores a random UUID
// internally and returns "" on subsequent GETs). With omitempty on the field
// the empty string is silently dropped, making it impossible to clear the email
// through the provider.
func TestUpdateOrganization_EmailEmpty(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: ""})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		Email: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["email"]
	if !exists {
		t.Fatal("email was missing from the PUT request body; omitempty prevents clearing the field")
	}
	if v != "" {
		t.Errorf("expected email=\"\" in request body, got %v", v)
	}
}

// TestUpdateOrganization_InvoiceEmailAddressEmpty ensures that an empty string
// is included in the PUT request body when invoice_email_address is explicitly
// set to "". The Quay API schema marks this field as nullable (type:
// ["string","null"]), so sending "" clears it. With omitempty on the field the
// empty string is silently dropped, making it impossible to clear the address
// through the provider.
func TestUpdateOrganization_InvoiceEmailAddressEmpty(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", InvoiceEmailAddress: ""})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		InvoiceEmailAddress: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["invoice_email_address"]
	if !exists {
		t.Fatal("invoice_email_address was missing from the PUT request body; omitempty prevents clearing the field")
	}
	if v != "" {
		t.Errorf("expected invoice_email_address=\"\" in request body, got %v", v)
	}
}

// TestUpdateOrganization_TagExpirationZero ensures that 0 is included in the
// PUT request body when tag_expiration_s is set to 0. Previously omitempty on
// the int field caused 0 to be silently dropped, preventing users from
// clearing the tag expiration.
func TestUpdateOrganization_TagExpirationZero(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", TagExpirationS: 0})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		TagExpirationS: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["tag_expiration_s"]
	if !exists {
		t.Fatal("tag_expiration_s was missing from the PUT request body")
	}
	if v != float64(0) {
		t.Errorf("expected tag_expiration_s=0 in request body, got %v", v)
	}
}

// TestCreateOrganization_GetFailsAfterPost simulates a transient failure on
// the follow-up GET that CreateOrganization issues after a successful POST.
// The Quay POST /organization/ endpoint returns an empty 201 body, so the GET
// is unavoidable. However, when the GET fails the error must make it clear
// that the organization was already created — otherwise operators will re-run
// apply, hit a 409 conflict, and be left with an orphaned resource and no
// Terraform state entry.
func TestCreateOrganization_GetFailsAfterPost(t *testing.T) {
	postCalled := false
	getCalled := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organization/"):
			postCalled = true
			writeJSON(w, http.StatusCreated, map[string]string{})
		case r.Method == http.MethodGet:
			getCalled = true
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "unavailable"})
		default:
			http.NotFound(w, r)
		}
	})

	_, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name:  "myorg",
		Email: "a@b.com",
	})

	if !postCalled {
		t.Fatal("POST /organization/ was never called")
	}
	if !getCalled {
		t.Fatal("GET /organization/myorg was never called")
	}
	if err == nil {
		t.Fatal("expected error when GET fails after POST, got nil")
	}
	// The error message must warn the operator that the organization was
	// already created, so they know not to re-run apply without checking.
	if !strings.Contains(err.Error(), "already created") {
		t.Errorf("error should mention that the org was already created; got: %v", err)
	}
}

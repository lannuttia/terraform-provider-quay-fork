package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quay/terraform-provider-quay/internal/client"
)

// newTestServer starts an httptest server. The handler receives the full request
// and writes a JSON response. It returns the server and a *client.Client pointed
// at it with retry delays set to zero so tests run at full speed.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, client.NewTestClient(srv.URL, "test-token")
}

// writeJSON is a helper that serialises v as JSON and writes it with status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ----- IsNotFound ---------------------------------------------------------

func TestIsNotFound_True(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetRepository(context.Background(), "org/missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound=true, got false for %v", err)
	}
}

func TestIsNotFound_False(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "boom"})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if client.IsNotFound(err) {
		t.Errorf("expected IsNotFound=false, got true for %v", err)
	}
}

// ----- Authorization header -----------------------------------------------

// ----- Timeout ------------------------------------------------------------

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block long enough for a short timeout to fire.
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	}))
	t.Cleanup(srv.Close)

	c := client.New(srv.URL, "test-token", 50*time.Millisecond)
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// The error should be a context / deadline exceeded or a net timeout — not
	// an APIError, since the server never responded.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("expected a transport-level timeout error, got APIError: %v", apiErr)
	}
}

// ----- Authorization header -----------------------------------------------

func TestAuthorizationHeader(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	})
	org, err := c.GetRepository(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org == nil {
		t.Fatal("expected non-nil organization")
	}
}

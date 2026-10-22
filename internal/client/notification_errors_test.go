package client_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateRepoNotification_Unauthorized(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message":    "Unauthorized",
			"error_type": "invalid_token",
		})
	})
	_, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo",
		client.CreateNotificationRequest{
			Event:       "repo_push",
			Method:      "webhook",
			Config:      map[string]any{"url": "https://example.com"},
			EventConfig: map[string]any{},
		})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("unauthorized should not be IsNotFound")
	}
}

func TestCreateRepoNotification_RepositoryNotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.CreateRepoNotification(context.Background(), "myorg/missing",
		client.CreateNotificationRequest{
			Event:       "repo_push",
			Method:      "webhook",
			Config:      map[string]any{"url": "https://example.com"},
			EventConfig: map[string]any{},
		})
	if err == nil {
		t.Fatal("expected error for missing repo, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing repo, got: %v", err)
	}
}

func TestDeleteRepoNotification_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	err := c.DeleteRepoNotification(context.Background(), "myorg/myrepo", "missing-uuid")
	if err == nil {
		t.Fatal("expected error for delete of non-existent notification, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing notification, got: %v", err)
	}
}

func TestDeleteRepoNotification_Forbidden(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":    "Forbidden",
			"error_type": "insufficient_scope",
		})
	})
	err := c.DeleteRepoNotification(context.Background(), "otherorg/repo", "some-uuid")
	if err == nil {
		t.Fatal("expected error for forbidden delete, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("forbidden should not be IsNotFound")
	}
}

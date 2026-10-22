package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lannuttia/terraform-provider-quay/internal/client"
)

func TestCreateRepoNotification(t *testing.T) {
	created := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/notification/") {
			created = true
			writeJSON(w, http.StatusCreated, client.Notification{
				UUID:        "abc-123",
				Event:       "repo_push",
				Method:      "webhook",
				Config:      map[string]any{"url": "https://example.com"},
				EventConfig: map[string]any{},
			})
		}
	})

	notif, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo", client.CreateNotificationRequest{
		Event:       "repo_push",
		Method:      "webhook",
		Config:      map[string]any{"url": "https://example.com"},
		EventConfig: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("POST notification endpoint was never called")
	}
	if notif.UUID != "abc-123" {
		t.Errorf("expected UUID abc-123, got %q", notif.UUID)
	}
}

func TestGetRepoNotification(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, client.Notification{
				UUID:   "abc-123",
				Event:  "repo_push",
				Method: "webhook",
				Config: map[string]any{"url": "https://example.com"},
			})
		}
	})

	notif, err := c.GetRepoNotification(context.Background(), "myorg/myrepo", "abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notif.UUID != "abc-123" {
		t.Errorf("expected UUID abc-123, got %q", notif.UUID)
	}
}

func TestDeleteRepoNotification(t *testing.T) {
	deleted := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if err := c.DeleteRepoNotification(context.Background(), "myorg/myrepo", "abc-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DELETE was never called")
	}
}

func TestGetRepoNotification_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetRepoNotification(context.Background(), "myorg/myrepo", "missing")
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

// TestCreateRepoNotification_VulnerabilityLevelSentAsInt32 asserts that the
// client passes the level integer through to the API request body unchanged.
func TestCreateRepoNotification_VulnerabilityLevelSentAsInt32(t *testing.T) {
	var capturedBody map[string]any

	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, client.Notification{
				UUID:        "abc-123",
				Event:       "vulnerability_found",
				Method:      "webhook",
				Config:      map[string]any{"url": "https://example.com"},
				EventConfig: map[string]any{"level": float64(1)},
			})
		}
	})

	_, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo", client.CreateNotificationRequest{
		Event:       "vulnerability_found",
		Method:      "webhook",
		Config:      map[string]any{"url": "https://example.com"},
		EventConfig: map[string]any{"level": int32(1)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eventConfig, _ := capturedBody["eventConfig"].(map[string]any)
	if eventConfig == nil {
		t.Fatal("request body missing eventConfig")
	}
	rawLevel := eventConfig["level"]
	switch v := rawLevel.(type) {
	case float64:
		if v != 1 {
			t.Errorf("eventConfig.level: want 1, got %v", v)
		}
	case string:
		t.Errorf("eventConfig.level was sent as a string %q; Quay expects an integer", v)
	default:
		t.Errorf("eventConfig.level: unexpected type %T with value %v", rawLevel, rawLevel)
	}
}

// TestEventConfig_VulnerabilityLevelInt32 asserts that EventConfig.VulnerabilityLevelInt32
// returns the level as an int32 and false when the key is absent.
func TestEventConfig_VulnerabilityLevelInt32(t *testing.T) {
	tests := []struct {
		name      string
		config    client.EventConfig
		wantLevel int32
		wantOK    bool
	}{
		{"critical", client.EventConfig{"level": float64(0)}, 0, true},
		{"high", client.EventConfig{"level": float64(1)}, 1, true},
		{"medium", client.EventConfig{"level": float64(2)}, 2, true},
		{"low", client.EventConfig{"level": float64(3)}, 3, true},
		{"negligible", client.EventConfig{"level": float64(4)}, 4, true},
		{"unknown", client.EventConfig{"level": float64(5)}, 5, true},
		{"absent", client.EventConfig{}, 0, false},
		{"nil value", client.EventConfig{"level": nil}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.config.VulnerabilityLevelInt32()
			if ok != tt.wantOK {
				t.Fatalf("VulnerabilityLevelInt32() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantLevel {
				t.Errorf("VulnerabilityLevelInt32() = %d, want %d", got, tt.wantLevel)
			}
		})
	}
}

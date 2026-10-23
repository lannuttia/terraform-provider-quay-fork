package client

import (
	"context"
	"fmt"
)

// EventConfig holds the event-specific configuration returned by the Quay API.
type EventConfig map[string]any

// vulnerabilityLevels maps the Quay severity name to its API integer index.
// Order is significant: position == index.
var vulnerabilityLevels = []string{
	"Critical",
	"High",
	"Medium",
	"Low",
	"Negligible",
	"Unknown",
}

// VulnerabilityLevelIndexFromString returns the API integer index for a Quay
// severity name. Case-sensitive. Returns (0, false) if unrecognised.
func VulnerabilityLevelIndexFromString(name string) (int32, bool) {
	for i, s := range vulnerabilityLevels {
		if s == name {
			return int32(i), true
		}
	}
	return 0, false
}

// VulnerabilityLevelInt32 returns the level index from the event config.
// Returns false if the key is absent or nil.
func (e EventConfig) VulnerabilityLevelInt32() (int32, bool) {
	v, ok := e["level"]
	if !ok || v == nil {
		return 0, false
	}
	if f, ok := v.(float64); ok {
		return int32(f), true
	}
	return 0, false
}

// VulnerabilityLevelString returns the Quay severity name for the level index
// in the event config. Returns ("", false) if absent or out of range.
func (e EventConfig) VulnerabilityLevelString() (string, bool) {
	idx, ok := e.VulnerabilityLevelInt32()
	if !ok {
		return "", false
	}
	if int(idx) < 0 || int(idx) >= len(vulnerabilityLevels) {
		return "", false
	}
	return vulnerabilityLevels[idx], true
}

// Notification represents a Quay repository notification.
type Notification struct {
	UUID             string         `json:"uuid"`
	Title            string         `json:"title,omitempty"`
	Event            string         `json:"event"`
	EventConfig      EventConfig    `json:"event_config"`
	Method           string         `json:"method"`
	Config           map[string]any `json:"config"`
	NumberOfFailures int            `json:"number_of_failures,omitempty"`
}

// CreateNotificationRequest is the request body for creating a notification.
// The Quay API uses camelCase "eventConfig" in requests but "event_config" in responses.
type CreateNotificationRequest struct {
	Event       string         `json:"event"`
	Method      string         `json:"method"`
	Config      map[string]any `json:"config"`
	EventConfig map[string]any `json:"eventConfig"`
	Title       string         `json:"title,omitempty"`
}

func (c *Client) CreateRepoNotification(ctx context.Context, repository string, req CreateNotificationRequest) (*Notification, error) {
	var notif Notification
	if err := c.post(ctx, fmt.Sprintf("repository/%s/notification/", repository), req, &notif); err != nil {
		return nil, fmt.Errorf("creating notification on repository %q: %w", repository, err)
	}
	created, err := c.GetRepoNotification(ctx, repository, notif.UUID)
	if err != nil {
		return nil, fmt.Errorf("notification on repository %q was created (uuid %q) but could not be read back: %w", repository, notif.UUID, err)
	}
	return created, nil
}

func (c *Client) GetRepoNotification(ctx context.Context, repository, uuid string) (*Notification, error) {
	var notif Notification
	if err := c.get(ctx, fmt.Sprintf("repository/%s/notification/%s", repository, uuid), &notif); err != nil {
		return nil, fmt.Errorf("getting notification %q on repository %q: %w", uuid, repository, err)
	}
	return &notif, nil
}

func (c *Client) DeleteRepoNotification(ctx context.Context, repository, uuid string) error {
	if err := c.delete(ctx, fmt.Sprintf("repository/%s/notification/%s", repository, uuid)); err != nil {
		return fmt.Errorf("deleting notification %q on repository %q: %w", uuid, repository, err)
	}
	return nil
}

package client

import (
	"context"
	"fmt"
)

// EventConfig holds the event-specific configuration returned by the Quay API.
type EventConfig map[string]any

// VulnerabilityLevelInt32 extracts the vulnerability severity level from the
// event config as an int32. The Quay API stores the level as an integer index
// (0=Critical, 1=High, 2=Medium, 3=Low, 4=Negligible, 5=Unknown). Returns
// false if the key is absent or nil.
func (e EventConfig) VulnerabilityLevelInt32() (int32, bool) {
	v, ok := e["level"]
	if !ok || v == nil {
		return 0, false
	}
	// JSON numbers unmarshal as float64.
	if f, ok := v.(float64); ok {
		return int32(f), true
	}
	return 0, false
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
// Note: the Quay API uses camelCase "eventConfig" in request bodies but
// snake_case "event_config" in response bodies.
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
	return &notif, nil
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

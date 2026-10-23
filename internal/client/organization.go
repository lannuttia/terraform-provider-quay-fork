package client

import (
	"context"
	"fmt"
)

// Organization represents a Quay organization.
type Organization struct {
	Name                string `json:"name"`
	Email               string `json:"email"`
	InvoiceEmail        bool   `json:"invoice_email,omitempty"`
	InvoiceEmailAddress string `json:"invoice_email_address,omitempty"`
	TagExpirationS      int    `json:"tag_expiration_s,omitempty"`
	IsOrganization      bool   `json:"is_organization,omitempty"`
	IsFreeAccount       bool   `json:"is_free_account,omitempty"`
}

type CreateOrganizationRequest struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type UpdateOrganizationRequest struct {
	Email               string `json:"email"`
	InvoiceEmail        bool   `json:"invoice_email"`
	InvoiceEmailAddress string `json:"invoice_email_address"`
	TagExpirationS      int    `json:"tag_expiration_s"`
}

func (c *Client) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (*Organization, error) {
	if err := c.post(ctx, "organization/", req, nil); err != nil {
		return nil, fmt.Errorf("creating organization %q: %w", req.Name, err)
	}
	return c.GetOrganization(ctx, req.Name)
}

func (c *Client) GetOrganization(ctx context.Context, name string) (*Organization, error) {
	var org Organization
	if err := c.get(ctx, fmt.Sprintf("organization/%s", name), &org); err != nil {
		return nil, fmt.Errorf("getting organization %q: %w", name, err)
	}
	return &org, nil
}

func (c *Client) UpdateOrganization(ctx context.Context, name string, req UpdateOrganizationRequest) (*Organization, error) {
	if err := c.put(ctx, fmt.Sprintf("organization/%s", name), req, nil); err != nil {
		return nil, fmt.Errorf("updating organization %q: %w", name, err)
	}
	return c.GetOrganization(ctx, name)
}

func (c *Client) DeleteOrganization(ctx context.Context, name string) error {
	if err := c.delete(ctx, fmt.Sprintf("organization/%s", name)); err != nil {
		return fmt.Errorf("deleting organization %q: %w", name, err)
	}
	return nil
}

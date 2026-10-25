package client

import (
	"net/http"
	"time"
)

// NewTestClient creates a client with zero retry delays and no HTTP timeout,
// for use in tests only.
func NewTestClient(baseURL, token string) *Client {
	return New(baseURL, token, 0).withRetryDelay(func(int) time.Duration { return 0 })
}

// NewTestTLSClient creates a client that uses the provided *http.Client (e.g.
// one configured with a test TLS certificate) with zero retry delays and no
// HTTP timeout, for use in tests only. The CheckRedirect policy from New is
// applied to the provided http.Client.
func NewTestTLSClient(baseURL, token string, httpClient *http.Client) *Client {
	c := New(baseURL, token, 0).withRetryDelay(func(int) time.Duration { return 0 })
	httpClient.CheckRedirect = c.httpClient.CheckRedirect
	c.httpClient = httpClient
	return c
}

// Package alb is a read-only NSX Advanced Load Balancer (Avi) controller client.
//
// TODO(phase-3): implement against the Avi REST API. Two things to get right:
// the API version must be negotiated rather than hardcoded (the controller
// rejects unknown versions outright), and the login flow returns a session
// cookie plus a CSRF token that must be carried on subsequent calls.
//
// The HAProxy Data Plane API client, if it is still worth building, belongs
// here too — see the note in internal/checks/alb.
package alb

import (
	"context"
	"fmt"

	"github.com/modern-apps-5/vks-inspector/internal/clients"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// Client is a read-only ALB controller client.
type Client struct {
	endpoint string
	opts     clients.Options
}

// New constructs a client. It does not connect.
func New(endpoint string, _ creds.Credential, opts clients.Options) *Client {
	return &Client{endpoint: endpoint, opts: opts}
}

// About returns controller version and cluster state.
func (c *Client) About(ctx context.Context) (map[string]any, error) {
	return nil, fmt.Errorf("alb client is not implemented in phase 1")
}

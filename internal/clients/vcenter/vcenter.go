// Package vcenter is a read-only vCenter API client.
//
// TODO(phase-3): implement. Expected shape: session-authenticated REST client
// against the vSphere Automation API, with govmomi for the inventory calls the
// REST API does not cover well (VDS and portgroup detail in particular).
//
// Constraints that must hold when this is implemented:
//   - read-only: no POST/PUT/DELETE except the session-create call itself,
//     and the session must be explicitly deleted on exit so the tool does not
//     leave sessions behind on a customer's vCenter;
//   - credentials arrive as a creds.Credential and are never logged;
//   - every call is context-bounded;
//   - responses are recorded verbatim into testdata fixtures during lab runs,
//     which is how the credentialed checks become CI-testable.
package vcenter

import (
	"context"
	"fmt"

	"github.com/modern-apps-5/vks-inspector/internal/clients"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// Client is a read-only vCenter client.
type Client struct {
	endpoint string
	opts     clients.Options
}

// New constructs a client. It does not connect.
func New(endpoint string, _ creds.Credential, opts clients.Options) *Client {
	return &Client{endpoint: endpoint, opts: opts}
}

// About returns version and build information. First call any check makes; also
// the cheapest proof that credentials work.
func (c *Client) About(ctx context.Context) (map[string]any, error) {
	return nil, fmt.Errorf("vcenter client is not implemented in phase 1")
}

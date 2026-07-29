// Package nsx is a read-only NSX Manager API client.
//
// TODO(phase-3): implement against the NSX Policy API (/policy/api/v1). Prefer
// the Policy API over the older Manager API: the Policy object model is what
// VKS itself consumes, so checking it means checking what the product will
// actually see. Read-only, context-bounded, credentials never logged.
//
// The VPC object model (VCF 9) is the least-understood part of this surface and
// must be confirmed against a live NSX before any VPC check is written.
package nsx

import (
	"context"
	"fmt"

	"github.com/modern-apps-5/vks-inspector/internal/clients"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// Client is a read-only NSX Manager client.
type Client struct {
	endpoint string
	opts     clients.Options
}

// New constructs a client. It does not connect.
func New(endpoint string, _ creds.Credential, opts clients.Options) *Client {
	return &Client{endpoint: endpoint, opts: opts}
}

// About returns NSX version and node information.
func (c *Client) About(ctx context.Context) (map[string]any, error) {
	return nil, fmt.Errorf("nsx client is not implemented in phase 1")
}

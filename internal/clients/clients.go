// Package clients holds read-only management-plane API clients.
//
// Subpackages: vcenter, nsx, alb. Each is constructed from an endpoint plus a
// credential and exposes only read operations. This is enforced by convention
// and by review, not by the type system — so it is stated loudly here and in
// docs/ADR/0007-read-only-by-default.md: no method in this tree may create,
// modify or delete anything.
//
// Clients are constructed lazily and are allowed to fail. A client that cannot
// connect yields a missing capability, which yields skipped checks with a
// reason — never a failed check. "We could not log in to NSX" is a statement
// about the tool's access, not about the customer's network.
package clients

import (
	"crypto/tls"
	"fmt"
	"time"
)

// Options are common to every client.
type Options struct {
	// Timeout bounds a single API call.
	Timeout time.Duration
	// CACertFile pins the trust anchor for this endpoint.
	CACertFile string
	// InsecureSkipVerify disables verification. When set, every certificate
	// check against this endpoint must downgrade itself to informational and
	// say why — an unverified connection cannot evidence a valid chain.
	InsecureSkipVerify bool
	// UserAgent identifies the tool in target-side audit logs. Customers will
	// ask what made these calls; the answer should be in their logs.
	UserAgent string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		Timeout:   30 * time.Second,
		UserAgent: "vksinspect (read-only preflight)",
	}
}

// TLSConfig builds a TLS config from Options.
//
// TODO(phase-3): load CACertFile into a RootCAs pool.
func (o Options) TLSConfig() (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: o.InsecureSkipVerify, //nolint:gosec // opt-in, reported in results
	}
	if o.CACertFile != "" {
		return nil, fmt.Errorf("CACertFile is not implemented yet")
	}
	return cfg, nil
}

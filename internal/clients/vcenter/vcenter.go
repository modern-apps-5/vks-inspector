// Package vcenter is a read-only vCenter API client.
//
// Read-only is a contract, not a preference (docs/ADR/0007). The only write
// this package performs is creating a session, and it deletes that session on
// Close — a preflight tool must not leave sessions accumulating on a customer's
// vCenter. No method here may create, modify or delete anything else.
//
// Every method returns JSON-safe values so a check can put them straight into
// Result.Observed.Data, where drift can diff them.
package vcenter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	"github.com/modern-apps-5/vks-inspector/internal/clients"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// Client is a read-only vCenter client.
type Client struct {
	endpoint string
	cred     creds.Credential
	opts     clients.Options

	mu     sync.Mutex
	gc     *govmomi.Client
	finder *find.Finder
}

// New constructs a client. It does not connect — dialling belongs in Connect so
// a caller can decide when to pay for it and how to report the failure.
func New(endpoint string, cred creds.Credential, opts clients.Options) *Client {
	return &Client{endpoint: endpoint, cred: cred, opts: opts}
}

// Endpoint returns the address this client talks to. Safe to log.
func (c *Client) Endpoint() string { return c.endpoint }

// Connect authenticates and establishes a session.
//
// Errors are deliberately unwrapped into plain messages rather than passed
// through: govmomi's transport errors can echo the request URL, which carries
// the username. See docs/ADR/0005.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gc != nil {
		return nil
	}

	u, err := soap.ParseURL(normaliseEndpoint(c.endpoint))
	if err != nil {
		return fmt.Errorf("vcenter endpoint %q is not usable", c.endpoint)
	}
	if c.cred.Username == "" {
		return fmt.Errorf("no vcenter credentials supplied")
	}
	u.User = url.UserPassword(c.cred.Username, c.cred.Password)

	soapClient := soap.NewClient(u, c.opts.InsecureSkipVerify || c.cred.InsecureSkipVerify)
	if ca := caFile(c.cred, c.opts); ca != "" {
		if err := soapClient.SetRootCAs(ca); err != nil {
			return fmt.Errorf("load CA certificate %s: %w", ca, err)
		}
	}
	if c.opts.UserAgent != "" {
		soapClient.UserAgent = c.opts.UserAgent
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return fmt.Errorf("connect to vcenter: %s", scrub(err, c.cred))
	}
	gc := &govmomi.Client{Client: vimClient, SessionManager: session.NewManager(vimClient)}

	if err := gc.Login(ctx, u.User); err != nil {
		return fmt.Errorf("authenticate to vcenter: %s", scrub(err, c.cred))
	}

	c.gc = gc
	c.finder = find.NewFinder(vimClient, true)
	return nil
}

// Close ends the session. Always call it: a tool that leaves sessions behind on
// a customer's vCenter is doing something it was not asked to do.
func (c *Client) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gc == nil {
		return nil
	}
	err := c.gc.Logout(ctx)
	c.gc, c.finder = nil, nil
	if err != nil {
		return fmt.Errorf("log out of vcenter: %s", scrub(err, c.cred))
	}
	return nil
}

// About returns version and build information.
//
// The cheapest proof that the endpoint is a vCenter and the credentials work,
// which is why it is the first call every credentialed check depends on.
func (c *Client) About(ctx context.Context) (map[string]any, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return nil, err
	}
	a := gc.Client.ServiceContent.About
	return map[string]any{
		"name":         a.Name,
		"version":      a.Version,
		"build":        a.Build,
		"api_version":  a.ApiVersion,
		"api_type":     a.ApiType,
		"product_line": a.ProductLineId,
		"os_type":      a.OsType,
	}, nil
}

// IsVCenter reports whether the endpoint is a vCenter rather than a bare ESXi
// host. Pointing this tool at an ESXi host is a common mistake and produces
// confusing downstream failures if not caught early.
func (c *Client) IsVCenter(ctx context.Context) (bool, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return false, err
	}
	return gc.Client.ServiceContent.About.ApiType == "VirtualCenter", nil
}

func (c *Client) connected(ctx context.Context) (*govmomi.Client, error) {
	c.mu.Lock()
	gc := c.gc
	c.mu.Unlock()
	if gc != nil {
		return gc, nil
	}
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gc, nil
}

func (c *Client) find(ctx context.Context) (*find.Finder, error) {
	if _, err := c.connected(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finder, nil
}

// normaliseEndpoint accepts what an operator naturally types — a bare FQDN, a
// host:port, or a full URL — and produces the SDK path govmomi expects.
func normaliseEndpoint(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if !strings.Contains(s, "/sdk") {
		s = strings.TrimSuffix(s, "/") + "/sdk"
	}
	return s
}

func caFile(cred creds.Credential, opts clients.Options) string {
	if cred.CACertFile != "" {
		return cred.CACertFile
	}
	return opts.CACertFile
}

// scrub removes credential material from an error before it is shown or logged.
// govmomi embeds the request URL in some transport errors, and that URL carries
// userinfo.
func scrub(err error, cred creds.Credential) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if cred.Password != "" {
		msg = strings.ReplaceAll(msg, cred.Password, "<redacted>")
	}
	if cred.Username != "" {
		msg = strings.ReplaceAll(msg, cred.Username, "<user>")
	}
	return msg
}

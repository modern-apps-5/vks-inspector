package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// certHorizon is how far ahead an expiry is treated as imminent.
//
// Not a product requirement — a certificate expiring inside the deployment
// window produces failures far from their cause, and 90 days is the interval at
// which someone can still act. Stated here rather than buried so it can be
// argued with.
const certHorizon = 90 * 24 * time.Hour

// TLSChain asserts each declared TLS endpoint presents a chain that validates.
type TLSChain struct{}

var _ checks.Check = (*TLSChain)(nil)

// Meta implements checks.Check.
func (TLSChain) Meta() checks.Meta {
	return checks.Meta{
		ID:             "tls.chain",
		Title:          "Declared endpoints present a valid certificate chain",
		RequirementIDs: []string{"COM-CRT-001", "COM-CRT-002"},
		Category:       results.CategoryCerts,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Replace the certificate or install the issuing CA. A SAN that omits the " +
			"address actually used is the common failure.",
	}
}

// Run implements checks.Check.
func (c TLSChain) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	eps := managementEndpoints(rc.Config)
	if len(eps) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no endpoints declared")}, nil
	}

	var failures []results.Result
	inspected := 0

	for _, ep := range eps {
		ans := rc.Probes.InspectTLS(ctx, ep.Address(), ep.Name, nil)

		r := checks.NewResult(c.Meta(), rc, ep.Address())
		if !ep.Required {
			r.Severity = results.SeverityWarning
		}
		r.Expected = results.Value{
			Summary: ep.Address() + " presents a chain that validates for " + ep.Host(),
			Data:    map[string]any{"address": ep.Address(), "server_name": ep.Host()},
		}

		if ans.Err != nil {
			// Could not complete a handshake. Reachability is tcp.port-open's
			// finding, not this check's, so this is indeterminate rather than a
			// certificate failure.
			r.Status = results.StatusUnknown
			r.Observed = results.Value{
				Summary: fmt.Sprintf("could not inspect the certificate: %v", ans.Err),
				Data:    map[string]any{"error": ans.Err.Error()},
			}
			checks.Finish(rc, &r)
			failures = append(failures, r)
			continue
		}

		inspected++
		leaf := ans.Leaf()
		r.Observed = results.Value{
			Data: map[string]any{
				"subject":     leaf.Subject.CommonName,
				"issuer":      leaf.Issuer.CommonName,
				"not_after":   leaf.NotAfter.UTC().Format(time.RFC3339),
				"dns_names":   leaf.DNSNames,
				"thumbprint":  thumbprint(leaf.Raw),
				"chain_depth": len(ans.Chain),
				"verified":    ans.Verified,
			},
		}

		switch {
		case !ans.Verified:
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("chain for %s does not validate: %v", ep.Host(), ans.VerifyErr)
			r.Observed.Data["verify_error"] = ans.VerifyErr.Error()
		default:
			r.Status = results.StatusPass
			r.Observed.Summary = fmt.Sprintf("valid chain, issued by %s, expires %s",
				orUnknown(leaf.Issuer.CommonName), leaf.NotAfter.UTC().Format("2006-01-02"))
		}

		// A pinned thumbprint is a stronger assertion than chain validity, and
		// a mismatch matters even when the chain validates.
		if want := pinnedThumbprint(rc, ep); want != "" {
			got := thumbprint(leaf.Raw)
			if !strings.EqualFold(normaliseThumb(got), normaliseThumb(want)) {
				r.Status = results.StatusFail
				r.Observed.Summary = "certificate does not match the pinned thumbprint"
				r.Observed.Data["expected_thumbprint"] = want
				r.Remediation = "The presented certificate differs from the one pinned in the config. " +
					"Either the endpoint's certificate was replaced or this is not the endpoint you think it is."
			}
		}

		checks.Finish(rc, &r)
		if r.Status != results.StatusPass {
			failures = append(failures, r)
		}
	}

	if len(failures) > 0 {
		return failures, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d endpoint(s) presented a valid chain", inspected),
		map[string]any{"endpoints_inspected": inspected},
	)}, nil
}

// ---------------------------------------------------------------------------

// CertExpiry warns about certificates expiring inside the deployment window.
type CertExpiry struct{}

var _ checks.Check = (*CertExpiry)(nil)

// Meta implements checks.Check.
func (CertExpiry) Meta() checks.Meta {
	return checks.Meta{
		ID:             "tls.expiry",
		Title:          "No declared certificate expires imminently",
		RequirementIDs: []string{"COM-CRT-003"},
		Category:       results.CategoryCerts,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityWarning,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Renew before deploying. A certificate that expires mid-lifecycle produces " +
			"failures far from their cause.",
	}
}

// Run implements checks.Check.
func (c CertExpiry) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	eps := managementEndpoints(rc.Config)
	if len(eps) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no endpoints declared")}, nil
	}
	now := rc.Now()

	var failures []results.Result
	inspected := 0

	for _, ep := range eps {
		ans := rc.Probes.InspectTLS(ctx, ep.Address(), ep.Name, nil)
		if ans.Err != nil || ans.Leaf() == nil {
			continue // tls.chain reports the inability to inspect
		}
		inspected++
		leaf := ans.Leaf()

		remaining := leaf.NotAfter.Sub(now)
		if remaining > certHorizon {
			continue
		}

		r := checks.NewResult(c.Meta(), rc, ep.Address())
		r.Expected = results.Value{
			Summary: fmt.Sprintf("expires more than %d days from now", int(certHorizon.Hours()/24)),
			Data:    map[string]any{"horizon_days": int(certHorizon.Hours() / 24)},
		}
		r.Observed = results.Value{
			Data: map[string]any{
				"not_after":      leaf.NotAfter.UTC().Format(time.RFC3339),
				"days_remaining": int(remaining.Hours() / 24),
			},
		}
		if remaining <= 0 {
			// An expired certificate is not a warning about the future.
			r.Severity = results.SeverityBlocker
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("certificate for %s expired on %s",
				ep.Host(), leaf.NotAfter.UTC().Format("2006-01-02"))
		} else {
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("certificate for %s expires in %d day(s), on %s",
				ep.Host(), int(remaining.Hours()/24), leaf.NotAfter.UTC().Format("2006-01-02"))
		}
		checks.Finish(rc, &r)
		failures = append(failures, r)
	}

	if len(failures) > 0 {
		return failures, nil
	}
	if inspected == 0 {
		return []results.Result{skip(c.Meta(), rc, "no certificate could be inspected")}, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d certificate(s) valid for more than %d days",
			inspected, int(certHorizon.Hours()/24)),
		map[string]any{"certificates_inspected": inspected},
	)}, nil
}

// ---------------------------------------------------------------------------

func thumbprint(der []byte) string {
	sum := sha256.Sum256(der)
	hexed := hex.EncodeToString(sum[:])
	var parts []string
	for i := 0; i+2 <= len(hexed); i += 2 {
		parts = append(parts, strings.ToUpper(hexed[i:i+2]))
	}
	return strings.Join(parts, ":")
}

func normaliseThumb(s string) string {
	return strings.ToUpper(strings.NewReplacer(":", "", " ", "", "-", "").Replace(s))
}

func pinnedThumbprint(rc *checks.RunContext, ep endpoint) string {
	cfg := rc.Config
	switch ep.Source {
	case "infrastructure.vcenter":
		return cfg.Infrastructure.VCenter.ExpectedThumbprint
	case "infrastructure.nsxManager":
		if cfg.Infrastructure.NSXManager != nil {
			return cfg.Infrastructure.NSXManager.ExpectedThumbprint
		}
	case "alb.controller":
		if cfg.ALB != nil {
			return cfg.ALB.Controller.ExpectedThumbprint
		}
	}
	return ""
}

func orUnknown(s string) string {
	if s == "" {
		return "an unnamed issuer"
	}
	return s
}

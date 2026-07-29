package network

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
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

	all := mapConcurrent(ctx, eps, func(ctx context.Context, ep endpoint) results.Result {
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
			return r
		}

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
		case ep.unverifiedTLS(rc):
			// ADR-0005: disabling verification makes any certificate assertion
			// meaningless, and the tool says so rather than reporting a pass it
			// did not earn or a failure it chose not to look for.
			r.Status = results.StatusSkip
			r.Severity = results.SeverityInfo
			r.Observed.Summary = fmt.Sprintf(
				"certificate presented by %s was NOT verified (verification disabled for this endpoint)",
				ep.Host())
			r.Observed.Data["skip_reason"] = "TLS verification disabled"
			r.Remediation = "This run cannot say whether the chain is valid. Remove " +
				"insecureSkipVerify for this endpoint, or install its issuing CA, to get a real answer."
		case !ans.Verified:
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("chain for %s does not validate: %v", ep.Host(), ans.VerifyErr)
			r.Observed.Data["verify_error"] = ans.VerifyErr.Error()
			// The commonest cause, and one the tool can identify precisely:
			// connecting by IP to a certificate issued for a name.
			// Older certificates carry only a CN and no SAN entries, so fall
			// back to it — otherwise the case this diagnosis exists for is
			// exactly the one it misses.
			if names := certNames(leaf); ep.DeclaredByIP && len(names) > 0 {
				r.Observed.Summary = fmt.Sprintf(
					"you connected to %s by IP, but its certificate is issued for %s",
					ep.Host(), strings.Join(names, ", "))
				r.Observed.Data["certificate_names"] = names
				r.Remediation = fmt.Sprintf(
					"Declare this endpoint by name (%s) instead of by IP, and make sure that name "+
						"resolves. A certificate is validated against the address you connect to, so "+
						"connecting by IP fails name matching even when the certificate is perfectly "+
						"good. If the issuing CA is also untrusted, install it or pass "+
						"--insecure-skip-tls-verify.",
					names[0])
			}
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
		return r
	})

	var failures []results.Result
	inspected := 0
	for _, r := range all {
		if r.Observed.Data["verified"] != nil {
			inspected++
		}
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
		if ep.unverifiedTLS(rc) {
			// The expiry date is readable regardless of trust, so this check
			// still means something — but the reader must not take it as any
			// statement about the chain, which tls.chain could not verify.
			r.Evidence = map[string]any{"note": "chain was not verified for this endpoint; expiry only"}
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
	summary := fmt.Sprintf("%d certificate(s) valid for more than %d days",
		inspected, int(certHorizon.Hours()/24))
	data := map[string]any{"certificates_inspected": inspected}
	// Expiry says nothing about trust. Next to a tls.chain failure on the same
	// endpoint, a bare pass here reads as "the certificate is fine".
	if unverified := unverifiedCount(rc, eps); unverified > 0 {
		summary += fmt.Sprintf("; %d had no verified chain — this covers expiry only", unverified)
		data["unverified_chains"] = unverified
	}
	return []results.Result{summaryPass(c.Meta(), rc, summary, data)}, nil
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

// certNames returns the names a certificate is valid for, preferring SANs and
// falling back to the common name.
func certNames(leaf *x509.Certificate) []string {
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames
	}
	if leaf.Subject.CommonName != "" {
		return []string{leaf.Subject.CommonName}
	}
	return nil
}

func unverifiedCount(rc *checks.RunContext, eps []endpoint) int {
	n := 0
	for _, ep := range eps {
		if ep.unverifiedTLS(rc) {
			n++
		}
	}
	return n
}

func orUnknown(s string) string {
	if s == "" {
		return "an unnamed issuer"
	}
	return s
}

package network

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Forward asserts every declared name resolves, from every declared resolver.
//
// Queried per resolver on purpose. "DNS works" from the operator's laptop says
// nothing about whether the resolver the Supervisor will actually use can
// answer, and that gap is one of the most common ways a green preflight is
// followed by a failed deployment.
type Forward struct{}

var _ checks.Check = (*Forward)(nil)

// Meta implements checks.Check.
func (Forward) Meta() checks.Meta {
	return checks.Meta{
		ID:             "dns.forward",
		Title:          "Declared names resolve from every declared resolver",
		RequirementIDs: []string{"COM-DNS-001"},
		Category:       results.CategoryDNS,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Add the missing A record on every declared resolver. Resolution must work " +
			"from the management segment, not only from the operator's workstation.",
	}
}

// Run implements checks.Check.
func (c Forward) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	targets := resolvableNames(rc.Config)
	if len(targets) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no names declared to resolve")}, nil
	}
	servers := resolvers(rc.Config)

	// One work item per (name, resolver) pair, probed concurrently. Sequential
	// lookups at a five-second timeout make an operator wait minutes for a
	// check that can finish in seconds.
	type job struct {
		target namedTarget
		server string
	}
	var jobs []job
	for _, t := range targets {
		for _, server := range servers {
			jobs = append(jobs, job{target: t, server: server})
		}
	}

	all := mapConcurrent(ctx, jobs, func(ctx context.Context, j job) results.Result {
		t, server := j.target, j.server
		ans := rc.Probes.LookupHost(ctx, t.Name, server)

		r := checks.NewResult(c.Meta(), rc, t.Name+" via "+resolverLabel(server))
		r.Expected = results.Value{
			Summary: expectedSummary(t),
			Data:    map[string]any{"name": t.Name, "resolver": resolverLabel(server), "expected_ip": t.ExpectIP},
		}

		switch {
		case ans.Timeout:
			// Silence is not a NXDOMAIN. We did not observe a missing record,
			// so we do not assert one.
			r.Status = results.StatusUnknown
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s did not answer for %s within the timeout", resolverLabel(server), t.Name),
				Data:    map[string]any{"resolved": nil, "timeout": true},
			}
			r.Remediation = "The resolver did not answer at all. Confirm it is reachable on 53/udp " +
				"from this vantage point before treating the record as missing."
		case ans.Err != nil:
			r.Status = results.StatusFail
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s could not resolve %s: %v", resolverLabel(server), t.Name, ans.Err),
				Data:    map[string]any{"resolved": nil, "error": ans.Err.Error()},
			}
		case len(ans.Addrs) == 0:
			r.Status = results.StatusFail
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s returned no addresses for %s", resolverLabel(server), t.Name),
				Data:    map[string]any{"resolved": []string{}},
			}
		default:
			got := addrStrings(ans.Addrs)
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s resolved to %s", t.Name, strings.Join(got, ", ")),
				Data:    map[string]any{"resolved": got},
			}
			if t.ExpectIP != "" && !contains(got, t.ExpectIP) {
				// A name resolving to the wrong address is worse than one that
				// does not resolve: the deployment proceeds and talks to
				// something else.
				r.Status = results.StatusFail
				r.Observed.Summary = fmt.Sprintf("%s resolved to %s, not the declared %s",
					t.Name, strings.Join(got, ", "), t.ExpectIP)
				r.Remediation = "The record exists but points elsewhere. Either the DNS record or " +
					"the address declared in the config is wrong, and a deployment will silently " +
					"talk to whatever the record points at."
			} else {
				r.Status = results.StatusPass
			}
		}
		checks.Finish(rc, &r)
		return r
	})

	checked := len(jobs)
	var failures []results.Result
	for _, r := range all {
		if r.Status != results.StatusPass {
			failures = append(failures, r)
		}
	}

	if len(failures) > 0 {
		return failures, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d name(s) resolved correctly from %d resolver(s)", len(targets), len(servers)),
		map[string]any{"names_checked": len(targets), "resolvers": len(servers), "lookups": checked},
	)}, nil
}

// ---------------------------------------------------------------------------

// Reverse asserts PTR records agree with forward records.
type Reverse struct{}

var _ checks.Check = (*Reverse)(nil)

// Meta implements checks.Check.
func (Reverse) Meta() checks.Meta {
	return checks.Meta{
		ID:             "dns.reverse",
		Title:          "Reverse (PTR) records agree with forward records",
		RequirementIDs: []string{"COM-DNS-002"},
		Category:       results.CategoryDNS,
		Layer:          results.LayerSupervisor,
		// Warning by default, escalated per-result when the config declares
		// services.dns.requireReverse. Matrix row COM-DNS-002 is flagged
		// precisely on whether PTR is a hard requirement for VCF 9; until that
		// is confirmed, the site decides rather than this tool guessing.
		Severity: results.SeverityWarning,
		Modes:    checks.AllModes,
		Needs:    []checks.Capability{checks.CapNetwork},
		Remediation: "Create matching PTR records in the reverse zone. Set " +
			"services.dns.requireReverse: true to treat a mismatch as a blocker.",
	}
}

// Run implements checks.Check.
func (c Reverse) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	targets := reverseTargets(rc.Config)
	if len(targets) == 0 {
		return []results.Result{skip(c.Meta(), rc,
			"no endpoint declares both an FQDN and an IP, so there is nothing to compare")}, nil
	}
	required := rc.Config.Services.DNS.RequireReverse
	servers := resolvers(rc.Config)

	all := mapConcurrent(ctx, targets, func(ctx context.Context, t namedTarget) results.Result {
		r := checks.NewResult(c.Meta(), rc, t.ExpectIP)
		if required {
			// The config declared this a hard requirement. The check is not
			// deciding importance — it is applying a decision the site made.
			r.Severity = results.SeverityBlocker
			r.Evidence = map[string]any{"escalated_by": "services.dns.requireReverse"}
		}
		r.Expected = results.Value{
			Summary: fmt.Sprintf("%s resolves back to %s", t.ExpectIP, t.Name),
			Data:    map[string]any{"address": t.ExpectIP, "expected_name": t.Name},
		}

		addr, err := netip.ParseAddr(t.ExpectIP)
		if err != nil {
			r.Status = results.StatusSkip
			r.Observed = results.Value{
				Summary: t.ExpectIP + " is not a usable address",
				Data:    map[string]any{"skip_reason": "unparseable address"},
			}
			checks.Finish(rc, &r)
			return r
		}

		// One resolver is enough for PTR: unlike forward records the reverse
		// zone is rarely split, and querying every resolver multiplies noise
		// without adding information.
		ans := rc.Probes.LookupAddr(ctx, addr, servers[0])

		switch {
		case ans.Timeout:
			r.Status = results.StatusUnknown
			r.Observed = results.Value{
				Summary: fmt.Sprintf("no answer for the PTR of %s", t.ExpectIP),
				Data:    map[string]any{"names": nil, "timeout": true},
			}
		case ans.Err != nil || len(ans.Names) == 0:
			r.Status = results.StatusFail
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s has no PTR record", t.ExpectIP),
				Data:    map[string]any{"names": []string{}},
			}
		default:
			r.Observed = results.Value{
				Summary: fmt.Sprintf("%s resolves to %s", t.ExpectIP, strings.Join(ans.Names, ", ")),
				Data:    map[string]any{"names": ans.Names},
			}
			if containsFold(ans.Names, t.Name) {
				r.Status = results.StatusPass
			} else {
				r.Status = results.StatusFail
				r.Observed.Summary = fmt.Sprintf("%s resolves to %s, not %s",
					t.ExpectIP, strings.Join(ans.Names, ", "), t.Name)
			}
		}
		checks.Finish(rc, &r)
		return r
	})

	var failures []results.Result
	for _, r := range all {
		if r.Status != results.StatusPass {
			failures = append(failures, r)
		}
	}

	if len(failures) > 0 {
		return failures, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d address(es) resolve back to their declared name", len(targets)),
		map[string]any{"addresses_checked": len(targets)},
	)}, nil
}

// ---------------------------------------------------------------------------

// ResolverAgreement reports resolvers that disagree about the same name.
//
// FLAGGED in the matrix (COM-DNS-005) as a field diagnostic rather than a
// sourced product requirement, and deliberately kept at warning severity for
// that reason. It catches a real and painful failure: disagreeing resolvers
// produce faults that move between reboots.
type ResolverAgreement struct{}

var _ checks.Check = (*ResolverAgreement)(nil)

// Meta implements checks.Check.
func (ResolverAgreement) Meta() checks.Meta {
	return checks.Meta{
		ID:             "dns.resolver-agreement",
		Title:          "Declared resolvers agree with each other",
		RequirementIDs: []string{"COM-DNS-005"},
		Category:       results.CategoryDNS,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityWarning,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Reconcile the split-horizon zones. Resolvers that disagree produce failures " +
			"that appear to move between reboots. Note this is a diagnostic, not a documented " +
			"product requirement (matrix COM-DNS-005).",
	}
}

// Run implements checks.Check.
func (c ResolverAgreement) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	servers := resolvers(rc.Config)
	if len(servers) < 2 {
		return []results.Result{skip(c.Meta(), rc,
			"fewer than two resolvers declared, so there is nothing to compare")}, nil
	}
	targets := resolvableNames(rc.Config)
	if len(targets) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no names declared to compare")}, nil
	}

	var failures []results.Result
	for _, t := range targets {
		answers := map[string][]string{}
		for _, s := range servers {
			ans := rc.Probes.LookupHost(ctx, t.Name, s)
			if ans.Err != nil {
				continue // resolvability is dns.forward's problem, not this check's
			}
			answers[resolverLabel(s)] = addrStrings(ans.Addrs)
		}
		if len(answers) < 2 {
			continue
		}

		var first []string
		agree := true
		for _, got := range answers {
			if first == nil {
				first = got
				continue
			}
			if !sameSet(first, got) {
				agree = false
			}
		}
		if agree {
			continue
		}

		r := checks.NewResult(c.Meta(), rc, t.Name)
		r.Status = results.StatusFail
		r.Expected = results.Value{
			Summary: "all resolvers return the same addresses for " + t.Name,
			Data:    map[string]any{"name": t.Name},
		}
		r.Observed = results.Value{
			Summary: t.Name + " resolves differently depending on the resolver",
			Data:    map[string]any{"answers": answers},
		}
		checks.Finish(rc, &r)
		failures = append(failures, r)
	}

	if len(failures) > 0 {
		return failures, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d resolver(s) agree on %d name(s)", len(servers), len(targets)),
		map[string]any{"resolvers": len(servers), "names_checked": len(targets)},
	)}, nil
}

// ---------------------------------------------------------------------------

func addrStrings(addrs []netip.Addr) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}
	sort.Strings(out)
	return out
}

func expectedSummary(t namedTarget) string {
	if t.ExpectIP != "" {
		return fmt.Sprintf("%s resolves to %s", t.Name, t.ExpectIP)
	}
	return t.Name + " resolves"
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func containsFold(items []string, want string) bool {
	for _, s := range items {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func resolverLabel(r string) string {
	if r == "" {
		return "the system resolver"
	}
	return r
}

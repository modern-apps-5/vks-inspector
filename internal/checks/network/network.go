// Package network holds checks verifiable from the network alone: no
// management-plane credentials, only a host with an IP address on the right
// segment. A network check — see docs/check-types.md.
//
// These are what make the tool useful to someone handed a jump host and no
// vCenter account, which is the common field situation.
//
// Three rules these checks must not break:
//
//  1. **Where the check ran is part of the answer.** "Port 443 is open" is a
//     fact about the environment *as seen from one machine*. A pass from an
//     operator's laptop says nothing about the workload segment. Every report
//     records the host the probes ran from for exactly this reason.
//
//  2. **A port has three states, not two.** Open, refused and filtered are
//     three findings with three different fixes. Filtered maps to
//     StatusUnknown: we did not see a failure, so we do not get to report one.
//
//  3. **Not seeing something is not the same as it not being there.** A
//     resolver that times out has not necessarily failed. Where a probe cannot
//     tell the difference, the result is unknown and says what was ambiguous.
package network

import (
	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Checks returns the checks in this package.
//
// TODO — blocked on flagged matrix rows or on capabilities the tool does not
// yet have:
//
//	dns.resolver-reachable  COM-DNS-003     needs per-segment vantage, not just this host
//	tcp.esxi-ports          COM-FW-003  ⚑   confirm whether 902 is still required
//	tcp.registry            COM-FW-005  ⚑   must be probed from the workload segment
//	icmp.gateway            COM-RTE-001     needs raw sockets; degrade to a skip when unprivileged
//	route.range-reachable   COM-RTE-002     needs a vantage outside the segment
//	ip.duplicate            COM-ADR-001 ⚑   open question on whether it is invasive
//	mtu.path                COM-MTU-005     INVASIVE, needs raw sockets
func Checks() []checks.Check {
	return []checks.Check{
		Forward{},
		Reverse{},
		ResolverAgreement{},
		PortOpen{},
		TLSChain{},
		CertExpiry{},
		NTPReachable{},
		NTPSkew{},
	}
}

// skip builds a single skipped result with a reason.
//
// Used wherever a check has nothing to examine. Never a pass: a green tick for
// work that was not done is worse than admitting it was not done.
func skip(m checks.Meta, rc *checks.RunContext, reason string) results.Result {
	r := checks.NewResult(m, rc, "")
	r.Status = results.StatusSkip
	r.Expected = results.Text(m.Title)
	r.Observed = results.Value{
		Summary: reason,
		Data:    map[string]any{"skip_reason": reason},
	}
	checks.Finish(rc, &r)
	return r
}

// summaryPass builds the single passing row a clean check returns.
//
// The fan-out idiom: one row summarising what was examined when everything
// passed, one row per problem when something did not. "47 endpoints were
// reachable" is not 47 findings.
func summaryPass(m checks.Meta, rc *checks.RunContext, summary string, data map[string]any) results.Result {
	r := checks.NewResult(m, rc, "")
	r.Status = results.StatusPass
	r.Expected = results.Text(m.Title)
	r.Observed = results.Value{Summary: summary, Data: data}
	checks.Finish(rc, &r)
	return r
}

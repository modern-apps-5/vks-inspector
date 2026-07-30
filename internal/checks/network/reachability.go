package network

import (
	"context"
	"fmt"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// PortOpen asserts each declared management endpoint accepts a connection.
//
// The tri-state result is the point. A refused connection proves the host is
// reachable with nothing listening; silence proves nothing at all and maps to
// StatusUnknown. Reporting a filtered port as a failure blocks deployments over
// a firewall rule the tool cannot see.
type PortOpen struct{}

var _ checks.Check = (*PortOpen)(nil)

// Meta implements checks.Check.
func (PortOpen) Meta() checks.Meta {
	return checks.Meta{
		ID:             "tcp.port-open",
		Title:          "Declared management endpoints are reachable",
		RequirementIDs: []string{"COM-FW-001", "COM-FW-002"},
		Category:       results.CategoryFirewall,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Open the path from the machine you ran this on to the endpoint. Note that the " +
			"report records which host did the probing: a pass from a jump host says nothing about " +
			"the management network.",
	}
}

// Run implements checks.Check.
func (c PortOpen) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	eps := managementEndpoints(rc.Config)
	if len(eps) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no management endpoints declared")}, nil
	}

	all := mapConcurrent(ctx, eps, func(ctx context.Context, ep endpoint) results.Result {
		ans := rc.Probes.DialTCP(ctx, ep.Address())

		r := checks.NewResult(c.Meta(), rc, ep.Address())
		if !ep.Required {
			// An endpoint declared for completeness is not one the deployment
			// dies without.
			r.Severity = results.SeverityWarning
		}
		r.Expected = results.Value{
			Summary: ep.Address() + " accepts a TCP connection",
			Data:    map[string]any{"address": ep.Address(), "source": ep.Source},
		}
		r.Observed = results.Value{
			Data: map[string]any{"address": ep.Address(), "state": string(ans.State), "vantage": rc.Vantage},
		}

		switch ans.State {
		case probes.PortOpen:
			r.Status = results.StatusPass
			r.Observed.Summary = fmt.Sprintf("%s accepted a connection in %s",
				ep.Address(), ans.RTT.Round(time.Millisecond))
		case probes.PortRefused:
			// Reachable, nothing listening. A real finding, and a different one
			// from a firewall.
			r.Status = results.StatusFail
			r.Observed.Summary = ep.Address() + " refused the connection — the host is reachable but nothing is listening"
			r.Remediation = "The host answered with a reset, so routing and firewalling are fine. " +
				"The service is not running, or is on a different port."
		case probes.PortFiltered:
			r.Status = results.StatusUnknown
			r.Observed.Summary = ep.Address() + " did not answer — filtered, not refused"
			r.Remediation = "A silent drop is a firewall, not a dead service. Confirm the path from " +
				"the machine you ran this on before treating it as a failure."
		default:
			r.Status = results.StatusUnknown
			r.Observed.Summary = fmt.Sprintf("could not probe %s: %v", ep.Address(), ans.Err)
			if ans.Err != nil {
				r.Observed.Data["error"] = ans.Err.Error()
			}
		}
		checks.Finish(rc, &r)
		return r
	})

	var notable []results.Result
	for _, r := range all {
		if r.Status != results.StatusPass {
			notable = append(notable, r)
		}
	}

	if len(notable) > 0 {
		return notable, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("%d declared endpoint(s) reachable from %s", len(eps), rc.Vantage),
		map[string]any{"endpoints_checked": len(eps), "vantage": rc.Vantage},
	)}, nil
}

// ---------------------------------------------------------------------------

// NTPReachable asserts each declared time source answers a real SNTP query.
type NTPReachable struct{}

var _ checks.Check = (*NTPReachable)(nil)

// Meta implements checks.Check.
func (NTPReachable) Meta() checks.Meta {
	return checks.Meta{
		ID:             "ntp.reachable",
		Title:          "Declared NTP sources answer",
		RequirementIDs: []string{"COM-NTP-001"},
		Category:       results.CategoryNTP,
		Layer:          results.LayerBoth,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Open 123/udp to the declared sources, or declare sources that are actually " +
			"reachable from the management segment.",
	}
}

// Run implements checks.Check.
func (c NTPReachable) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	servers := rc.Config.Services.NTP.Servers
	if len(servers) == 0 {
		return []results.Result{skip(c.Meta(), rc, "no NTP servers declared")}, nil
	}

	all := mapConcurrent(ctx, servers, func(ctx context.Context, s string) results.Result {
		ans := rc.Probes.QueryNTP(ctx, s)

		r := checks.NewResult(c.Meta(), rc, s)
		r.Expected = results.Value{
			Summary: s + " answers an SNTP query",
			Data:    map[string]any{"server": s},
		}
		r.Observed = results.Value{Data: map[string]any{"server": s}}

		switch {
		case ans.Err != nil:
			// UDP never refuses, so silence tells us nothing: the server may
			// be there behind a filter.
			r.Status = results.StatusUnknown
			r.Observed.Summary = fmt.Sprintf("%s did not answer: %v", s, ans.Err)
			r.Observed.Data["error"] = ans.Err.Error()
			r.Remediation = "UDP never refuses, so silence cannot tell a blocked path from a dead " +
				"service. Confirm 123/udp is open from the machine you ran this on."
		case ans.Stratum == 0 || ans.Stratum >= 16:
			// The server answered but is not itself synchronised, so its time
			// is worthless. A check that only asked "did it answer" would pass.
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("%s answered but is not synchronised (stratum %d)", s, ans.Stratum)
			r.Observed.Data["stratum"] = int(ans.Stratum)
			r.Remediation = "The source answered but reports itself unsynchronised. Its time cannot " +
				"be trusted; point at a source that is synchronised."
		default:
			r.Status = results.StatusPass
			r.Observed.Summary = fmt.Sprintf("%s answered, stratum %d, offset %s",
				s, ans.Stratum, ans.Offset.Round(time.Millisecond))
			r.Observed.Data["stratum"] = int(ans.Stratum)
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
		fmt.Sprintf("%d NTP source(s) answered and are synchronised", len(servers)),
		map[string]any{"servers_checked": len(servers)},
	)}, nil
}

// ---------------------------------------------------------------------------

// NTPSkew asserts the local clock is within the declared tolerance of the
// declared sources.
//
// The tolerance comes from the config, never a constant here. Matrix row
// COM-NTP-002 is flagged precisely because no documented product threshold is
// known — the 30-second figure inherited from this project's earlier test list
// appears to be a field heuristic. Hardcoding it would present a guess as a
// requirement.
type NTPSkew struct{}

var _ checks.Check = (*NTPSkew)(nil)

// Meta implements checks.Check.
func (NTPSkew) Meta() checks.Meta {
	return checks.Meta{
		ID:             "ntp.skew",
		Title:          "Clock offset is within the declared tolerance",
		RequirementIDs: []string{"COM-NTP-002"},
		Category:       results.CategoryNTP,
		Layer:          results.LayerBoth,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapNetwork},
		Remediation: "Point this host and the declared sources at the same stratum. Certificate " +
			"validation and Kubernetes token handling both fail in confusing ways when clocks " +
			"disagree. Note the tolerance is the one declared in services.ntp.maxSkewSeconds, " +
			"not a documented product limit (matrix COM-NTP-002).",
	}
}

// Run implements checks.Check.
func (c NTPSkew) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	servers := rc.Config.Services.NTP.Servers
	tolerance := time.Duration(rc.Config.Services.NTP.MaxSkewSeconds) * time.Second

	switch {
	case len(servers) == 0:
		return []results.Result{skip(c.Meta(), rc, "no NTP servers declared")}, nil
	case tolerance <= 0:
		// No declared tolerance means no assertion to make. Passing here would
		// claim the clock is fine against a threshold nobody set.
		return []results.Result{skip(c.Meta(), rc,
			"services.ntp.maxSkewSeconds is not set, so there is no tolerance to check against")}, nil
	}

	var failures []results.Result
	measured := 0

	for _, s := range servers {
		ans := rc.Probes.QueryNTP(ctx, s)
		if !ans.Usable() {
			continue // ntp.reachable reports this; skew has nothing to measure
		}
		measured++

		offset := ans.Offset
		if offset < 0 {
			offset = -offset
		}
		if offset <= tolerance {
			continue
		}

		r := checks.NewResult(c.Meta(), rc, s)
		r.Status = results.StatusFail
		r.Expected = results.Value{
			Summary: fmt.Sprintf("offset within %s", tolerance),
			Data:    map[string]any{"max_skew_seconds": rc.Config.Services.NTP.MaxSkewSeconds},
		}
		r.Observed = results.Value{
			Summary: fmt.Sprintf("this host is %s from %s", ans.Offset.Round(time.Millisecond), s),
			Data: map[string]any{
				"server":         s,
				"offset_seconds": int(ans.Offset.Round(time.Second).Seconds()),
			},
		}
		checks.Finish(rc, &r)
		failures = append(failures, r)
	}

	if len(failures) > 0 {
		return failures, nil
	}
	if measured == 0 {
		return []results.Result{skip(c.Meta(), rc,
			"no declared NTP source returned a usable time, so skew could not be measured")}, nil
	}
	return []results.Result{summaryPass(c.Meta(), rc,
		fmt.Sprintf("clock within %s of %d source(s)", tolerance, measured),
		map[string]any{"sources_measured": measured, "max_skew_seconds": rc.Config.Services.NTP.MaxSkewSeconds},
	)}, nil
}

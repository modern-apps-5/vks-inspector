package configval

import (
	"context"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/netx"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// RangeContainment asserts that every declared static range sits inside the
// subnet it belongs to.
//
// The load balancer VIP case (LB-VIP-001) is the one the matrix names, and it
// is the one that bites hardest — a VIP pool outside its own subnet produces a
// load balancer that provisions successfully and serves nothing. But the
// arithmetic is identical for the management and workload ranges, and a
// management range half-outside its subnet fails Supervisor enablement in a way
// that is genuinely hard to read from the error. So this check covers all of
// them and cites COM-CID-005, which was added to the matrix for the general
// case alongside the VIP-specific row.
type RangeContainment struct{}

var _ checks.Check = (*RangeContainment)(nil)

// Meta implements checks.Check.
func (RangeContainment) Meta() checks.Meta {
	return checks.Meta{
		ID:             "range.containment",
		Title:          "Declared address ranges sit inside their own subnet",
		RequirementIDs: []string{"COM-CID-005", "LB-VIP-001"},
		Category:       results.CategoryCIDR,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Remediation: "Correct the range so it falls inside the network's CIDR, or correct the CIDR. " +
			"A range that extends past its subnet boundary yields addresses that are unreachable " +
			"from the segment they were allocated on.",
	}
}

// Run implements checks.Check.
func (c RangeContainment) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	var out []results.Result
	var found []results.Result
	checked := 0

	inspect := func(source string, n config.NetworkSpec) {
		if n.CIDR == "" || len(n.Ranges) == 0 {
			return
		}
		p, err := netx.ParsePrefix(n.CIDR)
		if err != nil {
			return // reported by cidr.overlap's malformed handling
		}
		subnet := netx.RangeOfPrefix(p)

		for i, raw := range n.Ranges {
			rng, err := netx.ParseRange(raw.Start, raw.End)
			if err != nil {
				continue // likewise
			}
			checked++
			if subnet.ContainsRange(rng) {
				continue
			}

			target := sprintf("%s.ranges[%d]", source, i)
			r := checks.NewResult(c.Meta(), rc, target)
			r.Status = results.StatusFail
			r.Expected = results.Value{
				Summary: sprintf("%s is inside %s", rng, p),
				Data:    map[string]any{"subnet": p.String()},
			}

			// Say *how* it escapes. "Not contained" leaves the operator to work
			// out whether the start, the end or both are wrong.
			var how string
			switch {
			case !subnet.Contains(rng.Start) && !subnet.Contains(rng.End):
				how = "both ends are outside the subnet"
			case !subnet.Contains(rng.Start):
				how = "starts before the subnet"
			default:
				how = "extends past the end of the subnet"
			}

			r.Observed = results.Value{
				Summary: sprintf("%s range %s %s (%s spans %s)", n.Name, rng, how, p, subnet),
				Data: map[string]any{
					"range":         rng.String(),
					"range_source":  target,
					"subnet":        p.String(),
					"subnet_source": source + ".cidr",
					"how":           how,
				},
			}
			checks.Finish(rc, &r)
			found = append(found, r)
		}
	}

	cfg := rc.Config
	inspect("networks.management", cfg.Networks.Management)
	for i, w := range cfg.Networks.Workload {
		inspect(sprintf("networks.workload[%d]", i), w)
	}
	if cfg.Networks.Frontend != nil {
		inspect("networks.frontend", *cfg.Networks.Frontend)
	}
	if cfg.ALB != nil {
		if cfg.ALB.VIPNetwork != nil {
			inspect("alb.vipNetwork", *cfg.ALB.VIPNetwork)
		}
		if cfg.ALB.SEDataNetwork != nil {
			inspect("alb.seDataNetwork", *cfg.ALB.SEDataNetwork)
		}
	}

	if len(found) > 0 {
		return append(out, found...), nil
	}

	r := checks.NewResult(c.Meta(), rc, "")
	if checked == 0 {
		// No declared ranges is not a pass. It means there was nothing to
		// check, and a report that shows a green tick for work it did not do is
		// worse than one that admits it.
		r.Status = results.StatusSkip
		r.Expected = results.Text("declared ranges sit inside their own subnet")
		r.Observed = results.Value{
			Summary: "no static address ranges declared",
			Data:    map[string]any{"skip_reason": "no ranges declared in the config"},
		}
	} else {
		r.Status = results.StatusPass
		r.Expected = results.Text("declared ranges sit inside their own subnet")
		r.Observed = results.Value{
			Summary: sprintf("%d declared range(s) all sit inside their subnet", checked),
			Data:    map[string]any{"ranges_checked": checked, "out_of_subnet": 0},
		}
	}
	checks.Finish(rc, &r)
	return append(out, r), nil
}

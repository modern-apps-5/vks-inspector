// Package engine runs a selected set of checks and assembles a Report.
//
// It is mode-agnostic on purpose: `check`, `verify` and `snapshot` all call
// Run with a different Mode and nothing else differs. That is the mechanism
// that stops preflight assumptions leaking into the other modes — there is no
// preflight-specific code path to leak from.
package engine

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/buildinfo"
	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
	"github.com/modern-apps-5/vks-inspector/internal/registry"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Options configure a run.
type Options struct {
	Mode   checks.Mode
	Config *config.Config
	Creds  *creds.Set
	// Layer restricts the run to Supervisor-enablement or VKS-cluster
	// prerequisites. Empty means both.
	Layer    results.Layer
	Clients  checks.Clients
	Probes   checks.Probes
	Invasive bool
	Only     []string
	Skip     []string
	// Now is injected for deterministic tests.
	Now func() time.Time
	// Vantage overrides the recorded probe origin. Defaults to os.Hostname.
	Vantage string
	// Timeout bounds each individual check.
	Timeout time.Duration
}

// Run executes the eligible checks and returns a Report.
//
// Run never returns an error for an unhealthy environment — that is what
// Results are for. It returns an error only when it could not run at all.
func Run(ctx context.Context, reg *registry.Registry, opts Options) (*results.Report, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("engine: no config")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	vantage := opts.Vantage
	if vantage == "" {
		if h, err := os.Hostname(); err == nil {
			vantage = h
		} else {
			vantage = "unknown"
		}
	}

	// Policy skips are merged with CLI skips so a site's standing decision is
	// applied without the operator having to remember a flag — and so it lands
	// in the baseline, where it can be audited later.
	skip := append(append([]string(nil), opts.Skip...), opts.Config.Policy.Skip...)

	rc := &checks.RunContext{
		Mode:     opts.Mode,
		Config:   opts.Config,
		Creds:    opts.Creds,
		Clients:  opts.Clients,
		Probes:   opts.Probes,
		Now:      now,
		Invasive: opts.Invasive || opts.Config.Policy.AllowInvasive,
		Vantage:  vantage,
	}

	started := now()
	decisions := reg.Select(registry.Selector{
		Mode:     opts.Mode,
		Topology: opts.Config.Topology,
		Layer:    layerOrBoth(opts.Layer),
		Only:     opts.Only,
		Skip:     skip,
	})

	var out []results.Result
	for _, d := range decisions {
		m := d.Check.Meta()

		if !d.Selected {
			out = append(out, skipResult(m, rc, d.Reason))
			continue
		}
		if missing := rc.Missing(m.Needs); len(missing) > 0 {
			out = append(out, skipResult(m, rc, missingReason(missing)))
			continue
		}

		out = append(out, runOne(ctx, d.Check, rc, opts.Timeout)...)
	}

	// Apply severity overrides after the fact so a check never has to know it
	// was downgraded, and so the override is visible as a distinct step.
	applySeverityOverrides(out, opts.Config.Policy.SeverityOverrides)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CheckID != out[j].CheckID {
			return out[i].CheckID < out[j].CheckID
		}
		return out[i].Target < out[j].Target
	})

	finished := now()
	run := results.RunInfo{
		Mode:         string(opts.Mode),
		Topology:     opts.Config.Topology.String(),
		Layer:        string(layerOrBoth(opts.Layer)),
		ConfigDigest: opts.Config.Digest(),
		Vantage:      vantage,
		Invasive:     rc.Invasive,
		StartedAt:    started,
		FinishedAt:   finished,
		DurationMS:   finished.Sub(started).Milliseconds(),
	}
	return results.NewReport(results.KindReport, buildinfo.Tool(), run, out), nil
}

// runOne executes a single check with a timeout and a panic barrier.
//
// The panic barrier is not defensive padding: a check that panics mid-run must
// not take down a preflight report that already contains twenty useful
// findings, and the failure must be reported as a tool error (exit 3) rather
// than masquerading as an environment failure (exit 1).
func runOne(ctx context.Context, c checks.Check, rc *checks.RunContext, timeout time.Duration) (out []results.Result) {
	m := c.Meta()

	defer func() {
		if rec := recover(); rec != nil {
			r := checks.NewResult(m, rc, "")
			r.Status = results.StatusError
			r.Err = fmt.Sprintf("check panicked: %v", rec)
			r.Evidence = map[string]any{"stack": string(debug.Stack())}
			r.Observed = results.Text("check did not complete")
			checks.Finish(rc, &r)
			out = []results.Result{r}
		}
	}()

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := c.Run(cctx, rc)
	if err != nil {
		r := checks.NewResult(m, rc, "")
		r.Status = results.StatusError
		r.Err = err.Error()
		r.Observed = results.Text("check did not complete")
		checks.Finish(rc, &r)
		return []results.Result{r}
	}
	if len(res) == 0 {
		// A check that returns nothing has silently dropped a requirement.
		// Surface it rather than letting the report look complete.
		r := checks.NewResult(m, rc, "")
		r.Status = results.StatusError
		r.Err = "check returned no results"
		checks.Finish(rc, &r)
		return []results.Result{r}
	}
	return res
}

func skipResult(m checks.Meta, rc *checks.RunContext, reason string) results.Result {
	r := checks.NewResult(m, rc, "")
	r.Status = results.StatusSkip
	r.Observed = results.Value{
		Summary: reason,
		Data:    map[string]any{"skip_reason": reason},
	}
	r.Expected = results.Text(m.Title)
	checks.Finish(rc, &r)
	return r
}

func layerOrBoth(l results.Layer) results.Layer {
	if l == "" {
		return results.LayerBoth
	}
	return l
}

func missingReason(missing []checks.Capability) string {
	parts := make([]string, 0, len(missing))
	for _, c := range missing {
		switch c {
		case checks.CapInvasive:
			parts = append(parts, "requires --invasive")
		default:
			parts = append(parts, "requires "+string(c)+" credentials")
		}
	}
	return strings.Join(parts, "; ")
}

func applySeverityOverrides(res []results.Result, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	for i := range res {
		keys := append([]string{res[i].CheckID}, res[i].RequirementIDs...)
		for _, k := range keys {
			if v, ok := overrides[k]; ok {
				res[i].Severity = results.Severity(v)
				if res[i].Evidence == nil {
					res[i].Evidence = map[string]any{}
				}
				res[i].Evidence["severity_overridden_by"] = k
				break
			}
		}
	}
}

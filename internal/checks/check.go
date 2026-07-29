// Package checks defines the contract every check implements.
//
// The central design constraint: the same check unit must run in preflight mode
// (against declared intent, before anything exists) and in verify mode (against
// a live environment), and its output must serialise into a baseline a later
// drift run can diff. A check that can only answer "does this config make
// sense" is the wrong shape and will not be accepted here.
//
// The way that works: a check declares which Modes it supports and receives the
// Mode in its RunContext. It is the same type, the same ID, the same
// requirement IDs and the same Observed/Expected vocabulary in every mode —
// only the *source* of the observation changes. Preflight resolves DNS from
// this host; verify resolves it from this host AND cross-checks what the
// Supervisor actually got. Same CheckID, comparable Observed.Data.
//
// Concrete checks live in subpackages (checks/network, checks/vcenter, ...) and
// are wired into a registry by checks/all. Nothing here imports those.
package checks

import (
	"context"
	"strings"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Mode is the run mode. It is passed to every check rather than baked into
// separate command paths, so no check can hardcode an assumption that it is
// only ever doing preflight.
type Mode string

const (
	// ModePreflight — nothing is deployed yet. Observations come from the
	// network and from the management plane's *current* state; assertions are
	// against declared intent.
	ModePreflight Mode = "preflight"
	// ModeVerify — the environment is deployed. Observations come from the live
	// environment; assertions are against declared intent AND against what the
	// deployment actually configured.
	ModeVerify Mode = "verify"
	// ModeSnapshot — record observations without grading them. Checks run
	// normally but the engine records rather than gates. Used to capture a
	// baseline from a known-good environment.
	ModeSnapshot Mode = "snapshot"
)

// AllModes is the canonical ordered list.
var AllModes = []Mode{ModePreflight, ModeVerify, ModeSnapshot}

// Capability is a resource a check needs in order to run at all. The engine
// skips (never fails) checks whose capabilities are unavailable, because "we
// did not have NSX credentials" is not evidence of a broken environment.
type Capability string

const (
	CapNetwork  Capability = "network"  // can send packets from this host
	CapVCenter  Capability = "vcenter"  // vCenter API credentials present
	CapNSX      Capability = "nsx"      // NSX Manager API credentials present
	CapALB      Capability = "alb"      // ALB controller API credentials present
	CapHAProxy  Capability = "haproxy"  // HAProxy Data Plane API credentials present
	CapInvasive Capability = "invasive" // permitted to send potentially disruptive traffic
	// CapSupervisor is verify-mode only: a kubeconfig for the deployed
	// Supervisor. Declared now so verify-mode checks have somewhere to hang.
	CapSupervisor Capability = "supervisor"
)

// Meta is the static description of a check. It is what the registry filters
// on, what `explain` prints, and what ties a check to the requirements matrix.
// It must be answerable without running anything.
type Meta struct {
	// ID is stable and human-typeable, e.g. "dns.forward-reverse". Used by
	// --only/--skip and by policy overrides. Renaming one is a breaking change.
	ID string
	// Title is one line, operator-facing.
	Title string
	// RequirementIDs are the docs/REQUIREMENTS-MATRIX.md rows this check
	// evidences. A check with no requirement ID is not allowed to ship — if it
	// does not trace to a stated requirement, we cannot say why it is failing
	// someone's deployment.
	RequirementIDs []string
	Category       results.Category
	// Layer says whether this is a Supervisor enablement prerequisite or a VKS
	// workload-cluster one. Most checks in this tool are Supervisor-layer:
	// there is no VKS without a Supervisor, so the majority of "is this ready
	// for VKS" questions are really "can the Supervisor be enabled" questions.
	// Defaults to LayerSupervisor if left empty, because that is the honest
	// default and silently defaulting to "both" would overstate coverage.
	Layer results.Layer
	// Severity is the default grading. config.Policy.SeverityOverrides can
	// change it at runtime; the check itself never decides how much it matters.
	Severity results.Severity
	// Applies restricts the check by topology axis. The zero value applies
	// everywhere.
	Applies Applicability
	// Modes this check can run in. A check supporting only one mode is a smell
	// and should be justified in a comment.
	Modes []Mode
	// Needs are the capabilities required. Missing ones produce a skip.
	Needs []Capability
	// Remediation is the default operator guidance, shown on failure and by
	// `explain`.
	Remediation string
	// Invasive marks a check that sends traffic which could plausibly disturb
	// production. Requires CapInvasive. Documented as such in the matrix.
	Invasive bool
}

// Applicability restricts a check to particular topology axes.
//
// Requirements attach to the axis they actually depend on. The NSX overlay MTU
// requirement depends on the networking choice and is indifferent to the load
// balancer; the ALB licence-tier requirement depends on the load balancer and
// is indifferent to the networking. Expressing that directly means a new
// supported combination does not require touching every check.
//
// An empty slice means "any value on this axis". The zero Applicability applies
// everywhere, which is the right default for things like DNS and NTP.
type Applicability struct {
	Networking    []config.Networking
	LoadBalancers []config.LoadBalancer
}

// Matches reports whether the check applies to a topology.
func (a Applicability) Matches(t config.Topology) bool {
	if len(a.Networking) > 0 && !containsNetworking(a.Networking, t.Networking) {
		return false
	}
	if len(a.LoadBalancers) > 0 && !containsLB(a.LoadBalancers, t.LoadBalancer) {
		return false
	}
	return true
}

// Describe renders the applicability for `explain` and for skip reasons.
func (a Applicability) Describe() string {
	if len(a.Networking) == 0 && len(a.LoadBalancers) == 0 {
		return "all topologies"
	}
	parts := make([]string, 0, 2)
	if len(a.Networking) > 0 {
		vals := make([]string, 0, len(a.Networking))
		for _, n := range a.Networking {
			vals = append(vals, string(n))
		}
		parts = append(parts, "networking="+strings.Join(vals, ","))
	}
	if len(a.LoadBalancers) > 0 {
		vals := make([]string, 0, len(a.LoadBalancers))
		for _, lb := range a.LoadBalancers {
			vals = append(vals, string(lb))
		}
		parts = append(parts, "loadBalancer="+strings.Join(vals, ","))
	}
	return strings.Join(parts, " and ")
}

func containsNetworking(set []config.Networking, v config.Networking) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func containsLB(set []config.LoadBalancer, v config.LoadBalancer) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// AppliesTo reports whether this check is relevant to a topology.
func (m Meta) AppliesTo(t config.Topology) bool { return m.Applies.Matches(t) }

// EffectiveLayer returns the check's layer, defaulting to Supervisor.
func (m Meta) EffectiveLayer() results.Layer {
	if m.Layer == "" {
		return results.LayerSupervisor
	}
	return m.Layer
}

// SupportsMode reports whether this check can run in the given mode.
func (m Meta) SupportsMode(mode Mode) bool {
	for _, k := range m.Modes {
		if k == mode {
			return true
		}
	}
	return false
}

// Check is the unit of work. Implementations must be safe to run concurrently
// and must be read-only against the target environment.
//
// Run returns Results, plural, because one check may fan out over many targets
// (every DNS name, every declared CIDR pair) and each target deserves its own
// diffable row in the baseline. Returning an error means the *check* broke, not
// the environment; the engine converts it into a StatusError result.
//
// A Check must never print. It must never return a bare bool. It must never
// call os.Exit. It must honour ctx cancellation.
type Check interface {
	Meta() Meta
	Run(ctx context.Context, rc *RunContext) ([]results.Result, error)
}

// RunContext is everything a check is allowed to touch. Passing this explicitly
// rather than reaching for globals is what makes checks unit-testable without a
// lab — see docs/test-coverage.md.
type RunContext struct {
	Mode   Mode
	Config *config.Config
	// Creds is nil-safe: a check must handle absent credentials by declaring
	// the capability, not by dereferencing and panicking.
	Creds *creds.Set
	// Clients holds lazily-connected management-plane clients. Nil entries mean
	// the capability was not available.
	Clients Clients
	// Probes is the network probe surface. Injected so tests substitute a fake
	// instead of touching a real network.
	Probes Probes
	// Now is injected so results are deterministic in tests. Never call
	// time.Now() directly in a check.
	Now func() time.Time
	// Invasive reports whether disruptive probes are permitted.
	Invasive bool
	// InsecureTLS reports that certificate verification was disabled for this
	// run. Certificate checks must downgrade themselves and say so rather than
	// asserting a chain nobody verified.
	InsecureTLS bool
	// Vantage is the host identity recorded in the report.
	Vantage string
}

// Clients is the management-plane client surface a check may use.
// Interfaces, not concrete types, so recorded fixtures can stand in for a live
// vCenter in tests.
type Clients struct {
	VCenter VCenterClient
	NSX     NSXClient
	ALB     ALBClient
}

// Probes is the network probe surface. Same reasoning as Clients: an interface
// so the whole class-(a) suite is testable with a fake, on a laptop, with no
// network. Defined in internal/probes.
type Probes = probes.Prober

// VCenterClient is the read-only vCenter surface checks may use.
//
// An interface rather than the concrete client so a check can be unit-tested
// with a fake. Read-only by contract: no method here may mutate vCenter state.
type VCenterClient interface {
	Endpoint() string
	About(ctx context.Context) (map[string]any, error)
	IsVCenter(ctx context.Context) (bool, error)
	Discover(ctx context.Context) (*vcenter.Inventory, error)
	Cluster(ctx context.Context, datacenter, name string) (*vcenter.ClusterInfo, error)
	DistributedSwitch(ctx context.Context, name string) (*vcenter.SwitchInfo, error)
	PortGroup(ctx context.Context, name string) (*vcenter.PortGroupInfo, error)
	ClusterHostTime(ctx context.Context, datacenter, cluster string) ([]vcenter.HostTimeInfo, error)
}

// NSXClient is the read-only NSX Manager surface.
// TODO(phase-1b): widen.
type NSXClient interface {
	About(ctx context.Context) (map[string]any, error)
}

// ALBClient is the read-only NSX Advanced Load Balancer surface.
// TODO(phase-1b): widen.
type ALBClient interface {
	About(ctx context.Context) (map[string]any, error)
}

// Available reports which capabilities this RunContext can satisfy.
func (rc *RunContext) Available() map[Capability]bool {
	avail := map[Capability]bool{
		CapNetwork: true, // assumed; a host with no stack cannot run us anyway
	}
	avail[CapVCenter] = rc.Clients.VCenter != nil
	avail[CapNSX] = rc.Clients.NSX != nil
	avail[CapALB] = rc.Clients.ALB != nil
	avail[CapInvasive] = rc.Invasive
	return avail
}

// Missing returns the capabilities a check needs that this context lacks.
func (rc *RunContext) Missing(needs []Capability) []Capability {
	avail := rc.Available()
	var missing []Capability
	for _, n := range needs {
		if !avail[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// NewResult seeds a Result from a check's Meta and the run context, so every
// check produces consistently-populated rows without copy-pasting boilerplate
// (and so no check can forget to record its mode or requirement IDs).
func NewResult(m Meta, rc *RunContext, target string) results.Result {
	now := time.Now
	if rc != nil && rc.Now != nil {
		now = rc.Now
	}
	return results.Result{
		CheckID:        m.ID,
		RequirementIDs: m.RequirementIDs,
		Title:          m.Title,
		Category:       m.Category,
		Layer:          m.EffectiveLayer(),
		Severity:       m.Severity,
		Status:         results.StatusUnknown,
		Mode:           string(modeOf(rc)),
		Target:         target,
		Remediation:    m.Remediation,
		Invasive:       m.Invasive,
		StartedAt:      now(),
	}
}

func modeOf(rc *RunContext) Mode {
	if rc == nil {
		return ModePreflight
	}
	return rc.Mode
}

// Finish stamps the completion timestamps on a result.
func Finish(rc *RunContext, r *results.Result) {
	now := time.Now
	if rc != nil && rc.Now != nil {
		now = rc.Now
	}
	r.FinishedAt = now()
	r.DurationMS = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
}

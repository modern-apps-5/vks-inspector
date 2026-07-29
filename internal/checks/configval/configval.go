// Package configval holds pure input-validation checks: arithmetic over the
// declared config with no network and no API access. Taxonomy class (c) — see
// docs/CHECK-TAXONOMY.md.
//
// These are the cheapest, fastest and most deterministic checks in the tool,
// they need no lab to test, and they catch the single most common class of
// field failure (an address plan that could never have worked). They run first,
// before the tool spends thirty seconds discovering it also cannot reach
// vCenter.
//
// They are NOT config-file schema validation — that is internal/config's job
// and happens before any check runs. These produce Results and are reportable.
//
// In verify mode these same checks re-run against the address plan read back
// from the live environment rather than the declared one, which is how you
// catch a Supervisor that was enabled with a different pod CIDR than the config
// says. That is why they are Checks behind the standard interface rather than a
// validation pass in the loader.
package configval

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// Implemented:
//
//	cidr.overlap             COM-CID-001  no declared range overlaps another
//	cidr.external-collision  COM-CID-002  no declared range overlaps externalCIDRs
//	cidr.infra-collision     COM-CID-003  internal ranges don't swallow infra addresses
//	range.containment        COM-CID-005  ranges sit inside their own subnet
//	                         LB-VIP-001
//
// TODO — blocked on flagged matrix rows. Each of these needs a number that this
// project cannot supply from memory; implementing them now would launder a
// guess into an assertion. Confirm the matrix row first.
//
//	cidr.prefix-size         COM-CID-004 ⚑  minimum prefix per role
//	range.contiguity         SUP-MGT-001 ⚑  N consecutive management addresses ("5"?)
//	pool.sizing-egress       NSX-EGR-002 ⚑  1 SNAT IP per namespace?
//	pool.sizing-ingress      NSX-ING-002 ⚑  addresses per LB service?
//	pool.sizing-workload     VDS-WKL-001 ⚑  addresses per node, plus upgrade headroom?
//	pool.sizing-vip          LB-VIP-006  ⚑  Supervisor's own baseline VIP consumption?
//	vip.no-dhcp-overlap      LB-VIP-002  ⚑  blocked: config has no DHCP scope fields
//	mtu.consistency          COM-MTU-002     declared segment MTUs mutually consistent
func Checks() []checks.Check {
	return []checks.Check{
		Overlap{},
		ExternalCollision{},
		InfraCollision{},
		RangeContainment{},
	}
}

// Package configval will hold pure input-validation checks: arithmetic over the
// declared config with no network and no API access. Taxonomy class (c) — see
// docs/CHECK-TAXONOMY.md.
//
// These are the cheapest, fastest and most deterministic checks in the tool,
// they need no lab to test, and they catch the single most common class of
// field failure (an address plan that could never have worked). They run first.
//
// They are NOT config-file schema validation — that is internal/config's job and
// happens before any check runs. These produce Results and are reportable.
package configval

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// TODO(phase-2): implement.
//
//	cidr.overlap             COM-CID-001  no declared range overlaps another
//	cidr.external-collision  COM-CID-002  no declared range overlaps externalCIDRs
//	cidr.rfc1918-egress      COM-CID-003  routable ranges are actually routable space
//	cidr.prefix-size         COM-CID-004  each range meets its documented minimum prefix
//	vip.containment          LB-VIP-001   VIP range sits inside its frontend subnet
//	vip.no-dhcp-overlap      LB-VIP-002   VIP range does not overlap a declared DHCP scope
//	range.contiguity         SUP-MGT-001  management range has the required consecutive IPs
//	pool.sizing-egress       NSX-EGR-002  egress pool covers expectedNamespaces + headroom
//	pool.sizing-ingress      NSX-ING-002  ingress pool covers expected LB services + headroom
//	pool.sizing-workload     COM-POO-001  workload ranges cover expected node count + headroom
//	mtu.consistency          COM-MTU-002  declared segment MTUs are mutually consistent
//
// In verify mode these same checks re-run against the *observed* address plan
// read back from the live environment, not the declared one, which is why they
// live behind the same Check interface rather than being a validation pass in
// the config loader.
func Checks() []checks.Check { return nil }

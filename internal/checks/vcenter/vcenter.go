// Package vcenter will hold checks requiring vCenter API credentials.
// Taxonomy class (b) — see docs/CHECK-TAXONOMY.md.
//
// Every check here must declare checks.CapVCenter so a run without credentials
// reports them as skips rather than failing an environment it never inspected.
package vcenter

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// TODO(phase-3): implement.
//
//	vc.api-reachable      COM-API-001  API answers and credentials authenticate
//	vc.version-supported  COM-VER-001  vCenter build is in the supported matrix (FLAGGED)
//	vc.cluster-exists     INV-VC-001   declared datacenter/cluster exist
//	vc.vds-exists         INV-VC-002   declared VDS exists and hosts are attached
//	vc.portgroup-exists   INV-VC-003   declared portgroups exist on that VDS
//	vc.portgroup-vlan     INV-VC-004   portgroup VLAN matches the declared VLAN
//	vc.vds-mtu            COM-MTU-003  VDS MTU meets the topology's minimum
//	vc.host-ntp           COM-NTP-003  ESXi hosts have NTP configured and running
//	vc.host-time-skew     COM-NTP-004  host clocks agree within policy
//	vc.ha-drs             INV-VC-005   cluster prerequisites for Supervisor enablement
//
// All calls must be read-only. No method used here may create, modify or delete
// vCenter state; that is an ADR-level constraint, not a preference.
// See docs/ADR/0007-read-only-by-default.md.
func Checks() []checks.Check { return nil }

// Package network will hold checks verifiable from the network alone: no
// management-plane credentials, only a host with an IP address on the right
// segment. Taxonomy class (a) — see docs/CHECK-TAXONOMY.md.
//
// Phase 1 ships none of these. The planned set, each tied to a requirements
// matrix row, is listed below so the package's scope is settled before code
// lands and so nobody invents a check that traces to nothing.
package network

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// TODO(phase-2): implement, in roughly this order — the earlier ones are
// prerequisites for interpreting the later ones.
//
//	dns.forward             COM-DNS-001  A/AAAA for every declared endpoint
//	dns.reverse             COM-DNS-002  PTR agreement for every declared endpoint
//	dns.resolver-reachable  COM-DNS-003  each declared resolver answers on 53/udp+tcp
//	ntp.reachable           COM-NTP-001  each declared NTP source answers on 123/udp
//	ntp.skew                COM-NTP-002  offset within policy (threshold FLAGGED)
//	tcp.port-open           COM-FW-001   declared management ports reachable
//	tls.chain               COM-CRT-001  endpoint chain validates to a trusted root
//	tls.thumbprint          COM-CRT-002  endpoint matches pinned thumbprint if declared
//	icmp.gateway            COM-RTE-001  declared gateway responds
//	route.range-reachable   COM-RTE-002  declared routable ranges are routed, not blackholed
//	ip.duplicate            COM-ADR-001  declared static ranges are actually free
//	mtu.path                COM-MTU-001  path MTU to a target (INVASIVE — see matrix)
//
// Note on ip.duplicate and mtu.path: both send traffic to addresses that may be
// in production use. ip.duplicate is read-only in effect but noisy (ARP/ICMP to
// addresses that should be unused); mtu.path deliberately emits large
// DF-flagged packets. mtu.path is gated behind CapInvasive. Whether
// ip.duplicate should be too is an open question flagged in the matrix.
func Checks() []checks.Check { return nil }

// Package nsx will hold checks requiring NSX Manager API credentials.
// Taxonomy class (b) — see docs/CHECK-TAXONOMY.md.
package nsx

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// TODO(phase-3): implement.
//
//	nsx.api-reachable      COM-API-002  API answers and credentials authenticate
//	nsx.version-supported  COM-VER-002  NSX build is in the supported matrix (FLAGGED)
//	nsx.tier0-exists       NSX-T0-001   declared Tier-0 exists
//	nsx.tier0-uplink       NSX-T0-002   Tier-0 has an up uplink with north-south reachability
//	nsx.edge-cluster       NSX-EDG-001  edge cluster exists, nodes healthy
//	nsx.transport-zone     NSX-TZ-001   overlay transport zone exists and covers the cluster
//	nsx.uplink-profile-mtu COM-MTU-004  uplink profile MTU meets the Geneve minimum
//	nsx.ip-block-free      NSX-POO-001  declared pod/namespace blocks are unallocated
//	nsx.ingress-free       NSX-ING-001  declared ingress range is unallocated
//	nsx.egress-free        NSX-EGR-001  declared egress range is unallocated
//	nsx.vpc-profile        VPC-CFG-001  VPC connectivity profile exists (LOW CONFIDENCE)
//	nsx.vpc-ip-blocks      VPC-POO-001  VPC private/public blocks sized and free (LOW CONFIDENCE)
//
// The VPC rows are the least-confident part of the matrix. Do not implement
// them from the object model assumed in internal/config.NSXVPC until that
// model has been confirmed against a real VCF 9 NSX.
func Checks() []checks.Check { return nil }

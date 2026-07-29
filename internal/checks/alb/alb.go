// Package alb will hold checks requiring NSX Advanced Load Balancer (Avi)
// controller credentials. Taxonomy class (b) — see docs/CHECK-TAXONOMY.md.
//
// HAProxy checks will live here too rather than in a package of their own:
// HAProxy is a single small surface, it is believed deprecated in the VCF 9
// generation, and giving it its own package implies more future than it has.
package alb

import "github.com/modern-apps-5/vks-inspector/internal/checks"

// Checks returns the checks in this package.
//
// TODO(phase-3): implement.
//
//	alb.controller-reachable  LB-ALB-001  controller answers on 443 and authenticates
//	alb.version-supported     LB-ALB-002  controller version is compatible (FLAGGED)
//	alb.cluster-healthy       LB-ALB-003  all controller cluster nodes up, quorum present
//	alb.license-tier          LB-ALB-004  license tier supports the required features (FLAGGED)
//	alb.cloud-configured      LB-ALB-005  declared cloud exists and is in a healthy state
//	alb.se-group              LB-ALB-006  Service Engine group exists and has capacity
//	alb.vip-network           LB-VIP-003  VIP network exists with an allocatable static pool
//	alb.vip-pool-free         LB-VIP-004  declared VIP range is not already allocated
//	alb.data-network-routing  LB-VIP-005  SE data network can reach the workload network
//	hap.dataplane-reachable   LB-HAP-001  HAProxy Data Plane API answers (legacy topology)
//	hap.vip-range             LB-HAP-002  HAProxy-owned VIP range matches the declared one
func Checks() []checks.Check { return nil }

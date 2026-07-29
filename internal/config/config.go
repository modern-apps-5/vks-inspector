// Package config models the declarative description of an intended VKS
// environment.
//
// One config drives every mode: `check` compares it against the network before
// anything is deployed, `verify` compares it against what actually got built,
// `snapshot` records it alongside observations, and the future UI reads the same
// document. That is why the shape describes *intent* and never contains
// credentials, run options, or output preferences — those are flags and
// environment, not topology.
//
// Fields marked TODO(matrix) are placeholders whose exact meaning depends on
// requirements still flagged as unverified in docs/REQUIREMENTS-MATRIX.md.
// Do not build checks on them until the matrix row is confirmed.
package config

// APIVersion of the config document. Unrecognised versions are rejected rather
// than best-effort parsed, so a future breaking change cannot silently
// misinterpret an old file.
const APIVersion = "vksinspect/v1alpha1"

// Kind of the config document.
const Kind = "EnvironmentSpec"

// Config is the root document.
type Config struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind"       json:"kind"`
	Metadata   Metadata `yaml:"metadata"   json:"metadata"`

	// Topology selects which requirement set applies. Everything else is
	// interpreted through it.
	Topology Topology `yaml:"topology" json:"topology"`

	Infrastructure Infrastructure `yaml:"infrastructure" json:"infrastructure"`
	Services       Services       `yaml:"services"       json:"services"`
	VSphere        VSphere        `yaml:"vsphere"        json:"vsphere"`
	Networks       Networks       `yaml:"networks"       json:"networks"`
	Kubernetes     Kubernetes     `yaml:"kubernetes"     json:"kubernetes"`
	NSX            *NSX           `yaml:"nsx,omitempty"     json:"nsx,omitempty"`
	ALB            *ALB           `yaml:"alb,omitempty"     json:"alb,omitempty"`
	HAProxy        *HAProxy       `yaml:"haproxy,omitempty" json:"haproxy,omitempty"`

	Scale  Scale  `yaml:"scale"  json:"scale"`
	Policy Policy `yaml:"policy" json:"policy"`
}

// Metadata identifies the environment being described.
type Metadata struct {
	Name        string            `yaml:"name"                  json:"name"`
	Site        string            `yaml:"site,omitempty"        json:"site,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"      json:"labels,omitempty"`
}

// Endpoint is a management-plane endpoint the tool talks to or probes.
// Credentials for it never live here — see internal/creds.
type Endpoint struct {
	FQDN string `yaml:"fqdn"           json:"fqdn"`
	IP   string `yaml:"ip,omitempty"   json:"ip,omitempty"`
	Port int    `yaml:"port,omitempty" json:"port,omitempty"`
	// ExpectedThumbprint, when set, lets a cert check assert pinning rather
	// than merely chain validity. SHA-256, colon-separated hex.
	ExpectedThumbprint string `yaml:"expectedThumbprint,omitempty" json:"expectedThumbprint,omitempty"`
	// CredentialRef names a key in the credentials file / env prefix. It is a
	// *reference*, never a secret.
	CredentialRef string `yaml:"credentialRef,omitempty" json:"credentialRef,omitempty"`
}

// Infrastructure holds the management-plane components.
type Infrastructure struct {
	VCenter    Endpoint    `yaml:"vcenter"              json:"vcenter"`
	NSXManager *Endpoint   `yaml:"nsxManager,omitempty" json:"nsxManager,omitempty"`
	ESXiHosts  []Endpoint  `yaml:"esxiHosts,omitempty"  json:"esxiHosts,omitempty"`
	Registry   *Endpoint   `yaml:"registry,omitempty"   json:"registry,omitempty"`
	Extra      []Endpoint  `yaml:"extra,omitempty"      json:"extra,omitempty"`
	Proxy      *ProxyBlock `yaml:"proxy,omitempty"      json:"proxy,omitempty"`
}

// ProxyBlock describes an egress proxy the environment is expected to use.
type ProxyBlock struct {
	HTTP    string   `yaml:"http,omitempty"    json:"http,omitempty"`
	HTTPS   string   `yaml:"https,omitempty"   json:"https,omitempty"`
	NoProxy []string `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
}

// Services holds the shared infrastructure services every topology needs.
type Services struct {
	DNS    DNSBlock  `yaml:"dns"              json:"dns"`
	NTP    NTPBlock  `yaml:"ntp"              json:"ntp"`
	Syslog *Endpoint `yaml:"syslog,omitempty" json:"syslog,omitempty"`
}

// DNSBlock declares resolvers and the names that must resolve through them.
type DNSBlock struct {
	Servers []string `yaml:"servers" json:"servers"`
	// SearchDomains is used to build FQDNs for short-name checks.
	SearchDomains []string `yaml:"searchDomains,omitempty" json:"searchDomains,omitempty"`
	// RequireReverse turns PTR mismatches from warning into blocker. Supervisor
	// and NSX are sensitive to reverse resolution in ways plain workloads are
	// not.
	RequireReverse bool `yaml:"requireReverse" json:"requireReverse"`
	// AdditionalNames are extra records that must resolve, beyond the endpoints
	// already declared elsewhere in the config.
	AdditionalNames []string `yaml:"additionalNames,omitempty" json:"additionalNames,omitempty"`
}

// NTPBlock declares time sources and the tolerance.
type NTPBlock struct {
	Servers []string `yaml:"servers" json:"servers"`
	// MaxSkewSeconds is the tolerated offset between this host, the declared
	// NTP sources, and (in credentialed mode) vCenter and the ESXi hosts.
	//
	// FLAGGED: the pre-existing test-coverage doc asserted 30s. That number is
	// not, as far as this tool's authors can establish, a documented product
	// requirement — it is a field heuristic. It is configurable here precisely
	// so the tool does not hardcode an unsourced threshold.
	// See docs/REQUIREMENTS-MATRIX.md row COM-NTP-002.
	MaxSkewSeconds int `yaml:"maxSkewSeconds" json:"maxSkewSeconds"`
}

// VSphere describes the vCenter inventory objects the deployment targets.
type VSphere struct {
	Datacenter string `yaml:"datacenter" json:"datacenter"`
	Cluster    string `yaml:"cluster"    json:"cluster"`
	// DistributedSwitch is the VDS backing the Supervisor networks.
	DistributedSwitch string `yaml:"distributedSwitch,omitempty" json:"distributedSwitch,omitempty"`
	// StoragePolicy is recorded for completeness; not a networking check, but
	// it belongs to the same declared intent and matters to `verify`.
	StoragePolicy string `yaml:"storagePolicy,omitempty" json:"storagePolicy,omitempty"`
	// ContentLibrary is where TKr images come from.
	ContentLibrary string `yaml:"contentLibrary,omitempty" json:"contentLibrary,omitempty"`
}

// Networks holds the L2/L3 segments the Supervisor and workloads sit on.
type Networks struct {
	// Management carries the Supervisor control plane VMs.
	Management NetworkSpec `yaml:"management" json:"management"`
	// Workload carries Supervisor/TKG node interfaces. In NSX topologies this
	// is derived from the namespace network rather than declared as a
	// portgroup; leave the portgroup empty there.
	Workload []NetworkSpec `yaml:"workload,omitempty" json:"workload,omitempty"`
	// Frontend is the VIP/data network. Required for ALB and HAProxy
	// topologies; not used in plain NSX.
	Frontend *NetworkSpec `yaml:"frontend,omitempty" json:"frontend,omitempty"`
}

// NetworkSpec is one addressable segment.
type NetworkSpec struct {
	Name string `yaml:"name" json:"name"`
	// PortGroup is the vSphere portgroup name (VDS topologies) or the NSX
	// segment name.
	PortGroup string `yaml:"portGroup,omitempty" json:"portGroup,omitempty"`
	VLAN      int    `yaml:"vlan,omitempty"      json:"vlan,omitempty"`
	CIDR      string `yaml:"cidr"                json:"cidr"`
	Gateway   string `yaml:"gateway,omitempty"   json:"gateway,omitempty"`
	// MTU is the MTU this segment is expected to carry end to end.
	MTU int `yaml:"mtu,omitempty" json:"mtu,omitempty"`
	// Ranges are the static address ranges reserved for this deployment.
	Ranges []IPRange `yaml:"ranges,omitempty" json:"ranges,omitempty"`
	// Routable declares whether this range must be reachable from outside the
	// cluster. Drives whether a routing check is a blocker or informational.
	Routable bool `yaml:"routable" json:"routable"`
}

// IPRange is an inclusive start/end address range.
type IPRange struct {
	Start string `yaml:"start" json:"start"`
	End   string `yaml:"end"   json:"end"`
	// Purpose is free text for reporting, e.g. "supervisor-control-plane".
	Purpose string `yaml:"purpose,omitempty" json:"purpose,omitempty"`
}

// Kubernetes holds the cluster-internal address plan.
type Kubernetes struct {
	// PodCIDRs and ServiceCIDR are cluster-internal and normally non-routable,
	// but must still not collide with anything the cluster has to reach.
	PodCIDRs    []string `yaml:"podCIDRs"    json:"podCIDRs"`
	ServiceCIDR string   `yaml:"serviceCIDR" json:"serviceCIDR"`
	// IngressCIDR / EgressCIDR are NSX-topology concepts (LB VIPs and SNAT
	// pools). Empty on VDS topologies, where the frontend network plays that
	// role instead.
	IngressCIDR string `yaml:"ingressCIDR,omitempty" json:"ingressCIDR,omitempty"`
	EgressCIDR  string `yaml:"egressCIDR,omitempty"  json:"egressCIDR,omitempty"`
	// APIServerVIP is the Supervisor control plane endpoint.
	APIServerVIP string `yaml:"apiServerVIP,omitempty" json:"apiServerVIP,omitempty"`
	// ExternalCIDRs are ranges outside the deployment that the plan must not
	// collide with — existing corporate subnets, other clusters, VPN pools.
	// Overlap math is only as good as this list.
	ExternalCIDRs []string `yaml:"externalCIDRs,omitempty" json:"externalCIDRs,omitempty"`
}

// NSX describes the NSX objects a Supervisor-on-NSX deployment consumes.
type NSX struct {
	Tier0Gateway  string `yaml:"tier0Gateway"            json:"tier0Gateway"`
	EdgeCluster   string `yaml:"edgeCluster,omitempty"   json:"edgeCluster,omitempty"`
	TransportZone string `yaml:"transportZone,omitempty" json:"transportZone,omitempty"`
	// OverlayMTU is the MTU required on the physical underlay carrying Geneve.
	OverlayMTU int `yaml:"overlayMTU,omitempty" json:"overlayMTU,omitempty"`
	// VPC is populated only for the VPC-based topology.
	// TODO(matrix): every field here is low confidence — see matrix group VPC-*.
	VPC *NSXVPC `yaml:"vpc,omitempty" json:"vpc,omitempty"`
}

// NSXVPC describes VPC-based networking (VCF 9).
// TODO(matrix): field names are provisional and must be reconciled against the
// VCF 9 NSX VPC object model before any check reads them.
type NSXVPC struct {
	ConnectivityProfile string   `yaml:"connectivityProfile,omitempty" json:"connectivityProfile,omitempty"`
	TransitGateway      string   `yaml:"transitGateway,omitempty"      json:"transitGateway,omitempty"`
	PrivateIPBlocks     []string `yaml:"privateIPBlocks,omitempty"     json:"privateIPBlocks,omitempty"`
	PublicIPBlocks      []string `yaml:"publicIPBlocks,omitempty"      json:"publicIPBlocks,omitempty"`
	ExternalIPBlocks    []string `yaml:"externalIPBlocks,omitempty"    json:"externalIPBlocks,omitempty"`
}

// ALB describes an NSX Advanced Load Balancer deployment.
type ALB struct {
	Controller Endpoint `yaml:"controller" json:"controller"`
	// ControllerCluster lists all controller node addresses when clustered;
	// a single-node controller can leave this empty.
	ControllerCluster []string `yaml:"controllerCluster,omitempty" json:"controllerCluster,omitempty"`
	Cloud             string   `yaml:"cloud,omitempty"             json:"cloud,omitempty"`
	ServiceEngineGrp  string   `yaml:"serviceEngineGroup,omitempty" json:"serviceEngineGroup,omitempty"`
	// VIPNetwork is the network ALB allocates load balancer VIPs from.
	VIPNetwork *NetworkSpec `yaml:"vipNetwork,omitempty" json:"vipNetwork,omitempty"`
	// SEDataNetwork is the Service Engine data-plane network. May be the same
	// segment as VIPNetwork in a one-arm design.
	SEDataNetwork *NetworkSpec `yaml:"seDataNetwork,omitempty" json:"seDataNetwork,omitempty"`
	// LicenseTier is recorded because feature availability depends on it.
	// TODO(matrix): confirm the tier names and which tier VKS requires in the
	// VCF 9 generation — matrix rows ALB-LIC-*.
	LicenseTier string `yaml:"licenseTier,omitempty" json:"licenseTier,omitempty"`
}

// HAProxy describes the legacy HAProxy load balancer appliance.
// FLAGGED: believed removed/deprecated in VCF 9. Retained so the tool can still
// preflight older environments and so `verify` can report the topology it finds.
type HAProxy struct {
	Appliance Endpoint `yaml:"appliance" json:"appliance"`
	// DataPlanePort is the HAProxy Data Plane API port.
	DataPlanePort int `yaml:"dataPlanePort,omitempty" json:"dataPlanePort,omitempty"`
	// LoadBalancerCIDR is the VIP range HAProxy owns.
	LoadBalancerCIDR string `yaml:"loadBalancerCIDR,omitempty" json:"loadBalancerCIDR,omitempty"`
	// CACertRef references the appliance CA certificate by file path or
	// credential key — never inline PEM containing anything secret.
	CACertRef string `yaml:"caCertRef,omitempty" json:"caCertRef,omitempty"`
}

// Scale is the intended size of the deployment. Pool-sizing checks are pure
// arithmetic against these numbers, which is why they are declared intent and
// not discovered.
type Scale struct {
	SupervisorControlPlaneNodes int `yaml:"supervisorControlPlaneNodes" json:"supervisorControlPlaneNodes"`
	// ExpectedNamespaces drives egress/SNAT pool sizing in NSX topologies
	// (historically one SNAT IP per namespace).
	ExpectedNamespaces int `yaml:"expectedNamespaces" json:"expectedNamespaces"`
	// ExpectedWorkloadClusters and ExpectedNodesPerCluster drive workload
	// address-range sizing.
	ExpectedWorkloadClusters int `yaml:"expectedWorkloadClusters" json:"expectedWorkloadClusters"`
	ExpectedNodesPerCluster  int `yaml:"expectedNodesPerCluster"  json:"expectedNodesPerCluster"`
	// ExpectedLoadBalancerServices drives ingress/VIP pool sizing.
	ExpectedLoadBalancerServices int `yaml:"expectedLoadBalancerServices" json:"expectedLoadBalancerServices"`
	// GrowthHeadroomPercent is applied on top of the above before comparing
	// against declared pool sizes.
	GrowthHeadroomPercent int `yaml:"growthHeadroomPercent" json:"growthHeadroomPercent"`
}

// Policy tunes how results are graded. It exists so a site can downgrade a
// requirement it knowingly does not meet without patching the tool — and so
// that decision is recorded in the config, and therefore in the baseline,
// rather than living in someone's shell history.
type Policy struct {
	// SeverityOverrides maps a requirement or check ID to a severity.
	SeverityOverrides map[string]string `yaml:"severityOverrides,omitempty" json:"severityOverrides,omitempty"`
	// Skip lists check IDs or categories to skip entirely. Skipped checks are
	// still reported, as skips, with the reason.
	Skip []string `yaml:"skip,omitempty" json:"skip,omitempty"`
	// AllowInvasive permits invasive probes without the CLI flag. Off by
	// default; the CLI flag is still required unless this is true.
	AllowInvasive bool `yaml:"allowInvasive" json:"allowInvasive"`
}

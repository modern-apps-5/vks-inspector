package vcenter

import (
	"context"
	"fmt"
	"sort"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
)

// HostTimeInfo is one ESXi host's time configuration.
//
// Both the configured servers and the service state are reported, because they
// fail independently: a host can have the right NTP servers listed and the
// service stopped, and a check that looks at only one of them will pass an
// environment whose clocks are drifting.
type HostTimeInfo struct {
	Host string `json:"host"`
	// NTPServers is what the host is configured to use.
	NTPServers []string `json:"ntp_servers"`
	// ServiceRunning and ServicePolicy describe the ntpd service. Policy "on"
	// means start-with-host; a host that is running NTP now but will not after
	// a reboot is a latent failure.
	ServiceRunning bool   `json:"ntp_service_running"`
	ServicePolicy  string `json:"ntp_service_policy"`
	// InMaintenance and Connected qualify everything else — an unreachable host
	// tells you nothing about its clock.
	Connected     bool `json:"connected"`
	InMaintenance bool `json:"in_maintenance"`
}

// ClusterHostTime reads time configuration for every host in a cluster.
func (c *Client) ClusterHostTime(ctx context.Context, datacenter, cluster string) ([]HostTimeInfo, error) {
	f, err := c.find(ctx)
	if err != nil {
		return nil, err
	}
	if datacenter != "" {
		dc, err := f.Datacenter(ctx, datacenter)
		if err != nil {
			return nil, notFound("datacenter", datacenter, err)
		}
		f.SetDatacenter(dc)
	}
	cl, err := f.ClusterComputeResource(ctx, cluster)
	if err != nil {
		return nil, notFound("cluster", cluster, err)
	}

	var mcl mo.ClusterComputeResource
	pc := property.DefaultCollector(cl.Client())
	if err := pc.RetrieveOne(ctx, cl.Reference(), []string{"host"}, &mcl); err != nil {
		return nil, fmt.Errorf("read cluster %s: %w", cluster, err)
	}

	out := make([]HostTimeInfo, 0, len(mcl.Host))
	for _, ref := range mcl.Host {
		var h mo.HostSystem
		if err := pc.RetrieveOne(ctx, ref,
			[]string{"name", "config.dateTimeInfo", "config.service", "runtime"}, &h); err != nil {
			// One unreadable host must not lose the other nineteen.
			continue
		}
		out = append(out, hostTime(h))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out, nil
}

func hostTime(h mo.HostSystem) HostTimeInfo {
	info := HostTimeInfo{
		Host:          h.Name,
		Connected:     h.Runtime.ConnectionState == "connected",
		InMaintenance: h.Runtime.InMaintenanceMode,
	}
	if h.Config != nil {
		if dt := h.Config.DateTimeInfo; dt != nil && dt.NtpConfig != nil {
			info.NTPServers = append(info.NTPServers, dt.NtpConfig.Server...)
			sort.Strings(info.NTPServers)
		}
		if svc := h.Config.Service; svc != nil {
			for _, s := range svc.Service {
				if s.Key != "ntpd" {
					continue
				}
				info.ServiceRunning = s.Running
				info.ServicePolicy = s.Policy
			}
		}
	}
	return info
}

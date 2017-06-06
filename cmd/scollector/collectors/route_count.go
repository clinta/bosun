// +build linux

package collectors

import (
	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"net"

	"github.com/vishvananda/netlink"
)

func init() {
	registerInit(func(c *conf.Conf) {
		collectors = append(collectors, &IntervalCollector{F: c_routeCount, Enable: enableRouteCount(c.RouteCount)})
	})
}

func enableRouteCount(enable bool) func() bool {
	return func() bool {
		return enable
	}
}

func c_routeCount() (opentsdb.MultiDataPoint, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}
	type routeStat struct {
		ips, routes int
	}
	var md opentsdb.MultiDataPoint
	Add(&md, "routes.total", len(routes), nil, metadata.Gauge, "routes", "total routes installed")
	rs := make(map[string]map[string]routeStat)
	for _, r := range routes {
		if r.Gw == nil || r.Gw.Equal(net.IPv4zero) || r.Gw.Equal(net.IPv6zero) {
			continue
		}
		if r.Dst == nil || r.Dst.IP.Equal(net.IPv4zero) || r.Dst.IP.Equal(net.IPv6zero) {
			continue
		}
		f := "IPv4"
		ipo := 0
		if r.Gw.To4() == nil {
			f = "IPv6"
			ipo = 64
		}
		if rs[f] == nil {
			rs[f] = make(map[string]routeStat)
		}
		ones, bits := r.Dst.Mask.Size()
		rs[f][r.Gw.String()] = routeStat{
			routes: rs[f][r.Gw.String()].routes + 1,
			ips:    rs[f][r.Gw.String()].ips + 1<<uint(bits-ones-ipo),
		}
	}
	t := make(opentsdb.TagSet)
	for f, m := range rs {
		t["family"] = f
		ipm := func(md *opentsdb.MultiDataPoint, ips int, t opentsdb.TagSet) {
			Add(md, "routes.ip_per_gw", ips, t, metadata.Gauge, "ip addresses", "ip via gateway")
		}
		if f == "IPv6" {
			ipm = func(md *opentsdb.MultiDataPoint, ips int, t opentsdb.TagSet) {
				Add(md, "routes.64s_per_gw", ips, t, metadata.Gauge, "64-bit networks", "64-bit networks via gateway")
			}
		}
		for gw, rn := range m {
			t["gateway"] = gw
			Add(&md, "routes.route_per_gw", rn.routes, t, metadata.Gauge, "routes", "routes via gateway")
			ipm(&md, rn.ips, t)
		}
	}
	return md, nil
}

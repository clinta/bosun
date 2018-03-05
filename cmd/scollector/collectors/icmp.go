package collectors

import (
	"fmt"
	"net"
	"time"

	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"github.com/clinta/go-ping"
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

// ICMP registers an ICMP collector a given host.
func ICMP(host string) error {
	if host == "" {
		return fmt.Errorf("empty ICMP hostname")
	}
	collectors = append(collectors, &IntervalCollector{
		F: func() (opentsdb.MultiDataPoint, error) {
			return c_icmp(host)
		},
		name: fmt.Sprintf("icmp-%s", host),
	})
	return nil
}

func c_icmp(host string) (opentsdb.MultiDataPoint, error) {
	var md opentsdb.MultiDataPoint
	p, err := ping.NewPinger(host)
	if err != nil {
		return nil, err
	}
	p.Count = 1
	p.Timeout = time.Second * 5
	p.SetPrivileged(true)
	timeout := 1
	p.OnRecv = func(p *ping.Packet) {
		if p == nil {
			return
		}
		Add(&md, "ping.rtt", float64(p.Rtt)/float64(time.Millisecond), opentsdb.TagSet{"dst_host": host}, metadata.Unknown, metadata.None, "")
		Add(&md, "ping.ttl", p.Ttl, opentsdb.TagSet{"dst_host": host}, metadata.Unknown, metadata.None, "")
		timeout = 0
	}
	p.Run()
	Add(&md, "ping.timeout", timeout, opentsdb.TagSet{"dst_host": host}, metadata.Unknown, metadata.None, "")
	return md, nil
}

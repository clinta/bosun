package collectors

import (
	"context"
	"fmt"
	"net"
	"time"

	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"bosun.org/slog"

	"github.com/TrilliumIT/go-multiping/ping"
	"github.com/TrilliumIT/go-multiping/pinger"
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type ICMPCollector struct {
	host string
	TagOverride
}

func (i *ICMPCollector) Name() string {
	return fmt.Sprintf("icmp-%s", i.host)
}

func (c *ICMPCollector) Init() {}

func (i *ICMPCollector) Run(dpchan chan<- *opentsdb.DataPoint, quit <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())

	handle := func(ctx context.Context, p *ping.Ping, err error) {
		var md opentsdb.MultiDataPoint

		_, resolveFailed := err.(*net.DNSError)
		AddTS(&md, "ping.resolved", p.Sent.Unix(), !resolveFailed, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")

		timedOut := err != nil
		AddTS(&md, "ping.timeout", p.Sent.Unix(), timedOut, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")

		if p.RTT() != 0 {
			AddTS(&md, "ping.rtt", p.Sent.Unix(), float64(p.RTT())/float64(time.Millisecond), opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")
		}

		if p.TTL != 0 {
			AddTS(&md, "ping.ttl", p.Sent.Unix(), p.TTL, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")
		}

		for _, dp := range md {
			i.ApplyTagOverrides(dp.Tags)
			select {
			case <-ctx.Done():
				return
			case dpchan <- dp:
			}
		}
	}

	conf := pinger.DefaultPingConf()
	conf.Interval = DefaultFreq
	conf.ReResolveEvery = 1
	conf.RetryOnResolveError = true
	conf.RetryOnSendError = true
	conf.RandDelay = true

	ech := make(chan error)
	go func() {
		ech <- pinger.PingWithContext(ctx, i.host, handle, conf)
	}()

	<-quit
	cancel()
	err := <-ech
	if err != nil {
		slog.Errorf("%v: %v", i.Name(), err)
	}
}

// ICMP registers an ICMP collector a given host.
func ICMP(host string) error {
	if host == "" {
		return fmt.Errorf("empty ICMP hostname")
	}

	collectors = append(collectors, &ICMPCollector{
		host: host,
	})
	return nil
}

package collectors

import (
	"fmt"
	"net"
	"time"

	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"bosun.org/slog"

	"github.com/clinta/go-multiping/packet"
	"github.com/clinta/go-multiping/pinger"
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type ICMPCollector struct {
	host string
	dst  *pinger.Dst

	TagOverride
}

func (i *ICMPCollector) Name() string {
	return fmt.Sprintf("icmp-%s", i.host)
}

func (c *ICMPCollector) Init() {}

func (i *ICMPCollector) Run(dpchan chan<- *opentsdb.DataPoint, quit <-chan struct{}) {
	onReply := func(p *packet.Packet) {
		var md opentsdb.MultiDataPoint
		AddTS(&md, "ping.rtt", p.Sent.Unix(), float64(p.RTT)/float64(time.Millisecond), opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")
		AddTS(&md, "ping.timeout", p.Sent.Unix(), 0, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")
		AddTS(&md, "ping.ttl", p.Sent.Unix(), p.TTL, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")

		for _, dp := range md {
			i.ApplyTagOverrides(dp.Tags)
			dpchan <- dp
		}
	}

	onTimeout := func(p *packet.SentPacket) {
		var md opentsdb.MultiDataPoint
		AddTS(&md, "ping.timeout", p.Sent.Unix(), 1, opentsdb.TagSet{"dst_host": i.host}, metadata.Unknown, metadata.None, "")

		for _, dp := range md {
			i.ApplyTagOverrides(dp.Tags)
			dpchan <- dp
		}
	}

	i.dst.SetOnReply(onReply)
	i.dst.SetOnTimeout(onTimeout)
	ech := make(chan error)
	go func() {
		ech <- i.dst.Run()
	}()

	var err error
	select {
	case err = <-ech:
	case <-quit:
		i.dst.Stop()
		err = <-ech
	}

	if err != nil {
		slog.Errorf("%v: %v", i.Name(), err)
	}
}

// ICMP registers an ICMP collector a given host.
func ICMP(host string) error {
	if host == "" {
		return fmt.Errorf("empty ICMP hostname")
	}
	dst, err := pinger.NewDst(host, DefaultFreq, time.Second, 0)
	if err != nil {
		return err
	}

	collectors = append(collectors, &ICMPCollector{
		host: host,
		dst:  dst,
	})
	return nil
}

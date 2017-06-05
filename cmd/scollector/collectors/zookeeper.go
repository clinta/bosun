package collectors

import (
	"io"
	"net/textproto"
	"strconv"
	"strings"

	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"bosun.org/slog"
)

func init() {
	zkHost := "localhost:2181"
	registerInit(func(c *conf.Conf) {
		if c.ZKHost != "" {
			zkHost = c.ZKHost
		}
		collectors = append(collectors, &IntervalCollector{F: c_zookeeper(zkHost), Enable: enableZK(zkHost)})
	})
}

func enableZK(zkHost string) func() bool {
	return func() bool {
		_, err := c_zookeeper(zkHost)()
		return err == nil
	}
}

func c_zookeeper(zkHost string) func() (opentsdb.MultiDataPoint, error) {
	return func() (opentsdb.MultiDataPoint, error) {
		c, err := textproto.Dial("tcp", zkHost)
		if err != nil {
			slog.Errorf("Error connecting to %v: %v", zkHost, err)
			return nil, err
		}
		defer c.Close()
		id, err := c.Cmd("mntr")
		if err != nil {
			return nil, err
		}
		c.StartResponse(id)
		defer c.EndResponse(id)
		var md opentsdb.MultiDataPoint
		for {
			l, err := c.ReadLine()
			if err != nil {
				if err != io.EOF {
					slog.Errorf("Error reading line: %v", err)
				}
				break
			}
			p := strings.Fields(l)
			if len(p) != 2 {
				continue
			}
			k := p[0]
			var v int64
			switch k {
			case "zk_server_state":
				k = "zk_server_is_leader"
				if p[1] == "leader" {
					v = 1
				}
			default:
				v, err = strconv.ParseInt(p[1], 10, 0)
				if err != nil {
					continue
				}
			}
			Add(&md, "zookeeper."+k, v, nil, metadata.Unknown, metadata.None, "")
		}
		return md, nil
	}
}

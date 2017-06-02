// +build darwin linux

package collectors

import (
	"math"
	"strings"

	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
)

func init() {
	hdDnURL := "http://localhost:50075/jmx?qry=Hadoop:service=DataNode,name=*"
	hdNnURL := "http://localhost:50070/jmx?qry=Hadoop:service=NameNode,name=*"
	registerInit(func(c *conf.Conf) {
		if c.HDFSDnHost != "" {
			hdDnURL = strings.Replace(hdDnURL, "localhost:50075", c.HDFSDnHost, -1)
		}
		if c.HDFSNnHost != "" {
			hdNnURL = strings.Replace(hdNnURL, "localhost:50075", c.HDFSNnHost, -1)
		}
		collectors = append(collectors, &IntervalCollector{F: c_hdfs(hdDnURL), Enable: enableURL(hdDnURL)})
		collectors = append(collectors, &IntervalCollector{F: c_hdfs(hdNnURL), Enable: enableURL(hdNnURL)})
	})
}

func c_hdfs(url string) func() (opentsdb.MultiDataPoint, error) {
	return func() (opentsdb.MultiDataPoint, error) {
		var j jmx
		if err := getBeans(url, &j); err != nil {
			return nil, err
		}
		var md opentsdb.MultiDataPoint
		for _, b := range j.Beans {
			var bn string
			var ok bool
			if bn, ok = b["name"].(string); !ok {
				continue
			}
			snp := strings.SplitN(bn, "service=", 2)
			sn := strings.SplitN(snp[len(snp)-1], ",", 2)[0]
			bnp := strings.SplitN(bn, "name=", 2)
			bn = strings.SplitN(bnp[len(bnp)-1], ",", 2)[0]
			bnp = strings.SplitN(bn, "-", 2)
			var t opentsdb.TagSet
			if len(bnp) > 1 {
				bn = bnp[0]
				t = make(opentsdb.TagSet)
				t[bn] = bnp[len(bnp)-1]
			}
			mn := "hdfs." + sn + "." + bn + "."
			for k, v := range b {
				if vv, ok := v.(float64); !ok || vv >= math.MaxInt64 {
					continue
				}
				Add(&md, mn+k, v, t, metadata.Unknown, metadata.None, "")
			}
		}
		return md, nil
	}
}

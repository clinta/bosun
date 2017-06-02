// +build darwin linux

package collectors

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
)

var (
	solrMetricsURL = "/solr/admin/metrics?wt=json&type=all"
)

func init() {
	registerInit(func(c *conf.Conf) {
		host := ""
		if c.SolrHost != "" {
			host = "http://" + c.SolrHost
		} else {
			host = "http://localhost:8080"
		}
		solrMetricsURL = host + solrMetricsURL
		collectors = append(collectors, &IntervalCollector{F: c_solr_metrics, Enable: enableURL(solrMetricsURL)})
	})
}

type solrMetrics struct {
	ResponseHeader map[string]interface{} `json:responseHeader`
	Metrics        []interface{}          `json:"metrics"`
}

func getSolrMetrics(url string, sm *solrMetrics) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(&sm); err != nil {
		return err
	}
	return nil
}

func c_solr_metrics() (opentsdb.MultiDataPoint, error) {
	var sm solrMetrics
	if err := getSolrMetrics(solrMetricsURL, &sm); err != nil {
		return nil, err
	}
	var md opentsdb.MultiDataPoint
	for i := 0; i+1 < len(sm.Metrics); i += 2 {
		var ok bool
		var mp string
		if mp, ok = sm.Metrics[i].(string); !ok {
			continue
		}
		var processMetricName func(mp, m string, t opentsdb.TagSet) string
		switch {
		case strings.HasPrefix(mp, "solr.core"):
			processMetricName = processSolrCoreMetricName
		case strings.HasPrefix(mp, "solr.node"):
			processMetricName = processSolrNodeMetricName
		default:
			processMetricName = processSolrDefaultMetricName
		}
		var mm []interface{}
		if mm, ok = sm.Metrics[i+1].([]interface{}); !ok {
			continue
		}
		for ii := 0; ii+1 < len(mm); ii += 2 {
			var mn string
			if mn, ok = mm[ii].(string); !ok {
				continue
			}
			var mmm []interface{}
			if mmm, ok = mm[ii+1].([]interface{}); !ok {
				continue
			}
			t := make(opentsdb.TagSet)
			mn = processMetricName(mp, mn, t)
			addSolrMetrics(mn, t, mmm, &md)
		}
	}
	return md, nil
}

func processSolrDefaultMetricName(mp, m string, t opentsdb.TagSet) string {
	return mp + "." + m
}

func processSolrCoreMetricName(mp, m string, t opentsdb.TagSet) string {
	mps := strings.SplitN(mp, ".", 5)
	if len(mps) == 5 {
		mp = strings.Join(mps[:2], ".")
		t["index"] = mps[2]
		t["shard"] = mps[3]
		t["replica"] = mps[4]
	}
	return processSolrNodeMetricName(mp, m, t)
}

func processSolrNodeMetricName(mp, m string, t opentsdb.TagSet) string {
	ms := strings.SplitN(m, ".", 3)
	if len(ms) == 3 {
		m = ms[2]
		t["category"] = ms[0]
		t["path"] = ms[1]
	}
	return processSolrDefaultMetricName(mp, m, t)
}

func addSolrMetrics(m string, t opentsdb.TagSet, mm []interface{}, md *opentsdb.MultiDataPoint) {
	for i := 0; i+1 < len(mm); i += 2 {
		var ok bool
		var mt string
		if mt, ok = mm[i].(string); !ok {
			continue
		}
		if strings.HasSuffix(mt, "Rate") { // We must filter out rate on this side. Excluding it from the query with type= incorrectly omits jetty counters
			continue
		}
		if mm[i+1].(float64) > math.MaxInt64 {
			continue
		}
		rt := metadata.Unknown
		if mt == "count" {
			rt = metadata.Counter
		}
		ut := metadata.None
		if strings.HasSuffix(mt, "_ms") {
			ut = metadata.MilliSecond
		}
		Add(md, m, mm[i+1], t, rt, ut, "")
	}
}

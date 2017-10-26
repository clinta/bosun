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
	ResponseHeader map[string]interface{}                       `json:responseHeader`
	Metrics        map[string]map[string]map[string]interface{} `json:"metrics"`
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
	for k1, v1 := range sm.Metrics {
		t := make(opentsdb.TagSet)
		if strings.HasPrefix(k1, "solr.core.") {
			k1 = "solr.core"
			t["core"] = strings.Replace(k1, "solr.core.", "", 1)
		}
		for k2, v2 := range v1 {
			for k3, v3 := range v2 {
				mn := strings.Join([]string{k1, k2, k3}, ".")
				addSolrMetric(mn, t, v3, &md)
			}
		}
	}
	return md, nil
}

func addSolrMetric(m string, t opentsdb.TagSet, v interface{}, md *opentsdb.MultiDataPoint) {
	m = strings.Replace(m, "/", "_", math.MaxInt64)
	if strings.HasSuffix(m, "Rate") { // We must filter out rate on this side. Excluding it from the query with type= incorrectly omits jetty counters
		return
	}
	if fv, ok := v.(float64); !ok || fv > math.MaxInt64 {
		return
	}
	rt := metadata.Unknown
	if m == "count" {
		rt = metadata.Counter
	}
	ut := metadata.None
	if strings.HasSuffix(m, "_ms") {
		ut = metadata.MilliSecond
	}
	Add(md, m, v, t, rt, ut, "")
}

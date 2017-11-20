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
	Metrics        map[string]interface{} `json:"metrics"`
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
	addDataPoints("", sm.Metrics, &md)
	return md, nil
}

func addDataPoints(prefix string, metrics map[string]interface{}, md *opentsdb.MultiDataPoint) {
	for k, v := range metrics {
		t := make(opentsdb.TagSet)
		if strings.HasPrefix(k, "solr.core.") {
			t["core"] = strings.Replace(k, "solr.core.", "", 1)
			k = "solr.core"
		}
		mn := k
		if prefix != "" {
			mn = strings.Join([]string{prefix, k}, ".")
		}

		switch cv := v.(type) {
		case map[string]interface{}:
			addDataPoints(mn, v.(map[string]interface{}), md)
		case []interface{}:
			continue
		default:
			addSolrMetric(mn, t, cv, md)
		}
	}
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

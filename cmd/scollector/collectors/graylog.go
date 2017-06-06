package collectors

import (
	"encoding/json"
	"net/http"
	"strings"

	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
)

func init() {
	glURL := "http://localhost:9000"
	registerInit(func(c *conf.Conf) {
		if c.GraylogURL != "" {
			glURL = c.GraylogURL
		}
		collectors = append(collectors, &IntervalCollector{F: c_graylog(glURL), Enable: enableGraylog(glURL)})
	})
}

func enableGraylog(url string) func() bool {
	return func() bool {
		_, err := c_graylog(url)()
		return err == nil
	}
}

func c_graylog(url string) func() (opentsdb.MultiDataPoint, error) {
	return func() (opentsdb.MultiDataPoint, error) {
		var md opentsdb.MultiDataPoint
		type clusterResponse map[string]struct{}
		var cl clusterResponse
		r, err := http.Get(url + "/api/cluster")
		if err != nil {
			return md, err
		}
		err = json.NewDecoder(r.Body).Decode(&cl)
		if err != nil {
			return md, err
		}
		type metricResponse struct {
			Total   int `json:"total"`
			Metrics []struct {
				FullName string `json:"full_name"`
				Metric   struct {
					Time struct {
						Max float64 `json:"max"`
					} `json:"time"`
					Rate struct {
						Total float64 `json:"total"`
					} `json:"rate"`
				} `json:"metric"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"metrics"`
		}
		for c := range cl {
			for _, u := range []string{
				url + "/api/cluster/" + c + "/metrics/namespace/cluster",
				url + "/api/cluster/" + c + "/metrics/namespace/org",
				url + "/api/cluster/" + c + "/metrics/namespace/jvm",
			} {
				var m metricResponse
				r, err := http.Get(u)
				if err != nil {
					return md, err
				}
				err = json.NewDecoder(r.Body).Decode(&m)
				if err != nil {
					return md, err
				}
				for _, m := range m.Metrics {
					if m.Metric.Rate.Total == 0 && m.Metric.Time.Max == 0 {
						continue
					}
					n := m.FullName
					t := opentsdb.TagSet{
						"full_name": m.FullName,
						"type":      m.Type,
					}
					if len(strings.Split(m.FullName, ".")) > 3 {
						n = strings.Join(append(strings.Split(m.FullName, ".")[:2], m.Name), ".")
					}
					Add(&md, "graylog."+n, m.Metric.Rate.Total, t, metadata.Unknown, metadata.None, "")
				}
			}
		}
		return md, nil
	}
}

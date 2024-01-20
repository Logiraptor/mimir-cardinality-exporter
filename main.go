package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	var cfg ClientConfig
	cfg.RegisterFlagsWithPrefix("", flag.CommandLine)

	port := flag.Int("port", 8080, "port to listen on")
	selector := flag.String("selector", ``, "selector to get cardinality for")
	dimension := flag.String("dimension", "job", "dimension to get cardinality for")
	timeout := flag.Duration("timeout", time.Minute, "timeout for fetching cardinality data")

	flag.Parse()

	if *dimension == "" {
		flag.Usage()
		os.Exit(1)
	}

	reg := prometheus.DefaultRegisterer
	rt := http.DefaultTransport
	log := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	mimirClient := NewCardinalityClient(cfg, rt, log)

	collector := newCardinalityCollector(mimirClient, log, *dimension, *selector, *timeout)
	err := reg.Register(collector)
	if err != nil {
		level.Error(log).Log("msg", "failed to register collector", "err", err)
		os.Exit(1)
	}

	http.Handle("/metrics", promhttp.Handler())

	level.Info(log).Log("msg", "starting server", "port", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		level.Error(log).Log("msg", "failed to start server", "err", err)
		os.Exit(1)
	}
}

type cardinalityCollector struct {
	client *cardinalityClient
	log    log.Logger

	totalDesc *prometheus.Desc
	desc      *prometheus.Desc
	dimension string
	selector  string
	timeout   time.Duration
}

func newCardinalityCollector(client *cardinalityClient, log log.Logger, dimension, selector string, timeout time.Duration) *cardinalityCollector {
	constLabels := prometheus.Labels{
		"dimension": dimension,
	}

	return &cardinalityCollector{
		client:    client,
		log:       log,
		totalDesc: prometheus.NewDesc("grafana_mimir_top_cardinality_total", "Total number of time series in Mimir", nil, constLabels),
		desc: prometheus.NewDesc(
			"grafana_mimir_top_cardinality",
			"Cardinality of time series in Mimir",
			[]string{"exported_" + dimension},
			constLabels,
		),
		dimension: dimension,
		selector:  selector,
		timeout:   timeout,
	}
}

var _ prometheus.Collector = (*cardinalityCollector)(nil)

func (c *cardinalityCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.desc
}

func (c *cardinalityCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	response, err := c.client.LabelValuesCardinality(ctx, []string{c.dimension}, c.selector)
	if err != nil {
		level.Error(c.log).Log("msg", "failed to get cardinality", "err", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, float64(response.SeriesCountTotal))

	for _, lvl := range response.Labels {
		if lvl.LabelName != c.dimension {
			// This should never happen
			continue
		}

		for _, lvc := range lvl.Cardinality {
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(lvc.SeriesCount), lvc.LabelValue)
		}
	}

}

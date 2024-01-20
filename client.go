package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
)

type LabelValuesResponse struct {
	SeriesCountTotal int                `json:"series_count_total"`
	Labels           []LabelValuesLabel `json:"labels"`
}

type LabelValuesLabel struct {
	LabelName        string                   `json:"label_name"`
	LabelValuesCount int                      `json:"label_values_count"`
	SeriesCount      int                      `json:"series_count"`
	Cardinality      []LabelValuesCardinality `json:"cardinality"`
}

type LabelValuesCardinality struct {
	LabelValue  string `json:"label_value"`
	SeriesCount int    `json:"series_count"`
}

type LabelNamesResponse struct {
	LabelValuesCountTotal int                     `json:"label_values_count_total"`
	LabelNamesCount       int                     `json:"label_names_count"`
	Cardinality           []LabelNamesCardinality `json:"cardinality"`
}

type LabelNamesCardinality struct {
	LabelName        string `json:"label_name"`
	LabelValuesCount int    `json:"label_values_count"`
}

type headerVar http.Header

func (h *headerVar) Set(value string) error {
	if *h == nil {
		*h = make(headerVar)
	}

	name, value, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("header must be specified as name=value")
	}

	http.Header(*h).Add(name, value)
	return nil
}

func (h *headerVar) String() string {
	return fmt.Sprintf("%v", http.Header(*h))
}

type ClientConfig struct {
	Address  string
	User     string
	Password flagext.Secret
	Headers  headerVar
}

func (cfg *ClientConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.Address, prefix+"address", "", "Address of the Prometheus instance")
	f.StringVar(&cfg.User, prefix+"user", "", "User to be used in basic auth when contacting Prometheus")
	f.Var(&cfg.Password, prefix+"password", "Password to be used in basic auth when contacting Prometheus")
	f.Var(&cfg.Headers, prefix+"header", "Header to be used when contacting Prometheus, can be specified multiple times")
}

func NewCardinalityClient(cfg ClientConfig, rt http.RoundTripper, logger log.Logger) *cardinalityClient {
	return &cardinalityClient{
		address: cfg.Address,
		client: &http.Client{Transport: basicAuthRoundTripper{
			user:     cfg.User,
			password: cfg.Password.String(),
			next:     rt,
			headers:  http.Header(cfg.Headers),
		}},
	}
}

type cardinalityClient struct {
	address string
	client  *http.Client
}

func (c *cardinalityClient) LabelValuesCardinality(ctx context.Context, labels []string, selector string) (LabelValuesResponse, error) {
	url, err := url.JoinPath(c.address, "/prometheus/api/v1/cardinality/label_values")
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		panic(err)
	}

	q := req.URL.Query()
	for _, v := range labels {
		q.Add("label_names[]", v)
	}
	if selector != "" {
		q.Add("selector", selector)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		panic(err)
	}

	var result LabelValuesResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func (c *cardinalityClient) LabelNamesCardinality(ctx context.Context, selector string) (LabelNamesResponse, error) {
	url, err := url.JoinPath(c.address, "/prometheus/api/v1/cardinality/label_names")
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		panic(err)
	}

	q := req.URL.Query()
	q.Add("selector", selector)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		panic(err)
	}

	var result LabelNamesResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

type basicAuthRoundTripper struct {
	next           http.RoundTripper
	user, password string
	headers        http.Header
}

func (rt basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.user != "" || rt.password != "" {
		req.SetBasicAuth(rt.user, rt.password)
	}

	if len(rt.headers) > 0 {
		for key, values := range rt.headers {
			for _, value := range values {
				req.Header.Set(key, value)
			}
		}
	}

	return rt.next.RoundTrip(req)
}

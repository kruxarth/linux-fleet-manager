package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// promTimeout keeps the console snappy when Prometheus is down or slow.
const promTimeout = 2 * time.Second

// ScrapeTarget is one Prometheus scrape target, trimmed to what the console shows.
type ScrapeTarget struct {
	Job        string `json:"job"`
	Instance   string `json:"instance"`
	Hostname   string `json:"hostname,omitempty"` // set on fleet-nodes targets via HTTP SD
	Health     string `json:"health"`             // up | down | unknown
	LastScrape string `json:"last_scrape,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

// MetricsStatus is the body of GET /api/v1/metrics-status.
//
// enabled=false means FLEET_PROMETHEUS_URL is unset (the console hides the section).
// reachable=false means Prometheus did not answer in time; this is a normal 200 response
// so the console can render a friendly state rather than an error page.
type MetricsStatus struct {
	Enabled   bool           `json:"enabled"`
	Reachable bool           `json:"reachable"`
	Targets   []ScrapeTarget `json:"targets"`
}

type promTargetsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ActiveTargets []struct {
			Labels     map[string]string `json:"labels"`
			Health     string            `json:"health"`
			LastScrape string            `json:"lastScrape"`
			LastError  string            `json:"lastError"`
		} `json:"activeTargets"`
	} `json:"data"`
}

func (s *Server) metricsStatus(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.Cfg.PrometheusURL, "/")
	if base == "" {
		writeJSON(w, http.StatusOK, MetricsStatus{Targets: []ScrapeTarget{}})
		return
	}
	out := MetricsStatus{Enabled: true, Targets: []ScrapeTarget{}}
	targets, err := s.fetchPromTargets(r.Context(), base)
	if err != nil {
		s.Log.Warn("prometheus targets", "err", err)
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Reachable = true
	out.Targets = targets
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) fetchPromTargets(ctx context.Context, base string) ([]ScrapeTarget, error) {
	ctx, cancel := context.WithTimeout(ctx, promTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/targets?state=active", nil)
	if err != nil {
		return nil, err
	}
	client := s.PromClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &promStatusError{code: res.StatusCode}
	}
	var body promTargetsResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Status != "success" {
		return nil, &promStatusError{code: -1}
	}
	targets := make([]ScrapeTarget, 0, len(body.Data.ActiveTargets))
	for _, t := range body.Data.ActiveTargets {
		targets = append(targets, ScrapeTarget{
			Job:        t.Labels["job"],
			Instance:   t.Labels["instance"],
			Hostname:   t.Labels["hostname"],
			Health:     t.Health,
			LastScrape: t.LastScrape,
			LastError:  t.LastError,
		})
	}
	return targets, nil
}

type promStatusError struct{ code int }

func (e *promStatusError) Error() string {
	if e.code < 0 {
		return "prometheus: unsuccessful response"
	}
	return "prometheus: unexpected status " + http.StatusText(e.code)
}

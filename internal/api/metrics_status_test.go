package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func getMetricsStatus(t *testing.T, srv *Server, h http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	return do(h, http.MethodGet, "/api/v1/metrics-status", token, nil)
}

func TestMetricsStatusRequiresAdmin(t *testing.T) {
	_, h := testAPI(t)
	if rec := do(h, http.MethodGet, "/api/v1/metrics-status", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestMetricsStatusDisabledWhenURLUnset(t *testing.T) {
	srv, h := testAPI(t)
	rec := getMetricsStatus(t, srv, h, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got MetricsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Reachable || len(got.Targets) != 0 {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestMetricsStatusListsTargets(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/targets" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"activeTargets":[
			{"labels":{"job":"fleetmgr","instance":"fleetmgr:8080"},"health":"up","lastScrape":"2026-11-07T10:00:00Z","lastError":""},
			{"labels":{"job":"demo-node","instance":"node-exporter:9100","hostname":"web-1"},"health":"down","lastScrape":"2026-11-07T10:00:01Z","lastError":"connection refused"}
		],"droppedTargets":[]}}`))
	}))
	defer prom.Close()

	srv, h := testAPI(t)
	srv.Cfg.PrometheusURL = prom.URL + "/"
	rec := getMetricsStatus(t, srv, h, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got MetricsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || !got.Reachable || len(got.Targets) != 2 {
		t.Fatalf("unexpected body: %+v", got)
	}
	if got.Targets[0].Job != "fleetmgr" || got.Targets[0].Health != "up" {
		t.Fatalf("first target: %+v", got.Targets[0])
	}
	if got.Targets[1].Hostname != "web-1" || got.Targets[1].Health != "down" || got.Targets[1].LastError != "connection refused" {
		t.Fatalf("second target: %+v", got.Targets[1])
	}
}

func TestMetricsStatusUnreachable(t *testing.T) {
	prom := httptest.NewServer(http.NotFoundHandler())
	url := prom.URL
	prom.Close() // nothing listens any more

	srv, h := testAPI(t)
	srv.Cfg.PrometheusURL = url
	rec := getMetricsStatus(t, srv, h, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("unreachable Prometheus must not be an error page, got %d", rec.Code)
	}
	var got MetricsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Reachable || got.Targets == nil || len(got.Targets) != 0 {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestMetricsStatusTimesOutFast(t *testing.T) {
	done := make(chan struct{})
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer prom.Close()
	defer close(done)

	srv, h := testAPI(t)
	srv.Cfg.PrometheusURL = prom.URL
	start := time.Now()
	rec := getMetricsStatus(t, srv, h, "secret")
	if el := time.Since(start); el > promTimeout+time.Second {
		t.Fatalf("handler took %s, expected about %s", el, promTimeout)
	}
	var got MetricsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Reachable {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestMetricsStatusBadUpstream(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer prom.Close()
	srv, h := testAPI(t)
	srv.Cfg.PrometheusURL = prom.URL
	var got MetricsStatus
	rec := getMetricsStatus(t, srv, h, "secret")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got.Reachable {
		t.Fatalf("code=%d body=%+v", rec.Code, got)
	}
}

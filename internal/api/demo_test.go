package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetmgr/internal/auth"
	"fleetmgr/internal/demo"
	"fleetmgr/internal/demo/demotest"
	"fleetmgr/internal/store"
)

func call(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDemoInfoOffHidesToken(t *testing.T) {
	_, h := testAPI(t)
	rec := call(t, h, http.MethodGet, "/api/v1/demo-info", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/api/v1/demo/nodes", "secret"); rec.Code != http.StatusNotFound {
		t.Fatalf("create with demo off: %d", rec.Code)
	}
}

func TestDemoNodeLifecycle(t *testing.T) {
	srv, _ := testAPI(t)
	f, sock := demotest.NewEngine(t)
	srv.Demo = demo.New(demo.Config{Socket: sock, Image: "node:test", Network: "n", ManagerURL: "http://m", ExporterAddr: "e:9100"})
	h := srv.Router()

	rec := call(t, h, http.MethodGet, "/api/v1/demo-info", "")
	var info struct {
		Enabled    bool   `json:"enabled"`
		AdminToken string `json:"admin_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &info)
	if !info.Enabled || info.AdminToken != "secret" {
		t.Fatalf("demo-info %s", rec.Body)
	}

	if rec := call(t, h, http.MethodPost, "/api/v1/demo/nodes", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("demo routes must need the admin token, got %d", rec.Code)
	}
	if rec := call(t, h, http.MethodPost, "/api/v1/demo/nodes", "secret"); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}

	// The container got a real single-use invite code.
	var tok string
	for _, e := range f.Containers["demo-node-1"].Env {
		if v, ok := strings.CutPrefix(e, "FLEET_ENROLL_TOKEN="); ok {
			tok = v
		}
	}
	ctx := t.Context()
	if _, err := srv.Store.ConsumeEnrollToken(ctx, auth.HashToken(tok), srv.now()); err != nil {
		t.Fatalf("invite code not usable: %v", err)
	}
	if _, err := srv.Store.ConsumeEnrollToken(ctx, auth.HashToken(tok), srv.now()); err == nil {
		t.Fatal("invite code should be single use")
	}

	// Pretend it enrolled, then remove it: the node must be revoked too.
	if err := srv.Store.InsertNode(ctx, store.NodeRow{ID: "n1", Hostname: "demo-node-1", State: "online", TokenHash: "h", LabelsJSON: "{}", CreatedAt: rfc(srv.now()), UpdatedAt: rfc(srv.now())}); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, h, http.MethodPost, "/api/v1/demo/nodes/demo-node-1/stop", "secret"); rec.Code != http.StatusNoContent {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/api/v1/demo/nodes/somebody-else/stop", "secret"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown name: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodDelete, "/api/v1/demo/nodes/demo-node-1", "secret"); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	row, err := srv.Store.GetNode(ctx, "n1")
	if err != nil || row.State != "revoked" {
		t.Fatalf("node should be revoked: %+v %v", row, err)
	}
}

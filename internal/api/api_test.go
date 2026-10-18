package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetmgr/internal/agentproto"
	"fleetmgr/internal/auth"
	"fleetmgr/internal/command"
	"fleetmgr/internal/config"
	"fleetmgr/internal/store"
)

func testAPI(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sqlFS, err := fs.Sub(store.SQLFS, "sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(sqlFS); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	catPath := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(catPath, []byte("commands:\n  - id: uname\n    argv: [echo, hi]\n    timeout_seconds: 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := command.Load(catPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		Cfg: config.Server{
			AdminToken:   "secret",
			PublicURL:    "http://localhost:8080",
			OfflineAfter: 90 * time.Second,
			UIDir:        dir,
		},
		Store:   st,
		Catalog: cat,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:     func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) },
	}
	return srv, srv.Router()
}

func do(h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	_, h := testAPI(t)
	rec := do(h, http.MethodGet, "/healthz", "", nil)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestNodesUnauthorized(t *testing.T) {
	_, h := testAPI(t)
	rec := do(h, http.MethodGet, "/api/v1/nodes", "", nil)
	if rec.Code != 401 {
		t.Fatalf("%d", rec.Code)
	}
}

func TestNodesEmpty(t *testing.T) {
	_, h := testAPI(t)
	rec := do(h, http.MethodGet, "/api/v1/nodes", "secret", nil)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestEnrollHeartbeatCommand(t *testing.T) {
	srv, h := testAPI(t)
	rec := do(h, http.MethodPost, "/api/v1/enroll-tokens", "secret", map[string]any{"ttl_seconds": 3600, "max_uses": 1})
	if rec.Code != 201 {
		t.Fatalf("token %d %s", rec.Code, rec.Body.String())
	}
	var tok agentproto.CreateEnrollTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil {
		t.Fatal(err)
	}

	rec = do(h, http.MethodPost, "/api/v1/agent/enroll", tok.Token, agentproto.EnrollRequest{Hostname: "box-1", OS: "linux", Arch: "amd64"})
	if rec.Code != 201 {
		t.Fatalf("enroll %d %s", rec.Code, rec.Body.String())
	}
	var er agentproto.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", bytes.NewReader([]byte(`{"hostname":"box-1","os":"linux","kernel":"6.1","arch":"amd64"}`)))
	req.Header.Set("Authorization", "Bearer "+er.NodeToken)
	req.Header.Set("X-Node-ID", er.NodeID)
	req.Header.Set("Content-Type", "application/json")
	hb := httptest.NewRecorder()
	h.ServeHTTP(hb, req)
	if hb.Code != 200 {
		t.Fatalf("hb %d %s", hb.Code, hb.Body.String())
	}

	rec = do(h, http.MethodPost, "/api/v1/nodes/"+er.NodeID+"/commands", "secret", map[string]string{"command_id": "nope"})
	if rec.Code != 400 {
		t.Fatalf("unknown cmd %d", rec.Code)
	}

	rec = do(h, http.MethodPost, "/api/v1/nodes/"+er.NodeID+"/commands", "secret", map[string]string{"command_id": "uname"})
	if rec.Code != 202 {
		t.Fatalf("enqueue %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/jobs?wait=0", nil)
	req.Header.Set("Authorization", "Bearer "+er.NodeToken)
	req.Header.Set("X-Node-ID", er.NodeID)
	jobsRec := httptest.NewRecorder()
	h.ServeHTTP(jobsRec, req)
	if jobsRec.Code != 200 {
		t.Fatalf("jobs %d %s", jobsRec.Code, jobsRec.Body.String())
	}
	var jobs []agentproto.Job
	if err := json.Unmarshal(jobsRec.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs %#v", jobs)
	}

	_ = srv.Store.Heartbeat(context.Background(), er.NodeID, "box-1", "linux", "6.1", "amd64", "0.1", "", srv.now().Add(-5*time.Minute))
	srv.Now = func() time.Time { return time.Date(2026, 9, 17, 12, 10, 0, 0, time.UTC) }
	rec = do(h, http.MethodPost, "/api/v1/nodes/"+er.NodeID+"/commands", "secret", map[string]string{"command_id": "uname"})
	if rec.Code != 409 {
		t.Fatalf("offline enqueue %d %s", rec.Code, rec.Body.String())
	}
}

func TestTokenHash(t *testing.T) {
	a := auth.HashToken("abc")
	b := auth.HashToken("abc")
	if a != b || !auth.EqualHash(a, b) {
		t.Fatal("hash mismatch")
	}
}

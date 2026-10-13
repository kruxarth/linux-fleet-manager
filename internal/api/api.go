package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"fleetmgr/internal/agentproto"
	"fleetmgr/internal/auth"
	"fleetmgr/internal/command"
	"fleetmgr/internal/config"
	"fleetmgr/internal/demo"
	"fleetmgr/internal/metrics"
	nodestate "fleetmgr/internal/node"
	"fleetmgr/internal/store"
)

type Server struct {
	Cfg     config.Server
	Store   *store.Store
	Catalog *command.Catalog
	Log     *slog.Logger
	Now     func() time.Time
	SQLFS   fs.FS
	// PromClient is used for the Prometheus targets lookup; nil uses http.DefaultClient.
	PromClient *http.Client
	// Demo runs pretend nodes as containers; nil unless FLEET_DEMO_MODE is on.
	Demo *demo.Docker
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.logRequests)
	r.Use(middleware.Recoverer)
	r.Use(cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/metrics", promhttp.Handler())

	r.Get("/api/v1/prometheus/sd", s.prometheusSD)
	r.Get("/install.sh", s.installScript)
	r.Get("/api/v1/demo-info", s.demoInfo)

	r.Route("/api/v1/agent", func(r chi.Router) {
		r.Post("/enroll", s.agentEnroll)
		r.Group(func(r chi.Router) {
			r.Use(s.nodeAuth)
			r.Post("/heartbeat", s.agentHeartbeat)
			r.Get("/jobs", s.agentJobs)
			r.Post("/jobs/{id}/result", s.agentJobResult)
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.adminAuth)
		r.Get("/session", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		})
		r.Get("/catalog", s.catalog)
		r.Get("/metrics-status", s.metricsStatus)
		r.Get("/nodes", s.listNodes)
		r.Get("/nodes/{id}", s.getNode)
		r.Delete("/nodes/{id}", s.revokeNode)
		r.Post("/nodes/{id}/commands", s.enqueueCommand)
		r.Get("/nodes/{id}/commands", s.listNodeCommands)
		r.Post("/enroll-tokens", s.createEnrollToken)
		r.Get("/commands", s.listCommands)
		r.Get("/commands/{id}", s.getCommand)
		r.Get("/demo/nodes", s.demoList)
		r.Post("/demo/nodes", s.demoCreate)
		r.Post("/demo/nodes/{name}/stop", s.demoAction("stop"))
		r.Post("/demo/nodes/{name}/start", s.demoAction("start"))
		r.Delete("/demo/nodes/{name}", s.demoAction("remove"))
	})

	r.Get("/downloads/{name}", s.downloadAgent)
	r.Handle("/*", spa(s.Cfg.UIDir))
	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Node-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.Log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"ms", time.Since(start).Milliseconds(),
			"id", middleware.GetReqID(r.Context()),
		)
	})
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := auth.Bearer(r.Header.Get("Authorization"))
		if !ok || !auth.EqualHash(auth.HashToken(tok), auth.HashToken(s.Cfg.AdminToken)) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const nodeKey ctxKey = 1

func (s *Server) nodeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := auth.Bearer(r.Header.Get("Authorization"))
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		row, err := s.Store.GetNodeByTokenHash(r.Context(), auth.HashToken(tok))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if row.State == nodestate.Revoked {
			writeErr(w, http.StatusForbidden, "revoked")
			return
		}
		if hid := r.Header.Get("X-Node-ID"); hid != "" && hid != row.ID {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), nodeKey, row)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func nodeFrom(r *http.Request) store.NodeRow {
	return r.Context().Value(nodeKey).(store.NodeRow)
}

func (s *Server) catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Catalog.List())
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListNodes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	now := s.now()
	out := make([]agentproto.Node, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ToProto(now, s.Cfg.OfflineAfter))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.Store.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	n := row.ToProto(s.now(), s.Cfg.OfflineAfter)
	cmds, _ := s.Store.ListCommands(r.Context(), id, 20)
	writeJSON(w, http.StatusOK, map[string]any{"node": n, "commands": cmds})
}

func (s *Server) revokeNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Store.RevokeNode(r.Context(), id, s.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	_ = s.Store.Audit(r.Context(), "admin", "revoke", map[string]string{"node_id": id}, s.now())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createEnrollToken(w http.ResponseWriter, r *http.Request) {
	var req agentproto.CreateEnrollTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.TTLSeconds <= 0 {
		req.TTLSeconds = 3600
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	tok, err := auth.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	id := uuid.NewString()
	now := s.now()
	exp := now.Add(time.Duration(req.TTLSeconds) * time.Second)
	if err := s.Store.CreateEnrollToken(r.Context(), id, auth.HashToken(tok), store.LabelsJSON(req.Labels), exp, req.MaxUses, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	installURL := strings.TrimRight(s.Cfg.PublicURL, "/") + "/install.sh?token=" + tok
	_ = s.Store.Audit(r.Context(), "admin", "enroll_token", map[string]any{"id": id, "max_uses": req.MaxUses}, now)
	writeJSON(w, http.StatusCreated, agentproto.CreateEnrollTokenResponse{
		ID:         id,
		Token:      tok,
		ExpiresAt:  exp,
		MaxUses:    req.MaxUses,
		Labels:     req.Labels,
		InstallURL: installURL,
		InstallCmd: "curl -fsSL " + installURL + " | sudo bash",
	})
}

func (s *Server) enqueueCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req agentproto.EnqueueCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, ok := s.Catalog.Get(req.CommandID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown command_id")
		return
	}
	row, err := s.Store.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	n := row.ToProto(s.now(), s.Cfg.OfflineAfter)
	if !nodestate.CanDispatch(n.State) {
		writeErr(w, http.StatusConflict, "node is not online")
		return
	}
	cmd := agentproto.Command{
		ID:             uuid.NewString(),
		NodeID:         id,
		CommandID:      item.ID,
		Argv:           item.Argv,
		Status:         "queued",
		TimeoutSeconds: item.TimeoutSeconds,
		CreatedAt:      s.now(),
	}
	if err := s.Store.InsertCommand(r.Context(), cmd); err != nil {
		writeErr(w, http.StatusInternalServerError, "enqueue failed")
		return
	}
	metrics.Commands.WithLabelValues("queued").Inc()
	_ = s.Store.Audit(r.Context(), "admin", "command", map[string]string{"node_id": id, "command_id": item.ID, "id": cmd.ID}, s.now())
	writeJSON(w, http.StatusAccepted, cmd)
}

func (s *Server) listNodeCommands(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cmds, err := s.Store.ListCommands(r.Context(), id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, cmds)
}

func (s *Server) listCommands(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	cmds, err := s.Store.ListCommands(r.Context(), nodeID, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, cmds)
}

func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetCommand(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) agentEnroll(w http.ResponseWriter, r *http.Request) {
	tok, ok := auth.Bearer(r.Header.Get("Authorization"))
	if !ok {
		metrics.Enrollments.WithLabelValues("denied").Inc()
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req agentproto.EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Hostname) == "" {
		writeErr(w, http.StatusBadRequest, "hostname required")
		return
	}
	now := s.now()
	et, err := s.Store.ConsumeEnrollToken(r.Context(), auth.HashToken(tok), now)
	if err != nil {
		metrics.Enrollments.WithLabelValues("denied").Inc()
		writeErr(w, http.StatusUnauthorized, "invalid or expired enrollment token")
		return
	}
	nodeTok, err := auth.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	id := uuid.NewString()
	row := store.NodeRow{
		ID:              id,
		Hostname:        req.Hostname,
		OS:              req.OS,
		Kernel:          req.Kernel,
		Arch:            req.Arch,
		AgentVersion:    req.AgentVersion,
		LabelsJSON:      et.LabelsJSON,
		State:           nodestate.Pending,
		TokenHash:       auth.HashToken(nodeTok),
		NodeExporterURL: req.NodeExporterAddr,
		CreatedAt:       rfc(now),
		UpdatedAt:       rfc(now),
	}
	if err := s.Store.InsertNode(r.Context(), row); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	metrics.Enrollments.WithLabelValues("ok").Inc()
	_ = s.Store.Audit(r.Context(), "node:"+id, "enroll", map[string]string{"hostname": req.Hostname}, now)
	writeJSON(w, http.StatusCreated, agentproto.EnrollResponse{NodeID: id, NodeToken: nodeTok})
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *Server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	row := nodeFrom(r)
	var req agentproto.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Hostname == "" {
		req.Hostname = row.Hostname
	}
	if req.NodeExporterAddr == "" {
		req.NodeExporterAddr = row.NodeExporterURL
	}
	if err := s.Store.Heartbeat(r.Context(), row.ID, req.Hostname, req.OS, req.Kernel, req.Arch, req.AgentVersion, req.NodeExporterAddr, s.now()); err != nil {
		if errors.Is(err, store.ErrUnauthorized) {
			writeErr(w, http.StatusForbidden, "revoked")
			return
		}
		writeErr(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	metrics.Heartbeats.Inc()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) agentJobs(w http.ResponseWriter, r *http.Request) {
	row := nodeFrom(r)
	wait := 20 * time.Second
	if r.URL.Query().Get("wait") == "0" {
		wait = 0
	}
	deadline := time.Now().Add(wait)
	for {
		jobs, err := s.Store.ClaimJobs(r.Context(), row.ID, s.now())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "jobs")
			return
		}
		if len(jobs) > 0 {
			writeJSON(w, http.StatusOK, jobs)
			return
		}
		if time.Now().After(deadline) {
			writeJSON(w, http.StatusOK, []agentproto.Job{})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Server) agentJobResult(w http.ResponseWriter, r *http.Request) {
	row := nodeFrom(r)
	id := chi.URLParam(r, "id")
	cmd, err := s.Store.GetCommand(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if cmd.NodeID != row.ID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var req agentproto.JobResult
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.Store.FinishJob(r.Context(), id, req.Stdout, req.Stderr, req.ExitCode, req.TimedOut, s.now()); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	status := "succeeded"
	if req.TimedOut {
		status = "timeout"
	} else if req.ExitCode != 0 {
		status = "failed"
	}
	metrics.Commands.WithLabelValues(status).Inc()
	if req.DurationMS > 0 {
		metrics.CommandDuration.Observe(float64(req.DurationMS) / 1000.0)
	}
	lastErr := ""
	if status != "succeeded" {
		lastErr = status
	}
	_ = s.Store.SetNodeError(r.Context(), row.ID, lastErr, s.now())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) prometheusSD(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.PrometheusSD(r.Context(), s.now(), s.Cfg.OfflineAfter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sd")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) installScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	for _, c := range token {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
	}
	base := strings.TrimRight(s.Cfg.PublicURL, "/")
	script := `#!/bin/bash
set -euo pipefail
TOKEN="` + token + `"
MANAGER="` + base + `"
if [ "$(id -u)" -ne 0 ]; then
  echo "run as root" >&2
  exit 1
fi
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch $ARCH" >&2; exit 1 ;;
esac
mkdir -p /etc/fleetmgr /var/lib/fleetmgr-agent /usr/local/bin
curl -fsSL "$MANAGER/downloads/agent-linux-$ARCH" -o /usr/local/bin/fleetmgr-agent
chmod 755 /usr/local/bin/fleetmgr-agent
# Primary IPv4 address. "hostname -I" is GNU-only (Debian/Ubuntu), so try iproute2 first.
NODE_IP=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i+1); exit }}') || true
if [ -z "$NODE_IP" ]; then
  NODE_IP=$(hostname -I 2>/dev/null | awk '{print $1}') || true
fi
EXPORTER_ADDR=""
if [ -n "$NODE_IP" ]; then
  EXPORTER_ADDR="$NODE_IP:9100"
else
  echo "warning: could not detect this node's IP; set node_exporter_addr in /etc/fleetmgr/agent.yaml" >&2
fi
cat > /etc/fleetmgr/agent.yaml <<EOF
manager_url: "$MANAGER"
enroll_token: "$TOKEN"
data_dir: /var/lib/fleetmgr-agent
heartbeat_interval: 15s
node_exporter_addr: "$EXPORTER_ADDR"
EOF
chmod 600 /etc/fleetmgr/agent.yaml
cat > /etc/systemd/system/fleetmgr-agent.service <<'UNIT'
[Unit]
Description=Fleet Manager agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/fleetmgr-agent -config /etc/fleetmgr/agent.yaml
Restart=always
RestartSec=3
User=root
WorkingDirectory=/var/lib/fleetmgr-agent

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now fleetmgr-agent.service
echo "agent installed. install node_exporter separately if metrics scrape is required."
`
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = io.WriteString(w, script)
}

func (s *Server) downloadAgent(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if strings.Contains(name, "..") || strings.Contains(name, "/") {
		writeErr(w, http.StatusBadRequest, "invalid")
		return
	}
	path := filepath.Join(s.Cfg.AgentBinDir, name)
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "binary not built")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, name, time.Now(), f)
}

func (s *Server) RefreshMetrics(ctx context.Context) {
	m, err := s.Store.CountsByState(ctx, s.now(), s.Cfg.OfflineAfter)
	if err != nil {
		return
	}
	metrics.SetNodeCounts(m)
}

func (s *Server) ReaperLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Store.Reap(ctx, s.now(), s.Cfg.OfflineAfter); err != nil && !errors.Is(err, context.Canceled) {
				s.Log.Error("reap", "err", err)
			}
			s.RefreshMetrics(ctx)
		}
	}
}

func spa(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if !strings.HasPrefix(p, filepath.Clean(dir)+string(os.PathSeparator)) && filepath.Clean(p) != filepath.Clean(dir) {
			http.NotFound(w, r)
			return
		}
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "ui not built (cd web && npm run build)", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, index)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, agentproto.ErrorBody{Error: msg})
}

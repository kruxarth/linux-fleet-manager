package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"fleetmgr/internal/auth"
	"fleetmgr/internal/demo"
	"fleetmgr/internal/store"
)

// demoInfo is public: the login screen uses it to show the demo token.
// It only reveals anything when the manager was started with FLEET_DEMO_MODE.
func (s *Server) demoInfo(w http.ResponseWriter, _ *http.Request) {
	if s.Demo == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"admin_token":   s.Cfg.AdminToken,
		"offline_after": s.Cfg.OfflineAfter.Seconds(),
	})
}

func (s *Server) demoList(w http.ResponseWriter, r *http.Request) {
	if s.Demo == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "nodes": []demo.Node{}})
		return
	}
	nodes, err := s.Demo.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": true, "docker_ok": false, "nodes": []demo.Node{},
			"error": "Can't reach Docker from the manager: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "docker_ok": true, "nodes": nodes})
}

func (s *Server) demoCreate(w http.ResponseWriter, r *http.Request) {
	if s.Demo == nil {
		writeErr(w, http.StatusNotFound, "demo mode is off")
		return
	}
	n, err := s.spawnDemoNode(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) demoAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Demo == nil {
			writeErr(w, http.StatusNotFound, "demo mode is off")
			return
		}
		name := chi.URLParam(r, "name")
		var err error
		switch action {
		case "stop":
			err = s.Demo.Stop(r.Context(), name)
		case "start":
			err = s.Demo.Start(r.Context(), name)
		case "remove":
			err = s.Demo.Remove(r.Context(), name)
			if err == nil {
				s.revokeByHostname(r.Context(), name)
			}
		}
		if errors.Is(err, demo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "no demo node called "+name)
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		_ = s.Store.Audit(r.Context(), "admin", "demo_"+action, map[string]string{"name": name}, s.now())
		w.WriteHeader(http.StatusNoContent)
	}
}

// spawnDemoNode mints a single-use invite code and starts a container that enrolls with it,
// the same path a real machine takes through install.sh.
func (s *Server) spawnDemoNode(ctx context.Context) (demo.Node, error) {
	tok, err := auth.NewToken()
	if err != nil {
		return demo.Node{}, err
	}
	now := s.now()
	id := uuid.NewString()
	labels := store.LabelsJSON(map[string]string{"role": "demo"})
	if err := s.Store.CreateEnrollToken(ctx, id, auth.HashToken(tok), labels, now.Add(10*time.Minute), 1, now); err != nil {
		return demo.Node{}, err
	}
	n, err := s.Demo.Create(ctx, tok)
	if err != nil {
		return demo.Node{}, err
	}
	_ = s.Store.Audit(ctx, "admin", "demo_create", map[string]string{"name": n.Name, "enroll_token_id": id}, now)
	return n, nil
}

func (s *Server) revokeByHostname(ctx context.Context, hostname string) {
	rows, err := s.Store.ListNodes(ctx)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.Hostname == hostname {
			_ = s.Store.RevokeNode(ctx, row.ID, s.now())
		}
	}
}

// SeedDemo tops the fleet up to FLEET_DEMO_NODES containers, so a fresh
// `docker compose up` already has a fleet to look at.
func (s *Server) SeedDemo(ctx context.Context) {
	if s.Demo == nil {
		return
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := s.Demo.Ping(ctx); err == nil {
			break
		} else if time.Now().After(deadline) || ctx.Err() != nil {
			s.Log.Warn("demo: docker socket not reachable; demo nodes disabled", "socket", s.Cfg.DemoSocket, "err", err)
			return
		}
		time.Sleep(time.Second)
	}
	existing, err := s.Demo.List(ctx)
	if err != nil {
		s.Log.Warn("demo: list", "err", err)
		return
	}
	for i := len(existing); i < s.Cfg.DemoNodes; i++ {
		n, err := s.spawnDemoNode(ctx)
		if err != nil {
			s.Log.Warn("demo: start node", "err", err)
			return
		}
		s.Log.Info("demo: started node", "name", n.Name)
	}
}

// CleanupDemo removes demo containers and revokes their nodes. It runs on
// shutdown so `docker compose down` leaves nothing behind.
func (s *Server) CleanupDemo(ctx context.Context) {
	if s.Demo == nil {
		return
	}
	nodes, err := s.Demo.List(ctx)
	if err != nil {
		return
	}
	for _, n := range nodes {
		if err := s.Demo.Remove(ctx, n.Name); err == nil {
			s.revokeByHostname(ctx, n.Name)
			s.Log.Info("demo: removed node", "name", n.Name)
		}
	}
}

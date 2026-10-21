// Package demotest is an in-memory Docker Engine API for tests.
package demotest

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Engine is a tiny in-memory Docker Engine API on a unix socket.
type Engine struct {
	mu         sync.Mutex
	Containers map[string]*Container
	Images     map[string]bool
	Calls      []string
}

type Container struct {
	Name, State string
	Labels      map[string]string
	Env         []string
	Network     string
}

func NewEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	f := &Engine{Containers: map[string]*Container{}, Images: map[string]bool{"node:test": true}}
	srv := &http.Server{Handler: http.HandlerFunc(f.serve)}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return f, sock
}

func (f *Engine) Add(name, state string, labels map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Containers[name] = &Container{Name: name, State: state, Labels: labels}
}

func (f *Engine) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, r.Method+" "+r.URL.Path)
	p := r.URL.Path
	switch {
	case p == "/_ping":
		_, _ = w.Write([]byte("OK"))
	case p == "/containers/json":
		var filt map[string][]string
		_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filt)
		var out []map[string]any
		for _, c := range f.Containers {
			ok := true
			for _, l := range filt["label"] {
				k, v, _ := strings.Cut(l, "=")
				if c.Labels[k] != v {
					ok = false
				}
			}
			if ok {
				out = append(out, map[string]any{"Names": []string{"/" + c.Name}, "State": c.State, "Status": c.State})
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	case p == "/containers/create":
		var spec struct {
			Image      string
			Env        []string
			Labels     map[string]string
			HostConfig struct{ NetworkMode string }
		}
		_ = json.NewDecoder(r.Body).Decode(&spec)
		if !f.Images[spec.Image] {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such image: ` + spec.Image + `"}`))
			return
		}
		name := r.URL.Query().Get("name")
		f.Containers[name] = &Container{Name: name, State: "created", Labels: spec.Labels, Env: spec.Env, Network: spec.HostConfig.NetworkMode}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"Id":"` + name + `"}`))
	case strings.HasPrefix(p, "/containers/"):
		rest := strings.TrimPrefix(p, "/containers/")
		name, verb, _ := strings.Cut(rest, "/")
		c, ok := f.Containers[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch {
		case r.Method == http.MethodDelete:
			delete(f.Containers, name)
			w.WriteHeader(http.StatusNoContent)
		case verb == "start":
			if c.State == "running" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			c.State = "running"
			w.WriteHeader(http.StatusNoContent)
		case verb == "stop":
			if c.State != "running" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			c.State = "exited"
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

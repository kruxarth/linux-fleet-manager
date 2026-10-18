// Package demo runs pretend Linux nodes as Docker containers so the fleet can
// be demonstrated on one laptop. It is only wired up when FLEET_DEMO_MODE is on.
//
// It talks to the Docker Engine API over the local unix socket with plain
// net/http, and only ever touches containers carrying the demo label, so it
// cannot be used to stop or remove anything else on the host.
package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Label      = "fleetmgr.demo"
	namePrefix = "demo-node-"
)

var ErrNotFound = errors.New("demo node not found")

// Config describes how demo nodes are created.
type Config struct {
	Socket       string // e.g. /var/run/docker.sock
	Image        string // image with the agent and coreutils/procps
	Network      string // docker network shared with the manager
	ManagerURL   string // URL the agent uses to reach the manager from inside the network
	ExporterAddr string // node_exporter address reported by demo agents
}

// Node is one demo container.
type Node struct {
	Name    string `json:"name"`
	State   string `json:"state"`  // docker state: running, exited, created, ...
	Status  string `json:"status"` // human text from docker, e.g. "Up 3 minutes"
	Created int64  `json:"created"`
}

type Docker struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Docker {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", cfg.Socket)
		},
	}
	return &Docker{cfg: cfg, http: &http.Client{Transport: tr, Timeout: 20 * time.Second}}
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("docker: %d %s", e.Status, e.Message) }

func (d *Docker) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		var m struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &m)
		if m.Message == "" {
			m.Message = strings.TrimSpace(string(data))
		}
		return &apiError{Status: res.StatusCode, Message: m.Message}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Ping reports whether the Docker socket answers.
func (d *Docker) Ping(ctx context.Context) error {
	return d.do(ctx, http.MethodGet, "/_ping", nil, nil)
}

// List returns demo containers sorted by name.
func (d *Docker) List(ctx context.Context) ([]Node, error) {
	f, _ := json.Marshal(map[string][]string{"label": {Label + "=1"}})
	var raw []struct {
		Names   []string `json:"Names"`
		State   string   `json:"State"`
		Status  string   `json:"Status"`
		Created int64    `json:"Created"`
	}
	if err := d.do(ctx, http.MethodGet, "/containers/json?all=1&filters="+url.QueryEscape(string(f)), nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, Node{Name: name, State: c.State, Status: c.Status, Created: c.Created})
	}
	sort.Slice(out, func(i, j int) bool { return nameNum(out[i].Name) < nameNum(out[j].Name) })
	return out, nil
}

var numRe = regexp.MustCompile(`(\d+)$`)

func nameNum(name string) int {
	m := numRe.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// NextName picks demo-node-N with N one above the highest in use.
func NextName(existing []Node) string {
	max := 0
	for _, n := range existing {
		if v := nameNum(n.Name); v > max {
			max = v
		}
	}
	return fmt.Sprintf("%s%d", namePrefix, max+1)
}

// Create starts a new demo node that enrolls with the given one-time token.
func (d *Docker) Create(ctx context.Context, enrollToken string) (Node, error) {
	existing, err := d.List(ctx)
	if err != nil {
		return Node{}, err
	}
	name := NextName(existing)
	spec := map[string]any{
		"Image":    d.cfg.Image,
		"Hostname": name,
		"Env": []string{
			"FLEET_MANAGER_URL=" + d.cfg.ManagerURL,
			"FLEET_ENROLL_TOKEN=" + enrollToken,
			"FLEET_NODE_EXPORTER_ADDR=" + d.cfg.ExporterAddr,
		},
		"Labels": map[string]string{Label: "1"},
		"HostConfig": map[string]any{
			"NetworkMode": d.cfg.Network,
			"Init":        true,
		},
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := d.do(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), spec, &created); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return Node{}, fmt.Errorf("demo node image %q is missing; run docker compose up --build", d.cfg.Image)
		}
		return Node{}, err
	}
	if err := d.do(ctx, http.MethodPost, "/containers/"+created.ID+"/start", nil, nil); err != nil {
		_ = d.do(ctx, http.MethodDelete, "/containers/"+created.ID+"?force=1", nil, nil)
		return Node{}, err
	}
	return Node{Name: name, State: "running", Status: "Up"}, nil
}

// find makes sure name is one of our labelled containers before acting on it.
func (d *Docker) find(ctx context.Context, name string) (Node, error) {
	nodes, err := d.List(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return Node{}, ErrNotFound
}

func (d *Docker) act(ctx context.Context, name, method, suffix string) error {
	n, err := d.find(ctx, name)
	if err != nil {
		return err
	}
	err = d.do(ctx, method, "/containers/"+url.PathEscape(n.Name)+suffix, nil, nil)
	var ae *apiError
	if errors.As(err, &ae) && ae.Status == http.StatusNotModified {
		return nil // already in the requested state
	}
	return err
}

// Stop simulates pulling the plug: the agent stops, heartbeats stop.
func (d *Docker) Stop(ctx context.Context, name string) error {
	return d.act(ctx, name, http.MethodPost, "/stop?t=2")
}

// Start plugs a stopped node back in. It keeps its node identity.
func (d *Docker) Start(ctx context.Context, name string) error {
	return d.act(ctx, name, http.MethodPost, "/start")
}

// Remove deletes the container for good.
func (d *Docker) Remove(ctx context.Context, name string) error {
	return d.act(ctx, name, http.MethodDelete, "?force=1")
}

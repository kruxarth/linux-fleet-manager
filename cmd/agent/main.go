package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"fleetmgr/internal/agentproto"
	"fleetmgr/internal/command"
)

const agentVersion = "0.1.0"

var errRevoked = errors.New("node revoked")

type Config struct {
	ManagerURL        string `yaml:"manager_url"`
	EnrollToken       string `yaml:"enroll_token"`
	NodeToken         string `yaml:"node_token"`
	NodeID            string `yaml:"node_id"`
	HeartbeatInterval string `yaml:"heartbeat_interval"`
	CommandsPath      string `yaml:"commands_path"`
	NodeExporterAddr  string `yaml:"node_exporter_addr"`
	DataDir           string `yaml:"data_dir"`
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfgPath := flag.String("config", getenv("FLEET_AGENT_CONFIG", "./agent.yaml"), "config path")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Error("datadir", "err", err)
		os.Exit(1)
	}

	statePath := filepath.Join(cfg.DataDir, "state.yaml")
	if st, err := loadConfig(statePath); err == nil {
		if st.NodeID != "" {
			cfg.NodeID = st.NodeID
		}
		if st.NodeToken != "" {
			cfg.NodeToken = st.NodeToken
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: 30 * time.Second}

	if cfg.NodeToken == "" {
		if cfg.EnrollToken == "" {
			log.Error("need enroll_token or node_token")
			os.Exit(1)
		}
		if err := enroll(ctx, client, &cfg, *cfgPath, statePath, log); err != nil {
			log.Error("enroll", "err", err)
			os.Exit(1)
		}
	}

	hbEvery, _ := time.ParseDuration(cfg.HeartbeatInterval)
	if hbEvery <= 0 {
		hbEvery = 15 * time.Second
	}

	cat, err := command.Load(cfg.CommandsPath)
	if err != nil {
		log.Error("catalog", "err", err)
		os.Exit(1)
	}

	// A revoked node must stop, not retry forever.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go heartbeatLoop(ctx, cancel, client, cfg, hbEvery, log)
	jobLoop(ctx, cancel, client, cfg, cat, log)
	if errors.Is(context.Cause(ctx), errRevoked) {
		log.Error("node revoked by manager; exiting")
		os.Exit(2)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadConfig(path string) (Config, error) {
	var c Config
	c.DataDir = "./data/agent"
	c.HeartbeatInterval = "15s"
	if os.Getenv("FLEET_MANAGER_URL") != "" {
		c.ManagerURL = os.Getenv("FLEET_MANAGER_URL")
	}
	if os.Getenv("FLEET_ENROLL_TOKEN") != "" {
		c.EnrollToken = os.Getenv("FLEET_ENROLL_TOKEN")
	}
	if os.Getenv("FLEET_AGENT_COMMANDS_PATH") != "" {
		c.CommandsPath = os.Getenv("FLEET_AGENT_COMMANDS_PATH")
	}
	if os.Getenv("FLEET_NODE_EXPORTER_ADDR") != "" {
		c.NodeExporterAddr = os.Getenv("FLEET_NODE_EXPORTER_ADDR")
	}
	if os.Getenv("FLEET_AGENT_DATA_DIR") != "" {
		c.DataDir = os.Getenv("FLEET_AGENT_DATA_DIR")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && c.ManagerURL != "" {
			return c, nil
		}
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, nil
}

func saveState(path string, cfg Config) error {
	b, err := yaml.Marshal(map[string]string{
		"node_id":    cfg.NodeID,
		"node_token": cfg.NodeToken,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func inventory() agentproto.EnrollRequest {
	host, _ := os.Hostname()
	kernel := uname()
	return agentproto.EnrollRequest{
		Hostname:         host,
		OS:               runtime.GOOS,
		Kernel:           kernel,
		Arch:             runtime.GOARCH,
		AgentVersion:     agentVersion,
		NodeExporterAddr: os.Getenv("FLEET_NODE_EXPORTER_ADDR"),
	}
}

func uname() string {
	res := command.Run(context.Background(), []string{"uname", "-r"}, 3*time.Second, 4096, "/tmp")
	if res.ExitCode != 0 {
		return runtime.GOOS
	}
	return string(bytes.TrimSpace([]byte(res.Stdout)))
}

func enroll(ctx context.Context, client *http.Client, cfg *Config, cfgPath, statePath string, log *slog.Logger) error {
	inv := inventory()
	if cfg.NodeExporterAddr != "" {
		inv.NodeExporterAddr = cfg.NodeExporterAddr
	}
	body, _ := json.Marshal(inv)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ManagerURL+"/api/v1/agent/enroll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.EnrollToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("enroll %s: %s", resp.Status, b)
	}
	var out agentproto.EnrollResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	cfg.NodeID = out.NodeID
	cfg.NodeToken = out.NodeToken
	cfg.EnrollToken = ""
	if err := saveState(statePath, *cfg); err != nil {
		return err
	}
	log.Info("enrolled", "node_id", cfg.NodeID)
	return nil
}

func heartbeatLoop(ctx context.Context, cancel context.CancelCauseFunc, client *http.Client, cfg Config, every time.Duration, log *slog.Logger) {
	backoff := time.Second
	for {
		if err := heartbeat(ctx, client, cfg); err != nil {
			if errors.Is(err, errRevoked) {
				cancel(err)
				return
			}
			log.Warn("heartbeat", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 15*time.Second {
				backoff *= 2
				if backoff > 15*time.Second {
					backoff = 15 * time.Second
				}
			}
			continue
		}
		backoff = every
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

func heartbeat(ctx context.Context, client *http.Client, cfg Config) error {
	inv := inventory()
	addr := cfg.NodeExporterAddr
	if addr == "" {
		addr = inv.NodeExporterAddr
	}
	payload := agentproto.HeartbeatRequest{
		Hostname:         inv.Hostname,
		OS:               inv.OS,
		Kernel:           inv.Kernel,
		Arch:             inv.Arch,
		AgentVersion:     inv.AgentVersion,
		NodeExporterAddr: addr,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ManagerURL+"/api/v1/agent/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.NodeToken)
	req.Header.Set("X-Node-ID", cfg.NodeID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return errRevoked
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, b)
	}
	return nil
}

func jobLoop(ctx context.Context, cancel context.CancelCauseFunc, client *http.Client, cfg Config, cat *command.Catalog, log *slog.Logger) {
	for {
		if err := pollOnce(ctx, client, cfg, cat, log); err != nil {
			if errors.Is(err, errRevoked) {
				cancel(err)
				return
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Warn("jobs", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func pollOnce(ctx context.Context, client *http.Client, cfg Config, cat *command.Catalog, log *slog.Logger) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ManagerURL+"/api/v1/agent/jobs", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.NodeToken)
	req.Header.Set("X-Node-ID", cfg.NodeID)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusForbidden {
		return errRevoked
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, b)
	}
	var jobs []agentproto.Job
	if err := json.Unmarshal(b, &jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		runJob(ctx, client, cfg, cat, job, log)
	}
	return nil
}

func runJob(ctx context.Context, client *http.Client, cfg Config, cat *command.Catalog, job agentproto.Job, log *slog.Logger) {
	timeout := time.Duration(job.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	var res command.ExecResult
	if cat.Allows(job.CommandID, job.Argv) {
		res = command.Run(ctx, job.Argv, timeout, 1<<20, cfg.DataDir)
	} else {
		log.Warn("job rejected: not in local allowlist", "id", job.ID, "command", job.CommandID)
		res = command.ExecResult{ExitCode: 126, Stderr: "rejected by agent: command not in local allowlist"}
	}
	payload := agentproto.JobResult{
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		ExitCode:   res.ExitCode,
		DurationMS: res.Duration.Milliseconds(),
		TimedOut:   res.TimedOut,
	}
	body, _ := json.Marshal(payload)
	url := cfg.ManagerURL + "/api/v1/agent/jobs/" + job.ID + "/result"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Error("result request", "err", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.NodeToken)
	req.Header.Set("X-Node-ID", cfg.NodeID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Error("result post", "err", err)
		return
	}
	defer resp.Body.Close()
	log.Info("job", "id", job.ID, "command", job.CommandID, "exit", res.ExitCode, "timeout", res.TimedOut)
}

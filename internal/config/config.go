package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	HTTPAddr     string
	AdminToken   string
	DBPath       string
	PublicURL    string
	OnlineAfter  time.Duration
	OfflineAfter time.Duration
	CommandsPath string
	LogLevel     string
	UIDir        string
	AgentBinDir  string
	// PrometheusURL is the base URL of Prometheus (e.g. http://prometheus:9090).
	// Empty disables the metrics-status feature.
	PrometheusURL string

	// Demo mode: pretend nodes run as Docker containers, managed from the console.
	// Needs the Docker socket; meant for a single-laptop demo, never production.
	DemoMode         bool
	DemoNodes        int    // how many demo nodes to start when there are none
	DemoSocket       string // Docker Engine socket
	DemoImage        string // image with the agent plus uname/uptime/df/free
	DemoNetwork      string // docker network shared with the manager
	DemoManagerURL   string // manager URL as seen from inside that network
	DemoExporterAddr string // node_exporter address the demo agents report
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func LoadServer() (Server, error) {
	c := Server{
		HTTPAddr:      getenv("FLEET_HTTP_ADDR", ":8080"),
		AdminToken:    os.Getenv("FLEET_ADMIN_TOKEN"),
		DBPath:        getenv("FLEET_DB_PATH", "./data/fleet.db"),
		PublicURL:     getenv("FLEET_PUBLIC_URL", "http://localhost:8080"),
		CommandsPath:  os.Getenv("FLEET_COMMANDS_PATH"),
		LogLevel:      getenv("FLEET_LOG_LEVEL", "info"),
		UIDir:         getenv("FLEET_UI_DIR", "./web/dist"),
		AgentBinDir:   getenv("FLEET_AGENT_BIN_DIR", "./bin"),
		PrometheusURL: os.Getenv("FLEET_PROMETHEUS_URL"),

		DemoMode:         boolEnv("FLEET_DEMO_MODE"),
		DemoNodes:        IntEnv("FLEET_DEMO_NODES", 3),
		DemoSocket:       getenv("FLEET_DEMO_DOCKER_SOCKET", "/var/run/docker.sock"),
		DemoImage:        getenv("FLEET_DEMO_NODE_IMAGE", "fleetmgr-node:latest"),
		DemoNetwork:      getenv("FLEET_DEMO_NETWORK", "fleetmgr-aisle"),
		DemoManagerURL:   getenv("FLEET_DEMO_MANAGER_URL", "http://fleetmgr:8080"),
		DemoExporterAddr: getenv("FLEET_DEMO_EXPORTER_ADDR", "node-exporter:9100"),
	}
	var err error
	c.OnlineAfter, err = time.ParseDuration(getenv("FLEET_ONLINE_AFTER", "30s"))
	if err != nil {
		return c, err
	}
	c.OfflineAfter, err = time.ParseDuration(getenv("FLEET_OFFLINE_AFTER", "90s"))
	if err != nil {
		return c, err
	}
	if c.AdminToken == "" {
		return c, fmt.Errorf("FLEET_ADMIN_TOKEN is required")
	}
	return c, nil
}

func boolEnv(k string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func IntEnv(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

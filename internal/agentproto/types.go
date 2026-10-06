package agentproto

import "time"

type Node struct {
	ID              string            `json:"id"`
	Hostname        string            `json:"hostname"`
	OS              string            `json:"os"`
	Kernel          string            `json:"kernel"`
	Arch            string            `json:"arch"`
	AgentVersion    string            `json:"agent_version"`
	Labels          map[string]string `json:"labels"`
	State           string            `json:"state"`
	NodeExporterURL string            `json:"node_exporter_url,omitempty"`
	LastHeartbeatAt *time.Time        `json:"last_heartbeat_at,omitempty"`
	LastError       string            `json:"last_error,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type EnrollRequest struct {
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Kernel           string `json:"kernel"`
	Arch             string `json:"arch"`
	AgentVersion     string `json:"agent_version"`
	NodeExporterAddr string `json:"node_exporter_addr"`
}

type EnrollResponse struct {
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
}

type HeartbeatRequest struct {
	Hostname         string  `json:"hostname"`
	OS               string  `json:"os"`
	Kernel           string  `json:"kernel"`
	Arch             string  `json:"arch"`
	AgentVersion     string  `json:"agent_version"`
	NodeExporterAddr string  `json:"node_exporter_addr"`
	Load1            float64 `json:"load1,omitempty"`
	LastError        string  `json:"last_error,omitempty"`
}

type Job struct {
	ID             string   `json:"id"`
	CommandID      string   `json:"command_id"`
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type JobResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
}

type Command struct {
	ID             string     `json:"id"`
	NodeID         string     `json:"node_id"`
	CommandID      string     `json:"command_id"`
	Argv           []string   `json:"argv"`
	Status         string     `json:"status"`
	Stdout         string     `json:"stdout,omitempty"`
	Stderr         string     `json:"stderr,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

type CatalogItem struct {
	ID             string   `json:"id"`
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Description    string   `json:"description"`
}

type CreateEnrollTokenRequest struct {
	TTLSeconds int               `json:"ttl_seconds"`
	MaxUses    int               `json:"max_uses"`
	Labels     map[string]string `json:"labels"`
}

type CreateEnrollTokenResponse struct {
	ID         string            `json:"id"`
	Token      string            `json:"token"`
	ExpiresAt  time.Time         `json:"expires_at"`
	MaxUses    int               `json:"max_uses"`
	Labels     map[string]string `json:"labels"`
	InstallURL string            `json:"install_url"`
	InstallCmd string            `json:"install_cmd"`
}

type EnqueueCommandRequest struct {
	CommandID string `json:"command_id"`
}

type ErrorBody struct {
	Error string `json:"error"`
}

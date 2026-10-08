CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  hostname TEXT NOT NULL,
  os TEXT,
  kernel TEXT,
  arch TEXT,
  agent_version TEXT,
  labels_json TEXT NOT NULL DEFAULT '{}',
  state TEXT NOT NULL,
  node_token_hash TEXT NOT NULL,
  node_exporter_url TEXT,
  last_heartbeat_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_nodes_state ON nodes(state);
CREATE INDEX IF NOT EXISTS idx_nodes_heartbeat ON nodes(last_heartbeat_at);

CREATE TABLE IF NOT EXISTS enroll_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  labels_json TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  max_uses INTEGER NOT NULL,
  uses INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_enroll_expires ON enroll_tokens(expires_at);

CREATE TABLE IF NOT EXISTS commands (
  id TEXT PRIMARY KEY,
  node_id TEXT NOT NULL,
  command_id TEXT NOT NULL,
  argv_json TEXT NOT NULL,
  status TEXT NOT NULL,
  stdout TEXT,
  stderr TEXT,
  exit_code INTEGER,
  timeout_seconds INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  FOREIGN KEY (node_id) REFERENCES nodes(id)
);

CREATE INDEX IF NOT EXISTS idx_commands_node_status ON commands(node_id, status);

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  details_json TEXT NOT NULL
);

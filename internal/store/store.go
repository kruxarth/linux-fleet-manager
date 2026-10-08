package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"fleetmgr/internal/agentproto"
	"fleetmgr/internal/auth"
	nodestate "fleetmgr/internal/node"

	_ "modernc.org/sqlite"
)

//go:embed sql/*.sql
var SQLFS embed.FS

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrUnauthorized = errors.New("unauthorized")

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:mem-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	} else {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) Migrate(fsys fs.FS) error {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if _, err := s.DB.Exec(string(b)); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

func labelsJSON(m map[string]string) string {
	if m == nil {
		m = map[string]string{}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func parseLabels(s string) map[string]string {
	m := map[string]string{}
	if s == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func parseArgv(s string) []string {
	var v []string
	_ = json.Unmarshal([]byte(s), &v)
	return v
}

func rfc(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &t
}

type NodeRow struct {
	ID              string
	Hostname        string
	OS              string
	Kernel          string
	Arch            string
	AgentVersion    string
	LabelsJSON      string
	State           string
	TokenHash       string
	NodeExporterURL string
	LastHeartbeat   sql.NullString
	LastError       string
	CreatedAt       string
	UpdatedAt       string
}

func (r NodeRow) ToProto(now time.Time, offlineAfter time.Duration) agentproto.Node {
	created, _ := parseTime(r.CreatedAt)
	updated, _ := parseTime(r.UpdatedAt)
	hb := parseTimePtr(r.LastHeartbeat)
	eff := nodestate.Effective(nodestate.Snapshot{
		State:           r.State,
		LastHeartbeatAt: hb,
		LastError:       r.LastError,
		CreatedAt:       created,
	}, now, offlineAfter)
	return agentproto.Node{
		ID:              r.ID,
		Hostname:        r.Hostname,
		OS:              r.OS,
		Kernel:          r.Kernel,
		Arch:            r.Arch,
		AgentVersion:    r.AgentVersion,
		Labels:          parseLabels(r.LabelsJSON),
		State:           eff,
		NodeExporterURL: r.NodeExporterURL,
		LastHeartbeatAt: hb,
		LastError:       r.LastError,
		CreatedAt:       created,
		UpdatedAt:       updated,
	}
}

func scanNode(sc interface{ Scan(dest ...any) error }) (NodeRow, error) {
	var r NodeRow
	var os, kernel, arch, ver, exp, errStr sql.NullString
	if err := sc.Scan(&r.ID, &r.Hostname, &os, &kernel, &arch, &ver, &r.LabelsJSON, &r.State, &r.TokenHash, &exp, &r.LastHeartbeat, &errStr, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return r, err
	}
	r.OS, r.Kernel, r.Arch, r.AgentVersion = os.String, kernel.String, arch.String, ver.String
	r.NodeExporterURL = exp.String
	r.LastError = errStr.String
	return r, nil
}

const nodeCols = `id, hostname, os, kernel, arch, agent_version, labels_json, state, node_token_hash, node_exporter_url, last_heartbeat_at, last_error, created_at, updated_at`

func (s *Store) ListNodes(ctx context.Context) ([]NodeRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE state != ? ORDER BY hostname`, nodestate.Revoked)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeRow
	for rows.Next() {
		r, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetNode(ctx context.Context, id string) (NodeRow, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id)
	r, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) GetNodeByTokenHash(ctx context.Context, hash string) (NodeRow, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE node_token_hash = ?`, hash)
	r, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) InsertNode(ctx context.Context, r NodeRow) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO nodes (`+nodeCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Hostname, r.OS, r.Kernel, r.Arch, r.AgentVersion, r.LabelsJSON, r.State, r.TokenHash, r.NodeExporterURL, nullStr(r.LastHeartbeat), r.LastError, r.CreatedAt, r.UpdatedAt)
	return err
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func (s *Store) Heartbeat(ctx context.Context, id, hostname, osName, kernel, arch, ver, exporter string, now time.Time) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE nodes SET hostname=?, os=?, kernel=?, arch=?, agent_version=?, node_exporter_url=?, last_heartbeat_at=?, state=?, updated_at=? WHERE id=? AND state != ?`,
		hostname, osName, kernel, arch, ver, exporter, rfc(now), nodestate.Online, rfc(now), id, nodestate.Revoked)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		row, err := s.GetNode(ctx, id)
		if err != nil {
			return err
		}
		if row.State == nodestate.Revoked {
			return ErrUnauthorized
		}
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeNode(ctx context.Context, id string, now time.Time) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE nodes SET state=?, updated_at=? WHERE id=?`, nodestate.Revoked, rfc(now), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetNodeError(ctx context.Context, id, lastErr string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE nodes SET last_error=?, updated_at=? WHERE id=?`, lastErr, rfc(now), id)
	return err
}

func (s *Store) Reap(ctx context.Context, now time.Time, offlineAfter time.Duration) error {
	rows, err := s.ListNodes(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		n := r.ToProto(now, offlineAfter)
		if n.State == r.State {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE nodes SET state=?, updated_at=? WHERE id=? AND state != ?`, n.State, rfc(now), r.ID, nodestate.Revoked); err != nil {
			return err
		}
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE commands SET status=?, finished_at=? WHERE status=? AND started_at IS NOT NULL AND started_at < ?`,
		"timeout", rfc(now), "running", rfc(now.Add(-10*time.Minute)))
	return err
}

func (s *Store) CreateEnrollToken(ctx context.Context, id, hash, labels string, expires time.Time, maxUses int, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO enroll_tokens (id, token_hash, labels_json, expires_at, max_uses, uses, created_at) VALUES (?,?,?,?,?,0,?)`,
		id, hash, labels, rfc(expires), maxUses, rfc(now))
	return err
}

type EnrollToken struct {
	ID         string
	Hash       string
	LabelsJSON string
	ExpiresAt  time.Time
	MaxUses    int
	Uses       int
}

func (s *Store) ConsumeEnrollToken(ctx context.Context, hash string, now time.Time) (EnrollToken, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return EnrollToken{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var t EnrollToken
	var exp, created string
	err = tx.QueryRowContext(ctx, `SELECT id, token_hash, labels_json, expires_at, max_uses, uses FROM enroll_tokens WHERE token_hash=?`, hash).
		Scan(&t.ID, &t.Hash, &t.LabelsJSON, &exp, &t.MaxUses, &t.Uses)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrUnauthorized
	}
	if err != nil {
		return t, err
	}
	t.ExpiresAt, _ = parseTime(exp)
	if now.After(t.ExpiresAt) || t.Uses >= t.MaxUses {
		return t, ErrUnauthorized
	}
	res, err := tx.ExecContext(ctx, `UPDATE enroll_tokens SET uses = uses + 1 WHERE id=? AND uses < max_uses`, t.ID)
	if err != nil {
		return t, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return t, ErrUnauthorized
	}
	_ = created
	if err := tx.Commit(); err != nil {
		return t, err
	}
	t.Uses++
	return t, nil
}

func (s *Store) InsertCommand(ctx context.Context, c agentproto.Command) error {
	argv, _ := json.Marshal(c.Argv)
	var code any
	if c.ExitCode != nil {
		code = *c.ExitCode
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO commands (id, node_id, command_id, argv_json, status, stdout, stderr, exit_code, timeout_seconds, created_at, started_at, finished_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.NodeID, c.CommandID, string(argv), c.Status, c.Stdout, c.Stderr, code, c.TimeoutSeconds, rfc(c.CreatedAt), timePtr(c.StartedAt), timePtr(c.FinishedAt))
	return err
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rfc(*t)
}

func (s *Store) GetCommand(ctx context.Context, id string) (agentproto.Command, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, node_id, command_id, argv_json, status, stdout, stderr, exit_code, timeout_seconds, created_at, started_at, finished_at FROM commands WHERE id=?`, id)
	c, err := scanCommand(row)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func scanCommand(sc interface{ Scan(dest ...any) error }) (agentproto.Command, error) {
	var c agentproto.Command
	var argv, stdout, stderr, created string
	var code sql.NullInt64
	var started, finished sql.NullString
	err := sc.Scan(&c.ID, &c.NodeID, &c.CommandID, &argv, &c.Status, &stdout, &stderr, &code, &c.TimeoutSeconds, &created, &started, &finished)
	if err != nil {
		return c, err
	}
	c.Argv = parseArgv(argv)
	c.Stdout, c.Stderr = stdout, stderr
	if code.Valid {
		v := int(code.Int64)
		c.ExitCode = &v
	}
	c.CreatedAt, _ = parseTime(created)
	c.StartedAt = parseTimePtr(started)
	c.FinishedAt = parseTimePtr(finished)
	return c, nil
}

func (s *Store) ListCommands(ctx context.Context, nodeID string, limit int) ([]agentproto.Command, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, node_id, command_id, argv_json, status, stdout, stderr, exit_code, timeout_seconds, created_at, started_at, finished_at FROM commands`
	args := []any{}
	if nodeID != "" {
		q += ` WHERE node_id=?`
		args = append(args, nodeID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []agentproto.Command
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ClaimJobs(ctx context.Context, nodeID string, now time.Time) ([]agentproto.Job, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var running int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE node_id=? AND status=?`, nodeID, "running").Scan(&running); err != nil {
		return nil, err
	}
	if running > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	row := tx.QueryRowContext(ctx, `SELECT id, command_id, argv_json, timeout_seconds FROM commands WHERE node_id=? AND status=? ORDER BY created_at LIMIT 1`, nodeID, "queued")
	var id, cmdID, argv string
	var timeout int
	err = row.Scan(&id, &cmdID, &argv, &timeout)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status=?, started_at=? WHERE id=?`, "running", rfc(now), id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []agentproto.Job{{
		ID:             id,
		CommandID:      cmdID,
		Argv:           parseArgv(argv),
		TimeoutSeconds: timeout,
	}}, nil
}

func (s *Store) FinishJob(ctx context.Context, id, stdout, stderr string, exitCode int, timedOut bool, now time.Time) error {
	status := "succeeded"
	if timedOut {
		status = "timeout"
	} else if exitCode != 0 {
		status = "failed"
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE commands SET status=?, stdout=?, stderr=?, exit_code=?, finished_at=? WHERE id=?`,
		status, stdout, stderr, exitCode, rfc(now), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Audit(ctx context.Context, actor, action string, details any, now time.Time) error {
	b, _ := json.Marshal(details)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO audit_log (at, actor, action, details_json) VALUES (?,?,?,?)`, rfc(now), actor, action, string(b))
	return err
}

func (s *Store) CountsByState(ctx context.Context, now time.Time, offlineAfter time.Duration) (map[string]int, error) {
	rows, err := s.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int{
		nodestate.Pending:  0,
		nodestate.Online:   0,
		nodestate.Degraded: 0,
		nodestate.Offline:  0,
		nodestate.Revoked:  0,
	}
	for _, r := range rows {
		n := r.ToProto(now, offlineAfter)
		out[n.State]++
	}
	return out, nil
}

func (s *Store) PrometheusSD(ctx context.Context, now time.Time, offlineAfter time.Duration) ([]map[string]any, error) {
	rows, err := s.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, r := range rows {
		n := r.ToProto(now, offlineAfter)
		if strings.TrimSpace(n.NodeExporterURL) == "" {
			continue
		}
		out = append(out, map[string]any{
			"targets": []string{n.NodeExporterURL},
			"labels": map[string]string{
				"node_id":     n.ID,
				"hostname":    n.Hostname,
				"fleet_state": n.State,
			},
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

func LabelsJSON(m map[string]string) string { return labelsJSON(m) }
func HashOK(a, b string) bool               { return auth.EqualHash(a, b) }

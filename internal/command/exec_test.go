package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCatalog(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("commands:\n  - id: uname\n    argv: [uname, -a]\n    timeout_seconds: 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("uname"); !ok {
		t.Fatal("missing uname")
	}
	if _, ok := c.Get("rm"); ok {
		t.Fatal("unknown id should miss")
	}
}

func TestRunEcho(t *testing.T) {
	res := Run(context.Background(), []string{"echo", "hello-fleet"}, 2*time.Second, 1024, t.TempDir())
	if res.ExitCode != 0 {
		t.Fatalf("exit %d stderr %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-fleet") {
		t.Fatalf("stdout %q", res.Stdout)
	}
}

func TestRunTimeout(t *testing.T) {
	res := Run(context.Background(), []string{"sleep", "5"}, 200*time.Millisecond, 1024, t.TempDir())
	if !res.TimedOut {
		t.Fatalf("expected timeout, exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
}

func TestStdoutCap(t *testing.T) {
	res := Run(context.Background(), []string{"sh", "-c", "dd if=/dev/zero bs=100 count=1 2>/dev/null"}, 2*time.Second, 16, t.TempDir())
	if !res.Truncated {
		t.Fatalf("expected truncation, len=%d", len(res.Stdout))
	}
}

func TestAllows(t *testing.T) {
	c, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Allows("uname", []string{"uname", "-a"}) {
		t.Fatal("uname -a should be allowed")
	}
	if c.Allows("uname", []string{"uname", "-a", "; rm -rf /"}) || c.Allows("nope", []string{"x"}) {
		t.Fatal("tampered argv or unknown id must be rejected")
	}
}

package command

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"syscall"
	"time"
)

type ExecResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	TimedOut  bool
	Truncated bool
}

type capWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	remain := w.max - w.buf.Len()
	if remain <= 0 {
		w.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		w.truncated = true
		_, _ = w.buf.Write(p[:remain])
		return len(p), nil
	}
	return w.buf.Write(p)
}

func Run(ctx context.Context, argv []string, timeout time.Duration, maxBytes int, dir string) ExecResult {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	if dir == "" {
		dir = "/tmp"
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=/tmp", "LANG=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	outW := &capWriter{max: maxBytes}
	errW := &capWriter{max: maxBytes}
	cmd.Stdout = outW
	cmd.Stderr = errW

	start := time.Now()
	err := cmd.Run()
	res := ExecResult{
		Stdout:    outW.buf.String(),
		Stderr:    errW.buf.String(),
		Duration:  time.Since(start),
		TimedOut:  ctx.Err() == context.DeadlineExceeded,
		Truncated: outW.truncated || errW.truncated,
	}
	if res.Truncated {
		res.Stdout += "\n[truncated]\n"
	}
	if err == nil {
		return res
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
		return res
	}
	if res.TimedOut {
		res.ExitCode = 124
		if res.Stderr == "" {
			res.Stderr = "command timed out"
		}
		return res
	}
	res.ExitCode = 1
	if res.Stderr == "" {
		res.Stderr = err.Error()
	}
	return res
}

var _ io.Writer = (*capWriter)(nil)

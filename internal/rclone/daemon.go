package rclone

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/engine"
)

// DaemonPort is the local RC port.
const DaemonPort = 5572

// Daemon is the background rclone rcd process.
type Daemon struct {
	cmd    *exec.Cmd
	pg     engine.ProcessGroup
	client *Client
	addr   string
	done   chan error
	logf   func(level, message string)
}

// StartDaemon launches "rclone rcd" on the loopback RC port and waits until it
// answers. Configuration lives in daemon memory; the config path points at the
// OS null device so no rclone.conf is ever created or read.
func StartDaemon(ctx context.Context, binary string, port int, logf func(level, message string)) (*Daemon, error) {
	if binary == "" {
		return nil, apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "rclone"})
	}
	if port == 0 {
		port = DaemonPort
	}
	addr := "127.0.0.1:" + itoa(port)
	args := []string{
		"rcd",
		"--rc-addr=" + addr,
		"--rc-no-auth",
		"--config", nullDevice(),
		"--log-level", "ERROR",
	}
	pg, err := engine.NewProcessGroup()
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = nil
	cmd.Stderr = logWriter{logf: logf}
	if err := pg.Start(cmd); err != nil {
		_ = pg.Terminate()
		return nil, apierr.Wrap(apierr.CodeCloudUnreachable, err, map[string]any{"component": "rclone"})
	}

	d := &Daemon{
		cmd:    cmd,
		pg:     pg,
		client: NewClient("http://" + addr),
		addr:   addr,
		done:   make(chan error, 1),
		logf:   logf,
	}
	go func() { d.done <- pg.WaitFor(cmd) }()

	// Readiness: poll rc/noop instead of trusting a startup banner.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
		err := d.client.Ping(pingCtx)
		cancel()
		if err == nil {
			return d, nil
		}
		select {
		case <-ctx.Done():
			_ = pg.Terminate()
			return nil, apierr.New(apierr.CodeTransferAborted, nil)
		case <-d.done:
			_ = pg.Terminate()
			return nil, apierr.New(apierr.CodeCloudUnreachable, map[string]any{"component": "rclone"})
		case <-time.After(200 * time.Millisecond):
		}
	}
	_ = pg.Terminate()
	return nil, apierr.New(apierr.CodeCloudUnreachable, map[string]any{"component": "rclone", "reason": "startup_timeout"})
}

// Client returns the RC client.
func (d *Daemon) Client() *Client { return d.client }

// Address returns the RC address.
func (d *Daemon) Address() string { return d.addr }

// Ready reports whether the process is still alive.
func (d *Daemon) Ready() bool {
	select {
	case <-d.done:
		return false
	default:
		return true
	}
}

// Stop terminates the daemon process tree.
func (d *Daemon) Stop() {
	if d.pg != nil {
		_ = d.pg.Terminate()
	}
	select {
	case <-d.done:
	case <-time.After(3 * time.Second):
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func nullDevice() string {
	if runtime.GOOS == "windows" {
		return "NUL"
	}
	return "/dev/null"
}

type logWriter struct{ logf func(level, message string) }

func (w logWriter) Write(p []byte) (int, error) {
	if w.logf != nil && len(p) > 0 {
		w.logf("warn", "rclone: "+string(p))
	}
	return len(p), nil
}

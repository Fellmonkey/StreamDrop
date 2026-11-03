package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"streamdrop/internal/apierr"
)

// Transfer modes.
const (
	ModePureStream = "pure_stream"
	ModeRingBuffer = "ring_buffer"
)

// Engine identifiers. They are the same keys as the dependency catalog, so the
// engine picker and the component check share one vocabulary.
const (
	EngineAuto   = "auto"
	EngineCurl   = "curl"
	EngineYtDlp  = "yt-dlp"
	EngineDirect = "direct"
)

// ErrNeedsRestart is returned when a source without range support failed and
// the whole transfer has to be restarted (the cloud object cannot be appended
// to). The transfer manager performs cleanup and retries.
var ErrNeedsRestart = errors.New("engine: transfer restart required")

// StreamSpec describes one transfer task.
type StreamSpec struct {
	ID          string
	URL         string
	Filename    string
	SizeBytes   int64
	ContentType string

	Mode        string
	BufferBytes int
	Engine      string
	Headers     map[string]string

	// RemoteTarget is the fully qualified rclone destination,
	// e.g. "sd_abc:/ISOs/file.iso".
	RemoteTarget  string
	SupportsRange bool

	BandwidthDownBps int64
	BandwidthUpBps   int64
	ChecksumEnabled  bool
	MaxRetries       int

	// Gate pauses the pipeline at the buffer boundary when set.
	Gate *Gate
}

// Hooks receive pipeline events.
type Hooks struct {
	// Log receives diagnostic lines. Level is one of info|warn|error.
	Log func(level, source, message string)
	// OnRetry is called before a retry delay.
	OnRetry func(attempt int, delay time.Duration)
}

func (h Hooks) log(level, source, msg string) {
	if h.Log != nil {
		h.Log(level, source, msg)
	}
}

// Result summarises a finished stream.
type Result struct {
	Bytes     int64
	Confirmed int64
	Sha256    string
	Duration  time.Duration
	Retries   int
}

// Tools locates the external utilities and allows tests to replace them.
type Tools struct {
	Rclone string
	Curl   string
	YtDlp  string

	// Command overrides command construction (used by pipeline tests with
	// fake helper processes). kind is "rclone", "curl" or "yt-dlp".
	Command func(kind string, spec StreamSpec, offset int64, tools Tools) *exec.Cmd
}

// Pipeline orchestrates source -> buffer -> rclone rcat.
type Pipeline struct {
	Tools Tools
	HTTP  *http.Client
	// DefaultMaxRetries applies when StreamSpec.MaxRetries is not set.
	DefaultMaxRetries int
}

// NewPipeline builds a pipeline with a sane HTTP client.
func NewPipeline(tools Tools) *Pipeline {
	return &Pipeline{
		Tools:             tools,
		HTTP:              &http.Client{Timeout: 0},
		DefaultMaxRetries: 3,
	}
}

// EngineFor resolves the effective engine for a spec.
func (p *Pipeline) EngineFor(spec StreamSpec) (string, error) {
	switch spec.Engine {
	case "", EngineAuto:
		if p.Tools.Curl != "" {
			return EngineCurl, nil
		}
		return EngineDirect, nil
	case EngineCurl:
		if p.Tools.Curl == "" {
			return "", apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "curl"})
		}
		return EngineCurl, nil
	case EngineYtDlp:
		if p.Tools.YtDlp == "" {
			return "", apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "yt-dlp"})
		}
		return EngineYtDlp, nil
	case EngineDirect:
		return EngineDirect, nil
	default:
		return "", apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "engine"})
	}
}

// EnsureReady checks that the components this spec needs are present.
func (p *Pipeline) EnsureReady(spec StreamSpec) error {
	if p.Tools.Rclone == "" {
		return apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "rclone"})
	}
	_, err := p.EngineFor(spec)
	return err
}

// Run streams one task until completion, exhaustion of retries or abort.
func (p *Pipeline) Run(ctx context.Context, spec StreamSpec, state *TaskState, hooks Hooks) (Result, error) {
	started := time.Now()
	if state == nil {
		state = NewTaskState("running", 0)
	}
	engine, err := p.EngineFor(spec)
	if err != nil {
		return Result{}, err
	}
	if p.Tools.Rclone == "" {
		return Result{}, apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "rclone"})
	}

	gate := spec.Gate
	if gate == nil {
		gate = NewGate()
	}

	buf := NewStreamBuffer(spec.Mode, spec.BufferBytes)
	state.SetBufferSize(int64(buf.Size()))

	pg, err := NewProcessGroup()
	if err != nil {
		return Result{}, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	defer func() { _ = pg.Terminate() }()

	var hasher hash.Hash
	if spec.ChecksumEnabled {
		hasher = sha256.New()
	}

	// Uploader: rclone rcat reads the buffer from stdin. It is started first so
	// the download can flow as soon as the first bytes arrive.
	upload := p.buildUpload(spec)
	upload.Stdin = buf
	upload.Stderr = newLineWriter("rclone", func(level, source, message string) {
		hooks.log(level, source, message)
		if n, ok := parseRcloneStats(message); ok {
			if n > state.Confirmed() {
				state.SetConfirmed(n)
			}
		}
	})
	if err := pg.Start(upload); err != nil {
		buf.CloseWithCause(err)
		return Result{}, apierr.Wrap(apierr.CodeCloudWriteFailed, err, map[string]any{"remote": spec.RemoteTarget})
	}

	uploadDone := make(chan error, 1)
	go func() {
		waitErr := pg.WaitFor(upload)
		// If the uploader dies while the download is still writing, the writer
		// would block forever on a full buffer. Closing unblocks it.
		_ = buf.CloseWithCause(apierr.New(apierr.CodeCloudWriteFailed, nil))
		uploadDone <- waitErr
	}()

	writer := &flowWriter{
		ctx:   ctx,
		gate:  gate,
		buf:   buf,
		state: state,
		hash:  hasher,
	}

	// Abort watcher: cancelling the task context must unblock a writer that is
	// stuck on a full buffer and stop the subprocesses promptly. Without it an
	// abort while rclone is stalled would hang until the source times out.
	abortStop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = buf.CloseWithCause(apierr.New(apierr.CodeTransferAborted, nil))
			_ = pg.Terminate()
		case <-abortStop:
		}
	}()
	defer close(abortStop)

	// Live buffer occupancy (writes alone would miss the drain side).
	samplerStop := make(chan struct{})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-samplerStop:
				return
			case <-t.C:
				state.SetBufferUsed(int64(buf.UsedBytes()))
			}
		}
	}()
	defer close(samplerStop)

	maxRetries := spec.MaxRetries
	if maxRetries <= 0 {
		maxRetries = p.DefaultMaxRetries
	}

	attempt := 0
	offset := int64(0)
	var streamErr error

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			streamErr = apierr.New(apierr.CodeTransferAborted, nil)
			break
		}
		dl, startErr := p.startDownload(ctx, pg, spec, engine, offset, writer, hooks)
		if startErr != nil {
			streamErr = startErr
			break
		}
		waitErr := dl.Wait()
		if waitErr == nil {
			streamErr = nil
			break
		}
		if ctx.Err() != nil {
			streamErr = apierr.New(apierr.CodeTransferAborted, nil)
			break
		}

		attempt++
		state.SetRetries(int32(attempt))

		if attempt > maxRetries {
			streamErr = classifySourceError(waitErr, writer.written())
			break
		}

		delay := retryDelay(attempt)
		state.SetStatus("retrying")
		hooks.log("warn", "engine", fmt.Sprintf("source interrupted, retry %d/%d in %s", attempt, maxRetries, delay))
		if hooks.OnRetry != nil {
			hooks.OnRetry(attempt, delay)
		}
		if !sleepCtx(ctx, delay) {
			streamErr = apierr.New(apierr.CodeTransferAborted, nil)
			break
		}

		if !spec.SupportsRange {
			// Appending is impossible, so the cloud object is discarded and the
			// transfer restarts from zero. Cleanup and the next attempt belong
			// to the transfer manager.
			_ = buf.CloseWithCause(ErrNeedsRestart)
			select {
			case <-uploadDone:
			case <-time.After(10 * time.Second):
				_ = pg.Terminate()
				<-uploadDone
			}
			return Result{
				Bytes:    writer.written(),
				Retries:  attempt,
				Duration: time.Since(started),
			}, ErrNeedsRestart
		}

		offset = writer.written()
		state.SetStatus("running")
		hooks.log("info", "engine", fmt.Sprintf("resuming from byte %d", offset))
	}

	if streamErr != nil {
		_ = buf.CloseWithCause(streamErr)
		select {
		case <-uploadDone:
		case <-time.After(10 * time.Second):
			_ = pg.Terminate()
			<-uploadDone
		}
		state.SetStatus("failed")
		return Result{
			Bytes:     writer.written(),
			Confirmed: state.Confirmed(),
			Retries:   attempt,
			Duration:  time.Since(started),
		}, streamErr
	}

	// Normal completion: EOF reaches rclone, which flushes the final chunk.
	_ = buf.CloseWithCause(nil)
	uploadErr := <-uploadDone
	if uploadErr != nil {
		state.SetStatus("failed")
		return Result{
			Bytes:     writer.written(),
			Confirmed: state.Confirmed(),
			Retries:   attempt,
			Duration:  time.Since(started),
		}, apierr.Wrap(apierr.CodeCloudWriteFailed, uploadErr, map[string]any{"remote": spec.RemoteTarget})
	}

	state.SetConfirmed(writer.written())
	state.SetStatus("completed")

	res := Result{
		Bytes:     writer.written(),
		Confirmed: writer.written(),
		Retries:   attempt,
		Duration:  time.Since(started),
	}
	if hasher != nil {
		res.Sha256 = hex.EncodeToString(hasher.Sum(nil))
	}
	return res, nil
}

// downloader abstracts a source that feeds the buffer.
type downloader interface {
	Wait() error
}

type procDownloader struct {
	pg  ProcessGroup
	cmd *exec.Cmd
}

func (d procDownloader) Wait() error { return d.pg.WaitFor(d.cmd) }

type directDownloader struct{ done chan error }

func (d directDownloader) Wait() error { return <-d.done }

func (p *Pipeline) startDownload(ctx context.Context, pg ProcessGroup, spec StreamSpec, engine string, offset int64, w io.Writer, hooks Hooks) (downloader, error) {
	if engine == EngineDirect {
		d := &directDownloader{done: make(chan error, 1)}
		go func() {
			d.done <- p.fetch(ctx, spec, offset, w)
		}()
		return d, nil
	}

	cmd := p.buildDownload(engine, spec, offset)
	cmd.Stdout = w
	cmd.Stderr = newLineWriter(engine, func(level, source, message string) {
		hooks.log(level, source, message)
	})
	if err := pg.Start(cmd); err != nil {
		if ctx.Err() != nil {
			return nil, apierr.New(apierr.CodeTransferAborted, nil)
		}
		return nil, apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"engine": engine})
	}
	return procDownloader{pg: pg, cmd: cmd}, nil
}

// fetch is the built-in Go source used when curl is not installed. It keeps
// "auto" working on platforms where the portable archive does not ship curl.
func (p *Pipeline) fetch(ctx context.Context, spec StreamSpec, offset int64, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
	}
	for k, v := range spec.Headers {
		req.Header.Set(k, v)
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return apierr.New(apierr.CodeTransferAborted, nil)
		}
		return apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"url": spec.URL})
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return apierr.New(apierr.CodeAuthRequired, nil)
	case resp.StatusCode == http.StatusForbidden:
		return apierr.New(apierr.CodeAuthRejected, nil)
	case resp.StatusCode >= 400:
		return apierr.New(apierr.CodeSourceInterrupted, map[string]any{"status_code": resp.StatusCode})
	}

	buf := make([]byte, 256<<10)
	if _, err := io.CopyBuffer(w, resp.Body, buf); err != nil {
		if ctx.Err() != nil {
			return apierr.New(apierr.CodeTransferAborted, nil)
		}
		return apierr.Wrap(apierr.CodeSourceInterrupted, err, nil)
	}
	return nil
}

func (p *Pipeline) buildUpload(spec StreamSpec) *exec.Cmd {
	if p.Tools.Command != nil {
		return p.Tools.Command("rclone", spec, 0, p.Tools)
	}
	args := []string{
		"rcat", spec.RemoteTarget,
		"--config", nullDevice(),
		"--stats", "500ms",
		"--stats-one-line",
		"--log-level", "INFO",
		"--stats-log-level", "NOTICE",
	}
	if spec.BandwidthUpBps > 0 {
		args = append(args, "--bwlimit", strconv.FormatInt(spec.BandwidthUpBps, 10))
	}
	return exec.Command(p.Tools.Rclone, args...)
}

func (p *Pipeline) buildDownload(engine string, spec StreamSpec, offset int64) *exec.Cmd {
	if p.Tools.Command != nil {
		return p.Tools.Command(engine, spec, offset, p.Tools)
	}
	switch engine {
	case EngineYtDlp:
		args := []string{"--no-warnings", "--no-part", "--no-progress", "-o", "-"}
		if ua := spec.Headers["User-Agent"]; ua != "" {
			args = append(args, "--user-agent", ua)
		}
		for _, k := range sortedHeaderKeys(spec.Headers) {
			if strings.EqualFold(k, "User-Agent") {
				continue
			}
			args = append(args, "--add-header", k+": "+spec.Headers[k])
		}
		if spec.BandwidthDownBps > 0 {
			args = append(args, "--limit-rate", strconv.FormatInt(spec.BandwidthDownBps, 10))
		}
		args = append(args, spec.URL)
		return exec.Command(p.Tools.YtDlp, args...)

	default: // curl
		args := []string{
			"-L", "--fail", "--silent", "--show-error", "--globoff",
			"--max-redirs", "10", "--connect-timeout", "30",
			"-o", "-",
		}
		for _, k := range sortedHeaderKeys(spec.Headers) {
			args = append(args, "-H", k+": "+spec.Headers[k])
		}
		if spec.BandwidthDownBps > 0 {
			args = append(args, "--limit-rate", strconv.FormatInt(spec.BandwidthDownBps, 10))
		}
		if offset > 0 {
			args = append(args, "--range", strconv.FormatInt(offset, 10)+"-")
		}
		args = append(args, spec.URL)
		return exec.Command(p.Tools.Curl, args...)
	}
}

func sortedHeaderKeys(h map[string]string) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func nullDevice() string {
	if runtime.GOOS == "windows" {
		return "NUL"
	}
	return "/dev/null"
}

func retryDelay(attempt int) time.Duration {
	d := time.Second << (attempt - 1) // 1s, 2s, 4s, ...
	if d > 30*time.Second || d <= 0 {
		d = 30 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// classifySourceError maps a failed downloader to a machine-readable code.
func classifySourceError(err error, downloaded int64) *apierr.Error {
	if downloaded > 0 {
		return apierr.Wrap(apierr.CodeSourceInterrupted, err, nil)
	}
	return apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
}

// flowWriter writes source bytes into the buffer, honouring the pause gate and
// updating counters and the SHA-256 digest in-flight (no second disk read).
type flowWriter struct {
	ctx   context.Context
	gate  *Gate
	buf   StreamBuffer
	state *TaskState
	hash  hash.Hash
	total atomic.Int64
}

func (w *flowWriter) Write(p []byte) (int, error) {
	if err := w.gate.Wait(w.ctx); err != nil {
		return 0, err
	}
	n, err := w.buf.Write(p)
	if n > 0 {
		w.total.Add(int64(n))
		w.state.AddDownloaded(int64(n))
		w.state.SetBufferUsed(int64(w.buf.UsedBytes()))
		if w.hash != nil {
			_, _ = w.hash.Write(p[:n])
		}
	}
	return n, err
}

func (w *flowWriter) written() int64 { return w.total.Load() }

// lineWriter turns a subprocess stderr stream into whole log lines.
type lineWriter struct {
	mu     sync.Mutex
	buf    []byte
	source string
	onLine func(level, source, message string)
}

func newLineWriter(source string, onLine func(level, source, message string)) *lineWriter {
	return &lineWriter{source: source, onLine: onLine}
}

func (l *lineWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	for {
		idx := strings.IndexByte(string(l.buf), '\n')
		if idx < 0 {
			break
		}
		line := string(l.buf[:idx])
		l.buf = l.buf[idx+1:]
		l.emit(line)
	}
	if len(l.buf) > 64<<10 {
		l.emit(string(l.buf))
		l.buf = nil
	}
	return len(p), nil
}

func (l *lineWriter) emit(line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	level := "info"
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		level = "error"
	} else if strings.Contains(lower, "warning") || strings.Contains(lower, "retry") {
		level = "warn"
	}
	if l.onLine != nil {
		l.onLine(level, l.source, line)
	}
}

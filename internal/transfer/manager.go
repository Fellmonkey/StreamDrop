// Package transfer owns the lifecycle of tasks: creation with server-side
// pre-flight inspection, queue hand-off, pipeline execution, retries, fail-safe
// cleanup, persistence and notifications.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/auth"
	"streamdrop/internal/engine"
	"streamdrop/internal/logging"
	"streamdrop/internal/notify"
	"streamdrop/internal/queue"
	"streamdrop/internal/rclone"
	"streamdrop/internal/store"
)

// Broadcaster receives realtime events.
type Broadcaster interface {
	Broadcast(event string, data any)
}

// Settings is the runtime settings snapshot the manager needs.
type Settings struct {
	RetryMaxAttempts   int
	ChecksumEnabled    bool
	BandwidthDownBps   int64
	BandwidthUpBps     int64
	DefaultMode        string
	DefaultBufferBytes int64
}

// Manager coordinates tasks.
type Manager struct {
	Store       *store.Store
	Pipeline    *engine.Pipeline
	Queue       *queue.Manager
	Hub         Broadcaster
	Cleaner     *rclone.Cleaner
	Provisioner *rclone.Provisioner
	Notifier    *notify.Notifier
	Settings    func() Settings
	Tools       func() engine.Tools
	Log         *logging.Logger
	Client      *http.Client

	mu        sync.Mutex
	tasks     map[string]*runningTask
	batch     batchCounter
	priority  int
	inspector *http.Client
}

type batchCounter struct {
	completed int
	failed    int
	bytes     int64
	start     time.Time
}

// NewManager builds a transfer manager.
func NewManager(st *store.Store, pipeline *engine.Pipeline, q *queue.Manager) *Manager {
	return &Manager{
		Store:     st,
		Pipeline:  pipeline,
		Queue:     q,
		Client:    engine.NewInspectClient(),
		tasks:     map[string]*runningTask{},
		priority:  int(time.Now().Unix()),
		inspector: engine.NewInspectClient(),
	}
}

// ---------------------------------------------------------------------------
// Task creation
// ---------------------------------------------------------------------------

// CreateRequest is the POST /api/v1/transfers body. A single URL or a batch
// (urls) is accepted.
type CreateRequest struct {
	SourceURL        string   `json:"source_url"`
	URLs             []string `json:"urls"`
	FilenameOverride string   `json:"filename_override"`
	RemoteID         string   `json:"remote_id"`
	RemotePath       string   `json:"remote_path"`
	Engine           string   `json:"engine"`
	AuthProfileID    string   `json:"auth_profile_id"`
	Mode             string   `json:"mode"`
	BufferBytes      int64    `json:"buffer_size_bytes"`
	BandwidthDownBps *int64   `json:"bandwidth_down_bps"`
	BandwidthUpBps   *int64   `json:"bandwidth_up_bps"`
	FileSize         int64    `json:"file_size"`
}

// Create inspects every URL, enforces the HTML rule and queues the tasks. A
// failing URL rejects the whole request so a batch never half-starts.
func (m *Manager) Create(ctx context.Context, req CreateRequest) ([]store.Transfer, error) {
	urls := req.URLs
	if len(urls) == 0 && req.SourceURL != "" {
		urls = []string{req.SourceURL}
	}
	if len(urls) == 0 {
		return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "source_url"})
	}
	if req.RemoteID == "" {
		return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "remote_id"})
	}
	remote, err := m.Store.GetRemote(req.RemoteID)
	if err != nil {
		return nil, err
	}
	settings := Settings{}
	if m.Settings != nil {
		settings = m.Settings()
	}

	mode := req.Mode
	if mode == "" {
		mode = settings.DefaultMode
	}
	if mode != engine.ModePureStream && mode != engine.ModeRingBuffer {
		return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "mode"})
	}
	bufferBytes := req.BufferBytes
	if mode == engine.ModePureStream {
		bufferBytes = 0
	} else {
		if bufferBytes == 0 {
			bufferBytes = settings.DefaultBufferBytes
		}
		if bufferBytes == 0 {
			bufferBytes = 128 << 20
		}
		if !validBufferSize(bufferBytes) {
			return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "buffer_size_bytes"})
		}
	}

	engineID := req.Engine
	if engineID == "" {
		engineID = engine.EngineAuto
	}
	switch engineID {
	case engine.EngineAuto, engine.EngineCurl, engine.EngineYtDlp:
	default:
		return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "engine"})
	}

	headers, err := m.resolveHeaders(req.AuthProfileID, firstURL(urls))
	if err != nil {
		return nil, err
	}

	// Pre-flight for every URL before anything is written to the queue.
	type plan struct {
		source    string
		meta      *engine.FileMetadata
		filename  string
		sizeBytes int64
	}
	plans := make([]plan, 0, len(urls))
	for i, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "source_url", "index": i})
		}
		taskHeaders := headers
		if i > 0 && req.AuthProfileID != "" {
			taskHeaders, err = m.resolveHeaders(req.AuthProfileID, raw)
			if err != nil {
				return nil, err
			}
		}

		inspectCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		meta, inspectErr := engine.Inspect(inspectCtx, m.inspector, raw, taskHeaders)
		cancel()
		if inspectErr != nil {
			// yt-dlp handles pages whose headers cannot be probed; other
			// engines need a verified file before a transfer may start.
			if engineID != engine.EngineYtDlp {
				return nil, inspectErr
			}
			meta = &engine.FileMetadata{OriginalURL: raw, ResolvedURL: raw, EngineHint: "yt-dlp"}
		}

		if meta.IsHTML && engineID != engine.EngineYtDlp {
			return nil, apierr.New(apierr.CodeInspectHTMLSource, map[string]any{"engine": engineID})
		}

		filename := meta.Filename
		if len(urls) == 1 && req.FilenameOverride != "" {
			filename = req.FilenameOverride
		}
		filename = sanitizeFilename(filename)
		if filename == "" {
			filename = sanitizeFilename(engineFilenameFallback(raw))
		}
		size := meta.SizeBytes
		if len(urls) == 1 && req.FileSize > 0 {
			size = req.FileSize
		}
		plans = append(plans, plan{source: raw, meta: meta, filename: filename, sizeBytes: size})
	}
	if len(plans) == 0 {
		return nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "source_url"})
	}

	bwDown := settings.BandwidthDownBps
	if req.BandwidthDownBps != nil {
		bwDown = *req.BandwidthDownBps
	}
	bwUp := settings.BandwidthUpBps
	if req.BandwidthUpBps != nil {
		bwUp = *req.BandwidthUpBps
	}

	remotePath := rclone.JoinPath(req.RemotePath, "")
	created := make([]store.Transfer, 0, len(plans))
	for _, p := range plans {
		id := newID()
		t := store.Transfer{
			ID:            id,
			SourceURL:     p.source,
			ResolvedName:  p.filename,
			FileSize:      p.sizeBytes,
			RemoteID:      &remote.ID,
			RemoteLabel:   remote.Name,
			RemotePath:    remotePath,
			Engine:        engineID,
			Mode:          mode,
			BufferSize:    bufferBytes,
			Status:        queue.StatusQueued,
			Priority:      m.nextPriority(),
			BandwidthDown: bwDown,
			BandwidthUp:   bwUp,
		}
		if req.AuthProfileID != "" {
			profileID := req.AuthProfileID
			t.AuthProfileID = &profileID
		}
		if err := m.Store.CreateTransfer(t); err != nil {
			return nil, apierr.Wrap(apierr.CodeInternalError, err, nil)
		}
		created = append(created, t)

		task := newRunningTask(m, t, p.meta.SupportsRange)
		m.track(task)
		m.Queue.Submit(task)
	}
	m.broadcastQueue(created[0].ID)
	return created, nil
}

func firstURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func validBufferSize(n int64) bool {
	for _, mib := range engine.AllowedBufferSizes {
		if n == int64(mib)<<20 {
			return true
		}
	}
	return false
}

func (m *Manager) nextPriority() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.priority++
	return m.priority
}

func (m *Manager) track(t *runningTask) {
	m.mu.Lock()
	m.tasks[t.id] = t
	m.mu.Unlock()
}

func (m *Manager) untrack(id string) {
	m.mu.Lock()
	delete(m.tasks, id)
	m.mu.Unlock()
}

func (m *Manager) task(id string) *runningTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[id]
}

// ---------------------------------------------------------------------------
// Execution
// ---------------------------------------------------------------------------

type runningTask struct {
	mgr           *Manager
	id            string
	gate          *engine.Gate
	state         *engine.TaskState
	cancel        context.CancelFunc
	totalSize     int64
	mode          string
	engineID      string
	supportsRange bool

	mu          sync.Mutex
	status      string
	paused      bool
	startedAt   time.Time
	lastAt      time.Time
	lastDown    int64
	lastConfirm int64
	downBps     int64
	upBps       int64
}

func newRunningTask(mgr *Manager, t store.Transfer, supportsRange bool) *runningTask {
	return &runningTask{
		mgr:           mgr,
		id:            t.ID,
		gate:          engine.NewGate(),
		state:         engine.NewTaskState(queue.StatusQueued, t.BufferSize),
		totalSize:     t.FileSize,
		mode:          t.Mode,
		engineID:      t.Engine,
		supportsRange: supportsRange,
		status:        queue.StatusQueued,
	}
}

// ID implements queue.Task.
func (t *runningTask) ID() string { return t.id }

// Start implements queue.Task.
func (t *runningTask) Start(ctx context.Context) {
	if t.mgr != nil {
		t.mgr.execute(ctx, t)
	}
}

// Pause freezes the pipeline at the buffer boundary.
func (t *runningTask) Pause() {
	t.gate.Pause()
	t.mu.Lock()
	t.paused = true
	t.status = queue.StatusPaused
	t.mu.Unlock()
}

// Resume continues the pipeline.
func (t *runningTask) Resume() {
	t.gate.Resume()
	t.mu.Lock()
	t.paused = false
	t.status = queue.StatusRunning
	t.mu.Unlock()
}

// Abort cancels the task.
func (t *runningTask) Abort() {
	t.mu.Lock()
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	t.gate.Release()
}

// setCancel stores the per-task cancel function.
func (t *runningTask) setCancel(cancel context.CancelFunc) {
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()
}

func (t *runningTask) setStatus(s string) {
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
	t.state.SetStatus(s)
}

func (t *runningTask) Status() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// execute runs one task to completion.
func (m *Manager) execute(ctx context.Context, t *runningTask) {
	tr, err := m.Store.GetTransfer(t.id)
	if err != nil {
		t.setStatus(queue.StatusFailed)
		m.untrack(t.id)
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	t.setCancel(cancel)
	defer cancel()

	now := time.Now().UTC()
	tr.Status = queue.StatusRunning
	tr.StartedAt = &now
	_ = m.Store.UpdateTransfer(tr)
	t.setStatus(queue.StatusRunning)
	t.mu.Lock()
	t.startedAt = time.Now()
	t.lastAt = time.Now()
	t.mu.Unlock()
	m.Queue.SetStatus(t.id, queue.StatusRunning)
	m.broadcastQueue(t.id)
	m.logTransfer(t.id, "info", "engine", "transfer started")

	settings := Settings{}
	if m.Settings != nil {
		settings = m.Settings()
	}
	// Resolve tools per task so a component installed after startup is picked
	// up without restarting the application.
	if m.Tools != nil && m.Pipeline != nil {
		m.Pipeline.Tools = m.Tools()
	}

	headers, err := m.resolveHeaders(derefString(tr.AuthProfileID), tr.SourceURL)
	if err != nil {
		m.finishFailure(ctx, t, tr, err)
		return
	}

	remote, err := m.EnsureRemote(ctx, derefString(tr.RemoteID))
	if err != nil {
		m.finishFailure(ctx, t, tr, err)
		return
	}

	// The daemon keeps its rclone config in memory only; the uploader
	// subprocess needs a materialised config file containing the destination
	// remote section.
	remoteConfigPath := ""
	if m.Provisioner != nil {
		if ini, ierr := m.Provisioner.RC.RemoteConfig(ctx, remote.RcloneName); ierr == nil {
			if f, ferr := os.CreateTemp("", "streamdrop-rclone-*.conf"); ferr == nil {
				_ = os.Chmod(f.Name(), 0o600)
				_, _ = f.WriteString(ini)
				_ = f.Close()
				remoteConfigPath = f.Name()
				defer os.Remove(f.Name())
			}
		}
	}

	filename := sanitizeFilename(tr.FileName())
	target := rclone.JoinPath(tr.RemotePath, filename)
	remoteTarget := remote.RcloneName + ":" + target

	maxAttempts := settings.RetryMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	attemptsUsed := 0
	var res engine.Result
	var runErr error

	for {
		spec := engine.StreamSpec{
			ID:               tr.ID,
			URL:              tr.SourceURL,
			Filename:         filename,
			SizeBytes:        tr.FileSize,
			Mode:             tr.Mode,
			BufferBytes:      int(tr.BufferSize),
			Engine:           tr.Engine,
			Headers:          headers.Map(),
			RemoteTarget:     remoteTarget,
			ConfigPath:       remoteConfigPath,
			SupportsRange:    t.supportsRange,
			BandwidthDownBps: tr.BandwidthDown,
			BandwidthUpBps:   tr.BandwidthUp,
			ChecksumEnabled:  settings.ChecksumEnabled,
			MaxRetries:       maxAttempts,
			Gate:             t.gate,
		}
		state := t.state
		hooks := engine.Hooks{
			Log: func(level, source, message string) {
				m.logTransfer(tr.ID, level, source, message)
			},
			OnRetry: func(attempt int, delay time.Duration) {
				t.setStatus(queue.StatusRetrying)
				m.Queue.SetStatus(tr.ID, queue.StatusRetrying)
				tr.Status = queue.StatusRetrying
				tr.RetryAttempts = attemptsUsed + attempt
				_ = m.Store.UpdateTransfer(tr)
				m.logTransfer(tr.ID, "warn", "engine", fmt.Sprintf("retry %d scheduled in %s", attempt, delay))
			},
		}
		res, runErr = m.Pipeline.Run(ctx, spec, state, hooks)
		attemptsUsed += res.Retries
		if !errors.Is(runErr, engine.ErrNeedsRestart) {
			break
		}
		attemptsUsed++
		if attemptsUsed >= maxAttempts {
			runErr = apierr.New(apierr.CodeSourceInterrupted, map[string]any{"attempts": attemptsUsed})
			break
		}
		// The source cannot resume, so the partially written object must go
		// before a fresh attempt; otherwise the cloud object would be corrupt.
		if m.Cleaner != nil {
			if err := m.Cleaner.DeleteObject(ctx, remote.RcloneName, tr.RemotePath, filename); err != nil {
				m.logTransfer(tr.ID, "warn", "cleanup", fmt.Sprintf("cleanup before restart failed: %v", err))
			}
		}
		m.logTransfer(tr.ID, "warn", "engine", "source does not support resume, restarting transfer")
		t.state = engine.NewTaskState(queue.StatusRunning, tr.BufferSize)
		t.state.SetRetries(int32(attemptsUsed))
	}

	if runErr != nil {
		m.finishFailure(ctx, t, tr, runErr)
		return
	}

	completedAt := time.Now().UTC()
	tr.Status = queue.StatusCompleted
	tr.BytesTransferred = res.Bytes
	tr.BytesConfirmed = res.Confirmed
	tr.SHA256 = res.Sha256
	tr.RetryAttempts = attemptsUsed
	tr.CompletedAt = &completedAt
	tr.ErrorKind = ""
	tr.ErrorMessage = ""
	if res.Duration > 0 {
		tr.AverageSpeed = int64(float64(res.Bytes) / res.Duration.Seconds())
	}
	_ = m.Store.UpdateTransfer(tr)
	if res.Bytes > 0 {
		_ = m.Store.AddSavedBytes(res.Bytes)
	}
	t.setStatus(queue.StatusCompleted)
	m.untrack(t.id)
	m.Queue.SetStatus(tr.ID, queue.StatusCompleted)
	m.logTransfer(tr.ID, "info", "engine", fmt.Sprintf("transfer completed: %d bytes in %s", res.Bytes, res.Duration.Round(time.Millisecond)))

	m.mu.Lock()
	m.batch.completed++
	m.batch.bytes += res.Bytes
	if m.batch.start.IsZero() {
		m.batch.start = time.Now()
	}
	m.mu.Unlock()

	m.broadcastQueue(tr.ID)
	m.flushBatchIfIdle(ctx)
}

// ---------------------------------------------------------------------------
// Failure handling
// ---------------------------------------------------------------------------

func (m *Manager) finishFailure(ctx context.Context, t *runningTask, tr store.Transfer, err error) {
	apiErr := apierr.From(err)
	status := queue.StatusFailed
	if apiErr.Code == apierr.CodeTransferAborted || errors.Is(ctx.Err(), context.Canceled) {
		status = queue.StatusAborted
	}

	// Fail-safe cleanup: never leave a partial object consuming quota.
	remote, remoteErr := m.Store.GetRemote(derefString(tr.RemoteID))
	if remoteErr == nil && m.Cleaner != nil {
		if cleanupErr := m.Cleaner.DeleteObject(context.WithoutCancel(ctx), remote.RcloneName, tr.RemotePath, sanitizeFilename(tr.FileName())); cleanupErr != nil {
			m.logTransfer(tr.ID, "warn", "cleanup", fmt.Sprintf("cleanup failed: %v", apiErr.Details))
		} else {
			m.logTransfer(tr.ID, "info", "cleanup", "partial object removed from cloud")
		}
	}
	if apiErr.Code == apierr.CodeCloudCleanupFailed {
		// Already surfaced by the cleaner; keep the task failure code as is.
		_ = apiErr
	}

	completedAt := time.Now().UTC()
	tr.Status = status
	tr.BytesTransferred = t.state.Downloaded()
	tr.BytesConfirmed = t.state.Confirmed()
	tr.CompletedAt = &completedAt
	tr.ErrorKind = string(apiErr.Kind)
	tr.ErrorMessage = string(apiErr.Code)
	if tr.ErrorMessage == "" {
		tr.ErrorMessage = "internal_error"
	}
	tr.RetryAttempts = int(t.state.Retries())
	_ = m.Store.UpdateTransfer(tr)

	t.setStatus(status)
	m.untrack(t.id)
	m.Queue.SetStatus(tr.ID, status)
	m.logTransfer(tr.ID, "error", "engine", fmt.Sprintf("transfer %s: %s", status, apiErr.Code))

	if status != queue.StatusAborted && m.Notifier != nil {
		m.Notifier.Send(context.WithoutCancel(ctx), notify.Event{
			Kind:       notify.KindTransferFailed,
			Level:      "error",
			TransferID: tr.ID,
			FilesCount: 1,
			Code:       string(apiErr.Code),
		})
	}
	m.mu.Lock()
	m.batch.failed++
	if m.batch.start.IsZero() {
		m.batch.start = time.Now()
	}
	m.mu.Unlock()

	m.broadcastQueue(tr.ID)
	m.flushBatchIfIdle(ctx)
}

func (m *Manager) flushBatchIfIdle(ctx context.Context) {
	m.mu.Lock()
	active := len(m.tasks)
	b := m.batch
	m.mu.Unlock()
	view := m.Queue.View()
	if active > 0 || view.Pending > 0 {
		return
	}
	if b.completed == 0 && b.failed == 0 {
		return
	}
	m.mu.Lock()
	m.batch = batchCounter{}
	m.mu.Unlock()

	if m.Notifier != nil {
		duration := int64(0)
		if !b.start.IsZero() {
			duration = int64(time.Since(b.start).Seconds())
		}
		level := "success"
		if b.failed > 0 {
			level = "warn"
		}
		go m.Notifier.Send(context.WithoutCancel(ctx), notify.Event{
			Kind:            notify.KindBatchCompleted,
			Level:           level,
			FilesCount:      b.completed + b.failed,
			FailedCount:     b.failed,
			Bytes:           b.bytes,
			DurationSeconds: duration,
		})
	}
}

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

// Pause pauses one task.
func (m *Manager) Pause(id string) error {
	tr, err := m.Store.GetTransfer(id)
	if err != nil {
		return err
	}
	if tr.Status == queue.StatusCompleted || tr.Status == queue.StatusFailed || tr.Status == queue.StatusAborted {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "status"})
	}
	t := m.task(id)
	if t == nil {
		// Still queued and not started: mark paused in DB only.
		tr.Status = queue.StatusPaused
		_ = m.Store.UpdateTransfer(tr)
		m.Queue.SetStatus(id, queue.StatusPaused)
		m.broadcastQueue(id)
		return nil
	}
	t.Pause()
	tr.Status = queue.StatusPaused
	_ = m.Store.UpdateTransfer(tr)
	m.Queue.SetStatus(id, queue.StatusPaused)
	m.logTransfer(id, "info", "engine", "transfer paused")
	m.broadcastQueue(id)
	return nil
}

// Resume resumes one task.
func (m *Manager) Resume(id string) error {
	tr, err := m.Store.GetTransfer(id)
	if err != nil {
		return err
	}
	t := m.task(id)
	if t == nil {
		tr.Status = queue.StatusQueued
		_ = m.Store.UpdateTransfer(tr)
		m.Queue.SetStatus(id, queue.StatusQueued)
		m.broadcastQueue(id)
		return nil
	}
	t.Resume()
	tr.Status = queue.StatusRunning
	_ = m.Store.UpdateTransfer(tr)
	m.Queue.SetStatus(id, queue.StatusRunning)
	m.logTransfer(id, "info", "engine", "transfer resumed")
	m.broadcastQueue(id)
	return nil
}

// Abort interrupts a task and triggers cleanup.
func (m *Manager) Abort(id string) error {
	tr, err := m.Store.GetTransfer(id)
	if err != nil {
		return err
	}
	t := m.task(id)
	if t == nil {
		tr.Status = queue.StatusAborted
		completed := time.Now().UTC()
		tr.CompletedAt = &completed
		tr.ErrorKind = string(apierr.KindAborted)
		tr.ErrorMessage = string(apierr.CodeTransferAborted)
		_ = m.Store.UpdateTransfer(tr)
		m.Queue.Remove(id)
		m.broadcastQueue(id)
		return nil
	}
	m.logTransfer(id, "info", "engine", "abort requested")
	t.Abort()
	return nil
}

// Delete removes a finished task from history.
func (m *Manager) Delete(id string) error {
	if m.task(id) != nil {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "status", "reason": "active"})
	}
	if err := m.Store.DeleteTransfer(id); err != nil {
		return err
	}
	m.Queue.Remove(id)
	m.broadcastQueue(id)
	return nil
}

// ---------------------------------------------------------------------------
// Remote provisioning
// ---------------------------------------------------------------------------

// EnsureRemote decrypts credentials and materialises the remote in the daemon.
func (m *Manager) EnsureRemote(ctx context.Context, remoteID string) (store.Remote, error) {
	remote, err := m.Store.GetRemote(remoteID)
	if err != nil {
		return remote, err
	}
	opts, err := m.Store.RemoteOptions(remoteID)
	if err != nil {
		return remote, apierr.Wrap(apierr.CodeAuthRequired, err, map[string]any{"remote_id": remoteID})
	}
	m.applyOAuthToken(remote, opts)

	if m.Provisioner == nil {
		return remote, apierr.New(apierr.CodeCloudUnreachable, map[string]any{"reason": "daemon"})
	}
	cfg := rclone.RemoteConfig{
		ID: remote.ID, Name: remote.Name, Provider: remote.Provider,
		RcloneName: remote.RcloneName, Options: opts,
	}
	if err := m.Provisioner.Provision(ctx, cfg); err != nil {
		apiErr := apierr.From(err)
		_ = m.Store.SetRemoteStatus(remoteID, "error", string(apiErr.Code), 0)
		return remote, err
	}
	return remote, nil
}

func (m *Manager) applyOAuthToken(remote store.Remote, opts map[string]string) {
	switch remote.Provider {
	case "drive", "onedrive", "yandex":
	default:
		return
	}
	if opts["token"] != "" {
		return
	}
	tok, err := m.Store.GetOAuthToken(remote.Provider)
	if err != nil {
		return
	}
	expiry := time.Time{}
	if tok.ExpiresAt != nil {
		expiry = *tok.ExpiresAt
	}
	opts["token"] = rclone.TokenJSON(tok.AccessToken, tok.RefreshToken, "Bearer", expiry)
	if tok.ClientSecret != "" {
		opts["client_secret"] = tok.ClientSecret
	}
	if remote.Provider == "drive" && opts["scope"] == "" {
		opts["scope"] = "drive"
	}
}

// ProvisionAll re-materialises every stored remote at startup.
func (m *Manager) ProvisionAll(ctx context.Context) {
	remotes, err := m.Store.ListRemotes()
	if err != nil {
		return
	}
	for _, r := range remotes {
		if _, err := m.EnsureRemote(ctx, r.ID); err != nil && m.Log != nil {
			m.Log.Warn("remote %s not provisioned at startup: %v", r.Name, err)
		}
	}
}

// TestCredentials verifies credentials before they are stored. It creates a
// temporary remote, probes it and removes it again, so no remote_id is needed.
func (m *Manager) TestCredentials(ctx context.Context, provider string, options map[string]string) (rclone.Quota, int64, error) {
	if m.Provisioner == nil {
		return rclone.Quota{}, 0, apierr.New(apierr.CodeCloudUnreachable, map[string]any{"reason": "daemon"})
	}
	name := "sd_probe_" + strings.ReplaceAll(newID(), "-", "")[:12]
	cfg := rclone.RemoteConfig{Provider: provider, RcloneName: name, Name: name, Options: options}
	if err := m.Provisioner.Provision(ctx, cfg); err != nil {
		return rclone.Quota{}, 0, err
	}
	defer func() { _ = m.Provisioner.Remove(context.WithoutCancel(ctx), name) }()

	quota, qErr := m.Provisioner.Quota(ctx, name+":")
	rtt, lErr := m.Provisioner.MeasureLatency(ctx, name+":")
	if lErr != nil {
		return rclone.Quota{}, 0, apierr.Wrap(apierr.CodeCloudUnreachable, lErr, map[string]any{"provider": provider})
	}
	if qErr != nil {
		// Some backends (FTP/SFTP) have no quota; reachability is what matters.
		quota = rclone.Quota{}
	}
	return quota, rtt.Milliseconds(), nil
}

// CheckRemote re-tests a saved connection and updates its status.
func (m *Manager) CheckRemote(ctx context.Context, remoteID string) (rclone.Quota, int64, error) {
	remote, err := m.EnsureRemote(ctx, remoteID)
	if err != nil {
		return rclone.Quota{}, 0, err
	}
	quota, qErr := m.Provisioner.Quota(ctx, remote.RcloneName+":")
	rtt, err := m.Provisioner.MeasureLatency(ctx, remote.RcloneName+":")
	if err != nil {
		apiErr := apierr.From(err)
		_ = m.Store.SetRemoteStatus(remoteID, "error", string(apiErr.Code), 0)
		return rclone.Quota{}, 0, err
	}
	if qErr != nil {
		quota = rclone.Quota{}
	}
	_ = m.Store.SetRemoteStatus(remoteID, "online", "", rtt.Milliseconds())
	return quota, rtt.Milliseconds(), nil
}

// ---------------------------------------------------------------------------
// Auth header resolution
// ---------------------------------------------------------------------------

func (m *Manager) resolveHeaders(profileID, rawURL string) (auth.HeaderSet, error) {
	hs := auth.HeaderSet{UserAgent: auth.DefaultUserAgent, Referer: auth.RefererForSource(rawURL)}
	if profileID == "" {
		return hs, nil
	}
	profile, stored, err := m.Store.GetAuthProfile(profileID)
	if err != nil {
		return hs, err
	}
	if stored.UserAgent != "" {
		hs.UserAgent = stored.UserAgent
	}
	if stored.Referer != "" {
		hs.Referer = stored.Referer
	}
	if stored.Origin != "" {
		hs.Origin = stored.Origin
	}
	if len(stored.Extra) > 0 {
		hs.Extra = stored.Extra
	}
	hs.Cookie = stored.Cookie

	host := hostOf(rawURL)
	switch profile.Mode {
	case auth.ModeBrowser:
		cookie, cerr := auth.CookieHeaderFor(auth.Profile{
			Mode:       profile.Mode,
			Kind:       auth.BrowserKind(profile.Kind),
			ProfileDir: profile.ProfileDir,
			CookieFile: profile.CookieFile,
		}, host)
		if cerr != nil {
			if hs.Cookie == "" {
				return hs, apierr.New(apierr.CodeAuthRequired, map[string]any{"profile": profile.Name, "reason": "unsupported_browser"})
			}
			break
		}
		hs.Cookie = cookie
	case auth.ModeCookiesFile:
		path := stored.CookieFile
		if path == "" {
			path = profile.CookieFile
		}
		if path != "" {
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				return hs, apierr.Wrap(apierr.CodeAuthRequired, rerr, map[string]any{"profile": profile.Name})
			}
			cookie, perr := auth.ParseNetscapeCookies(string(content), host)
			if perr != nil {
				return hs, apierr.Wrap(apierr.CodeAuthRequired, perr, map[string]any{"profile": profile.Name})
			}
			hs.Cookie = cookie
		}
	}
	if hs.Cookie == "" && profile.Mode == auth.ModeManual && stored.Cookie == "" {
		return hs, apierr.New(apierr.CodeAuthRequired, map[string]any{"profile": profile.Name})
	}
	return hs, nil
}

// ProbeProfile performs a real request with a profile's headers for the profile
// test endpoint.
func (m *Manager) ProbeProfile(ctx context.Context, profileID, targetURL string) (*engine.FileMetadata, error) {
	headers, err := m.resolveHeaders(profileID, targetURL)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return engine.Inspect(probeCtx, m.inspector, targetURL, headers)
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// Metric is one transfer_metrics payload.
type Metric struct {
	TransferID      string  `json:"transfer_id"`
	Status          string  `json:"status"`
	Mode            string  `json:"mode"`
	BytesDownloaded int64   `json:"bytes_downloaded"`
	BytesConfirmed  int64   `json:"bytes_confirmed"`
	TotalSize       int64   `json:"total_size"`
	DownloadBps     int64   `json:"download_bps"`
	UploadBps       int64   `json:"upload_bps"`
	BufferUsedBytes int64   `json:"buffer_used_bytes"`
	BufferSizeBytes int64   `json:"buffer_size_bytes"`
	BufferUsagePct  float64 `json:"buffer_usage_pct"`
	ETASeconds      int64   `json:"eta_seconds"`
	RetryAttempt    int     `json:"retry_attempt"`
}

// StartMetrics broadcasts transfer metrics every 500 ms.
func (m *Manager) StartMetrics(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, metric := range m.Metrics(now) {
				if m.Hub != nil {
					m.Hub.Broadcast("transfer_metrics", metric)
				}
			}
		}
	}
}

// Metrics samples all live tasks.
func (m *Manager) Metrics(now time.Time) []Metric {
	m.mu.Lock()
	tasks := make([]*runningTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()

	out := make([]Metric, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.sample(now))
	}
	return out
}

func (t *runningTask) sample(now time.Time) Metric {
	t.mu.Lock()
	defer t.mu.Unlock()

	down := t.state.Downloaded()
	confirmed := t.state.Confirmed()
	if !t.lastAt.IsZero() {
		elapsed := now.Sub(t.lastAt).Seconds()
		if elapsed >= 0.2 {
			downBps := int64(float64(down-t.lastDown) / elapsed)
			upBps := int64(float64(confirmed-t.lastConfirm) / elapsed)
			if downBps < 0 {
				downBps = 0
			}
			if upBps < 0 {
				upBps = 0
			}
			t.downBps = downBps
			t.upBps = upBps
			t.lastDown = down
			t.lastConfirm = confirmed
			t.lastAt = now
		}
	} else {
		t.lastAt = now
		t.lastDown = down
		t.lastConfirm = confirmed
	}

	bufferUsed := t.state.BufferUsed()
	bufferSize := t.state.BufferSize()
	pct := 0.0
	if bufferSize > 0 {
		pct = float64(bufferUsed) / float64(bufferSize) * 100
	}
	eta := int64(0)
	if t.totalSize > confirmed && t.upBps > 0 {
		eta = int64(float64(t.totalSize-confirmed) / float64(t.upBps))
	}
	return Metric{
		TransferID:      t.id,
		Status:          t.status,
		Mode:            t.mode,
		BytesDownloaded: down,
		BytesConfirmed:  confirmed,
		TotalSize:       t.totalSize,
		DownloadBps:     t.downBps,
		UploadBps:       t.upBps,
		BufferUsedBytes: bufferUsed,
		BufferSizeBytes: bufferSize,
		BufferUsagePct:  pct,
		ETASeconds:      eta,
		RetryAttempt:    int(t.state.Retries()),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m *Manager) broadcastQueue(changedID string) {
	if m.Hub == nil || m.Queue == nil {
		return
	}
	m.Hub.Broadcast("queue_update", map[string]any{
		"queue":      m.Queue.View(),
		"changed_id": changedID,
	})
}

func (m *Manager) logTransfer(transferID, level, source, message string) {
	if m.Store != nil {
		_ = m.Store.AddLog(transferID, level, source, message)
	}
	if m.Hub != nil {
		m.Hub.Broadcast("transfer_log", store.TransferLog{
			TransferID: transferID,
			TS:         time.Now().UTC(),
			Level:      level,
			Source:     source,
			Message:    message,
		})
	}
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return "file.bin"
	}
	return name
}

func engineFilenameFallback(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := len(segments) - 1; i >= 0; i-- {
			if segments[i] != "" {
				return segments[i]
			}
		}
	}
	return "file-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".bin"
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

var _ = http.MethodGet

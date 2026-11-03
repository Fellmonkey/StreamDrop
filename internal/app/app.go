// Package app wires the application container and owns its lifecycle:
// configuration, database, daemon, server, tray and graceful shutdown.
package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"streamdrop/internal/config"
	"streamdrop/internal/crash"
	"streamdrop/internal/database"
	"streamdrop/internal/deps"
	"streamdrop/internal/engine"
	"streamdrop/internal/logging"
	"streamdrop/internal/notify"
	"streamdrop/internal/oauth"
	"streamdrop/internal/queue"
	"streamdrop/internal/rclone"
	"streamdrop/internal/security"
	"streamdrop/internal/server"
	"streamdrop/internal/server/handlers"
	"streamdrop/internal/server/middleware"
	"streamdrop/internal/store"
	"streamdrop/internal/transfer"
	"streamdrop/internal/tray"
	"streamdrop/internal/update"
	"streamdrop/internal/ws"
	"streamdrop/web"
)

// Run starts the application and blocks until the context is cancelled.
func Run(ctx context.Context, cfg config.Config) (runErr error) {
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	log, err := logging.New(cfg.LogsDir(), cfg.LogLevel)
	if err != nil {
		return err
	}
	defer log.Close()
	if os.Getenv("STREAMDROP_CONSOLE_LOG") == "1" {
		log.Attach(os.Stdout)
	}
	log.Info("StreamDrop %s starting (data: %s)", config.Version, cfg.DataDir)

	db, err := database.Open(cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	secret := security.New()
	st := store.New(db, secret)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			log.Error("fatal panic: %v\n%s", rec, stack)
			if st.SettingBool("crash_reporting_enabled", false) {
				var names []string
				if remotes, err := st.ListRemotes(); err == nil {
					for _, r := range remotes {
						names = append(names, r.Name+"/"+r.Provider)
					}
				}
				if path, werr := crash.Write(cfg.DataDir, config.Version, crash.Describe(rec), names, log.Tail(200), stack); werr == nil {
					log.Info("crash bundle written: %s", path)
				}
			}
			runErr = fmt.Errorf("fatal: %v", rec)
		}
	}()

	// A staged update is applied before the main loop, while replacing the
	// executable is still safe.
	token := newLocalToken()
	hub := ws.New(token, func() any { return nil }, log)
	upd := update.NewManager(cfg.UpdateRepo, config.Version, cfg.DataDir, st, hub, log)
	if err := upd.ApplyPending(); err != nil {
		log.Warn("pending update not applied: %v", err)
	}

	depMgr := deps.NewManager(
		deps.Paths{DataDir: cfg.DataDir, ExeDir: executableDir()},
		st,
		func(p deps.Progress) { hub.Broadcast("dep_progress", p) },
		log.Info,
	)
	tools := depMgr.Tools()

	// Rclone RC daemon: explorer, quota, provisioning and cleanup.
	var daemon *rclone.Daemon
	if tools.Rclone != "" {
		d, derr := rclone.StartDaemon(ctx, tools.Rclone, rclone.DaemonPort, func(level, msg string) {
			log.Warn("%s", strings.TrimSpace(msg))
		})
		if derr != nil {
			log.Warn("rclone daemon unavailable: %v", derr)
		} else {
			daemon = d
			log.Info("rclone daemon ready on %s", d.Address())
		}
	} else {
		log.Warn("rclone is not installed; explorer and uploads are unavailable until it is installed")
	}

	var provisioner *rclone.Provisioner
	var explorer *rclone.Explorer
	var cleaner *rclone.Cleaner
	if daemon != nil {
		rc := daemon.Client()
		provisioner = &rclone.Provisioner{RC: rc}
		explorer = &rclone.Explorer{RC: rc}
		cleaner = &rclone.Cleaner{RC: rc}
	}

	// Queue and transfer orchestration.
	q := queue.NewManager(st.SettingInt("parallel_limit", 1))
	pipeline := engine.NewPipeline(tools)
	transfers := transfer.NewManager(st, pipeline, q)
	transfers.Hub = hub
	transfers.Cleaner = cleaner
	transfers.Provisioner = provisioner
	transfers.Log = log
	transfers.Tools = depMgr.Tools
	transfers.Settings = func() transfer.Settings {
		return transfer.Settings{
			RetryMaxAttempts:   st.SettingInt("retry_max_attempts", 3),
			ChecksumEnabled:    st.SettingBool("checksum_enabled", false),
			BandwidthDownBps:   st.SettingInt64("bandwidth_down_bps", 0),
			BandwidthUpBps:     st.SettingInt64("bandwidth_up_bps", 0),
			DefaultMode:        st.GetSetting("default_mode", engine.ModeRingBuffer),
			DefaultBufferBytes: st.SettingInt64("default_buffer_bytes", 128<<20),
		}
	}

	notifier := notify.New(tray.Toaster(), func() notify.Settings {
		return notify.Settings{
			ToastEnabled:    st.SettingBool("notifications_toast", true),
			TelegramEnabled: st.SettingBool("telegram_enabled", false),
			TelegramToken:   decryptSetting(secret, st.GetSetting("telegram_token", "")),
			TelegramChatID:  decryptSetting(secret, st.GetSetting("telegram_chat_id", "")),
		}
	}, hub.Broadcast, log.Warn)
	transfers.Notifier = notifier

	// OAuth: the callback listener lives on the fixed port, independent of the
	// UI port. Tokens are encrypted before they reach SQLite.
	oauthMgr := oauth.NewManager(oauth.NewRegistry(), cfg.OAuthCallbackPort,
		func(ctx context.Context, providerID string, tok oauth.TokenSet) error {
			expiry := time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
			return st.SaveOAuthToken(store.OAuthToken{
				Provider:     providerID,
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Scope:        tok.Scope,
				ExpiresAt:    &expiry,
			})
		})

	// Port: prefer the configured port, fall back to the next free one and
	// report the actual address everywhere.
	ln, port, err := listenLoopback(cfg.Host, cfg.Port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	url := fmt.Sprintf("http://%s:%d", displayHost(cfg.Host), port)

	hub.SetSnapshotBuilder(func() any { return snapshot(st, q, depMgr, transfers) })

	h := &handlers.Handler{
		Store:       st,
		Transfers:   transfers,
		Queue:       q,
		Deps:        depMgr,
		OAuth:       oauthMgr,
		Explorer:    explorer,
		Daemon:      daemon,
		Provisioner: provisioner,
		Notifier:    notifier,
		Secret:      secret,
		Update:      upd,
		Hub:         hub,
		Snapshot:    func() any { return snapshot(st, q, depMgr, transfers) },
		Settings:    transfers.Settings,
		Version:     config.Version,
		Port:        port,
		DataDir:     cfg.DataDir,
		Started:     time.Now(),
		Log:         log,
	}

	// ensureDaemon (re)starts the rclone RC daemon once the binary is present
	// — e.g. right after a first-run automatic install — and re-wires all the
	// cloud-facing components to the fresh daemon.
	ensureDaemon := func() error {
		tools := depMgr.Tools()
		if tools.Rclone == "" {
			return nil
		}
		if daemon != nil && daemon.Ready() {
			return nil
		}
		d, derr := rclone.StartDaemon(ctx, tools.Rclone, rclone.DaemonPort, func(level, msg string) {
			log.Warn("%s", strings.TrimSpace(msg))
		})
		if derr != nil {
			return derr
		}
		daemon = d
		rc := daemon.Client()
		provisioner = &rclone.Provisioner{RC: rc}
		explorer = &rclone.Explorer{RC: rc}
		cleaner = &rclone.Cleaner{RC: rc}
		transfers.Provisioner = provisioner
		transfers.Cleaner = cleaner
		h.Explorer = explorer
		h.Daemon = daemon
		h.Provisioner = provisioner
		transfers.ProvisionAll(ctx)
		log.Info("rclone daemon started after install")
		return nil
	}
	h.EnsureDaemon = ensureDaemon
	h.OpenBrowser = func() error { openBrowser(url); return nil }
	h.OpenApp = func() error { openAppWindow(url); return nil }
	h.OpenURL = func(u, browser string) error { return openURLWith(browser, u) }
	h.InstalledBrowsers = func() []handlers.BrowserEntry {
		installed := InstalledBrowsers()
		out := make([]handlers.BrowserEntry, 0, len(installed)+1)
		out = append(out, handlers.BrowserEntry{Name: "default", Default: false})
		for _, b := range installed {
			out = append(out, handlers.BrowserEntry{Name: b.Name, Default: b.Default})
		}
		return out
	}

	router := server.New(h, token, port, hub, web.Handler(token))
	srv := &http.Server{
		Handler:           middleware.LocalGuard{Port: port}.Handler(router),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("http server stopped: %v", serveErr)
		}
	}()

	q.OnUpdate(func(view queue.View) {
		hub.Broadcast("queue_update", map[string]any{"queue": view, "changed_id": ""})
	})
	q.Start(ctx)
	go transfers.StartMetrics(ctx)
	go hub.PingLoop(ctx)
	go oauthMgr.CleanupLoop(ctx)
	go func() {
		if _, err := upd.Check(ctx, false); err != nil {
			log.Debug("update check: %v", err)
		}
	}()
	go func() {
		// Provisioning is best effort at startup; failures surface in the UI
		// as a remote status and are retried on the next task.
		if daemon != nil {
			transfers.ProvisionAll(ctx)
		}
	}()

	log.Info("StreamDrop listening on %s", url)
	if cfg.OpenBrowser {
		openAppWindow(url)
	}
	if !cfg.NoTray {
		tray.Start(ctx, tray.Callbacks{
			URL:          url,
			OnOpen:       func() { openAppWindow(url) },
			OnOpenBrowser: func() { openBrowser(url) },
			OnPause:      func() { transfers.PauseQueue() },
			OnResume:     func() { transfers.ResumeQueue() },
			OnQuit:       func() { cancel() },
		})
	}

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	oauthMgr.Shutdown(shutdownCtx)
	if daemon != nil {
		daemon.Stop()
	}
	return nil
}

// listenLoopback binds to loopback, trying subsequent ports when the preferred
// one is busy (8085 -> 8086 -> ...).
func listenLoopback(host string, start int) (net.Listener, int, error) {
	var lastErr error
	for i := 0; i < 50; i++ {
		port := start + i
		if port > 65535 {
			break
		}
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no free port")
	}
	return nil, 0, lastErr
}

func displayHost(host string) string {
	if host == "::1" {
		return "[::1]"
	}
	return host
}

// snapshot builds the WebSocket snapshot sent on every connection.
func snapshot(st *store.Store, q *queue.Manager, depMgr *deps.Manager, transfers *transfer.Manager) map[string]any {
	active, _ := st.ActiveTransfers()
	if active == nil {
		active = []store.Transfer{}
	}
	remotes, _ := st.ListRemotes()
	remoteView := make([]map[string]any, 0, len(remotes))
	for _, r := range remotes {
		remoteView = append(remoteView, map[string]any{
			"id": r.ID, "name": r.Name, "provider": r.Provider, "brand": r.Brand,
			"status": r.Status, "rtt_ms": r.LastRTTMs, "last_error": r.LastError,
		})
	}
	depView := make([]map[string]any, 0)
	if depMgr != nil {
		for _, it := range depMgr.List() {
			depView = append(depView, map[string]any{
				"id": it.ID, "status": string(it.Status), "version": it.Version,
				"required": it.Required, "verified": it.Verified,
			})
		}
	}
	return map[string]any{
		"server_time": time.Now().UTC(),
		"queue":       q.View(),
		"transfers":   active,
		"remotes":     remoteView,
		"deps":        depView,
		"metrics":     transfers.LiveMetrics(),
	}
}

func newLocalToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decryptSetting(secret security.SecretStore, value string) string {
	if !strings.HasPrefix(value, "enc:") {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "enc:"))
	if err != nil {
		return ""
	}
	plain, err := secret.Decrypt(raw)
	if err != nil {
		return ""
	}
	return string(plain)
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// openAppWindow opens the UI in a Chromium-based browser in app mode (a
// desktop-app window without tabs/chrome). It falls back to the default
// browser when neither Edge nor Chrome is installed.
func openAppWindow(url string) {
	viewURL := url
	if !strings.Contains(viewURL, "?") {
		viewURL += "?view=app"
	}
	for _, name := range []string{"edge", "chrome"} {
		if p := FindBrowserPath(name); p != "" {
			_ = exec.Command(p, "--app="+viewURL).Start()
			return
		}
	}
	openBrowser(url)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// openURLWith opens a URL in the selected browser (default falls back to the
// OS handler). Used for OAuth and external opening.
func openURLWith(browser, url string) error {
	if browser != "default" {
		if p := FindBrowserPath(browser); p != "" {
			_ = exec.Command(p, url).Start()
			return nil
		}
	}
	openBrowser(url)
	return nil
}

var _ = json.Marshal

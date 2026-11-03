// Package notify delivers completion and failure events: an optional native
// Windows toast, the WebSocket event for the open tab, and an optional Telegram
// message (back_spec.md §3.10).
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Event is a notification with machine-readable fields only. The client builds
// the localized text, so no language ever leaks from the server.
type Event struct {
	Kind            string `json:"kind"`
	Level           string `json:"level"`
	TransferID      string `json:"transfer_id,omitempty"`
	FilesCount      int    `json:"files_count"`
	FailedCount     int    `json:"failed_count,omitempty"`
	Bytes           int64  `json:"bytes,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
	Code            string `json:"code,omitempty"`
}

// Kinds of notifications.
const (
	KindBatchCompleted  = "batch_completed"
	KindTransferFailed  = "transfer_failed"
	KindTransferAborted = "transfer_aborted"
	KindQueuePaused     = "queue_paused"
)

// ToastSender delivers a native system notification.
type ToastSender interface {
	Show(title, message string) error
}

// Settings is the notification configuration snapshot.
type Settings struct {
	ToastEnabled    bool
	TelegramEnabled bool
	TelegramToken   string
	TelegramChatID  string
}

// Provider is a function returning the current settings (secrets decrypted).
type Provider func() Settings

// Notifier fans an event out to the enabled channels.
type Notifier struct {
	toast    ToastSender
	settings Provider
	hub      func(event string, data any)
	client   *http.Client
	logf     func(format string, args ...any)
}

// New builds a notifier.
func New(toast ToastSender, settings Provider, hub func(string, any), logf func(string, ...any)) *Notifier {
	return &Notifier{
		toast:    toast,
		settings: settings,
		hub:      hub,
		client:   &http.Client{Timeout: 5 * time.Second},
		logf:     logf,
	}
}

// Send delivers an event. Failures on one channel never affect others or the
// transfer itself.
func (n *Notifier) Send(ctx context.Context, ev Event) {
	if n.hub != nil {
		n.hub("notify", ev)
	}
	cfg := Settings{}
	if n.settings != nil {
		cfg = n.settings()
	}
	if cfg.ToastEnabled && n.toast != nil {
		title, message := plainText(ev)
		if err := n.toast.Show(title, message); err != nil && n.logf != nil {
			n.logf("toast failed: %v", err)
		}
	}
	if cfg.TelegramEnabled && cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		if err := n.sendTelegram(ctx, cfg, ev); err != nil && n.logf != nil {
			n.logf("telegram failed: %v", err)
		}
	}
}

// sendTelegram posts a summary. The message deliberately contains no URL, no
// file name and no credentials — only status, counts and volume.
func (n *Notifier) sendTelegram(ctx context.Context, cfg Settings, ev Event) error {
	title, body := plainText(ev)
	text := title + "\n" + body
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken)
	form := url.Values{}
	form.Set("chat_id", cfg.TelegramChatID)
	form.Set("text", text)

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// ValidateTelegram checks the credentials by calling getMe.
func ValidateTelegram(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet,
		"https://api.telegram.org/bot"+token+"/getMe", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram status %d", resp.StatusCode)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("telegram rejected the token")
	}
	return nil
}

// plainText builds the native/toast text. It is intentionally Russian-neutral
// English: the toast is an OS surface without locale negotiation. The in-app
// event carries only codes and numbers.
func plainText(ev Event) (string, string) {
	switch ev.Kind {
	case KindBatchCompleted:
		return "StreamDrop — completed",
			fmt.Sprintf("%d file(s), %s, %ds",
				ev.FilesCount, humanBytes(ev.Bytes), ev.DurationSeconds)
	case KindTransferFailed:
		return "StreamDrop — failed",
			fmt.Sprintf("A transfer failed (code: %s)", ev.Code)
	case KindTransferAborted:
		return "StreamDrop — aborted", "A transfer was aborted."
	case KindQueuePaused:
		return "StreamDrop — queue paused", "The transfer queue is paused."
	default:
		return "StreamDrop", ev.Kind
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// TestMessage sends a test notification to the configured channels.
func (n *Notifier) TestMessage(ctx context.Context, cfg Settings) error {
	if cfg.TelegramEnabled {
		if cfg.TelegramToken == "" || cfg.TelegramChatID == "" {
			return fmt.Errorf("telegram credentials are incomplete")
		}
		return n.sendTelegram(ctx, cfg, Event{Kind: KindBatchCompleted, FilesCount: 1, Bytes: 1024, DurationSeconds: 1})
	}
	if cfg.ToastEnabled && n.toast != nil {
		return n.toast.Show("StreamDrop", "Test notification")
	}
	return fmt.Errorf("no notification channel is enabled")
}

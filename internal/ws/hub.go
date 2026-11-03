// Package ws implements the single realtime channel: /api/v1/ws with a
// mandatory token, a snapshot on connect, and broadcast events for metrics,
// logs, queue changes, notifications and progress (back_spec.md §5.2).
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"streamdrop/internal/apierr"
	"streamdrop/internal/logging"
)

// Hub fans events out to connected browser tabs.
type Hub struct {
	token    string
	snapshot func() any
	log      *logging.Logger

	mu      sync.Mutex
	clients map[*client]struct{}
}

type client struct {
	mu sync.Mutex
	c  *websocket.Conn
}

// New creates a hub. token is the per-run local token; snapshot builds the
// initial state on every connection.
func New(token string, snapshot func() any, log *logging.Logger) *Hub {
	return &Hub{token: token, snapshot: snapshot, log: log, clients: map[*client]struct{}{}}
}

// Broadcast sends one event to every connected tab.
func (h *Hub) Broadcast(event string, data any) {
	payload, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c.mu.Lock()
		err := c.c.Write(ctx, websocket.MessageText, payload)
		c.mu.Unlock()
		cancel()
		if err != nil {
			h.drop(c)
		}
	}
}

// ClientCount reports the number of open tabs.
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) drop(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.c.Close(websocket.StatusNormalClosure, "")
}

// ServeHTTP upgrades the connection. The token is mandatory and checked before
// the upgrade, so an invalid token never establishes a socket.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || token != h.token {
		apierr.Write(w, apierr.New(apierr.CodeLocalAuthRequired, nil))
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// LocalGuard already validates Host and Origin; the patterns here keep
		// coder/websocket from rejecting the legitimate loopback origin.
		OriginPatterns: []string{"127.0.0.1:*", "localhost:*", "[::1]:*"},
	})
	if err != nil {
		if h.log != nil {
			h.log.Warn("ws accept: %v", err)
		}
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	c := &client{c: conn}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer h.drop(c)

	if h.snapshot != nil {
		if payload, err := json.Marshal(map[string]any{"event": "snapshot", "data": h.snapshot()}); err == nil {
			c.mu.Lock()
			werr := conn.Write(r.Context(), websocket.MessageText, payload)
			c.mu.Unlock()
			if werr != nil {
				return
			}
		}
	}

	// Read loop: the browser does not send commands, but reading detects
	// disconnects and processes pongs.
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
	}
}

// PingLoop keeps idle connections alive. It is started by the app and stops
// with the context.
func (h *Hub) PingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.Lock()
			clients := make([]*client, 0, len(h.clients))
			for c := range h.clients {
				clients = append(clients, c)
			}
			h.mu.Unlock()
			for _, c := range clients {
				pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
				err := c.c.Ping(pctx)
				cancel()
				if err != nil {
					h.drop(c)
				}
			}
		}
	}
}

var _ = strings.TrimSpace

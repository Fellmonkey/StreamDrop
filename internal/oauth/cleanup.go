package oauth

import (
	"context"
	"time"
)

// CleanupLoop drops expired sessions and stops the callback listener when no
// pending session is left. It is stopped by the application context.
func (m *Manager) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanupNow()
		}
	}
}

func (m *Manager) cleanupNow() {
	m.mu.Lock()
	now := m.now()
	pending := 0
	for id, s := range m.sessions {
		if s.Status == SessionPending && now.After(s.ExpiresAt) {
			s.Status = SessionFailed
			s.ErrorCode = "auth_token_expired"
		}
		if s.Status == SessionPending {
			pending++
			continue
		}
		if now.Sub(s.ExpiresAt) > time.Hour {
			delete(m.sessions, id)
		}
	}
	var srv = m.server
	if pending == 0 {
		m.server = nil
	}
	m.mu.Unlock()

	if pending == 0 && srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(shutdownCtx)
		cancel()
	}
}

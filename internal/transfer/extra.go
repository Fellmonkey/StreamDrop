package transfer

import (
	"context"
	"time"

	"streamdrop/internal/engine"
	"streamdrop/internal/queue"
	"streamdrop/internal/store"
)

// Inspect runs a pre-flight probe with the headers of an auth profile.
func (m *Manager) Inspect(ctx context.Context, rawURL, profileID string) (*engine.FileMetadata, error) {
	headers, err := m.resolveHeaders(profileID, rawURL)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return engine.Inspect(probeCtx, m.inspector, rawURL, headers)
}

// PauseQueue pauses the scheduler and marks active tasks paused in history.
func (m *Manager) PauseQueue() {
	m.Queue.Pause()
	active, err := m.Store.ActiveTransfers()
	if err != nil {
		return
	}
	for _, tr := range active {
		if tr.Status == queue.StatusRunning || tr.Status == queue.StatusRetrying {
			tr.Status = queue.StatusPaused
			_ = m.Store.UpdateTransfer(tr)
		}
	}
	m.broadcastQueue("")
}

// ResumeQueue resumes the scheduler and its queue-paused tasks.
func (m *Manager) ResumeQueue() {
	m.Queue.Resume()
	active, err := m.Store.ActiveTransfers()
	if err != nil {
		return
	}
	for _, tr := range active {
		if tr.Status == queue.StatusPaused {
			tr.Status = queue.StatusRunning
			_ = m.Store.UpdateTransfer(tr)
		}
	}
	m.broadcastQueue("")
}

// Reorder applies a manual queue order and persists priorities.
func (m *Manager) Reorder(ids []string) error {
	if err := m.Queue.Reorder(ids); err != nil {
		return err
	}
	for i, id := range ids {
		tr, err := m.Store.GetTransfer(id)
		if err != nil {
			continue
		}
		tr.Priority = i
		_ = m.Store.UpdateTransfer(tr)
	}
	m.broadcastQueue("")
	return nil
}

// QueueState is the queue view enriched with transfer rows for the UI.
type QueueState struct {
	Paused        bool             `json:"paused"`
	ParallelLimit int              `json:"parallel_limit"`
	Active        int              `json:"active"`
	Pending       int              `json:"pending"`
	Items         []QueueItem      `json:"items"`
	ActiveTasks   []store.Transfer `json:"active_tasks"`
}

// QueueItem is one row of the queue.
type QueueItem struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Remote   string `json:"remote_label"`
	Path     string `json:"remote_path"`
	Progress int64  `json:"bytes_transferred"`
}

// QueueState builds the API snapshot.
func (m *Manager) QueueState() QueueState {
	view := m.Queue.View()
	active, _ := m.Store.ActiveTransfers()
	byID := map[string]store.Transfer{}
	for _, tr := range active {
		byID[tr.ID] = tr
	}
	items := make([]QueueItem, 0, len(view.Items))
	for _, it := range view.Items {
		tr, ok := byID[it.ID]
		if !ok {
			continue
		}
		items = append(items, QueueItem{
			ID:       tr.ID,
			Status:   it.Status,
			Priority: it.Priority,
			Filename: tr.FileName(),
			Size:     tr.FileSize,
			Remote:   tr.RemoteLabel,
			Path:     tr.RemotePath,
			Progress: tr.BytesTransferred,
		})
	}
	return QueueState{
		Paused:        view.Paused,
		ParallelLimit: view.ParallelLimit,
		Active:        view.Active,
		Pending:       view.Pending,
		Items:         items,
		ActiveTasks:   active,
	}
}

// LiveMetrics returns the current metric samples usable by REST if needed.
func (m *Manager) LiveMetrics() []Metric { return m.Metrics(time.Now()) }

// Package queue schedules transfers: parallelism limit, manual priority,
// queue pause and per-task pause (back_spec.md §3.8).
package queue

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"streamdrop/internal/apierr"
)

// Status values used by the scheduler.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusRetrying  = "retrying"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusAborted   = "aborted"
)

// Task is a schedulable transfer.
type Task interface {
	ID() string
	// Start runs the task until it finishes or the context is cancelled.
	Start(ctx context.Context)
	Pause()
	Resume()
	Abort()
}

// Entry is the scheduler's view of one task.
type Entry struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
}

// View is the queue snapshot sent to clients.
type View struct {
	Paused        bool    `json:"paused"`
	ParallelLimit int     `json:"parallel_limit"`
	Active        int     `json:"active"`
	Pending       int     `json:"pending"`
	Items         []Entry `json:"items"`
}

// Manager is the scheduler.
type Manager struct {
	mu          sync.Mutex
	limit       int
	paused      bool
	order       []string
	tasks       map[string]Task
	status      map[string]string
	running     map[string]context.CancelFunc
	queuePaused map[string]bool
	started     bool
	wake        chan struct{}
	onUpdate    func(View)
	baseCtx     context.Context
}

// NewManager creates a scheduler with a parallelism limit (1..5).
func NewManager(limit int) *Manager {
	if limit < 1 || limit > 5 {
		limit = 1
	}
	return &Manager{
		limit:       limit,
		tasks:       map[string]Task{},
		status:      map[string]string{},
		running:     map[string]context.CancelFunc{},
		queuePaused: map[string]bool{},
		wake:        make(chan struct{}, 1),
	}
}

// OnUpdate registers a callback invoked after every state change.
func (m *Manager) OnUpdate(fn func(View)) {
	m.mu.Lock()
	m.onUpdate = fn
	m.mu.Unlock()
}

// Start launches the scheduling loop.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.baseCtx = ctx
	m.mu.Unlock()
	go m.loop(ctx)
}

func (m *Manager) loop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
		m.dispatch(ctx)
	}
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// dispatch starts queued tasks while the limit allows.
func (m *Manager) dispatch(ctx context.Context) {
	m.mu.Lock()
	if m.paused {
		m.mu.Unlock()
		m.notify()
		return
	}
	var toStart []string
	for _, id := range m.order {
		// Running tasks plus slots already reserved in this pass must stay
		// below the limit; m.running alone is not enough because it is filled
		// only after the collection loop.
		if len(m.running)+len(toStart) >= m.limit {
			break
		}
		if m.status[id] != StatusQueued {
			continue
		}
		toStart = append(toStart, id)
		// Reserve the slot so a later pass cannot oversubscribe.
		m.status[id] = StatusRunning
	}
	m.mu.Unlock()

	for _, id := range toStart {
		m.mu.Lock()
		task, ok := m.tasks[id]
		if !ok {
			m.mu.Unlock()
			continue
		}
		taskCtx, cancel := context.WithCancel(ctx)
		m.running[id] = cancel
		m.mu.Unlock()

		go func(id string, task Task, taskCtx context.Context) {
			defer func() {
				cancel()
				m.mu.Lock()
				delete(m.running, id)
				delete(m.tasks, id)
				m.mu.Unlock()
				m.signal()
				m.notify()
			}()
			task.Start(taskCtx)
		}(id, task, taskCtx)
	}
	m.notify()
}

// Submit enqueues a task at the end of the queue.
func (m *Manager) Submit(t Task) {
	m.mu.Lock()
	id := t.ID()
	if _, exists := m.tasks[id]; !exists {
		m.order = append(m.order, id)
	}
	m.tasks[id] = t
	if st := m.status[id]; st == "" || st == StatusCompleted || st == StatusFailed || st == StatusAborted {
		m.status[id] = StatusQueued
	}
	m.mu.Unlock()
	m.signal()
	m.notify()
}

// SetLimit changes the parallelism limit. Lowering it never interrupts running
// tasks; they finish and no new ones start until the count drops.
func (m *Manager) SetLimit(n int) error {
	if n < 1 || n > 5 {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "parallel_limit"})
	}
	m.mu.Lock()
	m.limit = n
	m.mu.Unlock()
	m.signal()
	m.notify()
	return nil
}

// Limit returns the current parallelism limit.
func (m *Manager) Limit() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.limit
}

// Pause stops new tasks and freezes active ones at the buffer boundary.
func (m *Manager) Pause() {
	m.mu.Lock()
	m.paused = true
	ids := make([]string, 0, len(m.running))
	for id, cancel := range m.running {
		ids = append(ids, id)
		_ = cancel
		m.queuePaused[id] = true
		if m.status[id] == StatusRunning || m.status[id] == StatusRetrying {
			m.status[id] = StatusPaused
		}
	}
	tasks := make([]Task, 0, len(ids))
	for _, id := range ids {
		if t, ok := m.tasks[id]; ok {
			tasks = append(tasks, t)
		}
	}
	m.mu.Unlock()

	for _, t := range tasks {
		t.Pause()
	}
	m.notify()
}

// Resume unfreezes queue-paused tasks.
func (m *Manager) Resume() {
	m.mu.Lock()
	m.paused = false
	var tasks []Task
	for id := range m.queuePaused {
		if m.status[id] == StatusPaused {
			m.status[id] = StatusRunning
		}
		if t, ok := m.tasks[id]; ok {
			tasks = append(tasks, t)
		}
	}
	m.queuePaused = map[string]bool{}
	m.mu.Unlock()

	for _, t := range tasks {
		t.Resume()
	}
	m.signal()
	m.notify()
}

// Paused reports the queue state.
func (m *Manager) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// Reorder applies a manual order. Ids that are not listed keep their relative
// position after the listed ones.
func (m *Manager) Reorder(ids []string) error {
	m.mu.Lock()
	known := map[string]bool{}
	for _, id := range m.order {
		known[id] = true
	}
	seen := map[string]bool{}
	var next []string
	for _, id := range ids {
		if known[id] && !seen[id] {
			next = append(next, id)
			seen[id] = true
		}
	}
	for _, id := range m.order {
		if !seen[id] {
			next = append(next, id)
		}
	}
	m.order = next
	m.mu.Unlock()
	m.signal()
	m.notify()
	return nil
}

// Remove drops a task from the queue (used on abort/history delete).
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.tasks, id)
	delete(m.status, id)
	delete(m.running, id)
	filtered := m.order[:0]
	for _, existing := range m.order {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	m.order = filtered
	m.mu.Unlock()
	m.notify()
}

// SetStatus lets the executor report a task status (running/retrying/failed).
func (m *Manager) SetStatus(id, status string) {
	m.mu.Lock()
	if _, ok := m.status[id]; ok {
		m.status[id] = status
	} else {
		m.status[id] = status
	}
	m.mu.Unlock()
	m.notify()
}

// Status returns a task status.
func (m *Manager) Status(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status[id]
}

// View returns the current snapshot.
func (m *Manager) View() View {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Entry, 0, len(m.order))
	pending := 0
	for i, id := range m.order {
		st := m.status[id]
		items = append(items, Entry{ID: id, Status: st, Priority: i})
		if st == StatusQueued || st == StatusPaused || st == StatusRetrying {
			pending++
		}
	}
	return View{
		Paused:        m.paused,
		ParallelLimit: m.limit,
		Active:        len(m.running),
		Pending:       pending,
		Items:         items,
	}
}

// IsActive reports whether a task is currently running.
func (m *Manager) IsActive(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[id]
	return ok
}

func (m *Manager) notify() {
	m.mu.Lock()
	fn := m.onUpdate
	m.mu.Unlock()
	if fn != nil {
		fn(m.View())
	}
}

// SortTasks orders tasks by priority then insertion.
func SortTasks(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Priority < entries[j].Priority })
}

var _ = fmt.Sprintf

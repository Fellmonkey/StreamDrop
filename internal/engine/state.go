package engine

import (
	"context"
	"sync"
	"sync/atomic"
)

// Gate pauses a running pipeline at the buffer boundary. Pausing blocks the
// writer (the download stalls through backpressure); processes are not killed
// and connections are not dropped, so resuming continues from the same byte.
type Gate struct {
	mu     sync.Mutex
	ch     chan struct{}
	closed bool
}

// NewGate returns an open gate.
func NewGate() *Gate {
	g := &Gate{ch: make(chan struct{})}
	close(g.ch)
	return g
}

// Pause blocks writers.
func (g *Gate) Pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	select {
	case <-g.ch:
		g.ch = make(chan struct{})
	default:
	}
}

// Resume releases writers.
func (g *Gate) Resume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	select {
	case <-g.ch:
	default:
		close(g.ch)
	}
}

// Paused reports the current state.
func (g *Gate) Paused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.ch:
		return false
	default:
		return true
	}
}

// Wait blocks while the gate is paused and returns when it opens or the
// context is cancelled.
func (g *Gate) Wait(ctx context.Context) error {
	g.mu.Lock()
	ch := g.ch
	g.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release wakes every waiter permanently; used at shutdown.
func (g *Gate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	select {
	case <-g.ch:
	default:
		close(g.ch)
	}
}

// TaskState holds live counters for one transfer.
type TaskState struct {
	downloaded atomic.Int64
	confirmed  atomic.Int64
	bufferUsed atomic.Int64
	bufferSize atomic.Int64
	retries    atomic.Int32
	status     atomic.Value // string
}

// NewTaskState creates a state with the given buffer size.
func NewTaskState(status string, bufferSize int64) *TaskState {
	s := &TaskState{}
	s.bufferSize.Store(bufferSize)
	s.SetStatus(status)
	return s
}

// SetStatus records the pipeline status.
func (s *TaskState) SetStatus(v string) { s.status.Store(v) }

// Status returns the pipeline status.
func (s *TaskState) Status() string {
	if v, ok := s.status.Load().(string); ok {
		return v
	}
	return ""
}

// AddDownloaded adds bytes accepted from the network.
func (s *TaskState) AddDownloaded(n int64) { s.downloaded.Add(n) }

// Downloaded returns bytes accepted from the network.
func (s *TaskState) Downloaded() int64 { return s.downloaded.Load() }

// SetConfirmed records bytes confirmed by the cloud.
func (s *TaskState) SetConfirmed(n int64) { s.confirmed.Store(n) }

// Confirmed returns bytes confirmed by the cloud.
func (s *TaskState) Confirmed() int64 { return s.confirmed.Load() }

// SetBufferUsed records current buffer occupancy.
func (s *TaskState) SetBufferUsed(n int64) { s.bufferUsed.Store(n) }

// BufferUsed returns current buffer occupancy.
func (s *TaskState) BufferUsed() int64 { return s.bufferUsed.Load() }

// SetBufferSize records the pool size.
func (s *TaskState) SetBufferSize(n int64) { s.bufferSize.Store(n) }

// BufferSize returns the pool size.
func (s *TaskState) BufferSize() int64 { return s.bufferSize.Load() }

// SetRetries records performed retry attempts.
func (s *TaskState) SetRetries(n int32) { s.retries.Store(n) }

// Retries returns performed retry attempts.
func (s *TaskState) Retries() int32 { return s.retries.Load() }

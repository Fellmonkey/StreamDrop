package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeTask struct {
	id      string
	mu      sync.Mutex
	started bool
	paused  bool
	block   chan struct{}
	done    chan struct{}
	aborted bool
}

func newFakeTask(id string) *fakeTask {
	return &fakeTask{id: id, block: make(chan struct{}), done: make(chan struct{})}
}

func (f *fakeTask) ID() string { return f.id }
func (f *fakeTask) Start(ctx context.Context) {
	f.mu.Lock()
	f.started = true
	f.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-f.block:
	}
	close(f.done)
}
func (f *fakeTask) Pause()  { f.mu.Lock(); f.paused = true; f.mu.Unlock() }
func (f *fakeTask) Resume() { f.mu.Lock(); f.paused = false; f.mu.Unlock() }
func (f *fakeTask) Abort()  { f.mu.Lock(); f.aborted = true; f.mu.Unlock() }
func (f *fakeTask) isStarted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
}
func (f *fakeTask) finish() { close(f.block) }

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", msg)
}

func TestLimitValidation(t *testing.T) {
	m := NewManager(1)
	if err := m.SetLimit(0); err == nil {
		t.Fatal("limit 0 must be rejected")
	}
	if err := m.SetLimit(6); err == nil {
		t.Fatal("limit 6 must be rejected")
	}
	if err := m.SetLimit(5); err != nil {
		t.Fatalf("limit 5: %v", err)
	}
	if m.Limit() != 5 {
		t.Fatalf("limit=%d", m.Limit())
	}
}

func TestParallelLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(1)
	m.Start(ctx)

	a, b := newFakeTask("a"), newFakeTask("b")
	m.Submit(a)
	waitFor(t, func() bool { return a.isStarted() }, "a started")
	m.Submit(b)
	time.Sleep(150 * time.Millisecond)
	if b.isStarted() {
		t.Fatal("b must wait while limit is 1")
	}
	if view := m.View(); view.Active != 1 || view.Pending != 1 {
		t.Fatalf("view=%+v", view)
	}

	a.finish()
	waitFor(t, func() bool { return b.isStarted() }, "b started after a finished")
}

func TestParallelLimitTwo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(2)
	m.Start(ctx)
	a, b, c := newFakeTask("a"), newFakeTask("b"), newFakeTask("c")
	m.Submit(a)
	m.Submit(b)
	m.Submit(c)
	waitFor(t, func() bool { return a.isStarted() && b.isStarted() }, "two started")
	time.Sleep(100 * time.Millisecond)
	if c.isStarted() {
		t.Fatal("third must wait")
	}
	a.finish()
	waitFor(t, func() bool { return c.isStarted() }, "third starts")
}

func TestQueuePauseResume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(1)
	m.Start(ctx)
	a := newFakeTask("a")
	m.Submit(a)
	waitFor(t, func() bool { return a.isStarted() }, "a started")

	m.Pause()
	if !m.Paused() {
		t.Fatal("queue must be paused")
	}
	a.mu.Lock()
	paused := a.paused
	a.mu.Unlock()
	if !paused {
		t.Fatal("active task must be paused")
	}
	if st := m.Status("a"); st != StatusPaused {
		t.Fatalf("status=%s", st)
	}

	m.Resume()
	a.mu.Lock()
	paused = a.paused
	a.mu.Unlock()
	if paused {
		t.Fatal("resume must release the task")
	}
	if st := m.Status("a"); st != StatusRunning {
		t.Fatalf("status=%s", st)
	}
	a.finish()
}

func TestPauseStopsNewTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(2)
	m.Start(ctx)
	m.Pause()
	a := newFakeTask("a")
	m.Submit(a)
	time.Sleep(150 * time.Millisecond)
	if a.isStarted() {
		t.Fatal("no task may start while paused")
	}
	m.Resume()
	waitFor(t, func() bool { return a.isStarted() }, "starts after resume")
	a.finish()
}

func TestReorder(t *testing.T) {
	m := NewManager(1)
	a, b, c := newFakeTask("a"), newFakeTask("b"), newFakeTask("c")
	m.Submit(a)
	m.Submit(b)
	m.Submit(c)
	if err := m.Reorder([]string{"c", "a"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	view := m.View()
	got := []string{view.Items[0].ID, view.Items[1].ID, view.Items[2].ID}
	if got[0] != "c" || got[1] != "a" || got[2] != "b" {
		t.Fatalf("order=%v", got)
	}
}

func TestReorderIgnoresUnknown(t *testing.T) {
	m := NewManager(1)
	a := newFakeTask("a")
	m.Submit(a)
	_ = m.Reorder([]string{"ghost", "a"})
	if view := m.View(); len(view.Items) != 1 || view.Items[0].ID != "a" {
		t.Fatalf("view=%+v", view)
	}
}

func TestRemove(t *testing.T) {
	m := NewManager(1)
	a := newFakeTask("a")
	m.Submit(a)
	m.Remove("a")
	if view := m.View(); len(view.Items) != 0 {
		t.Fatalf("view=%+v", view)
	}
}

func TestSetLimitDoesNotKillRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(3)
	m.Start(ctx)
	a, b := newFakeTask("a"), newFakeTask("b")
	m.Submit(a)
	m.Submit(b)
	waitFor(t, func() bool { return a.isStarted() && b.isStarted() }, "both started")
	if err := m.SetLimit(1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !m.IsActive("a") || !m.IsActive("b") {
		t.Fatal("running tasks must not be interrupted")
	}
	c := newFakeTask("c")
	m.Submit(c)
	time.Sleep(150 * time.Millisecond)
	if c.isStarted() {
		t.Fatal("new task must wait until active count drops below limit")
	}
	a.finish()
	b.finish()
	waitFor(t, func() bool { return c.isStarted() }, "c starts after slots free")
}

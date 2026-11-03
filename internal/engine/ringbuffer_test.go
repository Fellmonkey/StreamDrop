package engine

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestRingBufferSmallWriteRead(t *testing.T) {
	rb := NewRingBuffer(8)
	if _, err := rb.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rb.UsedBytes() != 5 {
		t.Fatalf("used=%d, want 5", rb.UsedBytes())
	}
	buf := make([]byte, 3)
	n, err := rb.Read(buf)
	if err != nil || string(buf[:n]) != "hel" {
		t.Fatalf("read = %q, %v", buf[:n], err)
	}
}

// TestRingBufferWrap verifies data integrity across the wrap point, which is
// exactly where pointer arithmetic (instead of the used counter) breaks.
func TestRingBufferWrap(t *testing.T) {
	rb := NewRingBuffer(8)
	// Fill 6 bytes, drain 5 => readPtr=5, writePtr=6, used=1.
	mustWrite(t, rb, []byte("abcdef"))
	drain(t, rb, 5)
	mustWrite(t, rb, []byte("ghijkl")) // wraps: 1 + 6 = 7 used
	got := drain(t, rb, 7)
	if string(got) != "fghijkl" {
		t.Fatalf("got %q, want %q", got, "fghijkl")
	}
	if rb.UsedBytes() != 0 {
		t.Fatalf("used=%d, want 0", rb.UsedBytes())
	}
	if rb.UsagePercentage() != 0 {
		t.Fatalf("usage=%v", rb.UsagePercentage())
	}
}

func mustWrite(t *testing.T, rb *RingBuffer, p []byte) {
	t.Helper()
	done := make(chan struct{})
	var n int
	var err error
	go func() {
		n, err = rb.Write(p)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("write blocked")
	}
	if err != nil || n != len(p) {
		t.Fatalf("write %q = %d, %v", p, n, err)
	}
}

func drain(t *testing.T, rb *RingBuffer, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	read := 0
	for read < n {
		m, err := rb.Read(buf[read:])
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		read += m
	}
	return buf
}

func TestRingBufferCloseSemantics(t *testing.T) {
	rb := NewRingBuffer(4)
	cause := errors.New("source died")
	if _, err := rb.Write([]byte("ab")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := rb.Close(cause); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Reader drains remaining bytes, then EOF.
	buf := make([]byte, 4)
	n, err := rb.Read(buf)
	if err != nil || string(buf[:n]) != "ab" {
		t.Fatalf("read after close = %q, %v", buf[:n], err)
	}
	if _, err := rb.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
	// Writer gets the close cause.
	if _, err := rb.Write([]byte("x")); !errors.Is(err, cause) {
		t.Fatalf("expected cause, got %v", err)
	}
	// Double close is harmless.
	if err := rb.Close(nil); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestRingBufferDefaultCloseCause(t *testing.T) {
	rb := NewRingBuffer(4)
	_ = rb.Close(nil)
	if _, err := rb.Write([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestRingBufferBackpressure(t *testing.T) {
	rb := NewRingBuffer(4)
	mustWrite(t, rb, []byte("1234"))

	done := make(chan int, 1)
	go func() {
		n, _ := rb.Write([]byte("56"))
		done <- n
	}()

	select {
	case <-done:
		t.Fatal("write should block while the buffer is full")
	case <-time.After(100 * time.Millisecond):
	}

	if got := string(drain(t, rb, 2)); got != "12" {
		t.Fatalf("drained %q", got)
	}
	select {
	case n := <-done:
		if n != 2 {
			t.Fatalf("unblocked write n=%d, want 2", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write did not resume")
	}
}

func TestRingBufferCloseUnblocksWriter(t *testing.T) {
	rb := NewRingBuffer(2)
	mustWrite(t, rb, []byte("ab"))
	cause := errors.New("aborted")
	errCh := make(chan error, 1)
	go func() {
		_, err := rb.Write([]byte("c"))
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = rb.Close(cause)
	select {
	case err := <-errCh:
		if !errors.Is(err, cause) {
			t.Fatalf("expected cause, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writer stayed blocked after close")
	}
}

func TestRingBufferCloseUnblocksReader(t *testing.T) {
	rb := NewRingBuffer(2)
	errCh := make(chan error, 1)
	go func() {
		_, err := rb.Read(make([]byte, 2))
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = rb.Close(nil)
	select {
	case err := <-errCh:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected EOF, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reader stayed blocked after close")
	}
}

func TestRingBufferLargeTransfer(t *testing.T) {
	rb := NewRingBuffer(1024)
	payload := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	var wg sync.WaitGroup
	wg.Add(1)
	var got []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, 777)
		for {
			n, err := rb.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				return
			}
		}
	}()
	if _, err := rb.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = rb.Close(nil)
	wg.Wait()
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestNewRingBufferSizeInvariant(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for non-positive size")
		}
	}()
	NewRingBuffer(0)
}

func TestAllowedBufferSizes(t *testing.T) {
	want := []int{32, 64, 128, 256, 512, 1024}
	if len(AllowedBufferSizes) != len(want) {
		t.Fatalf("sizes=%v", AllowedBufferSizes)
	}
	for i := range want {
		if AllowedBufferSizes[i] != want[i] {
			t.Fatalf("sizes=%v", AllowedBufferSizes)
		}
	}
}

func TestPipeBufferMode(t *testing.T) {
	b := NewStreamBuffer("pure_stream", 0)
	if b.Size() != 0 || b.UsedBytes() != 0 || b.UsagePercentage() != 0 {
		t.Fatal("pure stream must not report a pool")
	}
	go func() {
		_, _ = b.Write([]byte("streamed"))
		_ = b.CloseWithCause(nil)
	}()
	data, err := io.ReadAll(b)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "streamed" {
		t.Fatalf("got %q", data)
	}
}

func TestNewStreamBufferDefaults(t *testing.T) {
	b := NewStreamBuffer("ring_buffer", 0)
	if b.Size() != 128<<20 {
		t.Fatalf("default size=%d", b.Size())
	}
	if _, ok := b.(*RingBuffer); !ok {
		t.Fatal("expected ring buffer")
	}
}

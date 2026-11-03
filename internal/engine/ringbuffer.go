package engine

import (
	"errors"
	"io"
	"sync"
)

// ErrClosed is returned to a writer after the buffer is closed.
var ErrClosed = errors.New("ringbuffer: closed")

// RingBuffer is a blocking in-memory ring buffer. It implements io.Reader and
// io.Writer and provides explicit close semantics through Close(cause).
//
// State is described by readPtr, writePtr and used, where used is the exact
// number of unread bytes. No size is ever derived from pointer positions: that
// arithmetic is wrong once the buffer wraps and yields stale bytes.
type RingBuffer struct {
	buf  []byte
	size int

	readPtr  int
	writePtr int
	used     int

	closed bool
	err    error

	mu        sync.Mutex
	condRead  *sync.Cond
	condWrite *sync.Cond
}

// NewRingBuffer allocates a buffer of sizeInBytes.
func NewRingBuffer(sizeInBytes int) *RingBuffer {
	if sizeInBytes <= 0 {
		panic("ringbuffer: buffer size must be positive")
	}
	rb := &RingBuffer{buf: make([]byte, sizeInBytes), size: sizeInBytes}
	rb.condRead = sync.NewCond(&rb.mu)
	rb.condWrite = sync.NewCond(&rb.mu)
	return rb
}

// Write copies p into the buffer and blocks while it is full. Blocking here is
// the backpressure mechanism: the download stalls until the cloud reader frees
// memory.
func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	written := 0
	for len(p) > 0 {
		for rb.used == rb.size && !rb.closed {
			rb.condWrite.Wait()
		}
		if rb.closed {
			return written, rb.err
		}

		free := rb.size - rb.used
		chunk := min(len(p), free)

		first := min(chunk, rb.size-rb.writePtr)
		copy(rb.buf[rb.writePtr:rb.writePtr+first], p[:first])
		if rest := chunk - first; rest > 0 {
			copy(rb.buf[0:rest], p[first:chunk])
		}

		rb.writePtr = (rb.writePtr + chunk) % rb.size
		rb.used += chunk
		written += chunk
		p = p[chunk:]

		rb.condRead.Signal()
	}
	return written, nil
}

// Read copies buffered bytes into p. It returns io.EOF once the buffer is
// closed and drained.
func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for rb.used == 0 && !rb.closed {
		rb.condRead.Wait()
	}
	if rb.used == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}

	chunk := min(len(p), rb.used)

	first := min(chunk, rb.size-rb.readPtr)
	copy(p[:first], rb.buf[rb.readPtr:rb.readPtr+first])
	if rest := chunk - first; rest > 0 {
		copy(p[first:chunk], rb.buf[0:rest])
	}

	rb.readPtr = (rb.readPtr + chunk) % rb.size
	rb.used -= chunk

	rb.condWrite.Signal()
	return chunk, nil
}

// Close ends the stream: the writer receives cause (ErrClosed by default), and
// the reader gets io.EOF after draining what is left.
func (rb *RingBuffer) Close(cause error) error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return nil
	}
	rb.closed = true
	rb.err = cause
	if rb.err == nil {
		rb.err = ErrClosed
	}
	rb.condRead.Broadcast()
	rb.condWrite.Broadcast()
	return nil
}

// CloseWithCause mirrors Close for the shared StreamBuffer interface.
func (rb *RingBuffer) CloseWithCause(cause error) error { return rb.Close(cause) }

// UsedBytes reports how much data is waiting to be read.
func (rb *RingBuffer) UsedBytes() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.used
}

// Size returns the pool size in bytes.
func (rb *RingBuffer) Size() int { return rb.size }

// UsagePercentage returns the fill percentage.
func (rb *RingBuffer) UsagePercentage() float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return (float64(rb.used) / float64(rb.size)) * 100.0
}

// StreamBuffer is the common buffer contract for both transfer modes. The
// pipeline never knows which implementation it holds.
type StreamBuffer interface {
	io.Reader
	io.Writer
	CloseWithCause(error) error
	UsedBytes() int
	Size() int
	UsagePercentage() float64
}

// pipeBuffer adapts io.Pipe to StreamBuffer: a pure stream allocates no pool.
type pipeBuffer struct {
	r *io.PipeReader
	w *io.PipeWriter
}

// NewPipeBuffer creates a pure-stream buffer.
func NewPipeBuffer() StreamBuffer {
	r, w := io.Pipe()
	return &pipeBuffer{r: r, w: w}
}

func (p *pipeBuffer) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeBuffer) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeBuffer) CloseWithCause(err error) error {
	return p.w.CloseWithError(err)
}
func (p *pipeBuffer) UsedBytes() int           { return 0 }
func (p *pipeBuffer) Size() int                { return 0 }
func (p *pipeBuffer) UsagePercentage() float64 { return 0 }

// AllowedBufferSizes is the pool size list offered by the UI (MiB).
var AllowedBufferSizes = []int{32, 64, 128, 256, 512, 1024}

// NewStreamBuffer returns the buffer for a transfer mode.
func NewStreamBuffer(mode string, sizeBytes int) StreamBuffer {
	if mode == "pure_stream" {
		return NewPipeBuffer()
	}
	if sizeBytes <= 0 {
		sizeBytes = 128 << 20
	}
	return NewRingBuffer(sizeBytes)
}

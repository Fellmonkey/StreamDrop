// Package logging writes the application journal to a file with size-based
// rotation. A Windows GUI build has no console, so the file is the primary log;
// a bounded in-memory tail is exposed to the UI and to crash bundles.
package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxBytes = 10 << 20 // 10 MiB
	defaultKeep     = 5
	ringLines       = 500
)

// Logger is a small leveled logger with rotation.
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	maxBytes int64
	keep     int
	size     int64
	ring     []string
	level    int
	also     []io.Writer
}

var levels = map[string]int{"debug": 10, "info": 20, "warn": 30, "error": 40}

// New creates the logger writing to dir/streamdrop.log.
func New(dir, level string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{
		path:     filepath.Join(dir, "streamdrop.log"),
		maxBytes: defaultMaxBytes,
		keep:     defaultKeep,
		level:    levels[strings.ToLower(level)],
	}
	if l.level == 0 {
		l.level = levels["info"]
	}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.file = f
	l.size = st.Size()
	return nil
}

// Attach adds a secondary writer (the terminal in development builds).
func (l *Logger) Attach(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.also = append(l.also, w)
}

// Path returns the journal path.
func (l *Logger) Path() string { return l.path }

// Close flushes and closes the journal.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Logger) logf(level, format string, args ...any) {
	if levels[level] < l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %-5s %s\n", time.Now().Format("2006-01-02T15:04:05.000Z07:00"), strings.ToUpper(level), msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.ring = append(l.ring, strings.TrimRight(line, "\n"))
	if len(l.ring) > ringLines {
		l.ring = l.ring[len(l.ring)-ringLines:]
	}
	for _, w := range l.also {
		_, _ = io.WriteString(w, line)
	}
	if l.file == nil {
		return
	}
	n, err := l.file.WriteString(line)
	if err != nil {
		return
	}
	l.size += int64(n)
	if l.size >= l.maxBytes {
		l.rotateLocked()
	}
}

func (l *Logger) rotateLocked() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	// streamdrop.log -> streamdrop.log.1 -> ... -> streamdrop.log.N
	for i := l.keep - 1; i >= 1; i-- {
		from := l.path
		if i > 1 {
			from = fmt.Sprintf("%s.%d", l.path, i-1)
		}
		to := fmt.Sprintf("%s.%d", l.path, i)
		_ = os.Rename(from, to)
	}
	_ = l.open()
}

// Debug logs at debug level.
func (l *Logger) Debug(format string, args ...any) { l.logf("debug", format, args...) }

// Info logs at info level.
func (l *Logger) Info(format string, args ...any) { l.logf("info", format, args...) }

// Warn logs at warn level.
func (l *Logger) Warn(format string, args ...any) { l.logf("warn", format, args...) }

// Error logs at error level.
func (l *Logger) Error(format string, args ...any) { l.logf("error", format, args...) }

// Tail returns the last n journal lines.
func (l *Logger) Tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.ring) {
		n = len(l.ring)
	}
	out := make([]string, n)
	copy(out, l.ring[len(l.ring)-n:])
	return out
}

// Write lets the logger be used as an io.Writer (for the std log package and
// subprocess stderr scanners).
func (l *Logger) Write(p []byte) (int, error) {
	sc := bufio.NewScanner(strings.NewReader(string(p)))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			l.logf("info", "%s", line)
		}
	}
	return len(p), nil
}

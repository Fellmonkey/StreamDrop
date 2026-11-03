package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// TestPipelineHelperProcess is not a real test: it is the fake curl/rclone
// child used by the pipeline tests. The test binary re-executes itself so the
// behaviour is identical on Windows and POSIX.
func TestPipelineHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PIPELINE_HELPER") != "1" {
		return
	}
	kind := os.Getenv("HELPER_KIND")
	bytes, _ := strconv.ParseInt(os.Getenv("HELPER_BYTES"), 10, 64)

	if kind == "rclone" {
		received, _ := io.Copy(io.Discard, os.Stdin)
		fmt.Fprintf(os.Stderr, "Transferred: %d Bytes\n", received)
		os.Exit(0)
	}

	if os.Getenv("HELPER_MODE") == "fail" {
		writePattern(os.Stdout, bytes/2)
		os.Exit(2)
	}
	writePattern(os.Stdout, bytes)
	os.Exit(0)
}

func writePattern(w io.Writer, total int64) {
	chunk := make([]byte, 64<<10)
	for i := range chunk {
		chunk[i] = byte('a' + i%26)
	}
	for written := int64(0); written < total; {
		n := int64(len(chunk))
		if remaining := total - written; remaining < n {
			n = remaining
		}
		if _, err := w.Write(chunk[:n]); err != nil {
			return
		}
		written += n
	}
}

func helperTools(size int64, failSource bool) Tools {
	return Tools{
		Rclone: "fake-rclone",
		Curl:   "fake-curl",
		Command: func(kind string, _ StreamSpec, _ int64, _ Tools) *exec.Cmd {
			mode := "ok"
			if kind != "rclone" && failSource {
				mode = "fail"
			}
			cmd := exec.Command(os.Args[0], "-test.run=TestPipelineHelperProcess")
			cmd.Env = append(os.Environ(),
				"GO_WANT_PIPELINE_HELPER=1",
				"HELPER_KIND="+kind,
				"HELPER_BYTES="+strconv.FormatInt(size, 10),
				"HELPER_MODE="+mode,
			)
			return cmd
		},
	}
}

func expectedHash(total int64) string {
	h := sha256.New()
	chunk := make([]byte, 64<<10)
	for i := range chunk {
		chunk[i] = byte('a' + i%26)
	}
	for written := int64(0); written < total; {
		n := int64(len(chunk))
		if remaining := total - written; remaining < n {
			n = remaining
		}
		_, _ = h.Write(chunk[:n])
		written += n
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestPipelineZeroDiskStream verifies the Stage 1 guarantee: every source byte
// reaches the uploader without any payload file appearing on disk, and the
// in-flight SHA-256 matches the source.
func TestPipelineZeroDiskStream(t *testing.T) {
	const size = 3<<20 + 17
	dir := t.TempDir()
	t.Chdir(dir)

	p := NewPipeline(helperTools(size, false))
	p.DefaultMaxRetries = 0
	spec := StreamSpec{
		ID:              "t1",
		URL:             "https://example.invalid/payload.bin",
		Mode:            ModePureStream,
		Engine:          EngineCurl,
		RemoteTarget:    "sd_test:/payload.bin",
		ChecksumEnabled: true,
		MaxRetries:      0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := p.Run(ctx, spec, NewTaskState("running", 0), Hooks{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Bytes != size {
		t.Fatalf("bytes=%d want %d", res.Bytes, size)
	}
	if res.Confirmed != size {
		t.Fatalf("confirmed=%d want %d", res.Confirmed, size)
	}
	if want := expectedHash(size); res.Sha256 != want {
		t.Fatalf("sha256=%s want %s", res.Sha256, want)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("payload created files on disk: %v", names)
	}
}

// TestPipelineRingBufferStream exercises the ring buffer end-to-end.
func TestPipelineRingBufferStream(t *testing.T) {
	const size = 8 << 20
	p := NewPipeline(helperTools(size, false))
	spec := StreamSpec{
		ID:           "t2",
		URL:          "https://example.invalid/payload.bin",
		Mode:         ModeRingBuffer,
		BufferBytes:  32 << 20,
		Engine:       EngineCurl,
		RemoteTarget: "sd_test:/payload.bin",
		MaxRetries:   0,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := p.Run(ctx, spec, NewTaskState("running", 0), Hooks{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Bytes != size || res.Confirmed != size {
		t.Fatalf("bytes=%d confirmed=%d want %d", res.Bytes, res.Confirmed, size)
	}
}

// TestPipelineNoRangeRestart verifies that a source without Range support
// forces a full restart instead of silently appending a broken object.
func TestPipelineNoRangeRestart(t *testing.T) {
	const size = 1 << 20
	p := NewPipeline(helperTools(size, true))
	spec := StreamSpec{
		ID:            "t3",
		URL:           "https://example.invalid/payload.bin",
		Mode:          ModePureStream,
		Engine:        EngineCurl,
		RemoteTarget:  "sd_test:/payload.bin",
		SupportsRange: false,
		MaxRetries:    1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := p.Run(ctx, spec, NewTaskState("running", 0), Hooks{})
	if !errors.Is(err, ErrNeedsRestart) {
		t.Fatalf("err=%v want ErrNeedsRestart", err)
	}
}

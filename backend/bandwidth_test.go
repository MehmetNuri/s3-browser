package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestBandwidthLimiterPacesReads(t *testing.T) {
	var limiter bandwidthLimiter
	limiter.setLimit(1 << 20) // 1 MB/s
	data := bytes.Repeat([]byte("x"), 512<<10)
	pr := &progressReader{r: bytes.NewReader(data), emit: func(int64) {}, limiter: &limiter, ctx: context.Background()}
	start := time.Now()
	if n, err := io.Copy(io.Discard, pr); err != nil || n != int64(len(data)) {
		t.Fatalf("copy: %d, %v", n, err)
	}
	// Half a megabyte at one megabyte per second needs about half a second.
	if elapsed := time.Since(start); elapsed < 350*time.Millisecond || elapsed > 6*time.Second {
		t.Fatalf("512 KB took %v", elapsed)
	}
	limiter.setLimit(0)
	start = time.Now()
	pr = &progressReader{r: bytes.NewReader(data), emit: func(int64) {}, limiter: &limiter, ctx: context.Background()}
	_, _ = io.Copy(io.Discard, pr)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("unlimited copy took %v", elapsed)
	}
	// A cancelled transfer must not wait for its slot.
	limiter.setLimit(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pr = &progressReader{r: bytes.NewReader(data[:4096]), emit: func(int64) {}, limiter: &limiter, ctx: ctx}
	start = time.Now()
	_, _ = io.Copy(io.Discard, pr)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled read waited %v", elapsed)
	}
}

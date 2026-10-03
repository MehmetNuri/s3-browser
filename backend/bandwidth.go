package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// bandwidthLimiter spreads the bytes of every running transfer over time so
// their sum stays below the configured rate. It is a simple leaky bucket:
// each read reserves the moment the next read may start.
type bandwidthLimiter struct {
	mu    sync.Mutex
	limit int64 // bytes per second, 0 means unlimited
	next  time.Time
}

const maxBandwidthLimit = 10 << 30 // 10 GB/s, well above any real link

func (l *bandwidthLimiter) setLimit(limit int64) {
	l.mu.Lock()
	l.limit = limit
	l.next = time.Time{}
	l.mu.Unlock()
}

// wait blocks until n more bytes may pass, or the context ends.
func (l *bandwidthLimiter) wait(ctx context.Context, n int) {
	l.mu.Lock()
	if l.limit <= 0 || n <= 0 {
		l.mu.Unlock()
		return
	}
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	delay := l.next.Sub(now)
	l.next = l.next.Add(time.Duration(float64(n) / float64(l.limit) * float64(time.Second)))
	l.mu.Unlock()
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// SetBandwidthLimit caps the total transfer rate in kilobytes per second; zero removes the cap.
func (a *App) SetBandwidthLimit(kbps int64) error {
	if kbps < 0 || kbps > maxBandwidthLimit/1024 {
		return errors.New(T("invalidBandwidth"))
	}
	a.mu.Lock()
	settings := a.settings
	settings.BandwidthKBps = kbps
	err := a.saveSettingsLocked(settings)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.bandwidth.setLimit(kbps * 1024)
	a.transfersMu.Lock()
	a.queueBandwidth = kbps
	a.transfersMu.Unlock()
	a.emitQueue()
	return nil
}

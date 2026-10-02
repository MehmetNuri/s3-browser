package main

import (
	"context"
	"errors"
	"sync"
)

// BatchEvent reports the progress of one user operation that spans several
// transfers, so the desktop host can show "12 of 340" instead of only the
// few transfers running at this moment.
type BatchEvent struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"` // upload | download | copy | move | sync
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Failed    int    `json:"failed"`
	Cancelled int    `json:"cancelled"`
	Active    bool   `json:"active"`
}

type batch struct {
	app   *App
	mu    sync.Mutex
	ev    BatchEvent
	first error // of the first transfer that failed
}

// startBatch announces an operation. A total of zero means it is still
// being prepared, for example while a sync compares both sides.
func (a *App) startBatch(kind string, total int) *batch {
	b := &batch{app: a, ev: BatchEvent{ID: a.nextID.Add(1), Kind: kind, Total: total, Active: true}}
	b.emit()
	return b
}

func (b *batch) emit() {
	b.mu.Lock()
	ev := b.ev
	b.mu.Unlock()
	b.app.emitEvent("batch", ev)
}

func (b *batch) setTotal(total int) {
	b.mu.Lock()
	b.ev.Total = total
	b.mu.Unlock()
	b.emit()
}

// finish records the result of one transfer of the batch.
func (b *batch) finish(err error) {
	b.mu.Lock()
	switch {
	case err == nil:
		b.ev.Done++
	case errors.Is(err, context.Canceled):
		b.ev.Cancelled++
	default:
		b.ev.Failed++
		if b.first == nil {
			b.first = err
		}
	}
	b.mu.Unlock()
	b.emit()
}

// failure describes how a batch failed: the reason of the first failed
// transfer, with the count in front when several were involved.
func (b *batch) failure(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ev.Failed == 0 && b.ev.Cancelled == 0 {
		return nil
	}
	if b.first == nil {
		return errors.New(T(key, b.ev.Failed+b.ev.Cancelled))
	}
	reason := describeErr(b.first).Error()
	if b.ev.Total <= 1 {
		return errors.New(reason)
	}
	return errors.New(T(key, b.ev.Failed+b.ev.Cancelled) + ": " + reason)
}

func (b *batch) end() {
	b.mu.Lock()
	b.ev.Active = false
	b.mu.Unlock()
	b.emit()
}

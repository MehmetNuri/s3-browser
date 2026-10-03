package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTransferCancellation(t *testing.T) {
	a := &App{ctx: context.Background()}
	started := make(chan int64, 1)
	var mu sync.Mutex
	var last TransferEvent
	a.emitEvent = func(name string, data any) {
		ev, ok := data.(TransferEvent)
		if !ok {
			return
		}
		mu.Lock()
		last = ev
		mu.Unlock()
		if ev.State == "running" {
			started <- ev.ID
		}
	}
	finished := make(chan error, 1)
	go func() {
		finished <- a.startTransfer("upload", "file", func(ctx context.Context, _ func(int64, int64)) error { <-ctx.Done(); return ctx.Err() })
	}()
	var id int64
	select {
	case id = <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transfer did not start")
	}
	if err := a.CancelTransfer(id); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("transfer did not stop")
	}
	mu.Lock()
	event := last
	mu.Unlock()
	if event.State != "cancelled" || event.Error != "" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestTransferRetryAndClear(t *testing.T) {
	a := &App{ctx: context.Background()}
	var mu sync.Mutex
	var events []TransferEvent
	a.emitEvent = func(_ string, data any) {
		if ev, ok := data.(TransferEvent); ok {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}
	attempts := 0
	err := a.startTransfer("download", "file", func(_ context.Context, progress func(int64, int64)) error {
		attempts++
		if attempts == 1 {
			return errors.New("network failure")
		}
		progress(4, 4)
		return nil
	})
	if err == nil {
		t.Fatal("expected the first attempt to fail")
	}
	mu.Lock()
	id := events[0].ID
	mu.Unlock()
	if err := a.RetryTransfer(id); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	last := events[len(events)-1]
	mu.Unlock()
	if attempts != 2 || last.ID != id || last.State != "done" || last.Done != 4 || last.Error != "" {
		t.Fatalf("retry: %+v, attempts=%d", last, attempts)
	}
	if err := a.RetryTransfer(id); err == nil {
		t.Fatal("completed transfer was retried")
	}
	a.ClearTransfers()
	if err := a.RetryTransfer(id); err == nil {
		t.Fatal("cleared transfer was retried")
	}
}

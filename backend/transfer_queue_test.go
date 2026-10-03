package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// queueApp returns an app whose transfer events are recorded.
func queueApp(limit int) (*App, func() []TransferEvent, func() QueueEvent) {
	a := &App{ctx: context.Background()}
	a.queueLimit = limit
	var mu sync.Mutex
	var events []TransferEvent
	var queue QueueEvent
	a.emitEvent = func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		switch ev := data.(type) {
		case TransferEvent:
			events = append(events, ev)
		case QueueEvent:
			queue = ev
		}
	}
	return a, func() []TransferEvent { mu.Lock(); defer mu.Unlock(); return append([]TransferEvent(nil), events...) },
		func() QueueEvent { mu.Lock(); defer mu.Unlock(); return queue }
}

// blockingTransfer runs until released or cancelled.
func blockingTransfer(release chan struct{}) func(context.Context, func(int64, int64)) error {
	return func(ctx context.Context, _ func(int64, int64)) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQueueRunsLimitedNumberInOrder(t *testing.T) {
	a, _, queue := queueApp(2)
	release := make(chan struct{})
	var order []int64
	var mu sync.Mutex
	tasks := make([]*transferTask, 4)
	for i := range tasks {
		tasks[i] = a.enqueueTransfer("upload", "file", 10, func(ctx context.Context, _ func(int64, int64)) error {
			mu.Lock()
			order = append(order, int64(len(order)))
			mu.Unlock()
			return blockingTransfer(release)(ctx, nil)
		})
	}
	waitFor(t, "two running", func() bool { q := queue(); return q.Running == 2 && q.Queued == 2 })
	if got := a.GetTransferQueue(); got.Limit != 2 || got.Running != 2 || got.Queued != 2 {
		t.Fatalf("queue: %+v", got)
	}
	// The third task moves to the front and runs before the fourth once a slot frees.
	if err := a.PrioritizeTransfer(tasks[3].event.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, task := range tasks {
		if err := task.wait(); err != nil {
			t.Fatal(err)
		}
	}
	events := a.GetTransfers()
	if len(events) != 4 {
		t.Fatalf("history: %d", len(events))
	}
	for _, ev := range events {
		if ev.State != "done" || ev.Done != 10 || ev.Total != 10 {
			t.Fatalf("event: %+v", ev)
		}
	}
	if q := queue(); q.Running != 0 || q.Queued != 0 || q.Done != 4 {
		t.Fatalf("final queue: %+v", q)
	}
}

func TestQueuePrioritizeRunsFirst(t *testing.T) {
	a, _, queue := queueApp(1)
	release := make(chan struct{})
	first := a.enqueueTransfer("upload", "first", 0, blockingTransfer(release))
	var started []string
	var mu sync.Mutex
	record := func(name string) func(context.Context, func(int64, int64)) error {
		return func(context.Context, func(int64, int64)) error {
			mu.Lock()
			started = append(started, name)
			mu.Unlock()
			return nil
		}
	}
	a.enqueueTransfer("upload", "second", 0, record("second"))
	third := a.enqueueTransfer("upload", "third", 0, record("third"))
	waitFor(t, "running", func() bool { return queue().Running == 1 })
	if err := a.PrioritizeTransfer(third.event.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.PrioritizeTransfer(first.event.ID); err == nil {
		t.Fatal("running transfer was prioritized")
	}
	close(release)
	waitFor(t, "all done", func() bool { return queue().Done == 3 })
	mu.Lock()
	defer mu.Unlock()
	if len(started) != 2 || started[0] != "third" || started[1] != "second" {
		t.Fatalf("order: %v", started)
	}
}

func TestQueuePauseAndCancelQueued(t *testing.T) {
	a, _, queue := queueApp(1)
	a.PauseTransfers(true)
	release := make(chan struct{})
	task := a.enqueueTransfer("download", "a", 0, blockingTransfer(release))
	other := a.enqueueTransfer("download", "b", 0, blockingTransfer(release))
	time.Sleep(20 * time.Millisecond)
	if q := queue(); !q.Paused || q.Running != 0 || q.Queued != 2 {
		t.Fatalf("paused queue: %+v", q)
	}
	if n := a.CancelQueuedTransfers(); n != 2 {
		t.Fatalf("cancelled %d", n)
	}
	if err := task.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled task: %v", err)
	}
	if n := a.RetryFailedTransfers(); n != 2 {
		t.Fatalf("retried %d", n)
	}
	if q := queue(); q.Running != 0 || q.Queued != 2 {
		t.Fatalf("still paused: %+v", q)
	}
	a.PauseTransfers(false)
	waitFor(t, "one running", func() bool { q := queue(); return q.Running == 1 && q.Queued == 1 })
	close(release)
	waitFor(t, "done", func() bool { return queue().Done == 2 })
	if err := a.RemoveTransfer(other.event.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.GetTransfers()) != 1 {
		t.Fatal("transfer was not removed")
	}
}

func TestQueueCancelQueuedTransfer(t *testing.T) {
	a, events, _ := queueApp(1)
	release := make(chan struct{})
	a.enqueueTransfer("upload", "running", 0, blockingTransfer(release))
	waiting := a.enqueueTransfer("upload", "waiting", 0, blockingTransfer(release))
	waitFor(t, "queued event", func() bool {
		for _, ev := range events() {
			if ev.ID == waiting.event.ID && ev.State == "queued" {
				return true
			}
		}
		return false
	})
	if err := a.CancelTransfer(waiting.event.ID); err != nil {
		t.Fatal(err)
	}
	if err := waiting.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	close(release)
	if err := a.SetTransferLimit(0); err == nil {
		t.Fatal("limit 0 was accepted")
	}
}

func TestQueueReorder(t *testing.T) {
	a, _, queue := queueApp(1)
	release := make(chan struct{})
	a.enqueueTransfer("upload", "running", 0, blockingTransfer(release))
	var order []string
	var mu sync.Mutex
	record := func(name string) func(context.Context, func(int64, int64)) error {
		return func(context.Context, func(int64, int64)) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}
	first := a.enqueueTransfer("upload", "first", 0, record("first"))
	second := a.enqueueTransfer("upload", "second", 0, record("second"))
	third := a.enqueueTransfer("upload", "third", 0, record("third"))
	waitFor(t, "queued", func() bool { return queue().Queued == 3 })
	if err := a.ReorderTransfer(third.event.ID, second.event.ID); err != nil { // third before second
		t.Fatal(err)
	}
	if err := a.ReorderTransfer(first.event.ID, 0); err != nil { // first to the end
		t.Fatal(err)
	}
	if err := a.ReorderTransfer(first.event.ID, first.event.ID); err == nil {
		t.Fatal("reorder onto itself was accepted")
	}
	close(release)
	waitFor(t, "done", func() bool { return queue().Done == 4 })
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "third" || order[1] != "second" || order[2] != "first" {
		t.Fatalf("order: %v", order)
	}
}

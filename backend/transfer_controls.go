package main

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"
)

// Transfers of every operation share one queue: at most transferLimit of
// them run at a time, the rest wait in order and stay visible as "queued".
type transferTask struct {
	event  TransferEvent
	ctx    context.Context // of the running attempt
	cancel context.CancelFunc
	run    func(context.Context, func(int64, int64)) error
	seq    float64       // queue order; lower runs first
	done   chan struct{} // closed when the current attempt ends
	err    error         // result of the current attempt
	// Speed sampling of the current attempt.
	sampleAt   time.Time
	sampleDone int64
}

// QueueEvent summarizes the queue for the frontend and the tray.
type QueueEvent struct {
	Limit   int   `json:"limit"`
	Paused  bool  `json:"paused"`
	Queued  int   `json:"queued"`
	Running int   `json:"running"`
	Failed  int   `json:"failed"`
	Done    int   `json:"done"`
	Speed   int64 `json:"speed"` // bytes per second of all running transfers
	// Configured cap in kilobytes per second, 0 when unlimited.
	BandwidthKBps int64 `json:"bandwidthKBps"`
}

const (
	defaultTransferLimit = 4
	maxTransferLimit     = 16
	transferHistoryLimit = 500
)

func (t *transferTask) active() bool {
	return t.event.State == "running" || t.event.State == "queued"
}

// wait blocks until the current attempt of the task ended.
func (t *transferTask) wait() error {
	<-t.done
	return t.err
}

// enqueueTransfer registers a transfer and starts it when a slot is free.
// Total may be zero when the size is not known before the transfer starts.
func (a *App) enqueueTransfer(kind, name string, total int64, run func(context.Context, func(int64, int64)) error) *transferTask {
	return a.enqueue(kind, name, total, false, run)
}

// enqueueQuiet registers a transfer that the desktop must not announce.
func (a *App) enqueueQuiet(kind, name string, total int64, run func(context.Context, func(int64, int64)) error) *transferTask {
	return a.enqueue(kind, name, total, true, run)
}

func (a *App) enqueue(kind, name string, total int64, quiet bool, run func(context.Context, func(int64, int64)) error) *transferTask {
	id := a.nextID.Add(1)
	task := &transferTask{
		event: TransferEvent{ID: id, Kind: kind, Name: name, Total: total, State: "queued", Order: float64(id), Quiet: quiet},
		run:   run, seq: float64(id), done: make(chan struct{}),
	}
	a.transfersMu.Lock()
	if a.transferJobs == nil {
		a.transferJobs = make(map[int64]*transferTask)
	}
	// Scanning the history on every enqueue would be quadratic for a large folder.
	a.enqueuedSinceEvict++
	if a.enqueuedSinceEvict >= transferHistoryLimit {
		a.enqueuedSinceEvict = 0
		a.evictFinishedLocked()
	}
	a.transferJobs[id] = task
	event := task.event
	a.transfersMu.Unlock()
	a.emit(event)
	a.pumpTransfers()
	return task
}

// startTransfer queues a transfer and waits for its result.
func (a *App) startTransfer(kind, name string, run func(context.Context, func(int64, int64)) error) error {
	return a.enqueueTransfer(kind, name, 0, run).wait()
}

// evictFinishedLocked keeps the history of finished transfers bounded.
func (a *App) evictFinishedLocked() {
	var finished []*transferTask
	for _, task := range a.transferJobs {
		if !task.active() {
			finished = append(finished, task)
		}
	}
	if len(finished) < transferHistoryLimit {
		return
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].event.ID < finished[j].event.ID })
	for _, task := range finished[:len(finished)-transferHistoryLimit+1] {
		delete(a.transferJobs, task.event.ID)
	}
}

// transferLimit reads the configured concurrency; the copy under transfersMu
// avoids touching a.settings from the queue.
func (a *App) transferLimit() int {
	a.transfersMu.Lock()
	defer a.transfersMu.Unlock()
	return a.transferLimitLocked()
}

// pumpTransfers starts queued transfers while slots are free.
func (a *App) pumpTransfers() {
	limit := a.transferLimit()
	a.transfersMu.Lock()
	var started []*transferTask
	for !a.transfersPaused && a.transfersRunning < limit {
		var next *transferTask
		for _, task := range a.transferJobs {
			if task.event.State == "queued" && (next == nil || task.seq < next.seq) {
				next = task
			}
		}
		if next == nil {
			break
		}
		next.ctx, next.cancel = context.WithCancel(a.ctx)
		next.event.State, next.event.Error, next.event.Speed = "running", "", 0
		next.event.Done = 0
		next.sampleAt, next.sampleDone = time.Now(), 0
		a.transfersRunning++
		started = append(started, next)
	}
	queue := a.queueEventLocked()
	a.transfersMu.Unlock()
	for _, task := range started {
		a.emit(task.event)
		go a.executeTransfer(task)
	}
	a.emitEvent("queue", queue)
}

// executeTransfer runs one attempt of a task that pumpTransfers marked running.
func (a *App) executeTransfer(task *transferTask) {
	a.transfersMu.Lock()
	ctx := task.ctx
	a.transfersMu.Unlock()
	err := task.run(ctx, func(done, total int64) {
		a.transfersMu.Lock()
		task.event.Done, task.event.Total = done, total
		if now := time.Now(); now.Sub(task.sampleAt) >= 500*time.Millisecond {
			task.event.Speed = int64(float64(done-task.sampleDone) / now.Sub(task.sampleAt).Seconds())
			task.sampleAt, task.sampleDone = now, done
		}
		event := task.event
		a.transfersMu.Unlock()
		a.emit(event)
	})
	cancelled := errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
	a.transfersMu.Lock()
	task.cancel()
	task.ctx, task.cancel = nil, nil
	task.event.Speed = 0
	switch {
	case err == nil:
		task.event.State, task.event.Done = "done", task.event.Total
	case cancelled:
		task.event.State, task.event.Error = "cancelled", ""
		err = context.Canceled
	default:
		task.event.State, task.event.Error = "error", describeErr(err).Error()
	}
	task.err = err
	event := task.event
	a.transfersRunning--
	close(task.done)
	a.transfersMu.Unlock()
	a.emit(event)
	a.pumpTransfers()
}

func (a *App) queueEventLocked() QueueEvent {
	ev := QueueEvent{Limit: a.transferLimitLocked(), Paused: a.transfersPaused, BandwidthKBps: a.queueBandwidth}
	for _, task := range a.transferJobs {
		switch task.event.State {
		case "queued":
			ev.Queued++
		case "running":
			ev.Running++
			ev.Speed += task.event.Speed
		case "error", "cancelled":
			ev.Failed++
		case "done":
			ev.Done++
		}
	}
	return ev
}

func (a *App) transferLimitLocked() int {
	if a.queueLimit < 1 || a.queueLimit > maxTransferLimit {
		return defaultTransferLimit
	}
	return a.queueLimit
}

func (a *App) emitQueue() {
	a.transfersMu.Lock()
	queue := a.queueEventLocked()
	a.transfersMu.Unlock()
	a.emitEvent("queue", queue)
}

// GetTransfers returns every known transfer, oldest first.
func (a *App) GetTransfers() []TransferEvent {
	a.transfersMu.Lock()
	defer a.transfersMu.Unlock()
	events := make([]TransferEvent, 0, len(a.transferJobs))
	for _, task := range a.transferJobs {
		events = append(events, task.event)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	return events
}

func (a *App) GetTransferQueue() QueueEvent {
	a.transfersMu.Lock()
	defer a.transfersMu.Unlock()
	return a.queueEventLocked()
}

// CancelTransfer stops a running transfer or drops a queued one.
func (a *App) CancelTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	if task.event.State == "queued" {
		a.cancelQueuedLocked(task)
		event := task.event
		queue := a.queueEventLocked()
		a.transfersMu.Unlock()
		a.emit(event)
		a.emitEvent("queue", queue)
		return nil
	}
	cancel := task.cancel
	a.transfersMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (a *App) cancelQueuedLocked(task *transferTask) {
	task.event.State, task.event.Error = "cancelled", ""
	task.err = context.Canceled
	close(task.done)
}

// CancelQueuedTransfers drops every waiting transfer; running ones continue.
func (a *App) CancelQueuedTransfers() int {
	a.transfersMu.Lock()
	var events []TransferEvent
	for _, task := range a.transferJobs {
		if task.event.State == "queued" {
			a.cancelQueuedLocked(task)
			events = append(events, task.event)
		}
	}
	queue := a.queueEventLocked()
	a.transfersMu.Unlock()
	for _, event := range events {
		a.emit(event)
	}
	a.emitEvent("queue", queue)
	return len(events)
}

// requeueLocked puts a failed or cancelled task back into the queue.
func (a *App) requeueLocked(task *transferTask) {
	task.event.State, task.event.Error, task.event.Done, task.event.Speed = "queued", "", 0, 0
	task.seq = float64(a.nextID.Add(1))
	task.event.Order = task.seq
	task.done = make(chan struct{})
	task.err = nil
}

// RetryTransfer queues a failed or cancelled transfer again and waits for it.
// The closure keeps the original client and destination, even if profiles change.
func (a *App) RetryTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	if task.active() {
		a.transfersMu.Unlock()
		return errors.New(T("transferAlreadyRunning"))
	}
	if task.event.State != "error" && task.event.State != "cancelled" {
		a.transfersMu.Unlock()
		return errors.New(T("transferCannotRetry"))
	}
	a.requeueLocked(task)
	event := task.event
	a.transfersMu.Unlock()
	a.emit(event)
	a.pumpTransfers()
	return describeErr(task.wait())
}

// RetryFailedTransfers queues every failed or cancelled transfer again.
func (a *App) RetryFailedTransfers() int {
	a.transfersMu.Lock()
	var events []TransferEvent
	for _, task := range a.transferJobs {
		if task.event.State == "error" || task.event.State == "cancelled" {
			a.requeueLocked(task)
			events = append(events, task.event)
		}
	}
	a.transfersMu.Unlock()
	for _, event := range events {
		a.emit(event)
	}
	a.pumpTransfers()
	return len(events)
}

// PrioritizeTransfer moves a queued transfer to the front of the queue.
func (a *App) PrioritizeTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	if task.event.State != "queued" {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotQueued"))
	}
	a.transferFront--
	task.seq = float64(a.transferFront)
	task.event.Order = task.seq
	event := task.event
	a.transfersMu.Unlock()
	a.emit(event)
	a.pumpTransfers()
	return nil
}

// ReorderTransfer places a queued transfer right before another queued one,
// or at the end of the queue when before is zero.
func (a *App) ReorderTransfer(id, before int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	if task.event.State != "queued" || id == before {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotQueued"))
	}
	if before == 0 {
		task.seq = float64(a.nextID.Add(1))
		task.event.Order = task.seq
		event := task.event
		a.transfersMu.Unlock()
		a.emit(event)
		return nil
	}
	target, ok := a.transferJobs[before]
	if !ok || target.event.State != "queued" {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotQueued"))
	}
	// Halfway between the target and whatever is queued right before it.
	previous := math.Inf(-1)
	for _, other := range a.transferJobs {
		if other != task && other.event.State == "queued" && other.seq < target.seq && other.seq > previous {
			previous = other.seq
		}
	}
	if math.IsInf(previous, -1) {
		task.seq = target.seq - 1
	} else {
		task.seq = (previous + target.seq) / 2
	}
	task.event.Order = task.seq
	event := task.event
	a.transfersMu.Unlock()
	a.emit(event)
	return nil
}

// PauseTransfers stops starting queued transfers; running ones continue.
func (a *App) PauseTransfers(paused bool) {
	a.transfersMu.Lock()
	a.transfersPaused = paused
	a.transfersMu.Unlock()
	a.pumpTransfers()
}

// SetTransferLimit changes how many transfers run at the same time.
func (a *App) SetTransferLimit(limit int) error {
	if limit < 1 || limit > maxTransferLimit {
		return errors.New(T("transferLimitRange", maxTransferLimit))
	}
	a.mu.Lock()
	settings := a.settings
	settings.TransferLimit = limit
	err := a.saveSettingsLocked(settings)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.transfersMu.Lock()
	a.queueLimit = limit
	a.transfersMu.Unlock()
	a.pumpTransfers()
	return nil
}

// RemoveTransfer drops a finished transfer from the list.
func (a *App) RemoveTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	if task.active() {
		a.transfersMu.Unlock()
		return errors.New(T("transferAlreadyRunning"))
	}
	delete(a.transferJobs, id)
	queue := a.queueEventLocked()
	a.transfersMu.Unlock()
	a.emitEvent("queue", queue)
	return nil
}

// ClearTransfers removes every finished transfer from the list.
func (a *App) ClearTransfers() {
	a.transfersMu.Lock()
	for id, task := range a.transferJobs {
		if !task.active() {
			delete(a.transferJobs, id)
		}
	}
	queue := a.queueEventLocked()
	a.transfersMu.Unlock()
	a.emitEvent("queue", queue)
}

package main

import (
	"context"
	"errors"
)

type transferTask struct {
	event  TransferEvent
	cancel context.CancelFunc
	run    func(context.Context, func(int64, int64)) error
}

func (a *App) startTransfer(kind, name string, run func(context.Context, func(int64, int64)) error) error {
	task := &transferTask{event: TransferEvent{ID: a.nextID.Add(1), Kind: kind, Name: name, State: "queued"}, run: run}
	a.transfersMu.Lock()
	if a.transferJobs == nil {
		a.transferJobs = make(map[int64]*transferTask)
	}
	if len(a.transferJobs) >= 200 {
		var oldest int64
		for id, old := range a.transferJobs {
			if old.event.State != "running" && old.event.State != "queued" && (oldest == 0 || id < oldest) {
				oldest = id
			}
		}
		if oldest != 0 {
			delete(a.transferJobs, oldest)
		}
	}
	a.transferJobs[task.event.ID] = task
	a.transfersMu.Unlock()
	return a.executeTransfer(task)
}

func (a *App) executeTransfer(task *transferTask) error {
	a.transfersMu.Lock()
	if task.event.State == "running" {
		a.transfersMu.Unlock()
		return errors.New(T("transferAlreadyRunning"))
	}
	if task.event.State != "queued" && task.event.State != "error" && task.event.State != "cancelled" {
		a.transfersMu.Unlock()
		return errors.New(T("transferCannotRetry"))
	}
	ctx, cancel := context.WithCancel(a.ctx)
	task.cancel = cancel
	task.event.State, task.event.Error = "running", ""
	task.event.Done, task.event.Total = 0, 0
	event := task.event
	a.transfersMu.Unlock()
	defer cancel()
	a.emit(event)
	err := task.run(ctx, func(done, total int64) {
		a.transfersMu.Lock()
		task.event.Done, task.event.Total = done, total
		event := task.event
		a.transfersMu.Unlock()
		a.emit(event)
	})
	a.transfersMu.Lock()
	task.cancel = nil
	if err == nil {
		task.event.State, task.event.Done = "done", task.event.Total
	} else if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		task.event.State, task.event.Error = "cancelled", ""
	} else {
		task.event.State, task.event.Error = "error", describeErr(err).Error()
	}
	event = task.event
	a.transfersMu.Unlock()
	a.emit(event)
	return err
}

func (a *App) CancelTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	cancel := task.cancel
	a.transfersMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (a *App) RetryTransfer(id int64) error {
	a.transfersMu.Lock()
	task, ok := a.transferJobs[id]
	if !ok {
		a.transfersMu.Unlock()
		return errors.New(T("transferNotFound"))
	}
	state := task.event.State
	a.transfersMu.Unlock()
	if state != "error" && state != "cancelled" {
		return errors.New(T("transferCannotRetry"))
	}
	// The closure keeps the original client and destination, even if profiles change.
	return describeErr(a.executeTransfer(task))
}

func (a *App) ClearTransfers() {
	a.transfersMu.Lock()
	defer a.transfersMu.Unlock()
	for id, task := range a.transferJobs {
		if task.event.State != "running" && task.event.State != "queued" {
			delete(a.transferJobs, id)
		}
	}
}

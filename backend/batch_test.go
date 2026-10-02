package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestBatchEventsDescribeTheWholeOperation(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	var mu sync.Mutex
	var batches []BatchEvent
	kinds := map[string]bool{}
	a.emitEvent = func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		switch ev := data.(type) {
		case BatchEvent:
			batches = append(batches, ev)
		case TransferEvent:
			kinds[ev.Kind] = true
		}
	}
	dir := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := a.UploadPaths("test", "", []string{dir}); err != nil || count != 3 {
		t.Fatalf("upload: %d, %v", count, err)
	}
	first, last := batches[0], batches[len(batches)-1]
	if first != (BatchEvent{ID: first.ID, Kind: "upload", Total: 3, Active: true}) {
		t.Fatalf("first event: %+v", first)
	}
	if last != (BatchEvent{ID: first.ID, Kind: "upload", Total: 3, Done: 3}) {
		t.Fatalf("last event: %+v", last)
	}

	batches = nil
	c, _ := a.cli()
	if _, err := a.syncDirectory(c, "test", "", dir, false); err != nil {
		t.Fatal(err)
	}
	// Nothing changed: the sync announces its comparison and ends without transfers.
	if len(batches) < 2 || batches[0].Kind != "sync" || batches[0].Total != 0 || !batches[0].Active || batches[len(batches)-1].Active {
		t.Fatalf("sync events: %+v", batches)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.syncDirectory(c, "test", "", dir, false); err != nil {
		t.Fatal(err)
	}
	if !kinds["sync"] || !kinds["upload"] {
		t.Fatalf("transfer kinds: %v", kinds)
	}
}

// A server-side size limit must reach the user as a clear reason, not as a count.
func TestUploadFailureExplainsTheReason(t *testing.T) {
	fake := gofakes3.New(s3mem.New()).Server()
	a := connectedApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "big") {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>EntityTooLarge</Code><Message>The object exceeded the maximum allowed size</Message></Error>`))
			return
		}
		fake.ServeHTTP(w, r)
	}))
	dir := t.TempDir()
	write := func(name string) string {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	big, small := write("big.bin"), write("small.txt")
	want := T("entityTooLarge")

	_, err := a.UploadPaths("test", "", []string{big})
	if err == nil || err.Error() != want {
		t.Fatalf("single upload: %v", err)
	}
	count, err := a.UploadPaths("test", "", []string{big, small})
	if count != 1 || err == nil || !strings.HasPrefix(err.Error(), T("uploadFailed", 1)+": ") || !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("batch upload: %d, %v", count, err)
	}
	a.transfersMu.Lock()
	var failed int64
	for id, task := range a.transferJobs {
		if task.event.State == "error" {
			failed = id
			if task.event.Error != want {
				t.Errorf("transfer row shows %q", task.event.Error)
			}
		}
	}
	a.transfersMu.Unlock()
	if err := a.RetryTransfer(failed); err == nil || err.Error() != want {
		t.Fatalf("retry: %v", err)
	}
}

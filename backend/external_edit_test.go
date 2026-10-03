package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestEditExternallyUploadsChanges(t *testing.T) {
	editWatchInterval = 20 * time.Millisecond
	defer func() { editWatchInterval = 2 * time.Second }()
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	var mu sync.Mutex
	var edits []EditEvent
	a.emitEvent = func(name string, data any) {
		if name == "edit" {
			mu.Lock()
			edits = append(edits, data.(EditEvent))
			mu.Unlock()
		}
	}
	putObject(t, a, "notes/todo.txt", "first")
	// Without a desktop host the file cannot be opened, but it is downloaded and watched.
	if err := a.EditExternally("test", "notes/todo.txt"); err == nil {
		t.Fatal("expected the host error")
	}
	a.editsMu.Lock()
	local := a.edits["test\x00notes/todo.txt"].local
	a.editsMu.Unlock()
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(local)) })
	if body, err := os.ReadFile(local); err != nil || string(body) != "first" {
		t.Fatalf("temporary copy: %q, %v", body, err)
	}
	if !strings.Contains(local, "s3browser-edit-") || !strings.HasSuffix(local, "todo.txt") {
		t.Fatalf("unexpected path %s", local)
	}
	if err := a.EditExternally("test", "notes/"); err == nil {
		t.Fatal("a folder was opened")
	}
	putObject(t, a, "tools/installer.exe", "MZ")
	if err := a.EditExternally("test", "tools/installer.exe"); err == nil || err.Error() != T("editNotAllowed") {
		t.Fatalf("executable was opened: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(local, []byte("second version"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(edits)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(edits) != 1 || edits[0].State != "uploaded" || edits[0].Key != "notes/todo.txt" {
		t.Fatalf("edit events: %+v", edits)
	}
	if text, err := a.PreviewText("test", "notes/todo.txt"); err != nil || text != "second version" {
		t.Fatalf("object after edit: %q, %v", text, err)
	}
}

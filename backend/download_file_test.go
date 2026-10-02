package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadStaysInsideSelectedDirectory(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "downloads")
	outside := filepath.Join(parent, "downloads-other")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../downloads-other/file", filepath.Join(outside, "absolute"), ".", ""} {
		if err := saveDownload(context.Background(), directory, name, strings.NewReader("data")); err == nil {
			t.Fatalf("accepted unsafe path %q", name)
		}
	}
	if err := os.Symlink(outside, filepath.Join(directory, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := saveDownload(context.Background(), directory, "linked/file", strings.NewReader("data")); err == nil {
		t.Fatal("followed symlink outside directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrote outside selected directory: %v, %v", entries, err)
	}
	if err := saveDownload(context.Background(), directory, "folder/file", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "folder/file"))
	if err != nil || string(content) != "data" {
		t.Fatalf("download: %q, %v", content, err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("connection interrupted") }

func TestFailedDownloadPreservesDestination(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "file")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveDownload(context.Background(), directory, "file", brokenReader{}); err == nil {
		t.Fatal("expected download failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := saveDownload(ctx, directory, "file", strings.NewReader("replacement")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "original" {
		t.Fatalf("destination changed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

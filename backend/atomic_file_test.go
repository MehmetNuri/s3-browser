package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateFileReplacesAtomically(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "profiles.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("content: %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions: %v, %v", info.Mode().Perm(), err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
	if err := writePrivateFile(filepath.Join(directory, "missing", "file"), []byte("x")); err == nil {
		t.Fatal("wrote into a missing directory")
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestSyncToDirectory(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "site/index.html", "<h1>hi</h1>")
	putObject(t, a, "site/assets/app.css", "body{}")
	putObject(t, a, "site/assets/", "")    // folder marker
	putObject(t, a, "site/../escape", "x") // never written outside the folder
	dir := t.TempDir()
	c, _ := a.cli()
	res, err := a.syncToDirectory(c, "test", "site/", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Downloaded != 2 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("first sync: %+v", res)
	}
	if body, err := os.ReadFile(filepath.Join(dir, "assets", "app.css")); err != nil || string(body) != "body{}" {
		t.Fatalf("downloaded file: %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Fatal("a key with .. was written outside the folder")
	}
	// Unchanged files are skipped; an extra local file is kept without mirror.
	extra := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(extra, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = a.syncToDirectory(c, "test", "site/", dir, false)
	if err != nil || res.Downloaded != 0 || res.Skipped != 2 {
		t.Fatalf("second sync: %+v, %v", res, err)
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatal("extra file was removed without mirror")
	}
	res, err = a.syncToDirectory(c, "test", "site/", dir, true)
	if err != nil || res.Deleted != 1 {
		t.Fatalf("mirror sync: %+v, %v", res, err)
	}
	if _, err := os.Stat(extra); err == nil {
		t.Fatal("extra file survived mirror")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets")); err != nil {
		t.Fatal("mirror removed a folder")
	}
	if _, err := a.syncToDirectory(c, "test", "nothing/", dir, true); err == nil {
		t.Fatal("mirror with an empty prefix was allowed")
	}
}

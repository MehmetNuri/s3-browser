package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func backupFolder(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "docs")
	for name, content := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestMirrorDeletesOnlyWhatLeftTheFolder(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	c, _ := a.cli()
	dir := backupFolder(t, map[string]string{"a.txt": "a", "sub/b.txt": "b"})
	putObject(t, a, "backup/docs/old.txt", "left the folder")
	putObject(t, a, "backup/docs/sub/", "")
	putObject(t, a, "backup/other/keep.txt", "outside the synced folder")
	putObject(t, a, "backup/docs-archive/keep.txt", "sibling with a similar name")

	added, err := a.syncDirectory(c, "test", "backup/", dir, false)
	if err != nil || added != (SyncResult{Uploaded: 2}) {
		t.Fatalf("add-only sync: %+v, %v", added, err)
	}
	if _, err := a.HeadObject("test", "backup/docs/old.txt"); err != nil {
		t.Fatalf("add-only sync deleted an object: %v", err)
	}
	mirrored, err := a.syncDirectory(c, "test", "backup/", dir, true)
	if err != nil || mirrored != (SyncResult{Skipped: 2, Deleted: 1}) {
		t.Fatalf("mirror sync: %+v, %v", mirrored, err)
	}
	keys, err := a.listAll(c, "test", "backup/")
	want := "backup/docs-archive/keep.txt backup/docs/a.txt backup/docs/sub/ backup/docs/sub/b.txt backup/other/keep.txt"
	if err != nil || strings.Join(keys, " ") != want {
		t.Fatalf("remote keys: %v, %v", keys, err)
	}
}

func TestMirrorRefusesEmptyOrMissingSource(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	c, _ := a.cli()
	putObject(t, a, "docs/a.txt", "a")
	empty := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{empty, filepath.Join(t.TempDir(), "docs")} {
		if result, err := a.syncDirectory(c, "test", "", dir, true); err == nil {
			t.Fatalf("mirrored from %s: %+v", dir, result)
		}
		if _, err := a.HeadObject("test", "docs/a.txt"); err != nil {
			t.Fatalf("backup was deleted: %v", err)
		}
	}
}

func TestBackupJobs(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	var events []string
	a.emitEvent = func(name string, _ any) { events = append(events, name) }
	dir := backupFolder(t, map[string]string{"a.txt": "a"})
	job, err := a.addBackupJob(BackupJob{Name: "Docs", ProfileID: a.profile.ID, Bucket: "test", Prefix: "/nightly", Dir: dir, Mirror: true, IntervalMinutes: 1})
	if err != nil || job.Prefix != "nightly/" || job.IntervalMinutes != minBackupInterval {
		t.Fatalf("job: %+v, %v", job, err)
	}
	// Running a job must not need the profile to be the active connection.
	a.mu.Lock()
	a.client = nil
	a.mu.Unlock()
	result, err := a.RunBackupJob(job.ID)
	if err != nil || result != (SyncResult{Uploaded: 1}) {
		t.Fatalf("run: %+v, %v", result, err)
	}
	jobs, err := a.ListBackupJobs()
	if err != nil || len(jobs) != 1 || jobs[0].LastStatus != "ok" || jobs[0].LastRun == "" || jobs[0].Running {
		t.Fatalf("stored jobs: %+v, %v", jobs, err)
	}
	if !strings.Contains(strings.Join(events, ","), "backup") {
		t.Fatalf("no backup event: %v", events)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunBackupJob(job.ID); err == nil {
		t.Fatal("ran with a missing folder")
	}
	if jobs, _ = a.ListBackupJobs(); jobs[0].LastStatus != "error" || jobs[0].LastMessage == "" {
		t.Fatalf("failure was not recorded: %+v", jobs[0])
	}
	if _, err := a.RunBackupJob("missing"); err == nil {
		t.Fatal("ran an unknown job")
	}
	if err := a.DeleteProfile(job.ProfileID); err != nil {
		t.Fatal(err)
	}
	if jobs, _ = a.ListBackupJobs(); len(jobs) != 0 {
		t.Fatalf("jobs of a deleted profile remain: %+v", jobs)
	}
	if err := a.DeleteBackupJob(job.ID); err == nil {
		t.Fatal("deleted a job twice")
	}
}

func TestBackupDue(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) string { return now.Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339) }
	for _, test := range []struct {
		job  BackupJob
		want bool
	}{
		{BackupJob{IntervalMinutes: 0, LastRun: at(9999)}, false},
		{BackupJob{IntervalMinutes: 60}, true},
		{BackupJob{IntervalMinutes: 60, LastRun: "not a date"}, true},
		{BackupJob{IntervalMinutes: 60, LastRun: at(59)}, false},
		{BackupJob{IntervalMinutes: 60, LastRun: at(60)}, true},
	} {
		if got := backupDue(test.job, now); got != test.want {
			t.Errorf("%+v: got %v", test.job, got)
		}
	}
}

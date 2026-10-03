//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// TestMountBucket mounts a fake bucket through FUSE; it is skipped where
// /dev/fuse is missing or user mounts are not allowed.
func TestMountBucket(t *testing.T) {
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "docs/readme.txt", "hello mount")
	putObject(t, a, "docs/sub/", "")
	putObject(t, a, "docs/sub/deep.txt", "deep")
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := a.MountBucket("test", "docs", false)
	if err != nil {
		if strings.Contains(err.Error(), "fusermount") || strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "not permitted") {
			t.Skipf("mount not possible here: %v", err)
		}
		t.Fatal(err)
	}
	defer func() {
		_ = a.UnmountBucket(m.ID)
		deadline := time.Now().Add(5 * time.Second)
		for len(a.ListMounts()) > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
	}()
	if !strings.HasPrefix(m.Path, filepath.Join(home, mountRootName)) || m.Prefix != "docs/" {
		t.Fatalf("mount: %+v", m)
	}
	// Second mount of the same place returns the existing one.
	if again, err := a.MountBucket("test", "docs/", true); err != nil || again.ID != m.ID {
		t.Fatalf("duplicate mount: %+v, %v", again, err)
	}
	entries, err := os.ReadDir(m.Path)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name()+map[bool]string{true: "/", false: ""}[e.IsDir()])
	}
	if strings.Join(names, ",") != "readme.txt,sub/" {
		t.Fatalf("listing: %v", names)
	}
	if body, err := os.ReadFile(filepath.Join(m.Path, "readme.txt")); err != nil || string(body) != "hello mount" {
		t.Fatalf("read: %q, %v", body, err)
	}
	if body, err := os.ReadFile(filepath.Join(m.Path, "sub", "deep.txt")); err != nil || string(body) != "deep" {
		t.Fatalf("nested read: %q, %v", body, err)
	}
	// Writes become objects when the file is closed.
	if err := os.WriteFile(filepath.Join(m.Path, "new.txt"), []byte("created through the mount"), 0o600); err != nil {
		t.Fatal(err)
	}
	if text, err := a.PreviewText("test", "docs/new.txt"); err != nil || text != "created through the mount" {
		t.Fatalf("object after write: %q, %v", text, err)
	}
	f, err := os.OpenFile(filepath.Join(m.Path, "readme.txt"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("!"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if text, err := a.PreviewText("test", "docs/readme.txt"); err != nil || text != "hello mount!" {
		t.Fatalf("object after append: %q, %v", text, err)
	}
	if err := os.Mkdir(filepath.Join(m.Path, "made"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(m.Path, "new.txt"), filepath.Join(m.Path, "made", "moved.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PreviewText("test", "docs/made/moved.txt"); err != nil {
		t.Fatalf("moved object: %v", err)
	}
	if err := os.Remove(filepath.Join(m.Path, "made", "moved.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(m.Path, "made")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(m.Path, "sub")); err == nil {
		t.Fatal("non-empty folder was removed")
	}
	if _, err := a.PreviewText("test", "docs/made/moved.txt"); err == nil {
		t.Fatal("deleted object still exists")
	}
	if err := a.UnmountBucket(m.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(a.ListMounts()) > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(a.ListMounts()) != 0 {
		t.Fatal("mount still listed after unmount")
	}
	if _, err := os.Stat(m.Path); err == nil {
		t.Fatal("mount folder was left behind")
	}
	// The bucket root mounts too, and the file system reports room and a name limit.
	root, err := a.MountBucket("test", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(root.Path); err != nil || len(entries) != 1 || entries[0].Name() != "docs" || !entries[0].IsDir() {
		t.Fatalf("root listing: %v, %v", entries, err)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(root.Path, &st); err != nil || st.Namelen != 255 || st.Bavail == 0 {
		t.Fatalf("statfs: %+v, %v", st, err)
	}
	if err := os.Mkdir(filepath.Join(root.Path, "fresh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := a.HeadObject("test", "fresh/"); err != nil {
		t.Fatalf("folder marker: %v", err)
	}
	_ = a.UnmountBucket(root.ID)
	// Read-only mounts refuse writes.
	ro, err := a.MountBucket("test", "docs", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.UnmountBucket(ro.ID) }()
	if err := os.WriteFile(filepath.Join(ro.Path, "nope.txt"), []byte("x"), 0o600); err == nil {
		t.Fatal("write on a read-only mount succeeded")
	}
}

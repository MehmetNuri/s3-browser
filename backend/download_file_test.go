package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
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
		if err := saveDownload(context.Background(), directory, name, strings.NewReader("data"), ""); err == nil {
			t.Fatalf("accepted unsafe path %q", name)
		}
	}
	if err := os.Symlink(outside, filepath.Join(directory, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := saveDownload(context.Background(), directory, "linked/file", strings.NewReader("data"), ""); err == nil {
		t.Fatal("followed symlink outside directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrote outside selected directory: %v, %v", entries, err)
	}
	if err := saveDownload(context.Background(), directory, "folder/file", strings.NewReader("data"), ""); err != nil {
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
	if err := saveDownload(context.Background(), directory, "file", brokenReader{}, ""); err == nil {
		t.Fatal("expected download failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := saveDownload(ctx, directory, "file", strings.NewReader("replacement"), ""); !errors.Is(err, context.Canceled) {
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

func TestDownloadVerifiesChecksum(t *testing.T) {
	// gofakes3 reports the MD5 of the stored body as ETag, so a correct
	// download passes; a tampered ETag must reject and remove the file.
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "a.txt", "hello world")
	dir := t.TempDir()
	c, _ := a.cli()
	if err := a.downloadTo(c, "test", "a.txt", dir, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(body) != "hello world" {
		t.Fatalf("download: %q, %v", body, err)
	}
	tampered := connectedApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/b.txt") {
			w.Header().Set("ETag", `"00000000000000000000000000000000"`)
			w.Header().Set("Content-Length", "5")
			_, _ = w.Write([]byte("wrong"))
			return
		}
		gofakes3.New(s3mem.New()).Server().ServeHTTP(w, r)
	}))
	c, _ = tampered.cli()
	err := tampered.downloadTo(c, "test", "b.txt", dir, "b.txt")
	if err == nil || !strings.Contains(err.Error(), "00000000") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err == nil {
		t.Fatal("corrupt download was kept")
	}
}

func TestSegmentedDownload(t *testing.T) {
	segmentedMinSize, segmentSize = 64<<10, 16<<10
	defer func() { segmentedMinSize, segmentSize = 32<<20, 8<<20 }()
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	body := make([]byte, 200<<10+123)
	for i := range body {
		body[i] = byte(i * 7)
	}
	c, _ := a.cli()
	if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String("big.bin"), Body: bytes.NewReader(body)}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var mu sync.Mutex
	var reports []int64
	a.emitEvent = func(name string, data any) {
		if ev, ok := data.(TransferEvent); ok && ev.State == "running" {
			mu.Lock()
			reports = append(reports, ev.Done)
			mu.Unlock()
		}
	}
	if err := a.downloadTo(c, "test", "big.bin", dir, "big.bin"); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "big.bin"))
	if err != nil || !bytes.Equal(saved, body) {
		t.Fatalf("segmented download differs (%d bytes, %v)", len(saved), err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) < 2 || reports[len(reports)-1] != int64(len(body)) {
		t.Fatalf("progress reports: %v", reports)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temporary files left behind: %d entries", len(entries))
	}
}

func TestCorruptDownloadKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("old copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := saveDownload(context.Background(), dir, "keep.txt", strings.NewReader("new data"), "00000000000000000000000000000000")
	if err == nil || !strings.Contains(err.Error(), "00000000") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "keep.txt")); string(body) != "old copy" {
		t.Fatalf("previous copy was replaced: %q", body)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temporary file left behind: %d entries", len(entries))
	}
}

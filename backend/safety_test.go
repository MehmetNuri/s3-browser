package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func connectedApp(t *testing.T, handler http.Handler) *App {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	a := &App{ctx: context.Background(), store: &profileStore{path: filepath.Join(t.TempDir(), "p.json")}}
	a.emitEvent = func(string, any) {}
	p, err := a.SaveProfile(Profile{Name: "fake", Provider: "minio", Endpoint: srv.URL, AccessKey: "a", SecretKey: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Connect(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	return a
}

func putObject(t *testing.T, a *App, key, body string) {
	t.Helper()
	c, _ := a.cli()
	if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String(key), Body: strings.NewReader(body)}); err != nil {
		t.Fatal(err)
	}
}

func TestRenameOntoItselfKeepsObject(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "a.txt", "data")
	for _, target := range []string{"a.txt", ""} {
		if err := a.RenameObject("test", "a.txt", target); err == nil {
			t.Fatalf("accepted rename target %q", target)
		}
	}
	if _, err := a.HeadObject("test", "a.txt"); err != nil {
		t.Fatalf("object was lost: %v", err)
	}
}

func TestBucketPinsRequireActiveProfile(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	if err := a.DeleteProfile(a.profile.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveProfileBucket("test"); err == nil {
		t.Fatal("unpinned a bucket without an active profile")
	}
	if err := a.AddProfileBucket("test"); err == nil {
		t.Fatal("pinned a bucket without an active profile")
	}
	if profiles, err := a.ListProfiles(); err != nil || len(profiles) != 0 {
		t.Fatalf("a profile was created: %+v, %v", profiles, err)
	}
}

func TestPreviewIsBoundedWhenRangeIsIgnored(t *testing.T) {
	fake := gofakes3.New(s3mem.New()).Server()
	a := connectedApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Range")
		fake.ServeHTTP(w, r)
	}))
	putObject(t, a, "big.txt", strings.Repeat("x", 4*previewLimit))
	text, err := a.PreviewText("test", "big.txt")
	if err != nil || len(text) != previewLimit {
		t.Fatalf("preview: %d bytes, %v", len(text), err)
	}
}

func TestPresignExpiryIsCapped(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	link, err := a.Presign("test", "a.txt", 1<<30)
	if err != nil || !strings.Contains(link, "X-Amz-Expires=604800") {
		t.Fatalf("presign: %s, %v", link, err)
	}
}

func TestFolderUploadSkipsLinksAndSpecialFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra privileges on Windows")
	}
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	parent := t.TempDir()
	secret := filepath.Join(parent, "secret")
	folder := filepath.Join(parent, "folder")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "sub", "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(folder, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(folder, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	count, err := a.UploadPaths("test", "", []string{folder})
	if err != nil || count != 1 {
		t.Fatalf("upload: %d, %v", count, err)
	}
	c, _ := a.cli()
	keys, err := a.listAll(c, "test", "")
	if err != nil || len(keys) != 1 || keys[0] != "folder/sub/file.txt" {
		t.Fatalf("uploaded keys: %v, %v", keys, err)
	}
}

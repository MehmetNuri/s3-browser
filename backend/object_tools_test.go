package main

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestPreviewImage(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)
	putObject(t, a, "photo.png", png)
	putObject(t, a, "fake.png", "<html><script>alert(1)</script></html>")
	putObject(t, a, "vector.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	putObject(t, a, "huge.png", "\x89PNG\r\n\x1a\n"+strings.Repeat("x", imagePreviewLimit))

	url, err := a.PreviewImage("test", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	encoded, ok := strings.CutPrefix(url, "data:image/png;base64,")
	if decoded, err := base64.StdEncoding.DecodeString(encoded); !ok || err != nil || string(decoded) != png {
		t.Fatalf("unexpected data URL: %.40s, %v", url, err)
	}
	for _, key := range []string{"fake.png", "vector.svg", "huge.png", "missing.png"} {
		if url, err := a.PreviewImage("test", key); err == nil {
			t.Fatalf("%s produced a preview: %.40s", key, url)
		}
	}
}

func TestFolderStats(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "dir/", "")
	putObject(t, a, "dir/a.txt", "12345")
	putObject(t, a, "dir/sub/b.txt", "123")
	putObject(t, a, "other.txt", "1234567")
	stats, err := a.FolderStats("test", "dir/")
	if err != nil || stats.Objects != 2 || stats.Size != 8 || stats.Truncated {
		t.Fatalf("stats: %+v, %v", stats, err)
	}
	if stats, err = a.FolderStats("test", "empty/"); err != nil || stats.Objects != 0 {
		t.Fatalf("empty prefix: %+v, %v", stats, err)
	}
}

func TestSaveText(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	c, _ := a.cli()
	if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String("notes.md"),
		Body: strings.NewReader("old"), ContentType: aws.String("text/markdown"), Metadata: map[string]string{"owner": "team"}}); err != nil {
		t.Fatal(err)
	}
	before, err := a.HeadObject("test", "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveText("test", "notes.md", "new content", before.ETag); err != nil {
		t.Fatal(err)
	}
	after, err := a.HeadObject("test", "notes.md")
	if err != nil || after.ContentType != "text/markdown" || after.Metadata["owner"] != "team" || after.Size != 11 {
		t.Fatalf("saved object: %+v, %v", after, err)
	}
	if err := a.SaveText("test", "notes.md", "stale edit", before.ETag); err == nil {
		t.Fatal("overwrote an object that changed after it was opened")
	}
	putObject(t, a, "big.log", strings.Repeat("x", previewLimit+1))
	if err := a.SaveText("test", "big.log", "truncated", ""); err == nil {
		t.Fatal("replaced an object larger than the preview")
	}
	if text, err := a.PreviewText("test", "notes.md"); err != nil || text != "new content" {
		t.Fatalf("content: %q, %v", text, err)
	}
}

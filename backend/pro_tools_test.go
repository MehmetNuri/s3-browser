package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestSyncUploadsOnlyNewAndChangedFiles(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	c, _ := a.cli()
	dir := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(filepath.Join(dir, "css"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string, modified time.Time) {
		t.Helper()
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	write("index.html", "<html>", past)
	write("css/site.css", "body{}", past)

	first, err := a.syncDirectory(c, "test", "www/", dir, false, uploadOptions{})
	if err != nil || first != (SyncResult{Uploaded: 2}) {
		t.Fatalf("first sync: %+v, %v", first, err)
	}
	second, err := a.syncDirectory(c, "test", "www/", dir, false, uploadOptions{})
	if err != nil || second != (SyncResult{Skipped: 2}) {
		t.Fatalf("unchanged sync: %+v, %v", second, err)
	}
	write("index.html", "<html><body>", past)                  // size changed
	write("css/site.css", "body{}", time.Now().Add(time.Hour)) // modified after the upload
	write("new.txt", "new", past)
	third, err := a.syncDirectory(c, "test", "www/", dir, false, uploadOptions{})
	if err != nil || third != (SyncResult{Uploaded: 3}) {
		t.Fatalf("changed sync: %+v, %v", third, err)
	}
	keys, err := a.listAll(c, "test", "www/")
	if err != nil || len(keys) != 3 {
		t.Fatalf("remote keys: %v, %v", keys, err)
	}
	if text, err := a.PreviewText("test", "www/site/index.html"); err != nil || text != "<html><body>" {
		t.Fatalf("changed file was not uploaded: %q, %v", text, err)
	}
}

// copyRecorder keeps the headers of the last CopyObject request. The fake
// server ignores the source version and most headers of a copy, so those are
// checked on the request itself.
func copyRecorder(last *http.Header) http.Handler {
	fake := gofakes3.New(s3mem.New()).Server()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Copy-Source") != "" {
			*last = r.Header.Clone()
		}
		fake.ServeHTTP(w, r)
	})
}

func TestObjectVersions(t *testing.T) {
	var copied http.Header
	a := connectedApp(t, copyRecorder(&copied))
	c, _ := a.cli()
	if _, err := c.PutBucketVersioning(a.ctx, &s3.PutBucketVersioningInput{Bucket: aws.String("test"),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatal(err)
	}
	putObject(t, a, "doc.txt", "first")
	putObject(t, a, "doc.txt", "second")
	putObject(t, a, "doc.txt.bak", "other object")

	versions, err := a.ListVersions("test", "doc.txt")
	if err != nil || len(versions) != 2 || !versions[0].IsLatest || versions[1].IsLatest {
		t.Fatalf("versions: %+v, %v", versions, err)
	}
	old := versions[1].VersionID
	if err := a.RestoreVersion("test", "doc.txt", old); err != nil {
		t.Fatal(err)
	}
	if source := copied.Get("X-Amz-Copy-Source"); source != "test/doc.txt?versionId="+url.QueryEscape(old) {
		t.Fatalf("copy source: %q", source)
	}
	if err := a.DeleteVersion("test", "doc.txt", old); err != nil {
		t.Fatal(err)
	}
	versions, err = a.ListVersions("test", "doc.txt")
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions after delete: %+v, %v", versions, err)
	}
	for _, v := range versions {
		if v.VersionID == old {
			t.Fatal("deleted version is still listed")
		}
	}
	for _, call := range []func() error{
		func() error { return a.DeleteVersion("test", "doc.txt", "") },
		func() error { return a.RestoreVersion("test", "doc.txt", "") },
	} {
		if call() == nil {
			t.Fatal("accepted an empty version ID")
		}
	}
	if _, err := a.HeadObject("test", "doc.txt"); err != nil {
		t.Fatalf("current object was removed: %v", err)
	}
}

func TestUpdateObjectHeaders(t *testing.T) {
	var copied http.Header
	a := connectedApp(t, copyRecorder(&copied))
	c, _ := a.cli()
	if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String("page"),
		Body: nil, ContentType: aws.String("application/octet-stream"), Metadata: map[string]string{"owner": "team"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateObjectHeaders("test", "page", ObjectHeaders{ContentType: " text/html ", CacheControl: "max-age=60"}); err != nil {
		t.Fatal(err)
	}
	if copied.Get("X-Amz-Copy-Source") != "test/page" || copied.Get("X-Amz-Metadata-Directive") != "REPLACE" ||
		copied.Get("Content-Type") != "text/html" || copied.Get("Cache-Control") != "max-age=60" || copied.Get("X-Amz-Meta-Owner") != "team" {
		t.Fatalf("copy request: %v", copied)
	}
	info, err := a.HeadObject("test", "page")
	if err != nil || info.ContentType != "text/html" || info.Metadata["owner"] != "team" {
		t.Fatalf("headers: %+v, %v", info, err)
	}
	copied = nil
	if err := a.UpdateObjectHeaders("test", "page", ObjectHeaders{ContentType: "text/html\r\nx-amz-acl: public-read"}); err == nil {
		t.Fatal("accepted a header value with a line break")
	}
	if copied != nil {
		t.Fatal("an invalid header value reached the server")
	}
}

func TestSearchObjects(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	for _, key := range []string{"Report-2026.pdf", "docs/annual-REPORT.txt", "docs/deep/report/", "docs/deep/notes.txt", "other/report.csv"} {
		putObject(t, a, key, "x")
	}
	all, err := a.SearchObjects("test", "", " report ")
	if err != nil || len(all.Items) != 3 || all.Truncated || all.Scanned != 5 {
		t.Fatalf("bucket search: %+v, %v", all, err)
	}
	scoped, err := a.SearchObjects("test", "docs/", "report")
	if err != nil || len(scoped.Items) != 1 || scoped.Items[0].Key != "docs/annual-REPORT.txt" || scoped.Items[0].Name != "annual-REPORT.txt" {
		t.Fatalf("prefix search: %+v, %v", scoped, err)
	}
	if _, err := a.SearchObjects("test", "", "  "); err == nil {
		t.Fatal("accepted an empty query")
	}
}

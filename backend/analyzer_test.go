package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestAnalyzeBucket(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	video := strings.Repeat("v", 5000)
	putObject(t, a, "media/intro.MP4", video)
	putObject(t, a, "media/copy/intro-copy.mp4", video)
	putObject(t, a, "backup/intro.mp4", video)
	putObject(t, a, "docs/readme.md", strings.Repeat("d", 300))
	putObject(t, a, "docs/", "")
	putObject(t, a, "LICENSE", strings.Repeat("l", 100))
	putObject(t, a, "empty-a", "")
	putObject(t, a, "empty-b", "")

	result, err := a.AnalyzeBucket("test", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Objects != 7 || result.Size != 15400 || result.Truncated {
		t.Fatalf("totals: %+v", result)
	}
	if len(result.Folders) != 4 || result.Folders[0] != (AnalysisEntry{Name: "media/", Size: 10000, Objects: 2}) {
		t.Fatalf("folders: %+v", result.Folders)
	}
	if result.Types[0] != (AnalysisEntry{Name: "mp4", Size: 15000, Objects: 3}) {
		t.Fatalf("types: %+v", result.Types)
	}
	if len(result.Ages) != 4 || result.Ages[0].Name != "30d" || result.Ages[0].Objects != 7 {
		t.Fatalf("ages: %+v", result.Ages)
	}
	if len(result.Largest) != 7 || result.Largest[0].Size != 5000 || result.Largest[6].Size != 0 {
		t.Fatalf("largest: %+v", result.Largest)
	}
	// Empty objects share an ETag but waste nothing, so they are not duplicates.
	if len(result.Duplicates) != 1 || result.Wasted != 10000 {
		t.Fatalf("duplicates: %+v, wasted %d", result.Duplicates, result.Wasted)
	}
	if group := result.Duplicates[0]; group.Copies != 3 || group.Size != 5000 || len(group.Keys) != 3 {
		t.Fatalf("duplicate group: %+v", group)
	}

	scoped, err := a.AnalyzeBucket("test", "media/")
	if err != nil || scoped.Objects != 2 || len(scoped.Folders) != 2 || scoped.Folders[0].Name != "" || scoped.Folders[1].Name != "copy/" {
		t.Fatalf("scoped: %+v, %v", scoped, err)
	}
}

func TestTallyFoldsSmallEntries(t *testing.T) {
	counts := tally{}
	for i, name := range []string{"a", "b", "c", "d"} {
		counts.add(name, int64(10*(i+1)))
	}
	entries := counts.sorted(2)
	if len(entries) != 3 || entries[0].Name != "d" || entries[2] != (AnalysisEntry{Name: "*", Size: 30, Objects: 2}) {
		t.Fatalf("entries: %+v", entries)
	}
}

func TestCopyToAnotherProfile(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	other := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
	t.Cleanup(other.Close)
	destination, err := a.SaveProfile(Profile{Name: "other", Provider: "minio", Endpoint: other.URL, AccessKey: "x", SecretKey: "y"})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := newClient(a.ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dst.CreateBucket(a.ctx, &s3.CreateBucketInput{Bucket: aws.String("archive")}); err != nil {
		t.Fatal(err)
	}
	c, _ := a.cli()
	if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String("site/index.html"),
		Body: strings.NewReader("<html>"), ContentType: aws.String("text/html"), Metadata: map[string]string{"owner": "team"}}); err != nil {
		t.Fatal(err)
	}
	putObject(t, a, "site/img/logo.png", "png")
	putObject(t, a, "site/notes.txt", "notes")
	putObject(t, a, "unrelated.txt", "no")

	buckets, err := a.ListProfileBuckets(destination.ID)
	if err != nil || len(buckets.Buckets) != 1 || buckets.Buckets[0].Name != "archive" {
		t.Fatalf("destination buckets: %+v, %v", buckets, err)
	}
	count, err := a.CopyToProfile("test", "site/", []string{"site/index.html", "site/img/"}, destination.ID, " archive ", "/2026/backup", false)
	if err != nil || count != 2 {
		t.Fatalf("copy: %d, %v", count, err)
	}
	listed, err := dst.ListObjectsV2(a.ctx, &s3.ListObjectsV2Input{Bucket: aws.String("archive")})
	if err != nil || len(listed.Contents) != 2 {
		t.Fatalf("destination objects: %+v, %v", listed.Contents, err)
	}
	head, err := dst.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String("archive"), Key: aws.String("2026/backup/index.html")})
	if err != nil || aws.ToString(head.ContentType) != "text/html" || head.Metadata["owner"] != "team" || aws.ToInt64(head.ContentLength) != 6 {
		t.Fatalf("copied object: %+v, %v", head, err)
	}
	if _, err := dst.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String("archive"), Key: aws.String("2026/backup/img/logo.png")}); err != nil {
		t.Fatalf("folder contents were not copied: %v", err)
	}
	// The active connection must still be the source profile.
	if _, err := a.HeadObject("test", "unrelated.txt"); err != nil {
		t.Fatalf("active connection changed: %v", err)
	}
	if _, err := a.CopyToProfile("test", "site/", []string{"site/notes.txt"}, a.profile.ID, "test", "site", false); err == nil {
		t.Fatal("copied objects onto themselves")
	}
	for _, call := range []func() (int, error){
		func() (int, error) {
			return a.CopyToProfile("test", "", []string{"unrelated.txt"}, destination.ID, " ", "", false)
		},
		func() (int, error) {
			return a.CopyToProfile("test", "", []string{"unrelated.txt"}, "missing", "archive", "", false)
		},
		func() (int, error) {
			return a.CopyToProfile("test", "", []string{"unrelated.txt"}, destination.ID, "no-such-bucket", "", false)
		},
	} {
		if count, err := call(); err == nil || count != 0 {
			t.Fatalf("invalid destination accepted: %d, %v", count, err)
		}
	}
}

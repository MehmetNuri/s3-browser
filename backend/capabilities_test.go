package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestAgainstFakeS3(t *testing.T) {
	srv := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
	defer srv.Close()

	a := &App{ctx: context.Background(), store: &profileStore{path: t.TempDir() + "/p.json"}}
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
	c, _ := a.cli()
	for _, k := range []string{"a.txt", "dir/b.txt", "dir/sub/c.txt"} {
		if _, err := c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String("test"), Key: aws.String(k)}); err != nil {
			t.Fatal(err)
		}
	}
	l, err := a.ListObjects("test", "", "")
	if err != nil || len(l.Items) != 2 {
		t.Fatalf("root listing: %+v %v", l, err)
	}
	if err := a.RenameObject("test", "a.txt", "dir/a2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteKeys("test", []string{"dir/"}); err != nil {
		t.Fatal(err)
	}
	if l, _ = a.ListObjects("test", "", ""); len(l.Items) != 0 {
		t.Fatalf("expected empty bucket, got %+v", l.Items)
	}

	res, err := a.RunCapabilities(CapOptions{Bucket: "test", IncludeBucket: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		t.Logf("%-22s %-38s %-11s %s", r.Category, r.Name, r.Status, r.Detail)
	}
	if l, _ = a.ListObjects("test", "", ""); len(l.Items) != 0 {
		t.Errorf("capability test left objects behind: %+v", l.Items)
	}
}

// Simulates an IAM user scoped to one bucket: ListBuckets is denied.
func TestPinnedBucketsWhenListingDenied(t *testing.T) {
	fake := gofakes3.New(s3mem.New()).Server()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>not authorized to perform: s3:ListAllMyBuckets</Message></Error>`))
			return
		}
		fake.ServeHTTP(w, r)
	}))
	defer srv.Close()

	a := &App{ctx: context.Background(), store: &profileStore{path: t.TempDir() + "/p.json"}}
	a.emitEvent = func(string, any) {}
	base := Profile{Name: "scoped", Provider: "minio", Endpoint: srv.URL, AccessKey: "a", SecretKey: "b"}

	if _, err := a.TestProfile(base); err == nil || !strings.Contains(err.Error(), "Bucket'lar") {
		t.Fatalf("expected hint about Buckets field, got %v", err)
	}

	p, _ := a.SaveProfile(base)
	if _, err := a.Connect(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ListBuckets(); err == nil {
		t.Fatal("expected denied error without pinned buckets")
	}
	c, _ := a.cli()
	if _, err := c.CreateBucket(a.ctx, &s3.CreateBucketInput{Bucket: aws.String("scoped")}); err != nil {
		t.Fatal(err)
	}
	if err := a.AddProfileBucket("missing"); err == nil {
		t.Fatal("linking an inaccessible bucket must fail")
	}
	if err := a.AddProfileBucket("scoped"); err != nil {
		t.Fatal(err)
	}
	bl, err := a.ListBuckets()
	if err != nil || len(bl.Buckets) != 1 || !bl.Buckets[0].Pinned || bl.Warning == "" {
		t.Fatalf("pinned listing: %+v %v", bl, err)
	}
	// persisted on the profile
	saved, _ := a.store.get(p.ID)
	if len(saved.Buckets) != 1 {
		t.Fatalf("not persisted: %+v", saved)
	}
	base.Buckets = []string{"scoped"}
	if msg, err := a.TestProfile(base); err != nil {
		t.Fatal(err)
	} else {
		t.Log(msg)
	}
	if err := a.RemoveProfileBucket("scoped"); err != nil {
		t.Fatal(err)
	}
	if saved, _ = a.store.get(p.ID); len(saved.Buckets) != 0 {
		t.Fatal("unlink not persisted")
	}
}

// Every message key must exist in every language.
func TestMessageCatalogsMatch(t *testing.T) {
	for lang, m := range messages {
		for k := range messages["en"] {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing %q", lang, k)
			}
		}
		for k := range m {
			if _, ok := messages["en"][k]; !ok {
				t.Errorf("en: missing %q (present in %s)", k, lang)
			}
		}
	}
}

package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// multipartServer answers the multipart listing calls that the fake S3
// server does not implement, and records aborted uploads.
func multipartServer(aborted *[]string) http.Handler {
	fake := gofakes3.New(s3mem.New()).Server()
	var mu sync.Mutex
	stamp := func(age time.Duration) string { return time.Now().Add(-age).UTC().Format("2006-01-02T15:04:05.000Z") }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		mu.Lock()
		defer mu.Unlock()
		gone := func(id string) bool { return strings.Contains(strings.Join(*aborted, ","), id) }
		switch {
		case r.Method == http.MethodGet && query.Has("uploads"):
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<ListMultipartUploadsResult><Bucket>test</Bucket><IsTruncated>false</IsTruncated>`)
			for _, u := range []struct {
				key, id string
				age     time.Duration
			}{{"recent.bin", "new-1", time.Hour}, {"old/video.mp4", "old-1", 72 * time.Hour}, {"old/backup.tar", "old-2", 30 * 24 * time.Hour}} {
				if !gone(u.id) {
					fmt.Fprintf(w, `<Upload><Key>%s</Key><UploadId>%s</UploadId><Initiated>%s</Initiated></Upload>`, u.key, u.id, stamp(u.age))
				}
			}
			fmt.Fprint(w, `</ListMultipartUploadsResult>`)
		case r.Method == http.MethodGet && query.Has("uploadId"):
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<ListPartsResult><IsTruncated>false</IsTruncated><Part><PartNumber>1</PartNumber><Size>5000</Size></Part><Part><PartNumber>2</PartNumber><Size>1200</Size></Part></ListPartsResult>`)
		case r.Method == http.MethodDelete && query.Has("uploadId"):
			*aborted = append(*aborted, query.Get("uploadId"))
			w.WriteHeader(http.StatusNoContent)
		default:
			mu.Unlock()
			fake.ServeHTTP(w, r)
			mu.Lock()
		}
	})
}

func TestIncompleteUploads(t *testing.T) {
	var aborted []string
	a := connectedApp(t, multipartServer(&aborted))
	uploads, err := a.ListIncompleteUploads("test")
	if err != nil || len(uploads) != 3 {
		t.Fatalf("uploads: %+v, %v", uploads, err)
	}
	// Oldest first; only uploads older than a day are stale.
	if uploads[0].UploadID != "old-2" || !uploads[0].Stale || uploads[0].Size != 6200 || uploads[2].UploadID != "new-1" || uploads[2].Stale {
		t.Fatalf("order or staleness: %+v", uploads)
	}
	count, err := a.AbortStaleUploads("test")
	if err != nil || count != 2 || strings.Join(aborted, ",") != "old-2,old-1" {
		t.Fatalf("aborted %d (%v), %v", count, aborted, err)
	}
	if uploads, err = a.ListIncompleteUploads("test"); err != nil || len(uploads) != 1 || uploads[0].UploadID != "new-1" {
		t.Fatalf("remaining uploads: %+v, %v", uploads, err)
	}
	if count, err = a.AbortStaleUploads("test"); err != nil || count != 0 {
		t.Fatalf("second cleanup: %d, %v", count, err)
	}
}

func TestBucketVersioning(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	if status, err := a.BucketVersioning("test"); err != nil || status != "" {
		t.Fatalf("initial status: %q, %v", status, err)
	}
	if err := a.SetBucketVersioning("test", true); err != nil {
		t.Fatal(err)
	}
	if status, err := a.BucketVersioning("test"); err != nil || status != "Enabled" {
		t.Fatalf("after enabling: %q, %v", status, err)
	}
	if err := a.SetBucketVersioning("test", false); err != nil {
		t.Fatal(err)
	}
	if status, err := a.BucketVersioning("test"); err != nil || status != "Suspended" {
		t.Fatalf("after suspending: %q, %v", status, err)
	}
	if err := a.SetBucketVersioning("", true); err == nil {
		t.Fatal("accepted an empty bucket name")
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestProfileBackupRoundTrip(t *testing.T) {
	s := &profileStore{path: filepath.Join(t.TempDir(), "profiles.json")}
	original, err := s.upsert(Profile{Name: "Production", Provider: "minio", Endpoint: "https://storage.example.com", AccessKey: "access", SecretKey: "secret", SessionToken: "token", Buckets: []string{"photos"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.exportProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var backup profileBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatal(err)
	}
	p := backup.Profiles[0]
	if p.ID != "" || p.AccessKey != "" || p.SecretKey != "" || p.SessionToken != "" {
		t.Fatal("backup contains credentials or an existing profile ID")
	}
	count, err := s.importProfiles(data)
	if err != nil || count != 1 {
		t.Fatalf("import: %d, %v", count, err)
	}
	profiles, err := s.load()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles: %v, %v", profiles, err)
	}
	for _, imported := range profiles {
		if imported.ID == original.ID {
			if imported.SecretKey != "secret" {
				t.Fatal("existing credentials were changed")
			}
			continue
		}
		if imported.ID == "" || imported.Endpoint != original.Endpoint || !imported.PathStyle || len(imported.Buckets) != 1 {
			t.Fatalf("invalid imported profile: %+v", imported)
		}
	}
}

func TestInvalidProfileImportLeavesStoreUntouched(t *testing.T) {
	s := &profileStore{path: filepath.Join(t.TempDir(), "profiles.json")}
	if _, err := s.upsert(Profile{Name: "Existing"}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{`, `{"version":2,"profiles":[]}`, `{"version":1}`, `{"version":1,"profiles":[{"name":"Valid"},{"name":" "}]}`} {
		if _, err := s.importProfiles([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
		profiles, err := s.load()
		if err != nil || len(profiles) != 1 || profiles[0].Name != "Existing" {
			t.Fatalf("store changed: %v, %v", profiles, err)
		}
	}
}

func TestUploadDefaultsFromProfile(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	fake := gofakes3.New(s3mem.New()).Server()
	a := connectedApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/docs/") {
			mu.Lock()
			seen = append(seen, r.Header.Get("x-amz-storage-class")+"|"+r.Header.Get("x-amz-server-side-encryption")+"|"+r.Header.Get("x-amz-server-side-encryption-aws-kms-key-id"))
			mu.Unlock()
		}
		fake.ServeHTTP(w, r)
	}))
	a.mu.Lock()
	a.profile.StorageClass, a.profile.Encryption, a.profile.KMSKey = "STANDARD_IA", "aws:kms", "alias/docs"
	a.mu.Unlock()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UploadPaths("test", "docs/", []string{filepath.Join(dir, "a.txt")}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "STANDARD_IA|aws:kms|alias/docs" {
		t.Fatalf("upload headers: %v", seen)
	}
	// Unknown values are dropped when a profile is saved.
	p := normalize(Profile{StorageClass: "WEIRD", Encryption: "ROT13", KMSKey: "k"})
	if p.StorageClass != "" || p.Encryption != "" || p.KMSKey != "" {
		t.Fatalf("normalize kept invalid defaults: %+v", p)
	}
}

func TestMoveBetweenProfilesOnSameStorageRefusesSameLocation(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	putObject(t, a, "docs/keep.txt", "precious")
	a.mu.RLock()
	active := a.profile
	a.mu.RUnlock()
	// A second profile for the same service and bucket.
	twin, err := a.SaveProfile(Profile{Name: "twin", Provider: active.Provider, Endpoint: active.Endpoint, Region: active.Region, AccessKey: active.AccessKey, SecretKey: active.SecretKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CopyToProfile("test", "docs/", []string{"docs/keep.txt"}, twin.ID, "test", "docs/", true); err == nil || err.Error() != T("copySameLocation") {
		t.Fatalf("move onto itself: %v", err)
	}
	if text, err := a.PreviewText("test", "docs/keep.txt"); err != nil || text != "precious" {
		t.Fatalf("object after refused move: %q, %v", text, err)
	}
	// Moving to another folder of the same storage still works and keeps the data.
	if n, err := a.CopyToProfile("test", "docs/", []string{"docs/keep.txt"}, twin.ID, "test", "archive/", true); err != nil || n != 1 {
		t.Fatalf("move to another folder: %d, %v", n, err)
	}
	if text, err := a.PreviewText("test", "archive/keep.txt"); err != nil || text != "precious" {
		t.Fatalf("moved object: %q, %v", text, err)
	}
	if _, err := a.PreviewText("test", "docs/keep.txt"); err == nil {
		t.Fatal("source survived the move")
	}
}

func TestUploadFolderWithUnreadableSubfolderFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if _, err := a.UploadPaths("test", "", []string{dir}); err == nil {
		t.Fatal("an unreadable subfolder was silently skipped")
	}
}

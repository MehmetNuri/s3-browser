package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
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

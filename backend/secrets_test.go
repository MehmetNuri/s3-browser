package main

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCipher stands in for the operating system's credential store.
type fakeCipher struct {
	available bool
	locked    bool
}

func (f *fakeCipher) Available() bool { return f.available }
func (f *fakeCipher) Encrypt(plain string) (string, error) {
	return secretPrefix + base64.StdEncoding.EncodeToString([]byte("sealed:"+plain)), nil
}
func (f *fakeCipher) Decrypt(sealed string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, secretPrefix))
	if err != nil || f.locked {
		return "", errors.New("keychain is locked")
	}
	return strings.TrimPrefix(string(data), "sealed:"), nil
}

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	cipher := &fakeCipher{available: true}
	s := &profileStore{path: filepath.Join(t.TempDir(), "profiles.json"), cipher: cipher}
	saved, err := s.upsert(Profile{Name: "prod", AccessKey: "AKIAEXAMPLE", SecretKey: "top-secret", SessionToken: "session-token"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.SecretKey != "top-secret" {
		t.Fatalf("caller received a sealed value: %q", saved.SecretKey)
	}
	file, err := os.ReadFile(s.path)
	if err != nil || strings.Contains(string(file), "top-secret") || strings.Contains(string(file), "session-token") {
		t.Fatalf("plaintext secret on disk: %s, %v", file, err)
	}
	loaded, err := s.get(saved.ID)
	if err != nil || loaded.SecretKey != "top-secret" || loaded.SessionToken != "session-token" || loaded.AccessKey != "AKIAEXAMPLE" {
		t.Fatalf("round trip: %+v, %v", loaded, err)
	}

	// A locked keychain must not destroy stored secrets when another profile is saved.
	cipher.locked = true
	locked, err := s.get(saved.ID)
	if err != nil || !isSealed(locked.SecretKey) {
		t.Fatalf("locked profile: %+v, %v", locked, err)
	}
	if _, err := newClient(t.Context(), locked); err == nil {
		t.Fatal("connected with an undecrypted secret")
	}
	if _, err := s.upsert(Profile{Name: "other", SecretKey: "second"}); err != nil {
		t.Fatal(err)
	}
	cipher.locked = false
	if restored, err := s.get(saved.ID); err != nil || restored.SecretKey != "top-secret" {
		t.Fatalf("secret lost while locked: %+v, %v", restored, err)
	}
}

func TestPlaintextSecretsMigrateWhenKeychainAppears(t *testing.T) {
	s := &profileStore{path: filepath.Join(t.TempDir(), "profiles.json")}
	saved, err := s.upsert(Profile{Name: "legacy", SecretKey: "old-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if file, _ := os.ReadFile(s.path); !strings.Contains(string(file), "old-secret") {
		t.Fatal("expected the documented plaintext format without a keychain")
	}
	s.cipher = &fakeCipher{available: false}
	if _, err := s.list(); err != nil {
		t.Fatal(err)
	}
	if file, _ := os.ReadFile(s.path); !strings.Contains(string(file), "old-secret") {
		t.Fatal("encrypted without a usable keychain")
	}
	s.cipher = &fakeCipher{available: true}
	loaded, err := s.get(saved.ID)
	if err != nil || loaded.SecretKey != "old-secret" {
		t.Fatalf("migrated profile: %+v, %v", loaded, err)
	}
	if file, _ := os.ReadFile(s.path); strings.Contains(string(file), "old-secret") {
		t.Fatalf("plaintext secret left on disk: %s", file)
	}
}

func TestProfileInputIsTrimmed(t *testing.T) {
	s := &profileStore{path: filepath.Join(t.TempDir(), "profiles.json")}
	a := &App{store: s}
	saved, err := a.SaveProfile(Profile{Name: "  prod \n", Provider: "supabase", ProjectRef: " abcd ", Region: " eu-central-1 ",
		AccessKey: "\tAKIA \n", SecretKey: " secret\n", SessionToken: " token ", Buckets: []string{" photos ", "photos", ""}})
	if err != nil {
		t.Fatal(err)
	}
	want := Profile{ID: saved.ID, Name: "prod", Provider: "supabase", ProjectRef: "abcd", Region: "eu-central-1", AccessKey: "AKIA", SecretKey: "secret",
		SessionToken: "token", PathStyle: true, Endpoint: "https://abcd.storage.supabase.co/storage/v1/s3"}
	if saved.Name != want.Name || saved.ProjectRef != want.ProjectRef || saved.Region != want.Region || saved.AccessKey != want.AccessKey ||
		saved.SecretKey != want.SecretKey || saved.SessionToken != want.SessionToken || saved.Endpoint != want.Endpoint || len(saved.Buckets) != 1 || saved.Buckets[0] != "photos" {
		t.Fatalf("not trimmed: %+v", saved)
	}
	if _, err := a.SaveProfile(Profile{Name: " \t "}); err == nil {
		t.Fatal("accepted a blank name")
	}
	if err := s.remove(saved.ID); err != nil {
		t.Fatal(err)
	}
	// An empty store must stay a JSON list.
	s.cipher = &fakeCipher{available: true}
	if err := s.save(nil); err != nil {
		t.Fatal(err)
	}
	if file, _ := os.ReadFile(s.path); strings.TrimSpace(string(file)) != "[]" {
		t.Fatalf("empty store written as %q", file)
	}
}

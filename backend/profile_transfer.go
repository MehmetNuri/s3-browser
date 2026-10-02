package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	runtime "s3browser/internal/desktop"
)

type profileBackup struct {
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
}

func (s *profileStore) exportProfiles() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.load()
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		profiles[i].ID = ""
		profiles[i].AccessKey = ""
		profiles[i].SecretKey = ""
		profiles[i].SessionToken = ""
	}
	return json.MarshalIndent(profileBackup{Version: 1, Profiles: profiles}, "", "  ")
}

func (s *profileStore) importProfiles(data []byte) (int, error) {
	var backup profileBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return 0, errors.New(T("invalidProfileBackup"))
	}
	if backup.Version != 1 || backup.Profiles == nil {
		return 0, errors.New(T("invalidProfileBackup"))
	}
	for i := range backup.Profiles {
		p := normalize(backup.Profiles[i])
		if strings.TrimSpace(p.Name) == "" {
			return 0, errors.New(T("nameRequired"))
		}
		p.ID = randID()
		backup.Profiles[i] = p
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.load()
	if err != nil {
		return 0, err
	}
	if err := s.save(append(profiles, backup.Profiles...)); err != nil {
		return 0, err
	}
	return len(backup.Profiles), nil
}

func (a *App) ExportProfiles() (bool, error) {
	file, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: T("exportProfiles"), DefaultFilename: "s3browser-profiles.json",
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil || file == "" {
		return false, err
	}
	destination, err := filepath.Abs(file)
	if err != nil {
		return false, err
	}
	storePath, err := filepath.Abs(a.store.path)
	if err != nil {
		return false, err
	}
	if destination == storePath {
		return false, errors.New(T("profileBackupDestination"))
	}
	if dst, err := os.Stat(file); err == nil {
		if src, err := os.Stat(a.store.path); err == nil && os.SameFile(src, dst) {
			return false, errors.New(T("profileBackupDestination"))
		}
	}
	data, err := a.store.exportProfiles()
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(file, data, 0o600)
}

func (a *App) ImportProfiles() (int, error) {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   T("importProfiles"),
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil || file == "" {
		return 0, err
	}
	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	const limit = 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return 0, err
	}
	if len(data) > limit {
		return 0, errors.New(T("profileBackupTooLarge"))
	}
	return a.store.importProfiles(data)
}

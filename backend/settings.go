package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"s3browser/internal/desktop"
	"strings"
)

type Settings struct {
	CloseToTray bool `json:"closeToTray"`
}

func settingsPath(store *profileStore) string {
	return filepath.Join(filepath.Dir(store.path), "settings.json")
}
func loadSettings(store *profileStore) Settings {
	s := Settings{CloseToTray: true}
	if b, err := os.ReadFile(settingsPath(store)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}
func (a *App) GetSettings() Settings { a.mu.RLock(); defer a.mu.RUnlock(); return a.settings }
func (a *App) SetCloseToTray(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := Settings{CloseToTray: enabled}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivateFile(settingsPath(a.store), data); err != nil {
		return err
	}
	a.settings = settings
	return nil
}

// Shown in the About dialog. The version numbers come from the desktop host,
// which knows the packaged application and the Electron runtime it runs on.
const (
	appName      = "S3 Browser"
	appCopyright = "Copyright © 2026 Mehmet Nuri Öztürk"
)

type AppInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Copyright       string `json:"copyright"`
	GoVersion       string `json:"goVersion"`
	ElectronVersion string `json:"electronVersion"`
	Platform        string `json:"platform"`
	ConfigDir       string `json:"configDir"`
	// "keychain" when credentials are encrypted with the OS credential store, otherwise "plaintext".
	SecretStorage string `json:"secretStorage"`
}

func (a *App) GetAppInfo() AppInfo {
	storage := "plaintext"
	if a.store.cipher != nil && a.store.cipher.Available() {
		storage = "keychain"
	}
	return AppInfo{
		Name: appName, Version: os.Getenv("S3BROWSER_APP_VERSION"), Copyright: appCopyright,
		GoVersion: strings.TrimPrefix(goruntime.Version(), "go"), ElectronVersion: os.Getenv("S3BROWSER_ELECTRON_VERSION"),
		Platform: goruntime.GOOS + "/" + goruntime.GOARCH, ConfigDir: filepath.Dir(a.store.path), SecretStorage: storage,
	}
}
func (a *App) OpenConfigDir() error {
	return desktop.Call(a.ctx, "openConfigDir", filepath.Dir(a.store.path), nil)
}

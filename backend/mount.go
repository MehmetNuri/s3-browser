package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Mount is a bucket, or a folder of it, attached to the local file system.
type Mount struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`
	Profile   string `json:"profile"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Path      string `json:"path"`
	ReadOnly  bool   `json:"readOnly"`
}

// MountEvent tells the frontend that a mount appeared or went away.
type MountEvent struct {
	ID    string `json:"id"`
	State string `json:"state"` // mounted | unmounted
	Error string `json:"error"`
}

const (
	mountRootName = "S3 Browser" // folder under the home directory that holds the mount points
	maxMounts     = 16
)

var unsafeMountChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// mountPoint picks the folder for a mount: ~/S3 Browser/<profile>-<bucket>[-<prefix>].
func mountPoint(profile, bucket, prefix string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := unsafeMountChars.ReplaceAllString(profile, "-") + "-" + unsafeMountChars.ReplaceAllString(bucket, "-")
	if prefix != "" {
		name += "-" + unsafeMountChars.ReplaceAllString(strings.TrimSuffix(prefix, "/"), "-")
	}
	name = strings.Trim(name, "-.")
	if name == "" || len(name) > 120 {
		return "", errors.New(T("mountNameInvalid"))
	}
	return filepath.Join(home, mountRootName, name), nil
}

// ListMounts returns the active mounts, oldest first.
func (a *App) ListMounts() []Mount {
	a.mountsMu.Lock()
	defer a.mountsMu.Unlock()
	list := make([]Mount, 0, len(a.mounts))
	for _, m := range a.mounts {
		list = append(list, m.info)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// OpenMountFolder shows a mount point in the desktop's file manager.
func (a *App) OpenMountFolder(id string) error {
	a.mountsMu.Lock()
	m, ok := a.mounts[id]
	a.mountsMu.Unlock()
	if !ok {
		return errors.New(T("mountNotFound"))
	}
	return a.openMountFolder(m.info.Path)
}

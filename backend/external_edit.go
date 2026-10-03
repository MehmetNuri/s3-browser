package main

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	runtime "s3browser/internal/desktop"
)

// EditEvent tells the frontend that an object opened in an external
// application was uploaded again after it changed on disk.
type EditEvent struct {
	Key   string `json:"key"`
	State string `json:"state"` // uploaded | error
	Error string `json:"error"`
}

type externalEdit struct {
	bucket, key, local string
	client             *s3.Client
	opts               uploadOptions
	stop               chan struct{} // closed when the edit is forgotten
	unsynced           bool          // the last change could not be uploaded
	lastAttempt        time.Time
	modified           time.Time
	size               int64
}

// File types the desktop would execute rather than open in an editor.
var executableExtensions = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true,
	".wsf": true, ".wsh": true, ".hta": true, ".msi": true, ".msp": true, ".scr": true, ".pif": true, ".lnk": true, ".url": true,
	".desktop": true, ".app": true, ".jar": true, ".reg": true, ".sh": true, ".run": true, ".appimage": true, ".dmg": true, ".pkg": true,
}

// Watching stops after editWatchLimit; the file stays for the editor.
var (
	editWatchInterval = 2 * time.Second
	editWatchLimit    = 8 * time.Hour
	editRetryInterval = 30 * time.Second // between attempts after a failed upload
)

// Each open edit keeps a watcher goroutine and a temporary copy.
const maxExternalEdits = 50

// EditExternally downloads an object to a private temporary folder, opens it
// with the desktop's default application and uploads it again whenever the
// file changes, like editing with an external editor in Cyberduck.
func (a *App) EditExternally(bucket, key string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if key == "" || strings.HasSuffix(key, "/") {
		return errors.New(T("notAnObject"))
	}
	name := filepath.Base(filepath.FromSlash(path.Base(key)))
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		name = "object"
	}
	if executableExtensions[strings.ToLower(filepath.Ext(name))] {
		return errors.New(T("editNotAllowed"))
	}
	a.editsMu.Lock()
	edit, known := a.edits[bucket+"\x00"+key]
	if known && edit == nil {
		a.editsMu.Unlock()
		return errors.New(T("transferAlreadyRunning")) // another call is still downloading it
	}
	if known {
		// The temp cleaner may have removed the copy; fetch it again.
		if _, err := os.Stat(edit.local); err != nil {
			delete(a.edits, bucket+"\x00"+key)
			close(edit.stop)
			known = false
		}
	}
	if !known && len(a.edits) >= maxExternalEdits {
		a.editsMu.Unlock()
		return errors.New(T("tooManyEdits", maxExternalEdits))
	}
	if !known {
		if a.edits == nil {
			a.edits = make(map[string]*externalEdit)
		}
		a.edits[bucket+"\x00"+key] = nil // reserves the slot while downloading
	}
	a.editsMu.Unlock()
	if !known {
		dir, err := os.MkdirTemp("", "s3browser-edit-")
		if err != nil {
			a.forgetEdit(bucket, key, "")
			return err
		}
		if err := a.queueDownload(c, bucket, key, "", dir, name).wait(); err != nil {
			a.forgetEdit(bucket, key, dir)
			return describeErr(err)
		}
		edit = &externalEdit{bucket: bucket, key: key, local: filepath.Join(dir, name), client: c, opts: a.activeUploadOptions(), stop: make(chan struct{})}
		st, err := os.Stat(edit.local)
		if err != nil {
			a.forgetEdit(bucket, key, dir)
			return err
		}
		edit.modified, edit.size = st.ModTime(), st.Size()
		a.editsMu.Lock()
		a.edits[bucket+"\x00"+key] = edit
		a.editsMu.Unlock()
		go a.watchEdit(edit)
	}
	return runtime.OpenEditFile(a.ctx, edit.local)
}

// forgetEdit drops a reservation or edit and removes its temporary folder.
func (a *App) forgetEdit(bucket, key, dir string) {
	a.editsMu.Lock()
	delete(a.edits, bucket+"\x00"+key)
	a.editsMu.Unlock()
	if dir != "" {
		os.RemoveAll(dir)
	}
}

// releaseEdit ends one watcher: the entry is dropped only if it still belongs
// to it, and the temporary copy is removed unless keep asks to leave it.
func (a *App) releaseEdit(edit *externalEdit, keep bool) {
	a.editsMu.Lock()
	if current := a.edits[edit.bucket+"\x00"+edit.key]; current == edit {
		delete(a.edits, edit.bucket+"\x00"+edit.key)
	}
	a.editsMu.Unlock()
	if !keep {
		os.RemoveAll(filepath.Dir(edit.local))
	}
}

// watchEdit polls the file; a change that stayed stable for one interval is
// uploaded, so a half-written save is not sent. The copy is removed when the
// watch ends or the application exits.
func (a *App) watchEdit(edit *externalEdit) {
	ticker := time.NewTicker(editWatchInterval)
	defer ticker.Stop()
	deadline := time.After(editWatchLimit)
	defer func() {
		// A change that never reached the bucket is kept on disk and reported.
		if edit.unsynced {
			a.releaseEdit(edit, true)
			a.emitEvent("edit", EditEvent{Key: edit.key, State: "kept", Error: edit.local})
			return
		}
		a.releaseEdit(edit, false)
	}()
	var pending *os.FileInfo
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-deadline:
			return
		case <-edit.stop:
			return
		case <-ticker.C:
		}
		st, err := os.Stat(edit.local)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		changed := !st.ModTime().Equal(edit.modified) || st.Size() != edit.size
		if !changed {
			// A failed upload is retried on its own, even without a new change.
			if edit.unsynced && time.Since(edit.lastAttempt) >= editRetryInterval {
				a.uploadEdit(edit)
			}
			continue
		}
		if pending == nil || !(*pending).ModTime().Equal(st.ModTime()) || (*pending).Size() != st.Size() {
			pending = &st // changed since the last look; wait for it to settle
			continue
		}
		pending = nil
		edit.modified, edit.size = st.ModTime(), st.Size()
		a.uploadEdit(edit)
	}
}

func (a *App) uploadEdit(edit *externalEdit) {
	edit.lastAttempt = time.Now()
	up := manager.NewUploader(edit.client, func(u *manager.Uploader) { u.PartSize = 8 << 20 })
	job := uploadJob{local: edit.local, key: edit.key, size: edit.size, modified: edit.modified}
	err := a.queueUploadQuiet(up, "upload", edit.bucket, job, edit.opts).wait()
	event := EditEvent{Key: edit.key, State: "uploaded"}
	switch {
	case err == nil:
		edit.unsynced = false
	case errors.Is(err, context.Canceled):
		edit.unsynced = true
		event.State = "cancelled"
	default:
		edit.unsynced = true
		event.State, event.Error = "error", describeErr(err).Error()
	}
	a.emitEvent("edit", event)
}

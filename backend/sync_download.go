package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	desktop "s3browser/internal/desktop"
)

// SyncToFolder downloads the objects under a prefix that are missing or newer
// in the bucket into a local folder the user picks. With mirror, local files
// that have no object anymore are deleted afterwards; the bucket is never
// changed.
func (a *App) SyncToFolder(bucket, prefix string, mirror bool) (SyncResult, error) {
	c, err := a.cli()
	if err != nil {
		return SyncResult{}, err
	}
	dir, err := desktop.OpenDirectoryDialog(a.ctx, desktop.OpenDialogOptions{Title: T("dlgSyncToFolder")})
	if err != nil || dir == "" {
		return SyncResult{}, err
	}
	return a.syncToDirectory(c, bucket, prefix, dir, mirror)
}

type remoteFile struct {
	key, name string // name is the path relative to the folder, in slash form
	size      int64
	modified  time.Time
}

func (a *App) syncToDirectory(c *s3.Client, bucket, prefix, dir string, mirror bool) (SyncResult, error) {
	b := a.startBatch("sync", 0)
	defer b.end()
	var remote []remoteFile
	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return SyncResult{}, describeErr(err)
		}
		for _, o := range out.Contents {
			key := aws.ToString(o.Key)
			name := strings.TrimPrefix(key, prefix)
			// Folder markers and keys that would escape the folder are skipped.
			if name == "" || strings.HasSuffix(name, "/") || !filepath.IsLocal(filepath.FromSlash(name)) {
				continue
			}
			remote = append(remote, remoteFile{key, name, aws.ToInt64(o.Size), aws.ToTime(o.LastModified)})
		}
	}
	// A wrong or empty prefix would otherwise wipe the whole local folder.
	if mirror && len(remote) == 0 {
		return SyncResult{}, errors.New(T("mirrorEmptyRemote"))
	}
	var changed []remoteFile
	wanted := make(map[string]bool, len(remote))
	for _, file := range remote {
		local := filepath.Join(dir, filepath.FromSlash(file.name))
		wanted[local] = true
		// A file of the same size changed after the object was stored is current.
		if st, err := os.Stat(local); err == nil && st.Mode().IsRegular() && st.Size() == file.size && !st.ModTime().Before(file.modified) {
			continue
		}
		changed = append(changed, file)
	}
	result := SyncResult{Skipped: len(remote) - len(changed)}
	b.setTotal(len(changed))
	tasks := make([]*transferTask, len(changed))
	for i, file := range changed {
		tasks[i] = a.queueDownload(c, bucket, file.key, "", dir, filepath.FromSlash(file.name))
	}
	for _, task := range tasks {
		err := task.wait()
		b.finish(err)
		if err != nil {
			result.Failed++
		}
	}
	result.Downloaded = len(changed) - result.Failed
	if result.Failed > 0 {
		return result, b.failure("downloadFailed")
	}
	if mirror {
		// Only after every download succeeded, and only regular files: folders,
		// links and anything special stay as they are.
		root, err := os.OpenRoot(dir)
		if err != nil {
			return result, err
		}
		defer root.Close()
		// macOS and Windows match names regardless of case, so a local file
		// that only differs in case from an object is the object's copy.
		caseInsensitive := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
		wantedFolded := make(map[string]bool, len(wanted))
		for local := range wanted {
			wantedFolded[strings.ToLower(local)] = true
		}
		var extra []string
		_ = filepath.WalkDir(dir, func(fp string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".s3browser-download-") {
				return nil
			}
			if wanted[fp] || (caseInsensitive && wantedFolded[strings.ToLower(fp)]) {
				return nil
			}
			extra = append(extra, fp)
			return nil
		})
		for _, fp := range extra {
			rel, err := filepath.Rel(dir, fp)
			if err != nil {
				continue
			}
			if err := root.Remove(rel); err != nil {
				return result, err
			}
			result.Deleted++
		}
	}
	return result, nil
}

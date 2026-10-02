package main

import (
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	runtime "s3browser/internal/desktop"
)

type SyncResult struct {
	Uploaded int `json:"uploaded"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	Deleted  int `json:"deleted"` // mirror mode only
}

// SyncFolder uploads the files of a local folder that are missing or changed
// in the bucket. With mirror, objects that no longer exist in the folder are
// deleted from the bucket afterwards; the local folder is never changed.
func (a *App) SyncFolder(bucket, prefix string, mirror bool) (SyncResult, error) {
	c, err := a.cli()
	if err != nil {
		return SyncResult{}, err
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: T("dlgSyncFolder")})
	if err != nil || dir == "" {
		return SyncResult{}, err
	}
	return a.syncDirectory(c, bucket, prefix, dir, mirror)
}

func (a *App) syncDirectory(c *s3.Client, bucket, prefix, dir string, mirror bool) (SyncResult, error) {
	// The batch starts without a total while both sides are compared.
	b := a.startBatch("sync", 0)
	defer b.end()
	jobs, err := collectUploads(prefix, []string{dir})
	if err != nil {
		return SyncResult{}, err
	}
	type remoteObject struct {
		size     int64
		modified time.Time
	}
	remote := make(map[string]remoteObject)
	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(prefix + filepath.Base(dir) + "/"),
	})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return SyncResult{}, describeErr(err)
		}
		for _, o := range out.Contents {
			remote[aws.ToString(o.Key)] = remoteObject{aws.ToInt64(o.Size), aws.ToTime(o.LastModified)}
		}
	}
	// An empty or unmounted source would otherwise wipe the whole backup.
	if mirror && len(jobs) == 0 && len(remote) > 0 {
		return SyncResult{}, errors.New(T("mirrorEmptySource", len(remote)))
	}
	local := make(map[string]bool, len(jobs))
	var changed []uploadJob
	for _, job := range jobs {
		local[job.key] = true
		// An object of the same size uploaded after the file last changed is current.
		if existing, ok := remote[job.key]; ok && existing.size == job.size && !existing.modified.Before(job.modified) {
			continue
		}
		changed = append(changed, job)
	}
	result := SyncResult{Skipped: len(jobs) - len(changed)}
	b.setTotal(len(changed))
	result.Failed = a.runUploads(c, bucket, changed, b)
	result.Uploaded = len(changed) - result.Failed
	if result.Failed > 0 {
		return result, b.failure("uploadFailed")
	}
	if mirror {
		// Only after every upload succeeded; folder markers are left alone.
		var extra []string
		for key := range remote {
			if !local[key] && !strings.HasSuffix(key, "/") {
				extra = append(extra, key)
			}
		}
		if err := a.deleteBatch(c, bucket, extra); err != nil {
			return result, err
		}
		result.Deleted = len(extra)
	}
	return result, nil
}

type ObjectVersion struct {
	VersionID    string `json:"versionId"`
	Modified     string `json:"modified"`
	Size         int64  `json:"size"`
	IsLatest     bool   `json:"isLatest"`
	DeleteMarker bool   `json:"deleteMarker"`
}

const versionListLimit = 1000

// ListVersions returns the stored versions of one object, newest first.
func (a *App) ListVersions(bucket, key string) ([]ObjectVersion, error) {
	c, err := a.cli()
	if err != nil {
		return nil, err
	}
	type dated struct {
		ObjectVersion
		at time.Time
	}
	var found []dated
	p := s3.NewListObjectVersionsPaginator(c, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket), Prefix: aws.String(key)})
	for p.HasMorePages() && len(found) < versionListLimit {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return nil, describeErr(err)
		}
		// The prefix also matches longer keys; keep only this object.
		for _, v := range out.Versions {
			if aws.ToString(v.Key) == key {
				found = append(found, dated{ObjectVersion{VersionID: aws.ToString(v.VersionId), Modified: fmtTime(v.LastModified), Size: aws.ToInt64(v.Size), IsLatest: aws.ToBool(v.IsLatest)}, aws.ToTime(v.LastModified)})
			}
		}
		for _, m := range out.DeleteMarkers {
			if aws.ToString(m.Key) == key {
				found = append(found, dated{ObjectVersion{VersionID: aws.ToString(m.VersionId), Modified: fmtTime(m.LastModified), IsLatest: aws.ToBool(m.IsLatest), DeleteMarker: true}, aws.ToTime(m.LastModified)})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].IsLatest != found[j].IsLatest {
			return found[i].IsLatest
		}
		return found[i].at.After(found[j].at)
	})
	versions := make([]ObjectVersion, len(found))
	for i, v := range found {
		versions[i] = v.ObjectVersion
	}
	return versions, nil
}

// RestoreVersion makes an older version current again by copying it on top.
func (a *App) RestoreVersion(bucket, key, version string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if version == "" {
		return errors.New(T("versionRequired"))
	}
	_, err = c.CopyObject(a.ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		CopySource: aws.String(copySource(bucket, key) + "?versionId=" + url.QueryEscape(version)),
	})
	return describeErr(err)
}

// DeleteVersion permanently removes one version; the others are kept.
func (a *App) DeleteVersion(bucket, key, version string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	// Without a version ID this request would delete the current object instead.
	if version == "" {
		return errors.New(T("versionRequired"))
	}
	_, err = c.DeleteObject(a.ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), VersionId: aws.String(version)})
	return describeErr(err)
}

// DownloadVersion saves one version to a file chosen in a save dialog.
func (a *App) DownloadVersion(bucket, key, version string) (bool, error) {
	c, err := a.cli()
	if err != nil {
		return false, err
	}
	if version == "" {
		return false, errors.New(T("versionRequired"))
	}
	dst, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: T("dlgSaveVersion"), DefaultFilename: path.Base(key)})
	if err != nil || dst == "" {
		return false, err
	}
	b := a.startBatch("download", 1)
	defer b.end()
	err = a.downloadVersionTo(c, bucket, key, version, filepath.Dir(dst), filepath.Base(dst))
	b.finish(err)
	if err != nil {
		return false, err
	}
	return true, nil
}

type ObjectHeaders struct {
	ContentType        string `json:"contentType"`
	CacheControl       string `json:"cacheControl"`
	ContentDisposition string `json:"contentDisposition"`
	ContentEncoding    string `json:"contentEncoding"`
}

func validHeaderValue(value string) bool {
	return len(value) <= 1024 && !strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// UpdateObjectHeaders rewrites an object's HTTP headers in place. S3 can only
// do this by copying the object onto itself, so the user metadata and storage
// class are read first and sent again.
func (a *App) UpdateObjectHeaders(bucket, key string, headers ObjectHeaders) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	for _, value := range []*string{&headers.ContentType, &headers.CacheControl, &headers.ContentDisposition, &headers.ContentEncoding} {
		*value = strings.TrimSpace(*value)
		if !validHeaderValue(*value) {
			return errors.New(T("invalidHeader"))
		}
	}
	if headers.ContentType == "" {
		headers.ContentType = "application/octet-stream"
	}
	head, err := c.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return describeErr(err)
	}
	in := &s3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		CopySource:        aws.String(copySource(bucket, key)),
		MetadataDirective: types.MetadataDirectiveReplace,
		Metadata:          head.Metadata,
		ContentType:       aws.String(headers.ContentType),
		StorageClass:      head.StorageClass,
	}
	if headers.CacheControl != "" {
		in.CacheControl = aws.String(headers.CacheControl)
	}
	if headers.ContentDisposition != "" {
		in.ContentDisposition = aws.String(headers.ContentDisposition)
	}
	if headers.ContentEncoding != "" {
		in.ContentEncoding = aws.String(headers.ContentEncoding)
	}
	_, err = c.CopyObject(a.ctx, in)
	return describeErr(err)
}

// Search stops at these limits so a huge bucket cannot occupy the backend indefinitely.
const (
	searchScanLimit   = 200_000
	searchResultLimit = 500
)

type SearchResult struct {
	Items     []S3Object `json:"items"`
	Scanned   int64      `json:"scanned"`
	Truncated bool       `json:"truncated"` // stopped at a limit before the end
}

// SearchObjects finds objects below prefix whose key contains query,
// ignoring case and looking through every subfolder.
func (a *App) SearchObjects(bucket, prefix, query string) (SearchResult, error) {
	c, err := a.cli()
	if err != nil {
		return SearchResult{}, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return SearchResult{}, errors.New(T("searchQueryRequired"))
	}
	result := SearchResult{Items: []S3Object{}}
	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return SearchResult{}, describeErr(err)
		}
		for _, o := range out.Contents {
			result.Scanned++
			k := aws.ToString(o.Key)
			name := strings.TrimPrefix(k, prefix)
			if strings.HasSuffix(k, "/") || !strings.Contains(strings.ToLower(name), query) {
				continue
			}
			if len(result.Items) == searchResultLimit {
				result.Truncated = true
				return result, nil
			}
			result.Items = append(result.Items, S3Object{
				Key: k, Name: name, Size: aws.ToInt64(o.Size), Modified: fmtTime(o.LastModified),
				ETag: strings.Trim(aws.ToString(o.ETag), `"`), StorageClass: string(o.StorageClass),
			})
		}
		if result.Scanned >= searchScanLimit && p.HasMorePages() {
			result.Truncated = true
			break
		}
	}
	return result, nil
}

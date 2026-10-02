package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	runtime "s3browser/internal/desktop"
)

// App is bound to the frontend.
type App struct {
	ctx          context.Context
	store        *profileStore
	mu           sync.RWMutex
	client       *s3.Client
	profile      Profile
	nextID       atomic.Int64
	transfersMu  sync.Mutex
	transferJobs map[int64]*transferTask
	// emitEvent sends an event to the frontend (replaceable in tests).
	emitEvent func(name string, data any)

	settings Settings

	backupOnce  sync.Once
	backupStore *backupStore
}

func NewApp() *App {
	a := &App{store: newProfileStore()}
	a.settings = loadSettings(a.store)
	a.emitEvent = func(name string, data any) { runtime.EventsEmit(a.ctx, name, data) }
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.store.cipher = &hostCipher{ctx: ctx}
	go a.runBackupSchedule(ctx, time.Minute)
}

func (a *App) cli() (*s3.Client, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.client == nil {
		return nil, errors.New(T("notConnected"))
	}
	return a.client, nil
}

func (a *App) ListProfiles() ([]Profile, error) { return a.store.list() }

func (a *App) SaveProfile(p Profile) (Profile, error) {
	p = normalize(p)
	if strings.TrimSpace(p.Name) == "" {
		return p, errors.New(T("nameRequired"))
	}
	return a.store.upsert(p)
}

func (a *App) DeleteProfile(id string) error {
	a.mu.Lock()
	if a.profile.ID == id {
		a.client = nil
		a.profile = Profile{}
	}
	a.mu.Unlock()
	if err := a.store.remove(id); err != nil {
		return err
	}
	// Its backup jobs could never run again.
	return a.backups().removeProfile(id)
}

// TestProfile tries ListBuckets with an unsaved profile.
func (a *App) TestProfile(p Profile) (string, error) {
	c, err := newClient(a.ctx, p)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	out, err := c.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err == nil {
		return T("connOK", len(out.Buckets)), nil
	}
	p = normalize(p)
	if len(p.Buckets) == 0 {
		if status, _ := classify(err); status == "denied" {
			return "", errors.New(T("listDeniedHint", describeErr(err)))
		}
		return "", describeErr(err)
	}
	// Listing is not allowed; verify the hand-entered buckets instead.
	for _, b := range p.Buckets {
		if err := checkBucket(ctx, c, b); err != nil {
			return "", fmt.Errorf("%s: %v", b, err)
		}
	}
	return T("connOKPinned", len(p.Buckets)), nil
}

// checkBucket verifies read access to a single bucket with a 1-key listing,
// which only needs s3:ListBucket on that bucket.
func checkBucket(ctx context.Context, c *s3.Client, bucket string) error {
	_, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), MaxKeys: aws.Int32(1)})
	return describeErr(err)
}

// AddProfileBucket verifies access to a bucket and pins it to the active profile.
func (a *App) AddProfileBucket(name string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if err := checkBucket(a.ctx, c, name); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == nil {
		return errors.New(T("notConnected"))
	}
	a.profile.Buckets = append(a.profile.Buckets, name)
	p, err := a.store.upsert(normalize(a.profile))
	a.profile = p
	return err
}

// RemoveProfileBucket unpins a bucket from the active profile (the bucket itself is untouched).
func (a *App) RemoveProfileBucket(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Without an active profile the store would save a new, empty one.
	if a.client == nil {
		return errors.New(T("notConnected"))
	}
	out := []string{}
	for _, b := range a.profile.Buckets {
		if b != name {
			out = append(out, b)
		}
	}
	a.profile.Buckets = out
	p, err := a.store.upsert(a.profile)
	a.profile = p
	return err
}

func (a *App) Connect(id string) (Profile, error) {
	p, err := a.store.get(id)
	if err != nil {
		return p, err
	}
	c, err := newClient(a.ctx, p)
	if err != nil {
		return p, err
	}
	a.mu.Lock()
	a.client, a.profile = c, normalize(p)
	a.mu.Unlock()
	return p, nil
}

type Bucket struct {
	Name    string `json:"name"`
	Created string `json:"created"`
	Pinned  bool   `json:"pinned"` // added by hand to the profile
}

type BucketList struct {
	Buckets []Bucket `json:"buckets"`
	// Set when ListBuckets failed but pinned buckets are still shown.
	Warning string `json:"warning"`
}

// ListBuckets merges the account's buckets with the profile's pinned ones.
// If listing is denied, pinned buckets are returned with a warning.
func (a *App) ListBuckets() (BucketList, error) {
	c, err := a.cli()
	if err != nil {
		return BucketList{}, err
	}
	a.mu.RLock()
	pinned := append([]string(nil), a.profile.Buckets...)
	a.mu.RUnlock()
	return a.bucketList(c, pinned)
}

func (a *App) bucketList(c *s3.Client, pinned []string) (BucketList, error) {
	res := BucketList{Buckets: []Bucket{}}
	seen := map[string]bool{}
	out, err := c.ListBuckets(a.ctx, &s3.ListBucketsInput{})
	if err != nil {
		if len(pinned) == 0 {
			return res, describeErr(err)
		}
		res.Warning = describeErr(err).Error()
	} else {
		for _, b := range out.Buckets {
			n := aws.ToString(b.Name)
			seen[n] = true
			res.Buckets = append(res.Buckets, Bucket{Name: n, Created: fmtTime(b.CreationDate)})
		}
	}
	for _, n := range pinned {
		if !seen[n] {
			res.Buckets = append(res.Buckets, Bucket{Name: n, Pinned: true})
		} else {
			for i := range res.Buckets {
				if res.Buckets[i].Name == n {
					res.Buckets[i].Pinned = true
				}
			}
		}
	}
	sort.Slice(res.Buckets, func(i, j int) bool { return res.Buckets[i].Name < res.Buckets[j].Name })
	return res, nil
}

func (a *App) CreateBucket(name string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	in := &s3.CreateBucketInput{Bucket: aws.String(strings.TrimSpace(name))}
	a.mu.RLock()
	region := a.profile.Region
	provider := a.profile.Provider
	a.mu.RUnlock()
	if provider == "aws" && region != "" && region != "us-east-1" {
		in.CreateBucketConfiguration = &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(region)}
	}
	_, err = c.CreateBucket(a.ctx, in)
	return describeErr(err)
}

func (a *App) DeleteBucket(name string, force bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if force {
		if err := a.deletePrefix(c, name, ""); err != nil {
			return err
		}
	}
	_, err = c.DeleteBucket(a.ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	return describeErr(err)
}

type S3Object struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Modified     string `json:"modified"`
	ETag         string `json:"etag"`
	StorageClass string `json:"storageClass"`
	IsFolder     bool   `json:"isFolder"`
}

type Listing struct {
	Items     []S3Object `json:"items"`
	NextToken string     `json:"nextToken"`
}

func (a *App) ListObjects(bucket, prefix, token string) (Listing, error) {
	c, err := a.cli()
	if err != nil {
		return Listing{}, err
	}
	in := &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(1000),
	}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	out, err := c.ListObjectsV2(a.ctx, in)
	if err != nil {
		return Listing{}, describeErr(err)
	}
	res := Listing{Items: []S3Object{}}
	for _, cp := range out.CommonPrefixes {
		k := aws.ToString(cp.Prefix)
		res.Items = append(res.Items, S3Object{Key: k, Name: strings.TrimSuffix(strings.TrimPrefix(k, prefix), "/"), IsFolder: true})
	}
	for _, o := range out.Contents {
		k := aws.ToString(o.Key)
		if k == prefix { // folder placeholder
			continue
		}
		res.Items = append(res.Items, S3Object{
			Key:          k,
			Name:         strings.TrimPrefix(k, prefix),
			Size:         aws.ToInt64(o.Size),
			Modified:     fmtTime(o.LastModified),
			ETag:         strings.Trim(aws.ToString(o.ETag), `"`),
			StorageClass: string(o.StorageClass),
		})
	}
	if aws.ToBool(out.IsTruncated) {
		res.NextToken = aws.ToString(out.NextContinuationToken)
	}
	return res, nil
}

func (a *App) CreateFolder(bucket, prefix string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	_, err = c.PutObject(a.ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(prefix), Body: strings.NewReader("")})
	return describeErr(err)
}

// DeleteKeys deletes objects; keys ending with "/" are deleted recursively.
func (a *App) DeleteKeys(bucket string, keys []string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	var plain []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/") {
			if err := a.deletePrefix(c, bucket, k); err != nil {
				return err
			}
		} else {
			plain = append(plain, k)
		}
	}
	return a.deleteBatch(c, bucket, plain)
}

func (a *App) deletePrefix(c *s3.Client, bucket, prefix string) error {
	keys, err := a.listAll(c, bucket, prefix)
	if err != nil {
		return err
	}
	if prefix != "" {
		keys = append(keys, prefix) // placeholder, ignore failures
	}
	return a.deleteBatch(c, bucket, keys)
}

func (a *App) listAll(c *s3.Client, bucket, prefix string) ([]string, error) {
	var keys []string
	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return nil, describeErr(err)
		}
		for _, o := range out.Contents {
			keys = append(keys, aws.ToString(o.Key))
		}
	}
	return keys, nil
}

func (a *App) deleteBatch(c *s3.Client, bucket string, keys []string) error {
	for len(keys) > 0 {
		n := min(len(keys), 1000)
		chunk := keys[:n]
		keys = keys[n:]
		ids := make([]types.ObjectIdentifier, len(chunk))
		for i, k := range chunk {
			ids[i] = types.ObjectIdentifier{Key: aws.String(k)}
		}
		out, err := c.DeleteObjects(a.ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			// Fallback for backends without batch delete.
			for _, k := range chunk {
				if _, err := c.DeleteObject(a.ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); err != nil && !strings.HasSuffix(k, "/") {
					return describeErr(err)
				}
			}
			continue
		}
		for _, e := range out.Errors {
			if !strings.HasSuffix(aws.ToString(e.Key), "/") {
				return fmt.Errorf("%s: %s", aws.ToString(e.Key), aws.ToString(e.Message))
			}
		}
	}
	return nil
}

func (a *App) CopyObject(bucket, src, dst string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	return a.copyObject(c, bucket, src, dst)
}

func (a *App) copyObject(c *s3.Client, bucket, src, dst string) error {
	_, err := c.CopyObject(a.ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(copySource(bucket, src)),
		Key:        aws.String(dst),
	})
	return describeErr(err)
}

func (a *App) RenameObject(bucket, src, dst string) error {
	// Copying an object onto itself and deleting the source would destroy it.
	if dst == "" || dst == src {
		return errors.New(T("renameTarget"))
	}
	c, err := a.cli()
	if err != nil {
		return err
	}
	if err := a.copyObject(c, bucket, src, dst); err != nil {
		return err
	}
	_, err = c.DeleteObject(a.ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(src)})
	return describeErr(err)
}

type ObjectInfo struct {
	Key                string            `json:"key"`
	Size               int64             `json:"size"`
	ContentType        string            `json:"contentType"`
	ETag               string            `json:"etag"`
	Modified           string            `json:"modified"`
	StorageClass       string            `json:"storageClass"`
	CacheControl       string            `json:"cacheControl"`
	ContentDisposition string            `json:"contentDisposition"`
	ContentEncoding    string            `json:"contentEncoding"`
	VersionID          string            `json:"versionId"`
	Metadata           map[string]string `json:"metadata"`
}

func (a *App) HeadObject(bucket, key string) (ObjectInfo, error) {
	c, err := a.cli()
	if err != nil {
		return ObjectInfo{}, err
	}
	out, err := c.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return ObjectInfo{}, describeErr(err)
	}
	return ObjectInfo{
		Key:                key,
		Size:               aws.ToInt64(out.ContentLength),
		ContentType:        aws.ToString(out.ContentType),
		ETag:               strings.Trim(aws.ToString(out.ETag), `"`),
		Modified:           fmtTime(out.LastModified),
		StorageClass:       string(out.StorageClass),
		CacheControl:       aws.ToString(out.CacheControl),
		ContentDisposition: aws.ToString(out.ContentDisposition),
		ContentEncoding:    aws.ToString(out.ContentEncoding),
		VersionID:          aws.ToString(out.VersionId),
		Metadata:           out.Metadata,
	}, nil
}

func (a *App) Presign(bucket, key string, seconds int) (string, error) {
	c, err := a.cli()
	if err != nil {
		return "", err
	}
	if seconds <= 0 {
		seconds = 3600
	}
	seconds = min(seconds, maxPresignSeconds)
	req, err := s3.NewPresignClient(c).PresignGetObject(a.ctx,
		&s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)},
		s3.WithPresignExpires(time.Duration(seconds)*time.Second))
	if err != nil {
		return "", describeErr(err)
	}
	return req.URL, nil
}

// SigV4 presigned URLs are valid for at most seven days.
const maxPresignSeconds = 7 * 24 * 60 * 60

// Servers that ignore Range would otherwise return the whole object.
const previewLimit = 64 * 1024

// PreviewText returns the first bytes of an object as text.
func (a *App) PreviewText(bucket, key string) (string, error) {
	c, err := a.cli()
	if err != nil {
		return "", err
	}
	out, err := c.GetObject(a.ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Range: aws.String(fmt.Sprintf("bytes=0-%d", previewLimit-1))})
	if err != nil {
		return "", describeErr(err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(io.LimitReader(out.Body, previewLimit))
	return string(b), err
}

type TransferEvent struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"` // upload | download | copy | move | sync
	Name  string `json:"name"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	State string `json:"state"` // running | done | error | cancelled
	Error string `json:"error"`
}

type progressReader struct {
	r    io.Reader
	n    int64
	last time.Time
	emit func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if time.Since(p.last) > 150*time.Millisecond {
		p.last = time.Now()
		p.emit(p.n)
	}
	return n, err
}

func (a *App) emit(ev TransferEvent) { a.emitEvent("transfer", ev) }

// PickAndUpload opens a file dialog and uploads the selected files.
func (a *App) PickAndUpload(bucket, prefix string) (int, error) {
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: T("dlgUploadFiles")})
	if err != nil || len(files) == 0 {
		return 0, err
	}
	return a.UploadPaths(bucket, prefix, files)
}

// PickFolderAndUpload uploads a whole local directory.
func (a *App) PickFolderAndUpload(bucket, prefix string) (int, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: T("dlgUploadFolder")})
	if err != nil || dir == "" {
		return 0, err
	}
	return a.UploadPaths(bucket, prefix, []string{dir})
}

// UploadPaths uploads local files/directories (directories recursively).
func (a *App) UploadPaths(bucket, prefix string, paths []string) (int, error) {
	c, err := a.cli()
	if err != nil {
		return 0, err
	}
	jobs, err := collectUploads(prefix, paths)
	if err != nil {
		return 0, err
	}
	b := a.startBatch("upload", len(jobs))
	defer b.end()
	if failed := a.runUploads(c, bucket, jobs, b); failed > 0 {
		return len(jobs) - failed, b.failure("uploadFailed")
	}
	return len(jobs), nil
}

type uploadJob struct {
	local, key string
	size       int64
	modified   time.Time
}

// collectUploads maps local files, and the regular files inside local
// directories, to object keys under prefix.
func collectUploads(prefix string, paths []string) ([]uploadJob, error) {
	var jobs []uploadJob
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if st.Mode().IsRegular() {
			jobs = append(jobs, uploadJob{p, prefix + filepath.Base(p), st.Size(), st.ModTime()})
			continue
		}
		if !st.IsDir() {
			continue
		}
		base := filepath.Dir(p)
		// Symbolic links, devices and pipes inside a folder are not uploaded:
		// links can point outside the selection and special files can block.
		_ = filepath.WalkDir(p, func(fp string, d os.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				info, err := d.Info()
				if err != nil {
					return nil
				}
				rel, _ := filepath.Rel(base, fp)
				jobs = append(jobs, uploadJob{fp, prefix + filepath.ToSlash(rel), info.Size(), info.ModTime()})
			}
			return nil
		})
	}
	return jobs, nil
}

// runUploads uploads the jobs four at a time and returns how many failed.
func (a *App) runUploads(c *s3.Client, bucket string, jobs []uploadJob, b *batch) int {
	up := manager.NewUploader(c, func(u *manager.Uploader) { u.PartSize = 8 << 20 })
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var failed atomic.Int64
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j uploadJob) {
			defer wg.Done()
			defer func() { <-sem }()
			err := a.uploadOne(up, b.ev.Kind, bucket, j.local, j.key)
			b.finish(err)
			if err != nil {
				failed.Add(1)
			}
		}(j)
	}
	wg.Wait()
	return int(failed.Load())
}

func (a *App) uploadOne(up *manager.Uploader, kind, bucket, local, key string) error {
	return a.startTransfer(kind, key, func(ctx context.Context, progress func(int64, int64)) error {
		f, err := os.Open(local)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		progress(0, st.Size())
		pr := &progressReader{r: f, emit: func(n int64) { progress(n, st.Size()) }}
		ct := mime.TypeByExtension(path.Ext(key))
		if ct == "" {
			ct = "application/octet-stream"
		}
		_, err = up.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), Body: pr, ContentType: aws.String(ct),
		})
		return err
	})
}

// Download saves objects/folders. With a single plain object a save dialog is
// shown, otherwise a target directory is asked.
func (a *App) Download(bucket, prefix string, keys []string) (int, error) {
	c, err := a.cli()
	if err != nil {
		return 0, err
	}
	if len(keys) == 1 && !strings.HasSuffix(keys[0], "/") {
		dst, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: path.Base(keys[0])})
		if err != nil || dst == "" {
			return 0, err
		}
		b := a.startBatch("download", 1)
		defer b.end()
		err = a.downloadOne(c, bucket, keys[0], dst)
		b.finish(err)
		if err != nil {
			return 0, describeErr(err)
		}
		return 1, nil
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: T("dlgDownloadDir"), CanCreateDirectories: true})
	if err != nil || dir == "" {
		return 0, err
	}
	var all []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/") {
			sub, err := a.listAll(c, bucket, k)
			if err != nil {
				return 0, err
			}
			all = append(all, sub...)
		} else {
			all = append(all, k)
		}
	}
	type target struct{ key, name string }
	var targets []target
	seen := make(map[string]bool)
	for _, k := range all {
		if strings.HasSuffix(k, "/") {
			continue
		}
		rel := strings.TrimPrefix(k, prefix)
		dst := filepath.FromSlash(rel)
		if !filepath.IsLocal(dst) || dst == "." {
			return 0, errors.New(T("unsafeDownloadPath"))
		}
		if seen[dst] {
			continue
		}
		seen[dst] = true
		targets = append(targets, target{k, dst})
	}
	b := a.startBatch("download", len(targets))
	defer b.end()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var failed atomic.Int64
	for _, target := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(k, dst string) {
			defer wg.Done()
			defer func() { <-sem }()
			err := a.downloadTo(c, bucket, k, dir, dst)
			b.finish(err)
			if err != nil {
				failed.Add(1)
			}
		}(target.key, target.name)
	}
	wg.Wait()
	if f := failed.Load(); f > 0 {
		return len(targets) - int(f), b.failure("downloadFailed")
	}
	return len(targets), nil
}

func (a *App) downloadOne(c *s3.Client, bucket, key, dst string) error {
	return a.downloadTo(c, bucket, key, filepath.Dir(dst), filepath.Base(dst))
}

func (a *App) downloadTo(c *s3.Client, bucket, key, directory, name string) error {
	return a.downloadVersionTo(c, bucket, key, "", directory, name)
}

// downloadVersionTo saves one version of an object; an empty version is the current one.
func (a *App) downloadVersionTo(c *s3.Client, bucket, key, version, directory, name string) error {
	return a.startTransfer("download", key, func(ctx context.Context, progress func(int64, int64)) error {
		in := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
		if version != "" {
			in.VersionId = aws.String(version)
		}
		out, err := c.GetObject(ctx, in)
		if err != nil {
			return err
		}
		defer out.Body.Close()
		total := aws.ToInt64(out.ContentLength)
		progress(0, total)
		pr := &progressReader{r: out.Body, emit: func(n int64) { progress(n, total) }}
		return saveDownload(ctx, directory, name, pr)
	})
}

func (a *App) CopyToClipboard(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func copySource(bucket, key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = urlPathEscape(p)
	}
	return bucket + "/" + strings.Join(parts, "/")
}

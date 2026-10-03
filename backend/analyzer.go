package main

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// The analysis keeps one small record per distinct object, so the scan is bounded.
const (
	analysisScanLimit = 500_000
	analysisTopItems  = 25
	analysisTopTypes  = 10
	duplicateKeyLimit = 20
)

type AnalysisEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Objects int64  `json:"objects"`
}

// DuplicateGroup lists objects with the same ETag and size. Every copy
// beyond the first counts as wasted space.
type DuplicateGroup struct {
	Size   int64    `json:"size"`
	Copies int64    `json:"copies"`
	Wasted int64    `json:"wasted"`
	Keys   []string `json:"keys"` // at most duplicateKeyLimit
}

type BucketAnalysis struct {
	Objects    int64            `json:"objects"`
	Size       int64            `json:"size"`
	Truncated  bool             `json:"truncated"` // the scan limit was reached
	Folders    []AnalysisEntry  `json:"folders"`   // direct children of the prefix; "" holds its own files
	Types      []AnalysisEntry  `json:"types"`     // by file extension; "" is no extension, "*" everything else
	Classes    []AnalysisEntry  `json:"classes"`
	Ages       []AnalysisEntry  `json:"ages"` // 30d | 90d | 1y | older, by last modification
	Largest    []S3Object       `json:"largest"`
	Duplicates []DuplicateGroup `json:"duplicates"`
	Wasted     int64            `json:"wasted"` // across all duplicate groups, not only the listed ones
}

type tally map[string]*AnalysisEntry

func (t tally) add(name string, size int64) {
	entry := t[name]
	if entry == nil {
		entry = &AnalysisEntry{Name: name}
		t[name] = entry
	}
	entry.Size += size
	entry.Objects++
}

// sorted returns the entries by size, largest first, folding the tail into "*".
func (t tally) sorted(limit int) []AnalysisEntry {
	entries := make([]AnalysisEntry, 0, len(t))
	for _, entry := range t {
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Size != entries[j].Size {
			return entries[i].Size > entries[j].Size
		}
		return entries[i].Name < entries[j].Name
	})
	if limit > 0 && len(entries) > limit {
		other := AnalysisEntry{Name: "*"}
		for _, entry := range entries[limit:] {
			other.Size += entry.Size
			other.Objects += entry.Objects
		}
		entries = append(entries[:limit], other)
	}
	return entries
}

// AnalyzeBucket scans everything below prefix and summarises where the space goes.
func (a *App) AnalyzeBucket(bucket, prefix string) (BucketAnalysis, error) {
	c, err := a.cli()
	if err != nil {
		return BucketAnalysis{}, err
	}
	type identical struct {
		size   int64
		copies int64
		keys   []string
	}
	folders, types, classes := tally{}, tally{}, tally{}
	ages := tally{"30d": {Name: "30d"}, "90d": {Name: "90d"}, "1y": {Name: "1y"}, "older": {Name: "older"}}
	seen := make(map[string]*identical)
	result := BucketAnalysis{Largest: []S3Object{}, Duplicates: []DuplicateGroup{}}
	now := time.Now()

	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return BucketAnalysis{}, describeErr(err)
		}
		for _, o := range out.Contents {
			key, size := aws.ToString(o.Key), aws.ToInt64(o.Size)
			if strings.HasSuffix(key, "/") { // folder placeholder
				continue
			}
			result.Objects++
			result.Size += size
			name := strings.TrimPrefix(key, prefix)
			folder := ""
			if i := strings.Index(name, "/"); i >= 0 {
				folder = name[:i+1]
			}
			folders.add(folder, size)
			types.add(strings.ToLower(strings.TrimPrefix(path.Ext(name), ".")), size)
			class := string(o.StorageClass)
			if class == "" {
				class = "STANDARD"
			}
			classes.add(class, size)
			switch age := now.Sub(aws.ToTime(o.LastModified)); {
			case age < 30*24*time.Hour:
				ages.add("30d", size)
			case age < 90*24*time.Hour:
				ages.add("90d", size)
			case age < 365*24*time.Hour:
				ages.add("1y", size)
			default:
				ages.add("older", size)
			}
			result.Largest = append(result.Largest, S3Object{Key: key, Name: name, Size: size, Modified: fmtTime(o.LastModified), StorageClass: string(o.StorageClass)})
			if len(result.Largest) > 4*analysisTopItems {
				result.Largest = largestObjects(result.Largest)
			}
			if etag := aws.ToString(o.ETag); size > 0 && etag != "" {
				id := etag + "/" + formatInt(size)
				group := seen[id]
				if group == nil {
					group = &identical{size: size}
					seen[id] = group
				}
				group.copies++
				if len(group.keys) < duplicateKeyLimit {
					group.keys = append(group.keys, key)
				}
			}
		}
		if result.Objects >= analysisScanLimit && p.HasMorePages() {
			result.Truncated = true
			break
		}
	}
	result.Largest = largestObjects(result.Largest)
	result.Folders, result.Types, result.Classes = folders.sorted(0), types.sorted(analysisTopTypes), classes.sorted(0)
	for _, name := range []string{"30d", "90d", "1y", "older"} {
		result.Ages = append(result.Ages, *ages[name])
	}
	for _, group := range seen {
		if group.copies < 2 {
			continue
		}
		wasted := group.size * (group.copies - 1)
		result.Wasted += wasted
		result.Duplicates = append(result.Duplicates, DuplicateGroup{Size: group.size, Copies: group.copies, Wasted: wasted, Keys: group.keys})
	}
	sort.Slice(result.Duplicates, func(i, j int) bool {
		if result.Duplicates[i].Wasted != result.Duplicates[j].Wasted {
			return result.Duplicates[i].Wasted > result.Duplicates[j].Wasted
		}
		return result.Duplicates[i].Keys[0] < result.Duplicates[j].Keys[0]
	})
	if len(result.Duplicates) > analysisTopItems {
		result.Duplicates = result.Duplicates[:analysisTopItems]
	}
	return result, nil
}

func largestObjects(objects []S3Object) []S3Object {
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Size != objects[j].Size {
			return objects[i].Size > objects[j].Size
		}
		return objects[i].Key < objects[j].Key
	})
	if len(objects) > analysisTopItems {
		objects = objects[:analysisTopItems]
	}
	return objects
}

func formatInt(n int64) string {
	var digits [20]byte
	i := len(digits)
	for {
		i--
		digits[i] = byte('0' + n%10)
		if n /= 10; n == 0 {
			return string(digits[i:])
		}
	}
}

// ListProfileBuckets lists the buckets of a saved profile without switching
// the active connection, so it can be picked as a copy destination.
func (a *App) ListProfileBuckets(profileID string) (BucketList, error) {
	p, err := a.store.get(profileID)
	if err != nil {
		return BucketList{}, err
	}
	c, err := newClient(a.ctx, p)
	if err != nil {
		return BucketList{}, err
	}
	return a.bucketList(c, normalize(p).Buckets)
}

// CopyToProfile copies objects, and everything below keys ending in "/", to
// a bucket of another saved profile. The data is streamed through this
// computer, so it also works between different providers and accounts.
// basePrefix is removed from each key before dstPrefix is added. With move,
// each source object is deleted once its copy has been stored.
func (a *App) CopyToProfile(bucket, basePrefix string, keys []string, profileID, dstBucket, dstPrefix string, move bool) (int, error) {
	src, err := a.cli()
	if err != nil {
		return 0, err
	}
	dstBucket = strings.TrimSpace(dstBucket)
	if dstBucket == "" {
		return 0, errors.New(T("bucketRequired"))
	}
	dstPrefix = strings.TrimLeft(strings.TrimSpace(dstPrefix), "/")
	if dstPrefix != "" && !strings.HasSuffix(dstPrefix, "/") {
		dstPrefix += "/"
	}
	profile, err := a.store.get(profileID)
	if err != nil {
		return 0, err
	}
	a.mu.RLock()
	sameStorage := a.profile.ID == profile.ID || sameEndpoint(a.profile, profile)
	a.mu.RUnlock()
	// Two profiles can point at the same service; copying an object onto
	// itself and then deleting the "source" would destroy it.
	if sameStorage && dstBucket == bucket && dstPrefix == basePrefix {
		return 0, errors.New(T("copySameLocation"))
	}
	dst, err := newClient(a.ctx, profile)
	if err != nil {
		return 0, err
	}
	var all []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/") {
			sub, err := a.listAll(src, bucket, k)
			if err != nil {
				return 0, err
			}
			all = append(all, sub...)
		} else {
			all = append(all, k)
		}
	}
	type target struct{ from, to string }
	var targets []target
	var placeholders []string
	for _, k := range all {
		if strings.HasSuffix(k, "/") {
			placeholders = append(placeholders, k)
		} else {
			targets = append(targets, target{k, dstPrefix + strings.TrimPrefix(k, basePrefix)})
		}
	}
	kind := "copy"
	if move {
		kind = "move"
	}
	b := a.startBatch(kind, len(targets))
	defer b.end()
	up := manager.NewUploader(dst, func(u *manager.Uploader) { u.PartSize = 8 << 20 })
	opts := profile.uploadOptions()
	opts.sameStorage = sameStorage
	tasks := make([]*transferTask, len(targets))
	for i, t := range targets {
		tasks[i] = a.queueCopy(src, up, kind, bucket, t.from, dstBucket, t.to, move, opts)
	}
	var failed atomic.Int64
	for _, task := range tasks {
		err := task.wait()
		b.finish(err)
		if err != nil {
			failed.Add(1)
		}
	}
	if f := failed.Load(); f > 0 {
		return len(targets) - int(f), b.failure("copyFailed")
	}
	if move {
		// Empty folder markers of fully moved folders; failures leave only a marker behind.
		_ = a.deleteBatch(src, bucket, placeholders)
	}
	return len(targets), nil
}

func (a *App) queueCopy(src *s3.Client, up *manager.Uploader, kind, bucket, key, dstBucket, dstKey string, move bool, opts uploadOptions) *transferTask {
	return a.enqueueTransfer(kind, key, 0, func(ctx context.Context, progress func(int64, int64)) error {
		out, err := src.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		if err != nil {
			return err
		}
		defer out.Body.Close()
		total := aws.ToInt64(out.ContentLength)
		progress(0, total)
		in := &s3.PutObjectInput{
			Bucket: aws.String(dstBucket), Key: aws.String(dstKey),
			Body:        &progressReader{r: out.Body, emit: func(n int64) { progress(n, total) }, limiter: &a.bandwidth, ctx: ctx},
			ContentType: out.ContentType, CacheControl: out.CacheControl,
			ContentDisposition: out.ContentDisposition, ContentEncoding: out.ContentEncoding,
			Metadata: out.Metadata,
		}
		opts.apply(in)
		_, err = up.Upload(ctx, in)
		if err == nil && move && !(opts.sameStorage && bucket == dstBucket && key == dstKey) {
			// The source is removed only after its copy was stored, and never
			// when the "copy" landed on the source itself.
			_, err = src.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		}
		return err
	})
}

// sameEndpoint reports whether two profiles address the same storage service.
func sameEndpoint(a, b Profile) bool {
	a, b = normalize(a), normalize(b)
	return strings.EqualFold(a.Endpoint, b.Endpoint) && (a.Endpoint != "" || a.Region == b.Region)
}

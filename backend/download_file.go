package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// saveDownload streams source into directory/name through a temporary file
// that replaces the target only once it is complete and, when expectedMD5 is
// given, matches that checksum. A previous copy is never overwritten by a
// corrupt download.
func saveDownload(ctx context.Context, directory, name string, source io.Reader, expectedMD5 string) error {
	if !filepath.IsLocal(name) || name == "." {
		return errors.New(T("unsafeDownloadPath"))
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := filepath.Dir(name)
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary := filepath.Join(parent, ".s3browser-download-"+randID())
	f, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	hash := md5.New()
	var writer io.Writer = f
	if expectedMD5 != "" {
		writer = io.MultiWriter(f, hash)
	}
	_, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: source})
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if expectedMD5 != "" {
		if actual := hex.EncodeToString(hash.Sum(nil)); actual != expectedMD5 {
			return errors.New(T("checksumMismatch", actual[:8], expectedMD5[:8]))
		}
	}
	// Root also blocks symlinks that lead outside the selected directory.
	return root.Rename(temporary, name)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Large objects are fetched as several byte ranges at once and written at
// their offsets into one temporary file, which is renamed when every range
// arrived. The thresholds are variables so tests can use small objects.
var (
	segmentedMinSize  int64 = 32 << 20
	segmentSize       int64 = 8 << 20
	segmentWorkers          = 4
	segmentMaxRetries       = 3
)

// saveSegmented downloads size bytes through fetch, which returns a reader
// for one range, and reports progress with the total bytes written so far.
func saveSegmented(ctx context.Context, directory, name string, size int64, fetch func(ctx context.Context, start, end int64) (io.ReadCloser, error), progress func(int64), expectedMD5 string) error {
	if !filepath.IsLocal(name) || name == "." {
		return errors.New(T("unsafeDownloadPath"))
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := filepath.Dir(name)
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary := filepath.Join(parent, ".s3browser-download-"+randID())
	f, err := root.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var written atomic.Int64
	var lastReport atomic.Int64
	report := func() {
		// Progress arrives from several workers; only report every few hundred kilobytes.
		if n := written.Load(); n-lastReport.Load() >= 512<<10 || n == size {
			lastReport.Store(n)
			progress(n)
		}
	}
	type segment struct{ start, end int64 }
	segments := make(chan segment)
	errs := make(chan error, segmentWorkers)
	var wg sync.WaitGroup
	for range segmentWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seg := range segments {
				if err := fetchSegment(ctx, f, seg.start, seg.end, fetch, &written, report); err != nil {
					errs <- err
					cancel()
					return
				}
			}
		}()
	}
	for start := int64(0); start < size; start += segmentSize {
		end := min(start+segmentSize, size) - 1
		select {
		case segments <- segment{start, end}:
		case <-ctx.Done():
			start = size
		}
	}
	close(segments)
	wg.Wait()
	select {
	case err := <-errs:
		f.Close()
		return err
	default:
	}
	if err := ctx.Err(); err != nil {
		f.Close()
		return err
	}
	if expectedMD5 != "" {
		hash := md5.New()
		if _, err := io.Copy(hash, io.NewSectionReader(f, 0, size)); err != nil {
			f.Close()
			return err
		}
		if actual := hex.EncodeToString(hash.Sum(nil)); actual != expectedMD5 {
			f.Close()
			return errors.New(T("checksumMismatch", actual[:8], expectedMD5[:8]))
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, name)
}

// fetchSegment writes one range at its offset, retrying transient failures.
func fetchSegment(ctx context.Context, f *os.File, start, end int64, fetch func(context.Context, int64, int64) (io.ReadCloser, error), written *atomic.Int64, report func()) error {
	var err error
	for attempt := 0; attempt < segmentMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var done int64
		done, err = copySegment(ctx, f, start, end, fetch)
		written.Add(done)
		report()
		if err == nil {
			return nil
		}
		// The bytes of a failed attempt are rewritten, so take them back.
		written.Add(-done)
	}
	return err
}

func copySegment(ctx context.Context, f *os.File, start, end int64, fetch func(context.Context, int64, int64) (io.ReadCloser, error)) (int64, error) {
	body, err := fetch(ctx, start, end)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	n, err := io.Copy(io.NewOffsetWriter(f, start), io.LimitReader(&contextReader{ctx: ctx, reader: body}, end-start+1))
	if err != nil {
		return n, err
	}
	if n != end-start+1 {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

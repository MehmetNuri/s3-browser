package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func saveDownload(ctx context.Context, directory, name string, source io.Reader) error {
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
	_, copyErr := io.Copy(f, &contextReader{ctx: ctx, reader: source})
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

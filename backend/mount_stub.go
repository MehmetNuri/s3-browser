//go:build !linux && !darwin

package main

import "errors"

// Mounting needs FUSE, which this platform does not provide.
func (a *App) MountBucket(bucket, prefix string, readOnly bool) (Mount, error) {
	return Mount{}, errors.New(T("mountUnsupported"))
}

func (a *App) UnmountBucket(id string) error { return errors.New(T("mountNotFound")) }

func (a *App) unmountAll() {}

func (a *App) openMountFolder(dir string) error { return errors.New(T("mountUnsupported")) }

type bucketMount struct{ info Mount }

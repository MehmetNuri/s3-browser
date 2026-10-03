//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"mime"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	runtime "s3browser/internal/desktop"
)

// A mounted bucket is served by FUSE: directories come from listings with a
// delimiter, reads are byte-range requests, and a file opened for writing is
// kept in a temporary copy that is uploaded when it is closed.
type bucketMount struct {
	info   Mount
	server *fuse.Server
}

const (
	mountListTimeout = 5 * time.Second // directory listings are reused this long
	mountReadChunk   = 4 << 20         // bytes fetched per range request
	mountListLimit   = 20000           // entries per directory
)

func (a *App) MountBucket(bucket, prefix string, readOnly bool) (Mount, error) {
	c, err := a.cli()
	if err != nil {
		return Mount{}, err
	}
	if bucket == "" {
		return Mount{}, errors.New(T("bucketRequired"))
	}
	prefix = strings.TrimLeft(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	a.mu.RLock()
	profile := a.profile
	a.mu.RUnlock()
	dir, err := mountPoint(profile.Name, bucket, prefix)
	if err != nil {
		return Mount{}, err
	}
	a.mountsMu.Lock()
	defer a.mountsMu.Unlock()
	for _, m := range a.mounts {
		if m.info.Path == dir {
			return m.info, nil
		}
	}
	if len(a.mounts) >= maxMounts {
		return Mount{}, errors.New(T("tooManyMounts", maxMounts))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Mount{}, err
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) > 0 {
		return Mount{}, errors.New(T("mountPointNotEmpty", dir))
	}
	id := randID()
	info := Mount{ID: id, ProfileID: profile.ID, Profile: profile.Name, Bucket: bucket, Prefix: prefix, Path: dir, ReadOnly: readOnly}
	root := newS3FS(a, c, bucket, prefix, readOnly, profile.uploadOptions())
	timeout := mountListTimeout
	server, err := fs.Mount(dir, root.root(), &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName: "s3browser", Name: "s3browser", Options: mountOptions(readOnly),
			MaxReadAhead: mountReadChunk, DisableXAttrs: true,
		},
		EntryTimeout: &timeout, AttrTimeout: &timeout, NegativeTimeout: &timeout,
		UID: uint32(os.Getuid()), GID: uint32(os.Getgid()),
	})
	if err != nil {
		_ = os.Remove(dir)
		return Mount{}, errors.New(T("mountFailed", err.Error()))
	}
	if a.mounts == nil {
		a.mounts = make(map[string]*bucketMount)
	}
	a.mounts[id] = &bucketMount{info: info, server: server}
	go func() {
		server.Wait()
		a.mountsMu.Lock()
		delete(a.mounts, id)
		a.mountsMu.Unlock()
		_ = os.Remove(dir) // only succeeds when nothing is mounted there anymore
		a.emitEvent("mount", MountEvent{ID: id, State: "unmounted"})
	}()
	a.emitEvent("mount", MountEvent{ID: id, State: "mounted"})
	return info, nil
}

func mountOptions(readOnly bool) []string {
	options := []string{"noatime"}
	if readOnly {
		options = append(options, "ro")
	}
	return options
}

func (a *App) UnmountBucket(id string) error {
	a.mountsMu.Lock()
	m, ok := a.mounts[id]
	a.mountsMu.Unlock()
	if !ok {
		return errors.New(T("mountNotFound"))
	}
	if err := m.server.Unmount(); err != nil {
		return errors.New(T("unmountFailed", err.Error()))
	}
	return nil
}

// unmountAll detaches every mount, for shutdown.
func (a *App) unmountAll() {
	a.mountsMu.Lock()
	servers := make([]*fuse.Server, 0, len(a.mounts))
	for _, m := range a.mounts {
		servers = append(servers, m.server)
	}
	a.mountsMu.Unlock()
	for _, s := range servers {
		_ = s.Unmount()
	}
}

func (a *App) openMountFolder(dir string) error {
	return runtime.Call(a.ctx, "openMountFolder", dir, nil)
}

// --- file system ---

type s3FS struct {
	app      *App
	client   *s3.Client
	bucket   string
	prefix   string
	readOnly bool
	opts     uploadOptions
	cacheMu  sync.Mutex
	cache    map[string]*dirListing // by directory key ("" for the root)
}

type dirEntry struct {
	name  string
	isDir bool
	size  int64
	mtime time.Time
}

type dirListing struct {
	entries []dirEntry
	byName  map[string]dirEntry
	at      time.Time
}

func newS3FS(a *App, c *s3.Client, bucket, prefix string, readOnly bool, opts uploadOptions) *s3FS {
	return &s3FS{app: a, client: c, bucket: bucket, prefix: prefix, readOnly: readOnly, opts: opts, cache: map[string]*dirListing{}}
}

func (f *s3FS) root() *s3Node {
	return &s3Node{fsys: f, key: "", isDir: true}
}

// listDir lists the direct children of a directory key (relative to the mount prefix).
func (f *s3FS) listDir(ctx context.Context, dir string) (*dirListing, error) {
	f.cacheMu.Lock()
	if l, ok := f.cache[dir]; ok && time.Since(l.at) < mountListTimeout {
		f.cacheMu.Unlock()
		return l, nil
	}
	f.cacheMu.Unlock()
	full := f.prefix + dir
	p := s3.NewListObjectsV2Paginator(f.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(f.bucket), Prefix: aws.String(full), Delimiter: aws.String("/"),
	})
	listing := &dirListing{byName: map[string]dirEntry{}, at: time.Now()}
	for p.HasMorePages() && len(listing.entries) < mountListLimit {
		out, err := p.NextPage(ctx)
		if err != nil {
			debugf("mount: listing %q failed: %v", full, err)
			return nil, err
		}
		for _, cp := range out.CommonPrefixes {
			name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), full), "/")
			if name == "" || strings.Contains(name, "/") {
				continue
			}
			listing.add(dirEntry{name: name, isDir: true})
		}
		for _, o := range out.Contents {
			name := strings.TrimPrefix(aws.ToString(o.Key), full)
			if name == "" || strings.Contains(name, "/") {
				continue // the folder marker itself, or a key with an odd delimiter
			}
			listing.add(dirEntry{name: name, size: aws.ToInt64(o.Size), mtime: aws.ToTime(o.LastModified)})
		}
	}
	f.cacheMu.Lock()
	f.cache[dir] = listing
	f.cacheMu.Unlock()
	return listing, nil
}

func (l *dirListing) add(e dirEntry) {
	if _, dup := l.byName[e.name]; dup {
		return
	}
	l.byName[e.name] = e
	l.entries = append(l.entries, e)
}

func (f *s3FS) invalidate(dir string) {
	f.cacheMu.Lock()
	delete(f.cache, dir)
	f.cacheMu.Unlock()
}

func fuseErrno(err error) syscall.Errno {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return syscall.ENOENT
		case "AccessDenied", "Forbidden":
			return syscall.EACCES
		}
	}
	if errors.Is(err, context.Canceled) {
		return syscall.EINTR
	}
	return syscall.EIO
}

func inodeNumber(key string, isDir bool) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	if isDir {
		_, _ = h.Write([]byte{'/'})
	}
	return h.Sum64()
}

// s3Node is a directory (key ends with "/" or is "") or an object.
type s3Node struct {
	fs.Inode
	fsys  *s3FS
	key   string // relative to the mount prefix; directories end with "/"
	isDir bool
	mu    sync.Mutex
	size  int64
	mtime time.Time
}

var (
	_ fs.NodeLookuper  = (*s3Node)(nil)
	_ fs.NodeReaddirer = (*s3Node)(nil)
	_ fs.NodeGetattrer = (*s3Node)(nil)
	_ fs.NodeSetattrer = (*s3Node)(nil)
	_ fs.NodeOpener    = (*s3Node)(nil)
	_ fs.NodeCreater   = (*s3Node)(nil)
	_ fs.NodeMkdirer   = (*s3Node)(nil)
	_ fs.NodeUnlinker  = (*s3Node)(nil)
	_ fs.NodeRmdirer   = (*s3Node)(nil)
	_ fs.NodeRenamer   = (*s3Node)(nil)
	_ fs.NodeStatfser  = (*s3Node)(nil)
)

// Statfs reports generous, fictional sizes: file managers refuse to create
// anything on a file system that claims no space or a name limit of zero.
func (n *s3Node) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	const block = 4096
	out.Bsize, out.Frsize = block, block
	out.Blocks, out.Bfree, out.Bavail = 1<<40, 1<<39, 1<<39
	out.Files, out.Ffree = 1<<30, 1<<29
	out.NameLen = 255
	return 0
}

func (n *s3Node) fillAttr(out *fuse.Attr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isDir {
		out.Mode = syscall.S_IFDIR | 0o700
	} else {
		out.Mode = syscall.S_IFREG | 0o600
		out.Size = uint64(n.size)
	}
	out.SetTimes(&n.mtime, &n.mtime, &n.mtime)
	out.Uid, out.Gid = uint32(os.Getuid()), uint32(os.Getgid())
}

func (n *s3Node) child(ctx context.Context, name string, e dirEntry) *fs.Inode {
	key := n.key + name
	if e.isDir {
		key += "/"
	}
	node := &s3Node{fsys: n.fsys, key: key, isDir: e.isDir, size: e.size, mtime: e.mtime}
	mode := uint32(syscall.S_IFREG)
	if e.isDir {
		mode = syscall.S_IFDIR
	}
	return n.NewInode(ctx, node, fs.StableAttr{Mode: mode, Ino: inodeNumber(key, e.isDir)})
}

func (n *s3Node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if !n.isDir {
		return nil, syscall.ENOTDIR
	}
	listing, err := n.fsys.listDir(ctx, n.key)
	if err != nil {
		return nil, fuseErrno(err)
	}
	e, ok := listing.byName[name]
	if !ok {
		return nil, syscall.ENOENT
	}
	inode := n.child(ctx, name, e)
	inode.Operations().(*s3Node).fillAttr(&out.Attr)
	return inode, 0
}

func (n *s3Node) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	listing, err := n.fsys.listDir(ctx, n.key)
	if err != nil {
		return nil, fuseErrno(err)
	}
	entries := make([]fuse.DirEntry, 0, len(listing.entries))
	for _, e := range listing.entries {
		mode := uint32(syscall.S_IFREG)
		key := n.key + e.name
		if e.isDir {
			mode = syscall.S_IFDIR
			key += "/"
		}
		entries = append(entries, fuse.DirEntry{Name: e.name, Mode: mode, Ino: inodeNumber(key, e.isDir)})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *s3Node) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f, ok := fh.(*s3File); ok && f.tmp != nil {
		if st, err := f.tmp.Stat(); err == nil {
			n.fillAttr(&out.Attr)
			out.Size = uint64(st.Size())
			return 0
		}
	}
	n.fillAttr(&out.Attr)
	return 0
}

// Setattr handles truncation; ownership and permissions are fixed.
func (n *s3Node) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if size, ok := in.GetSize(); ok {
		if n.isDir {
			return syscall.EISDIR
		}
		if n.fsys.readOnly {
			return syscall.EROFS
		}
		f, ok := fh.(*s3File)
		if !ok {
			// Truncate without an open handle: rewrite through a temporary handle.
			f = &s3File{node: n}
			if errno := f.ensureTemp(ctx, size == 0); errno != 0 {
				return errno
			}
			if err := f.tmp.Truncate(int64(size)); err != nil {
				return syscall.EIO
			}
			f.dirty = true
			return f.Flush(ctx)
		}
		if errno := f.ensureTemp(ctx, size == 0); errno != 0 {
			return errno
		}
		if err := f.tmp.Truncate(int64(size)); err != nil {
			return syscall.EIO
		}
		f.dirty = true
	}
	if mtime, ok := in.GetMTime(); ok {
		n.mu.Lock()
		n.mtime = mtime
		n.mu.Unlock()
	}
	return n.Getattr(ctx, fh, out)
}

func (n *s3Node) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if n.isDir {
		return nil, 0, syscall.EISDIR
	}
	write := flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_TRUNC|syscall.O_APPEND) != 0
	if write && n.fsys.readOnly {
		return nil, 0, syscall.EROFS
	}
	f := &s3File{node: n}
	if write {
		if errno := f.ensureTemp(ctx, flags&syscall.O_TRUNC != 0); errno != 0 {
			return nil, 0, errno
		}
	}
	return f, fuse.FOPEN_DIRECT_IO, 0
}

func (n *s3Node) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	if n.fsys.readOnly {
		return nil, nil, 0, syscall.EROFS
	}
	if !validMountName(name) {
		return nil, nil, 0, syscall.EINVAL
	}
	inode := n.child(ctx, name, dirEntry{name: name, mtime: time.Now()})
	node := inode.Operations().(*s3Node)
	f := &s3File{node: node, dirty: true}
	if errno := f.ensureTemp(ctx, true); errno != 0 {
		return nil, nil, 0, errno
	}
	node.fillAttr(&out.Attr)
	return inode, f, fuse.FOPEN_DIRECT_IO, 0
}

func (n *s3Node) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if n.fsys.readOnly {
		return nil, syscall.EROFS
	}
	if !validMountName(name) {
		return nil, syscall.EINVAL
	}
	key := n.fsys.prefix + n.key + name + "/"
	if _, err := n.fsys.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(n.fsys.bucket), Key: aws.String(key), Body: strings.NewReader("")}); err != nil {
		return nil, fuseErrno(err)
	}
	n.fsys.invalidate(n.key)
	inode := n.child(ctx, name, dirEntry{name: name, isDir: true, mtime: time.Now()})
	inode.Operations().(*s3Node).fillAttr(&out.Attr)
	return inode, 0
}

func (n *s3Node) Unlink(ctx context.Context, name string) syscall.Errno {
	if n.fsys.readOnly {
		return syscall.EROFS
	}
	key := n.fsys.prefix + n.key + name
	if _, err := n.fsys.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(n.fsys.bucket), Key: aws.String(key)}); err != nil {
		return fuseErrno(err)
	}
	n.fsys.invalidate(n.key)
	return 0
}

func (n *s3Node) Rmdir(ctx context.Context, name string) syscall.Errno {
	if n.fsys.readOnly {
		return syscall.EROFS
	}
	dir := n.key + name + "/"
	n.fsys.invalidate(dir)
	listing, err := n.fsys.listDir(ctx, dir)
	if err != nil {
		return fuseErrno(err)
	}
	if len(listing.entries) > 0 {
		return syscall.ENOTEMPTY
	}
	if _, err := n.fsys.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(n.fsys.bucket), Key: aws.String(n.fsys.prefix + dir)}); err != nil {
		return fuseErrno(err)
	}
	n.fsys.invalidate(n.key)
	return 0
}

// Rename moves a file by copying it; folders would need every key moved, so they are refused.
func (n *s3Node) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if n.fsys.readOnly {
		return syscall.EROFS
	}
	target, ok := newParent.(*s3Node)
	if !ok || !validMountName(newName) {
		return syscall.EINVAL
	}
	listing, err := n.fsys.listDir(ctx, n.key)
	if err != nil {
		return fuseErrno(err)
	}
	e, ok := listing.byName[name]
	if !ok {
		return syscall.ENOENT
	}
	if e.isDir {
		return syscall.ENOTSUP
	}
	from, to := n.fsys.prefix+n.key+name, n.fsys.prefix+target.key+newName
	if from == to {
		return 0
	}
	if _, err := n.fsys.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: aws.String(n.fsys.bucket), Key: aws.String(to), CopySource: aws.String(copySource(n.fsys.bucket, from)),
	}); err != nil {
		return fuseErrno(err)
	}
	if _, err := n.fsys.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(n.fsys.bucket), Key: aws.String(from)}); err != nil {
		return fuseErrno(err)
	}
	n.fsys.invalidate(n.key)
	n.fsys.invalidate(target.key)
	return 0
}

func validMountName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00") && len(name) <= 255
}

// s3File is an open handle. Reads of an unchanged object go straight to S3
// in large chunks; once the file is written, everything goes through tmp.
type s3File struct {
	node  *s3Node
	mu    sync.Mutex
	tmp   *os.File
	dirty bool
	// Last range fetched for reads without a temporary copy.
	buf    []byte
	bufOff int64
}

var (
	_ fs.FileReader   = (*s3File)(nil)
	_ fs.FileWriter   = (*s3File)(nil)
	_ fs.FileFlusher  = (*s3File)(nil)
	_ fs.FileReleaser = (*s3File)(nil)
	_ fs.FileFsyncer  = (*s3File)(nil)
)

func (f *s3File) objectKey() string { return f.node.fsys.prefix + f.node.key }

// ensureTemp materializes the object in a temporary file; truncate skips the download.
func (f *s3File) ensureTemp(ctx context.Context, truncate bool) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tmp != nil {
		return 0
	}
	tmp, err := os.CreateTemp("", "s3browser-mount-")
	if err != nil {
		return syscall.EIO
	}
	// The name is removed right away; the descriptor keeps the data.
	_ = os.Remove(tmp.Name())
	if !truncate {
		out, err := f.node.fsys.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(f.node.fsys.bucket), Key: aws.String(f.objectKey())})
		if err != nil {
			tmp.Close()
			return fuseErrno(err)
		}
		_, err = io.Copy(tmp, out.Body)
		out.Body.Close()
		if err != nil {
			tmp.Close()
			return syscall.EIO
		}
	}
	f.tmp = tmp
	return 0
}

func (f *s3File) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tmp != nil {
		n, err := f.tmp.ReadAt(dest, off)
		if err != nil && err != io.EOF {
			return nil, syscall.EIO
		}
		return fuse.ReadResultData(dest[:n]), 0
	}
	f.node.mu.Lock()
	size := f.node.size
	f.node.mu.Unlock()
	if off >= size {
		return fuse.ReadResultData(nil), 0
	}
	// Serve from the last chunk when it covers the request, otherwise fetch a new one.
	if f.buf == nil || off < f.bufOff || off >= f.bufOff+int64(len(f.buf)) {
		end := min(off+mountReadChunk, size) - 1
		out, err := f.node.fsys.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(f.node.fsys.bucket), Key: aws.String(f.objectKey()),
			Range: aws.String("bytes=" + itoa(off) + "-" + itoa(end)),
		})
		if err != nil {
			return nil, fuseErrno(err)
		}
		data, err := io.ReadAll(out.Body)
		out.Body.Close()
		if err != nil {
			return nil, syscall.EIO
		}
		f.buf, f.bufOff = data, off
	}
	start := off - f.bufOff
	n := copy(dest, f.buf[start:])
	return fuse.ReadResultData(dest[:n]), 0
}

func (f *s3File) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	if f.node.fsys.readOnly {
		return 0, syscall.EROFS
	}
	if errno := f.ensureTemp(ctx, false); errno != 0 {
		return 0, errno
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.tmp.WriteAt(data, off)
	if err != nil {
		return 0, syscall.EIO
	}
	f.dirty = true
	return uint32(n), 0
}

// Flush uploads a changed copy; it runs on every close of the descriptor.
func (f *s3File) Flush(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	if !f.dirty || f.tmp == nil {
		f.mu.Unlock()
		return 0
	}
	f.dirty = false
	tmp := f.tmp
	f.mu.Unlock()
	st, err := tmp.Stat()
	if err != nil {
		return syscall.EIO
	}
	fsys := f.node.fsys
	key := f.objectKey()
	up := manager.NewUploader(fsys.client, func(u *manager.Uploader) { u.PartSize = 8 << 20 })
	task := fsys.app.enqueueQuiet("upload", key, st.Size(), func(ctx context.Context, progress func(int64, int64)) error {
		progress(0, st.Size())
		reader := io.NewSectionReader(tmp, 0, st.Size())
		in := &s3.PutObjectInput{Bucket: aws.String(fsys.bucket), Key: aws.String(key), Body: &progressReader{r: reader, emit: func(n int64) { progress(n, st.Size()) }, limiter: &fsys.app.bandwidth, ctx: ctx}, ContentType: aws.String(contentTypeFor(key))}
		fsys.opts.apply(in)
		_, err := up.Upload(ctx, in)
		return err
	})
	if err := task.wait(); err != nil {
		f.mu.Lock()
		f.dirty = true
		f.mu.Unlock()
		return fuseErrno(err)
	}
	f.node.mu.Lock()
	f.node.size, f.node.mtime = st.Size(), time.Now()
	f.node.mu.Unlock()
	fsys.invalidate(parentKey(f.node.key))
	return 0
}

func (f *s3File) Fsync(ctx context.Context, flags uint32) syscall.Errno { return f.Flush(ctx) }

func (f *s3File) Release(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tmp != nil {
		f.tmp.Close()
		f.tmp = nil
	}
	f.buf = nil
	return 0
}

// parentKey returns the directory key ("" for the root) of a relative key.
func parentKey(key string) string {
	key = strings.TrimSuffix(key, "/")
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i+1]
	}
	return ""
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func contentTypeFor(key string) string {
	if ct := mime.TypeByExtension(path.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

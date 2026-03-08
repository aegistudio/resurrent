package fusefs

import (
	"context"
	"errors"
	"io"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"

	"github.com/aegistudio/resurrent"
)

type File struct {
	*resurrent.File
}

var _ fs.FileHandle = (*File)(nil)

func (f *File) Getattr(
	ctx context.Context, out *fuse.AttrOut,
) syscall.Errno {
	stat, err := f.File.Stat()
	if err != nil {
		return filterErrno(err)
	}
	node := f.File.DeriveNode()
	defer node.Free()
	fillAttrWithFileInfoNode(&out.Attr, stat, node)
	return 0
}

var _ fs.FileGetattrer = (*File)(nil)

func (f *File) Release(ctx context.Context) syscall.Errno {
	return filterErrno(f.Close())
}

var _ fs.FileReleaser = (*File)(nil)

func (f *File) Read(
	ctx context.Context, dest []byte, off int64,
) (fuse.ReadResult, syscall.Errno) {
	fd, err := f.File.Fd()
	if err == nil {
		return fuse.ReadResultFd(fd, off, len(dest)), 0
	}
	if !errors.Is(err, resurrent.ErrNotBackedByFD) {
		return nil, filterErrno(err)
	}
	n, err := f.File.ReadAt(dest, off)
	buf := dest[:max(n, len(dest))]
	if err == io.EOF {
		// "ReadAt always returns a non-nil error"
		// when n < len(b), at the end of file,
		// that error is io.EOF.
		err = nil
	}
	return fuse.ReadResultData(buf), filterErrno(err)
}

var _ fs.FileReader = (*File)(nil)

func (f *File) utimens(a *time.Time, m *time.Time) error {
	fd, err := f.File.Fd()
	if err != nil {
		return err
	}
	timeval := func(t *time.Time) unix.Timeval {
		if t != nil {
			return unix.NsecToTimeval((*t).UnixNano())
		} else {
			return unix.Timeval{Usec: unix.UTIME_OMIT}
		}
	}
	return unix.Futimes(int(fd), []unix.Timeval{
		timeval(a), timeval(m),
	})
}

func (f *File) Write(
	ctx context.Context, data []byte, off int64,
) (written uint32, errno syscall.Errno) {
	n, err := f.File.WriteAt(data, off)
	return uint32(n), filterErrno(err)
}

var _ fs.FileWriter = (*File)(nil)

func (f *File) Setattr(
	ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut,
) syscall.Errno {
	var a *time.Time
	if atime, ok := in.GetATime(); ok {
		a = &atime
	}
	var m *time.Time
	if mtime, ok := in.GetATime(); ok {
		m = &mtime
	}
	if a != nil || m != nil {
		if err := f.utimens(a, m); err != nil {
			return filterErrno(err)
		}
	}
	if size, ok := in.GetSize(); ok {
		if err := f.Truncate(int64(size)); err != nil {
			return filterErrno(err)
		}
	}

	return f.Getattr(ctx, out)
}

var _ fs.FileSetattrer = (*File)(nil)

func (f *File) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	return filterErrno(f.File.Sync())
}

var _ fs.FileFsyncer = (*File)(nil)

func (fs *FS) createFile(file *resurrent.File) *File {
	return &File{
		File: file,
	}
}

package fusefs

import (
	"errors"
	"os"
	"syscall"

	"github.com/aegistudio/resurrent"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func filterErrno(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	if errors.Is(err, resurrent.ErrNotBackedByFD) {
		return syscall.ENOSYS
	}
	if os.IsNotExist(err) {
		return syscall.ENOENT
	}
	if os.IsExist(err) {
		return syscall.EEXIST
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return syscall.EACCES
}

func fillAttrWithFileInfoNode(
	attr *fuse.Attr, stat os.FileInfo, node *resurrent.Node,
) {
	attr.Ino = node.ID()
	attr.Size = uint64(stat.Size())
	attr.Blksize = uint32((attr.Size + 511) / 512)
	modTime := stat.ModTime()
	attr.SetTimes(&modTime, &modTime, &modTime)
	attr.Mode = fileModeToStatMode(stat.Mode())
	attr.Nlink = 1
	attr.Uid = uint32(os.Getuid())
	attr.Gid = uint32(os.Getgid())
}

func fileModeToStatMode(mode os.FileMode) uint32 {
	perm := uint32(mode.Perm())
	switch mode.Type() {
	case 0:
		return syscall.S_IFREG | perm
	case os.ModeDir:
		return syscall.S_IFDIR | perm
	case os.ModeCharDevice:
		return syscall.S_IFCHR | perm
	case os.ModeDevice:
		return syscall.S_IFBLK | perm
	case os.ModeSymlink:
		return syscall.S_IFLNK | perm
	case os.ModeNamedPipe:
		return syscall.S_IFIFO | perm
	case os.ModeSocket:
		return syscall.S_IFSOCK | perm
	default:
		return perm
	}
}

func fileModeFromStatMode(mode uint32) os.FileMode {
	typ := mode & syscall.S_IFMT
	perm := os.FileMode((mode | syscall.S_IFMT) ^ syscall.S_IFMT).Perm()
	switch typ {
	case syscall.S_IFREG:
		return perm
	case syscall.S_IFDIR:
		return os.ModeDir | perm
	case syscall.S_IFCHR:
		return os.ModeCharDevice | perm
	case syscall.S_IFBLK:
		return os.ModeDevice | perm
	case syscall.S_IFLNK:
		return os.ModeSymlink | perm
	case syscall.S_IFIFO:
		return os.ModeNamedPipe | perm
	case syscall.S_IFSOCK:
		return os.ModeSocket | perm
	default:
		return os.ModeIrregular | perm
	}
}

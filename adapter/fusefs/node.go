package fusefs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/aegistudio/resurrent"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type Node struct {
	fs.Inode
	*resurrent.Node
	fs   *FS
	once sync.Once
}

var _ fs.InodeEmbedder = (*Node)(nil)

func (n *Node) OnForget() {
	n.once.Do(func() {
		defer n.Node.Free()
		defer n.fs.m.Delete(n.Node.ID())
	})
}

var _ fs.NodeOnForgetter = (*Node)(nil)

func (n *Node) childInodeForFile(
	ctx context.Context, f *resurrent.File,
) (*fs.Inode, error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	name := stat.Name()

	n.fs.inodeMtx.Lock()
	defer n.fs.inodeMtx.Unlock()

	result := n.GetChild(name)
	if result == nil {
		node := n.fs.createNode(f.DeriveNode())
		result = n.Inode.NewInode(ctx, node, fs.StableAttr{
			Mode: fileModeToStatMode(stat.Mode()) & syscall.S_IFMT,
		})
		n.Inode.AddChild(name, result, true)
	}
	return result, nil
}

func (n *Node) Create(
	ctx context.Context,
	name string, flags uint32, mode uint32,
	out *fuse.EntryOut,
) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	openFlags := int(flags)
	fileMode := fileModeFromStatMode(mode)
	if openFlags&syscall.O_CREAT == 0 {
		// They must create a file.
		return nil, nil, 0, syscall.EINVAL
	}
	if flags&syscall.O_DIRECTORY != 0 {
		// They should invoke Node.Mkdir then.
		return nil, nil, 0, syscall.EINVAL
	}
	if fileMode.Type() != 0 {
		// Only regular file can be created.
		return nil, nil, 0, syscall.EINVAL
	}

	plock, err := n.Node.TryLock()
	if err != nil {
		return nil, nil, 0, filterErrno(err)
	}
	defer plock.Unlock()

	f, err := n.fs.FS.OpenFile(
		filepath.Join(plock.FilePath(), name),
		openFlags, fileMode,
	)
	if err != nil {
		return nil, nil, 0, filterErrno(err)
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()

	stat, err := f.Stat()
	if err != nil {
		return nil, nil, 0, filterErrno(err)
	}
	node := f.DeriveNode()
	defer node.Free()
	fillAttrWithFileInfoNode(
		&out.Attr, stat, f.DeriveNode(),
	)

	handle := n.fs.createFile(f)
	child, err := n.childInodeForFile(ctx, f)
	if err != nil {
		return nil, nil, 0, filterErrno(err)
	}
	success = true
	return child, handle, 0, 0
}

var _ fs.NodeCreater = (*Node)(nil)

func (n *Node) Getattr(
	ctx context.Context, f fs.FileHandle, out *fuse.AttrOut,
) syscall.Errno {
	if f != nil {
		fga, ok := f.(fs.FileGetattrer)
		if ok {
			return fga.Getattr(ctx, out)
		}
	}
	plock, err := n.Node.TryLock()
	if err != nil {
		return filterErrno(err)
	}
	defer plock.Unlock()
	stat, err := n.fs.Stat(plock.FilePath())
	if err != nil {
		return filterErrno(err)
	}
	fillAttrWithFileInfoNode(&out.Attr, stat, n.Node)
	return 0
}

var _ fs.NodeGetattrer = (*Node)(nil)

func (n *Node) Lookup(
	ctx context.Context, name string, out *fuse.EntryOut,
) (*fs.Inode, syscall.Errno) {
	plock, err := n.Node.TryLock()
	if err != nil {
		return nil, filterErrno(err)
	}
	defer plock.Unlock()

	f, err := n.fs.FS.OpenFile(
		filepath.Join(plock.FilePath(), name),
		os.O_RDONLY, os.FileMode(0),
	)
	if err != nil {
		return nil, filterErrno(err)
	}
	defer func() { _ = f.Close() }()

	inode, err := n.childInodeForFile(ctx, f)
	if err != nil {
		return nil, filterErrno(err)
	}
	return inode, 0
}

var _ fs.NodeLookuper = (*Node)(nil)

func (n *Node) Mkdir(
	ctx context.Context,
	name string, mode uint32,
	out *fuse.EntryOut,
) (*fs.Inode, syscall.Errno) {
	plock, err := n.Node.TryLock()
	if err != nil {
		return nil, filterErrno(err)
	}
	defer plock.Unlock()

	dirPath := filepath.Join(plock.FilePath(), name)

	if err := n.fs.Mkdir(
		dirPath, os.FileMode(mode),
	); err != nil {
		return nil, filterErrno(err)
	}

	f, err := n.fs.FS.OpenFile(
		dirPath, os.O_RDONLY, os.FileMode(0),
	)
	if err != nil {
		return nil, filterErrno(err)
	}
	defer func() { _ = f.Close() }()

	inode, err := n.childInodeForFile(ctx, f)
	if err != nil {
		return nil, filterErrno(err)
	}
	return inode, 0
}

var _ fs.NodeMkdirer = (*Node)(nil)

func (n *Node) open(
	ctx context.Context, flags uint32,
) (*resurrent.File, syscall.Errno) {
	openFlags := int(flags)
	if openFlags&(syscall.O_CREAT|syscall.O_EXCL) != 0 {
		// Because they should invoke Node.Create then.
		return nil, syscall.EINVAL
	}

	plock, err := n.Node.TryLock()
	if err != nil {
		return nil, filterErrno(err)
	}
	defer plock.Unlock()

	checkOpenDir := false
	if flags&syscall.O_DIRECTORY != 0 {
		flags ^= syscall.O_DIRECTORY
		checkOpenDir = true
	}

	f, err := n.fs.FS.OpenFile(
		plock.FilePath(), openFlags, os.FileMode(0),
	)
	if err != nil {
		return nil, filterErrno(err)
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()

	if checkOpenDir {
		stat, err := f.Stat()
		if err != nil {
			return nil, syscall.ENOTDIR
		}
		if !stat.IsDir() {
			return nil, syscall.ENOTDIR
		}
	}
	success = true
	return f, 0
}

func (n *Node) Open(
	ctx context.Context, flags uint32,
) (fs.FileHandle, uint32, syscall.Errno) {
	f, err := n.open(ctx, flags)
	if err != 0 {
		return nil, 0, err
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()
	handle := n.fs.createFile(f)
	success = true
	return handle, 0, 0
}

var _ fs.NodeOpener = (*Node)(nil)

func (n *Node) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	plock, err := n.Node.TryLock()
	if err != nil {
		return nil, filterErrno(err)
	}
	defer plock.Unlock()

	dentries, err := n.fs.ReadDir(plock.FilePath())
	if err != nil {
		return nil, filterErrno(err)
	}

	var result []fuse.DirEntry
	for _, dentry := range dentries {
		result = append(result, fuse.DirEntry{
			Mode: uint32(dentry.Mode()),
			Name: dentry.Name(),
		})
	}
	return fs.NewListDirStream(result), 0
}

var _ fs.NodeReaddirer = (*Node)(nil)

func (n *Node) Rename(
	ctx context.Context, oldName string,
	newParentEmbedder fs.InodeEmbedder, newName string,
	flags uint32,
) syscall.Errno {
	if flags != 0 {
		return syscall.ENOSYS
	}
	if oldName == "" || newName == "" {
		// The name to remove can never be empty,
		// since we obtain the directory read lock,
		// and the file to remove must be write lock.
		return syscall.EINVAL
	}
	newParent := newParentEmbedder.(*Node)

	oldLock, err := n.Node.TryLock()
	if err != nil {
		return filterErrno(err)
	}
	defer oldLock.Unlock()

	newLock, err := newParent.TryLock()
	if err != nil {
		return filterErrno(err)
	}
	defer newLock.Unlock()

	n.fs.inodeMtx.Lock()
	defer n.fs.inodeMtx.Unlock()

	oldPath := filepath.Join(oldLock.FilePath(), oldName)
	oldFile, err := n.fs.OpenFile(oldPath, os.O_RDONLY, 0)
	if err != nil {
		return filterErrno(err)
	}
	defer func() { _ = oldFile.Close() }()

	oldStat, err := oldFile.Stat()
	if err != nil {
		return filterErrno(err)
	}
	oldName = oldStat.Name()
	_ = oldFile.Close()

	newPath := filepath.Join(newLock.FilePath(), newName)
	if err := n.fs.Rename(oldPath, newPath); err != nil {
		return filterErrno(err)
	}

	n.Inode.MvChild(oldName, &newParent.Inode, newName, true)
	return 0
}

var _ fs.NodeRenamer = (*Node)(nil)

func (n *Node) unlinkCheck(
	ctx context.Context, name string,
	checkIsDir bool,
) syscall.Errno {
	if name == "" {
		// The name to remove can never be empty,
		// since we obtain the directory read lock,
		// and the file to remove must be write lock.
		return syscall.EINVAL
	}

	plock, err := n.Node.TryLock()
	if err != nil {
		return filterErrno(err)
	}
	defer plock.Unlock()

	n.fs.inodeMtx.Lock()
	defer n.fs.inodeMtx.Unlock()

	filePath := filepath.Join(plock.FilePath(), name)
	f, err := n.fs.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return filterErrno(err)
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return filterErrno(err)
	}
	name = stat.Name()
	_ = f.Close()

	if checkIsDir && !stat.IsDir() {
		return syscall.ENOTDIR
	}

	if err := n.fs.Remove(filePath); err != nil {
		return filterErrno(err)
	}

	_, _ = n.Inode.RmChild(name)
	return 0
}

func (n *Node) Unlink(
	ctx context.Context, name string,
) syscall.Errno {
	return n.unlinkCheck(ctx, name, false)
}

var _ fs.NodeUnlinker = (*Node)(nil)

func (n *Node) Rmdir(
	ctx context.Context, name string,
) syscall.Errno {
	return n.unlinkCheck(ctx, name, true)
}

var _ fs.NodeRmdirer = (*Node)(nil)

func (n *Node) Setattr(
	ctx context.Context,
	fh fs.FileHandle, in *fuse.SetAttrIn,
	out *fuse.AttrOut,
) syscall.Errno {
	if fh == nil {
		f, err := n.open(ctx, syscall.O_RDWR)
		if err != 0 {
			return err
		}
		defer func() { _ = f.Close() }()
		fh = n.fs.createFile(f)
	}
	if fh != nil {
		if fsa, ok := fh.(fs.FileSetattrer); ok {
			return fsa.Setattr(ctx, in, out)
		}
	}
	return syscall.ENOSYS
}

var _ fs.NodeSetattrer = (*Node)(nil)

func (n *Node) Fsync(
	ctx context.Context, fh fs.FileHandle, flags uint32,
) syscall.Errno {
	if fh == nil {
		f, err := n.open(ctx, syscall.O_RDWR)
		if err != 0 {
			return err
		}
		defer func() { _ = f.Close() }()
		fh = n.fs.createFile(f)
	}
	if fh != nil {
		if fsc, ok := fh.(fs.FileFsyncer); ok {
			return fsc.Fsync(ctx, flags)
		}
	}
	return syscall.ENOSYS
}

var _ fs.NodeFsyncer = (*Node)(nil)

func (fs *FS) createNode(node *resurrent.Node) *Node {
	newNode := &Node{
		fs:   fs,
		Node: node,
	}
	reusedNode, reused := fs.m.LoadOrStore(node.ID(), newNode)
	if reused {
		defer node.Free()
	}
	return reusedNode.(*Node)
}

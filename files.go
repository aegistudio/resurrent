package resurrent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/winfsp/go-winfsp/treelock"

	"github.com/aegistudio/resurrent/format"
)

func mkdirSingle(p string, mode os.FileMode) error {
	err := os.Mkdir(p, mode)
	if os.IsExist(err) {
		stat, err1 := os.Stat(p)
		if err1 != nil {
			return err1
		}
		if stat.IsDir() {
			err = nil
		}
	}
	return err
}

// FileInfo is the value implementing any method
// returning os.FileInfo in this filesystem when
// it is not nil.
//
// It is presented to the caller so that they
// can view more detailed state of the filesystem
// and implement OS-specific features.
type FileInfo struct {
	Meta format.FileMeta
	Stat os.FileInfo
}

func (f *FileInfo) IsDir() bool   { return f.Meta.Type == format.TypeDir }
func (f *FileInfo) Name() string  { return f.Meta.Name }
func (f *FileInfo) Sys() any      { return nil }
func (f *FileInfo) Present() bool { return f.Stat != nil }

func (f *FileInfo) ModTime() time.Time {
	modifiedAt := f.Meta.ModifiedAt
	if f.Stat != nil {
		modifiedAt = f.Stat.ModTime()
	}
	return modifiedAt
}

func (f *FileInfo) Size() int64 {
	size := f.Meta.Size
	if f.Stat != nil {
		size = f.Stat.Size()
	}
	return size
}

func (f *FileInfo) Mode() fs.FileMode {
	result := os.FileMode(f.Meta.Perm)
	// XXX: when the file has not been uploaded
	// and cannot be stated, we will render it
	// as an irregular file.
	if f.Meta.Object == "" && f.Stat == nil {
		result |= os.ModeIrregular
		return result
	}
	switch f.Meta.Type {
	case format.TypeDir:
		result |= os.ModeDir
	case format.TypeUnknown:
		result |= os.ModeIrregular
	}
	return result
}

var _ os.FileInfo = (*FileInfo)(nil)

func (fs *FS) cleanFilterPath(p string) string {
	p = filepath.Clean(filepath.Join(cleanRoot, p))
	if !fs.caseSensitive {
		p = strings.ToUpper(p)
	}
	return p
}

// prefixDirPath converts the path in the form
// of /a/b/c into /D.a/D.b/D.c, as is specified.
// The filepath must be cleaned before use.
func (fs *FS) prefixDirPath(p string) string {
	if p == "" || p == cleanRoot {
		return p
	}
	var result string
	result, ok := fs.prefixDirPathLRU.Get(p)
	if ok {
		return result
	}
	dir, base := filepath.Split(p)
	if base == "" {
		panic("invalid empty base")
	}
	dir = filepath.Clean(dir)
	result = filepath.Join(
		fs.prefixDirPath(dir), "D."+base,
	)
	_ = fs.prefixDirPathLRU.Add(p, result)
	return result
}

func (fs *FS) splitAndClean(p string) (dir, base string) {
	dir, base = filepath.Split(p)
	if base == "" {
		panic("invalid empty base")
	}
	dir = filepath.Clean(dir)
	return
}

func (fs *FS) evaluateMetaPath(dir, base string) string {
	if base == "" {
		panic("invalid empty base")
	}
	return filepath.Join(
		fs.root, "files",
		fs.prefixDirPath(dir), "M."+base,
	)
}

func (fs *FS) evaluateDataPath(
	dir, base string, ty format.FileType,
) string {
	switch ty {
	case format.TypeDir:
		return filepath.Join(
			fs.root, "files",
			fs.prefixDirPath(dir), "D."+base,
		)
	case format.TypeFile:
		return filepath.Join(
			fs.root, "files",
			fs.prefixDirPath(dir), "F."+base,
		)
	default:
		return ""
	}
}

func (fs *FS) statClean(p string) (*FileInfo, error) {
	if p == "" || p == cleanRoot {
		localMetaDir := filepath.Join(fs.root, "files")
		if err := os.MkdirAll(localMetaDir, localDirMode); err != nil {
			return nil, err
		}
		stat, err := os.Stat(localMetaDir)
		if err != nil {
			return nil, err
		}
		return &FileInfo{
			Meta: format.FileMeta{
				Version: format.CurrentVersion,
				Name:    "",
				Type:    format.TypeDir,
				Perm:    format.FilePerm(localDirMode),
			},
			Stat: stat,
		}, nil
	}

	dir, base := fs.splitAndClean(p)
	var err error
	metaPath := fs.evaluateMetaPath(dir, base)
	metaBytes, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if len(metaBytes) == 0 {
		return nil, os.ErrNotExist
	}
	meta, err := format.LoadFileMeta(metaBytes)
	if err != nil {
		return nil, err
	}
	result := &FileInfo{
		Meta: *meta,
	}

	dataPath := fs.evaluateDataPath(dir, base, meta.Type)
	if dataPath == "" {
		return result, nil
	}
	stat, err := os.Stat(dataPath)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	result.Stat = stat
	return result, nil
}

func (fs *FS) Stat(p string) (os.FileInfo, error) {
	p = fs.cleanFilterPath(p)
	fs.mtx.RLock()
	defer fs.mtx.RUnlock()
	stat, err := fs.statClean(p)
	if err != nil {
		// XXX: (*fileStat)(nil) is not nil os.FileInfo.
		return nil, err
	}
	return stat, nil
}

func (fs *FS) listDirClean(p string) ([]*FileInfo, error) {
	dirStat, err := fs.statClean(p)
	if err != nil {
		return nil, err
	}
	if dirStat.Meta.Type != format.TypeDir {
		return nil, syscall.ENOTDIR
	}

	prefixedDir := fs.prefixDirPath(p)
	dirPath := filepath.Join(fs.root, "files", prefixedDir)
	if err := os.MkdirAll(dirPath, localDirMode); err != nil {
		return nil, err
	}
	dentries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var pathsToStat []string
	for _, dentry := range dentries {
		name := dentry.Name()
		if !strings.HasPrefix(strings.ToUpper(name), "M.") {
			continue
		}
		name = name[len("M."):]
		if len(name) == 0 {
			continue
		}
		pathToStat := filepath.Join(p, name)
		pathToStat = fs.cleanFilterPath(pathToStat)
		pathsToStat = append(pathsToStat, pathToStat)
	}

	var result []*FileInfo
	for _, pathToStat := range pathsToStat {
		stat, err := fs.statClean(pathToStat)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		result = append(result, stat)
	}
	return result, nil
}

func (fs *FS) ReadDir(p string) ([]os.FileInfo, error) {
	p = fs.cleanFilterPath(p)
	stats, err := func() ([]*FileInfo, error) {
		fs.mtx.RLock()
		defer fs.mtx.RUnlock()
		return fs.listDirClean(p)
	}()
	if err != nil {
		return nil, err
	}
	var result []os.FileInfo
	for _, stat := range stats {
		result = append(result, stat)
	}
	return result, nil
}

func (fs *FS) ensureDir(p string) error {
	if p == "" || p == cleanRoot {
		// The root directory must exist and created
		// when creating the filesystem.
		return nil
	}
	parentStat, err := fs.statClean(p)
	if err != nil {
		return err
	}
	if parentStat.Meta.Type != format.TypeDir {
		return syscall.ENOTDIR
	}
	dir, base := fs.splitAndClean(p)
	if base == "" {
		panic("invalid empty base")
	}
	dataPath := fs.evaluateDataPath(
		dir, base, format.TypeDir,
	)
	return mkdirSingle(dataPath, localDirMode)
}

func (fs *FS) Mkdir(p string, perm os.FileMode) error {
	perm = perm.Perm()
	p = treelock.UnifyFilePath(p)
	name := filepath.Base(p)
	p = fs.cleanFilterPath(p)
	if p == "" || p == cleanRoot {
		return nil
	}
	fs.mtx.Lock()
	defer fs.mtx.Unlock()
	plock := fs.tl.TryWLockFile(p)
	if plock == nil {
		return syscall.EACCES
	}
	defer plock.Unlock()

	// Fast path: if there has already been a file
	// or directory present there, just report it.
	fileStat, err := fs.statClean(p)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	if fileStat != nil {
		return syscall.EEXIST
	}

	// Slow path: we have to create the directory
	// if the parent directory presents.
	dir, base := fs.splitAndClean(p)
	if err := fs.ensureDir(dir); err != nil {
		return err
	}

	success := false

	dirMetaData, err := (&format.FileMeta{
		Version: format.CurrentVersion,
		Name:    name,
		Type:    format.TypeDir,
		Perm:    format.FilePerm(perm),
	}).Save()
	if err != nil {
		return err
	}

	dirMetaPath := fs.evaluateMetaPath(dir, base)
	if err := os.WriteFile(
		dirMetaPath, dirMetaData, localFileMode,
	); err != nil {
		return err
	}
	defer func() {
		// NOTE: Since we have checked there is
		// no metadata present, so we are the one
		// to create this file, and we must remove
		// this file upon failed.
		if !success {
			_ = os.Remove(dirMetaPath)
		}
	}()
	dirPath := fs.evaluateDataPath(dir, base, format.TypeDir)
	if dirPath == "" {
		panic("invalid empty path")
	}
	if err := os.Mkdir(dirPath, localDirMode); err != nil {
		return err
	}
	defer func() {
		if !success {
			_ = os.Remove(dirPath)
		}
	}()
	success = true
	return nil
}

type File struct {
	fs       *FS
	node     *treelock.Node
	fileType format.FileType
	file     *os.File

	dentriesMtx sync.Mutex
	dentries    *[]os.FileInfo
}

func (f *File) Close() error {
	defer f.node.Free()
	return f.file.Close()
}

func (f *File) Read(b []byte) (int, error) {
	return f.file.Read(b)
}

func (f *File) ReadAt(b []byte, off int64) (int, error) {
	return f.file.ReadAt(b, off)
}

func (f *File) Write(b []byte) (int, error) {
	return f.file.Write(b)
}

func (f *File) WriteAt(b []byte, off int64) (int, error) {
	return f.file.WriteAt(b, off)
}

func (f *File) Sync() error {
	return f.file.Sync()
}

func (f *File) Truncate(size int64) error {
	return f.file.Truncate(size)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	if f.fileType != format.TypeDir {
		return nil, syscall.ENOTDIR
	}
	f.dentriesMtx.Lock()
	defer f.dentriesMtx.Unlock()

	if f.dentries == nil {
		if err := func() error {
			f.fs.mtx.RLock()
			defer f.fs.mtx.RUnlock()
			p := f.node.FilePath()
			p = f.fs.cleanFilterPath(p)
			result, err := f.fs.listDirClean(p)
			if err != nil {
				return err
			}
			var dentries []os.FileInfo
			for _, dentry := range result {
				dentries = append(dentries, dentry)
			}
			f.dentries = new([]os.FileInfo)
			*f.dentries = dentries
			return nil
		}(); err != nil {
			return nil, err
		}
	}
	if count > 0 {
		size := min(len(*f.dentries), count)
		result := make([]os.FileInfo, size)
		copied := copy(result, *f.dentries)
		if copied > 0 {
			*f.dentries = (*f.dentries)[copied:]
		}
		var err error
		if len(*f.dentries) == 0 {
			err = io.EOF
		}
		return result[:count], err
	} else {
		result := *f.dentries
		f.dentries = nil
		return result, nil
	}
}

func (f *File) Stat() (os.FileInfo, error) {
	f.fs.mtx.RLock()
	defer f.fs.mtx.RUnlock()

	if f.node.IsExile() {
		// If the file has been removed, then
		// we simulate the stat.
		stat, err := f.file.Stat()
		if err != nil {
			return nil, err
		}
		return &FileInfo{
			Meta: format.FileMeta{
				Version:    format.CurrentVersion,
				Type:       f.fileType,
				Perm:       format.FilePerm(stat.Mode().Perm()),
				ModifiedAt: stat.ModTime(),
				Size:       stat.Size(),
			},
		}, nil
	}

	p := f.node.FilePath()
	p = f.fs.cleanFilterPath(p)
	result, err := f.fs.statClean(p)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (fs *FS) shouldAutoDownload(p string) bool {
	return fs.downloadOnDemand.Match(filepath.ToSlash(p))
}

func (fs *FS) openLocalFileClean(
	p, name string, lock *treelock.PathLock,
	flag int, perm os.FileMode,
	waitForRemote bool,
) (*File, *downloadTask, error) {
	if p == "" {
		panic("root directory should not go this way")
	}

	var success bool

	node := lock.RetainNode()
	defer func() {
		if !success {
			node.Free()
		}
	}()

	dir, base := fs.splitAndClean(p)

	if err := fs.ensureDir(dir); err != nil {
		return nil, nil, err
	}

	// Create the file if it does not
	// exist. This is valid since we will
	// be the one holding the write lock.
	if flag&os.O_CREATE != 0 {
		fileMetaPath := fs.evaluateMetaPath(dir, base)
		stat, err := os.Stat(fileMetaPath)
		if os.IsNotExist(err) {
			err = nil
			fileMeta := &format.FileMeta{
				Version: format.CurrentVersion,
				Name:    name,
				Type:    format.TypeFile,
				Perm:    format.FilePerm(perm),
			}
			fileMetaData, err := fileMeta.Save()
			if err != nil {
				return nil, nil, err
			}
			if err := os.WriteFile(
				fileMetaPath, fileMetaData, localFileMode,
			); err != nil {
				return nil, nil, err
			}
			defer func() {
				if !success {
					_ = os.Remove(fileMetaPath)
				}
			}()
		}
		if err != nil {
			return nil, nil, err
		}
		if flag&os.O_EXCL != 0 {
			if stat != nil {
				return nil, nil, syscall.EEXIST
			}
			flag ^= os.O_EXCL
		}
	}

	// The corresponding file must exist if so.
	fileStat, err := fs.statClean(p)
	if err != nil {
		return nil, nil, err
	}

	fileType := fileStat.Meta.Type
	fileDataPath := fs.evaluateDataPath(dir, base, fileType)
	switch fileType {
	case format.TypeDir:
		stat, err := os.Stat(fileDataPath)
		if os.IsNotExist(err) {
			err = os.Mkdir(fileDataPath, localDirMode)
		}
		if err != nil {
			return nil, nil, err
		}
		if stat != nil {
			if !stat.Mode().IsDir() {
				return nil, nil, syscall.EACCES
			}
		}
	case format.TypeFile:
		stat, err := os.Stat(fileDataPath)
		if os.IsNotExist(err) {
			err = nil
			object := fileStat.Meta.Object

			if object != "" && flag&os.O_TRUNC == 0 {
				if !waitForRemote {
					return nil, nil, syscall.EACCES
				}
				if !fs.shouldAutoDownload(p) {
					return nil, nil, syscall.EACCES
				}
				downloadTask := fs.findOrNewDownloadTask(
					fileDataPath, &fileStat.Meta,
				)
				return nil, downloadTask, nil
			}
		}
		if err != nil {
			return nil, nil, err
		}
		if stat != nil {
			if !stat.Mode().IsRegular() {
				return nil, nil, syscall.EACCES
			}
		}
	default:
		return nil, nil, syscall.EACCES
	}

	f, err := os.OpenFile(fileDataPath, flag, perm)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()

	result := &File{
		fs:       fs,
		node:     node,
		file:     f,
		fileType: fileType,
	}
	success = true
	return result, nil, nil
}

func (fs *FS) OpenFile(
	p string, flag int, perm os.FileMode,
) (*File, error) {
	perm = perm.Perm()
	p = treelock.UnifyFilePath(p)
	name := filepath.Base(p)
	p = fs.cleanFilterPath(p)

	// Fast path: open the filesystem root.
	if p == "" || p == cleanRoot {
		var success bool

		fs.mtx.RLock()
		defer fs.mtx.RUnlock()
		rootNode := fs.tl.AllocFile(cleanRoot)
		if rootNode == nil {
			panic("invalid empty root node")
		}
		defer func() {
			if !success {
				rootNode.Free()
			}
		}()

		rootFile, err := os.OpenFile(
			filepath.Join(fs.root, "files"),
			flag, perm,
		)
		if err != nil {
			return nil, err
		}
		defer func() {
			if !success {
				_ = rootFile.Close()
			}
		}()

		result := &File{
			fs:       fs,
			node:     rootNode,
			file:     rootFile,
			fileType: format.TypeDir,
		}
		success = true
		return result, nil
	}

	// Pass 1: Attempt to find the file and
	// allocate the download task.
	var plock *treelock.PathLock
	defer func() {
		if plock != nil {
			plock.Unlock()
		}
	}()
	f, download, err := func() (*File, *downloadTask, error) {
		// See if there's existing download task for
		// the path first. If so, wait for that
		// download task to be done.
		fs.mtx.Lock()
		defer fs.mtx.Unlock()

		plock = fs.tl.TryRLockFile(p)
		if plock == nil {
			return nil, nil, syscall.EACCES
		}

		return fs.openLocalFileClean(
			p, name, plock, flag, perm, true,
		)
	}()
	if err != nil {
		return nil, err
	}
	if f != nil {
		return f, nil
	}

	// Interlude: wait for the download task.
	if download != nil {
		<-download.doneCh
		if err := download.err; err != nil {
			return nil, err
		}
	}

	// Pass 2: Attempt to open the file again,
	// but this time they said they have downloaded
	// that file, so it must complete.
	return func() (*File, error) {
		fs.mtx.Lock()
		defer fs.mtx.Unlock()

		f, _, err := fs.openLocalFileClean(
			p, name, plock, flag, perm, false,
		)
		return f, err
	}()
}

func (fs *FS) Remove(p string) error {
	p = fs.cleanFilterPath(p)
	if p == "" || p == cleanRoot {
		// Cannot remove root directory.
		return syscall.EACCES
	}

	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	exile := fs.tl.WLockExile()
	defer exile.Unlock()

	plock := fs.tl.TryWLockFile(p)
	if plock == nil {
		return syscall.EACCES
	}
	defer plock.Unlock()

	dir, base := fs.splitAndClean(p)

	fileStat, err := fs.statClean(p)
	if err != nil {
		return err
	}
	fileType := fileStat.Meta.Type
	if err := os.Remove(
		fs.evaluateDataPath(dir, base, fileType),
	); err != nil {
		return err
	}
	if err := os.Remove(
		fs.evaluateMetaPath(dir, base),
	); err != nil {
		return err
	}

	treelock.Exchange(plock, exile)
	return nil
}

func (fs *FS) Rename(source, target string) error {
	var success bool
	source = fs.cleanFilterPath(source)
	target = treelock.UnifyFilePath(target)
	targetName := filepath.Base(target)
	target = fs.cleanFilterPath(target)

	// Cannot move root directory.
	if source == "" || source == cleanRoot {
		return syscall.EACCES
	}
	if target == "" || target == cleanRoot {
		return syscall.EACCES
	}

	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	sourceLock := fs.tl.TryWLockFile(source)
	if sourceLock == nil {
		return syscall.EACCES
	}
	defer sourceLock.Unlock()

	// Ensure the presence of source file.
	sourceStat, err := fs.statClean(source)
	if err != nil {
		return err
	}
	sourceDir, sourceBase := fs.splitAndClean(source)
	sourceDataPath := fs.evaluateDataPath(
		sourceDir, sourceBase, sourceStat.Meta.Type,
	)
	if sourceDataPath == "" {
		// The file type is not known to us, we
		// can't perform the operation.
		return syscall.EACCES
	}
	sourceMetaPath := fs.evaluateMetaPath(sourceDir, sourceBase)

	targetDir, targetBase := fs.splitAndClean(target)
	targetMetaPath := fs.evaluateMetaPath(targetDir, targetBase)
	targetDataPath := fs.evaluateDataPath(
		targetDir, targetBase, sourceStat.Meta.Type,
	)
	if targetDataPath == "" {
		// Since sourceDataPath was not empty.
		panic("impossible empty target data path")
	}

	// Special case: if source == target, then we
	// are just updating the name of file. If so,
	// it suffices to write to the metadata.
	if source == target {
		// We assert that if source == target after
		// fs.cleanFilterPath, then the metadata and
		// data path must also be identical.
		if sourceMetaPath != targetMetaPath {
			panic("impossible mismatch meta path")
		}
		if sourceDataPath != targetDataPath {
			panic("impossible mismatch data path")
		}
		meta := sourceStat.Meta
		meta.Name = targetName
		metaData, err := meta.Save()
		if err != nil {
			return err
		}
		return os.WriteFile(
			sourceMetaPath, metaData, localFileMode,
		)
	}

	// If not so, the source metadata must be
	// removed when the rename is successful.
	defer func() {
		if success {
			_ = os.Remove(sourceMetaPath)
		}
	}()

	exile := fs.tl.WLockExile()
	defer exile.Unlock()

	targetLock := fs.tl.TryWLockFile(target)
	if targetLock == nil {
		return syscall.EACCES
	}
	defer targetLock.Unlock()

	// First, we make sure the parent of target
	// directory exists and is a directory.
	if err := fs.ensureDir(targetDir); err != nil {
		return err
	}

	// Then, we try to remove the target file.
	targetStat, err := fs.statClean(target)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	if targetStat != nil {
		if targetStat.Meta.Type == format.TypeDir {
			// XXX: If the target is a directory, we
			// won't try to remove it.
			return syscall.EISDIR
		}
		existingDataPath := fs.evaluateDataPath(
			targetDir, targetBase, targetStat.Meta.Type,
		)
		if existingDataPath == "" {
			// The file type is not known to us, we
			// can't perform the operation.
			return syscall.EACCES
		}
		if err := os.Remove(existingDataPath); err != nil {
			return err
		}
		if err := os.Remove(targetMetaPath); err != nil {
			return err
		}
		treelock.Exchange(targetLock, exile)
	}

	// Now the target path has nothing, and the
	// parent directory exists. We are save to
	// move into it.
	targetMeta := sourceStat.Meta
	targetMeta.Name = targetName
	targetMetaData, err := targetMeta.Save()
	if err != nil {
		return err
	}
	if err := os.WriteFile(
		targetMetaPath, targetMetaData, localFileMode,
	); err != nil {
		return err
	}
	defer func() {
		if !success {
			_ = os.Remove(targetMetaPath)
		}
	}()

	if err := os.Rename(sourceDataPath, targetDataPath); err != nil {
		return err
	}
	treelock.Exchange(sourceLock, targetLock)
	success = true
	return nil
}

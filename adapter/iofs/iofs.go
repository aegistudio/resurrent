// Package iofs adapts the resurrent filesystem
// into the io/fs interface.
//
// The path is always asserted to be slash separated,
// since io/fs/walk.go uses path.Join to build paths,
// confirming their intention.
package iofs

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aegistudio/resurrent"
)

type File struct {
	*resurrent.File
}

func (f *File) ReadDir(n int) ([]fs.DirEntry, error) {
	var result []fs.DirEntry
	fileInfos, err := f.File.Readdir(n)
	for _, fileInfo := range fileInfos {
		result = append(result, fs.FileInfoToDirEntry(fileInfo))
	}
	return result, err
}

var _ fs.File = (*File)(nil)

var _ fs.ReadDirFile = (*File)(nil)

type FS struct {
	*resurrent.FS
}

func (f *FS) Open(name string) (fs.File, error) {
	return f.FS.OpenFile(
		filepath.FromSlash(name), os.O_RDONLY, 0,
	)
}

var _ fs.FS = (*FS)(nil)

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	var result []fs.DirEntry
	fileInfos, err := f.FS.ReadDir(filepath.FromSlash(name))
	for _, fileInfo := range fileInfos {
		result = append(result, fs.FileInfoToDirEntry(fileInfo))
	}
	return result, err
}

var _ fs.ReadDirFS = (*FS)(nil)

func (fs *FS) Stat(name string) (fs.FileInfo, error) {
	return fs.FS.Stat(filepath.FromSlash(name))
}

var _ fs.StatFS = (*FS)(nil)

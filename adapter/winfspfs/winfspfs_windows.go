package winfspfs

import (
	"os"

	"github.com/winfsp/go-winfsp/gofs"

	"github.com/aegistudio/resurrent"
)

type File struct {
	*resurrent.File
}

var _ gofs.File = (*File)(nil)

type FS struct {
	*resurrent.FS
}

func (fs *FS) OpenFile(
	name string, flag int, perm os.FileMode,
) (gofs.File, error) {
	f, err := fs.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &File{f}, nil
}

var _ gofs.FileSystem = (*FS)(nil)

func (fs *FS) DefaultOptions() []gofs.NewOption {
	var result []gofs.NewOption
	result = append(
		result, gofs.WithCaseInsensitive(!fs.CaseSensitive()),
	)
	return result
}

var _ gofs.FileSystemDefaultOptions = (*FS)(nil)

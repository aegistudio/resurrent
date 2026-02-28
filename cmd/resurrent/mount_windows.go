package main

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/winfsp/go-winfsp"
	"github.com/winfsp/go-winfsp/gofs"

	"github.com/aegistudio/resurrent"
)

type winFile struct {
	*resurrent.File
}

var _ gofs.File = (*winFile)(nil)

type winFS struct {
	*resurrent.FS
}

func (fs *winFS) OpenFile(
	name string, flag int, perm os.FileMode,
) (gofs.File, error) {
	f, err := fs.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var _ gofs.FileSystem = (*winFS)(nil)

func (fs *winFS) DefaultOptions() []gofs.NewOption {
	var result []gofs.NewOption
	result = append(
		result, gofs.WithCaseInsensitive(!fs.CaseSensitive()),
	)
	return result
}

var _ gofs.FileSystemDefaultOptions = (*winFS)(nil)

var cfgMount struct {
	mountpoint string
}

var cmdMount = &cobra.Command{
	Use:   "mount",
	Short: "Mount and serve the filesystem",
	RunE: func(cmd *cobra.Command, args []string) error {
		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		fsHandle, err := winfsp.Mount(
			gofs.New(&winFS{fs}), cfgMount.mountpoint,
		)
		if err != nil {
			return errors.Wrap(err, "mount fs")
		}
		defer fsHandle.Unmount()

		select {
		case <-cmd.Context().Done():
			return nil
		case <-fs.Done():
			return fs.Close()
		}
	},
}

func init() {
	cmdMount.PersistentFlags().StringVarP(
		&cfgMount.mountpoint,
		"mountpoint", "m", "R:",
		"Mountpoint of the filesystem",
	)
	rootCmd.AddCommand(cmdMount)
}

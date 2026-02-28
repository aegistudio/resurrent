package main

import (
	"io/fs"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/adapter/iofs"
)

var cmdDownload = &cobra.Command{
	Use:   "download",
	Short: "download content at path from object storage",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rfs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer rfs.Close()

		root := args[0]
		if _, err := rfs.Stat(root); err != nil {
			return errors.Wrapf(err, "stat root %q", root)
		}

		adaptedIOFS := &iofs.FS{FS: rfs}
		slashRoot := filepath.ToSlash(root)
		return fs.WalkDir(adaptedIOFS, slashRoot, func(
			path string, d fs.DirEntry, err error,
		) error {
			if err != nil {
				return err
			}
			path = filepath.FromSlash(path)
			return rfs.Download(path)
		})
	},
}

func init() {
	rootCmd.AddCommand(cmdDownload)
}

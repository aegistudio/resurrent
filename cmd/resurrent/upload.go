package main

import (
	"io/fs"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/adapter/iofs"
)

var cfgUpload struct {
	engines []string
}

var cmdUpload = &cobra.Command{
	Use:   "upload",
	Short: "Upload content at path to object storage",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfgUpload.engines == nil {
			return errors.Errorf("must specify engine to upload")
		}

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
			for _, engine := range cfgUpload.engines {
				if err := rfs.Upload(path, engine); err != nil {
					return errors.Wrapf(
						err, "upload %q engine %q",
						path, engine,
					)
				}
			}
			return nil
		})
	},
}

func init() {
	cmdUpload.PersistentFlags().StringArrayVarP(
		&cfgUpload.engines, "engine", "e", nil,
		"Specify the engine name(s) to upload",
	)
	rootCmd.AddCommand(cmdUpload)
}

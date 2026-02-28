package main

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/adapter/iofs"
)

var cfgFind struct {
	name string
}

var cmdFind = &cobra.Command{
	Use:   "find",
	Short: "Find files by pattern",
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
			if pattern := cfgFind.name; pattern != "" {
				pathMatch, err := filepath.Match(pattern, path)
				if err != nil {
					return errors.Wrap(err, "match path")
				}
				nameMatch, err := filepath.Match(pattern, d.Name())
				if err != nil {
					return errors.Wrap(err, "match name")
				}
				if !(pathMatch || nameMatch) {
					return nil
				}
			}
			fmt.Println(path)
			return nil
		})
	},
}

func init() {
	cmdFind.PersistentFlags().StringVar(
		&cfgFind.name, "name", "",
		"Pattern to match the file names",
	)
	rootCmd.AddCommand(cmdFind)
}

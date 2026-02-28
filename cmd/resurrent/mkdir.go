package main

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/format"
)

var cfgMkdir struct {
	perm string
}

var cmdMkdir = &cobra.Command{
	Use:   "mkdir",
	Short: "Create a directory at path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		targetPath := args[0]
		perm, err := format.ParseFilePerm(cfgMkdir.perm)
		if err != nil {
			return err
		}

		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		err = fs.Mkdir(
			targetPath, os.ModeDir|os.FileMode(perm),
		)
		if os.IsExist(err) {
			stat, err1 := fs.Stat(targetPath)
			if err1 != nil {
				return errors.Wrapf(err, "stat %q", targetPath)
			}
			if stat.IsDir() {
				err = nil
			}
		}
		if err != nil {
			return errors.Wrapf(err, "mkdir %q", targetPath)
		}
		return nil
	},
}

func init() {
	cmdMkdir.PersistentFlags().StringVar(
		&cfgMkdir.perm, "perm",
		format.FilePerm(os.FileMode(0755)).String(),
		"permission of the directory",
	)
	rootCmd.AddCommand(cmdMkdir)
}

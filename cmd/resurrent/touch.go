package main

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/format"
)

var cfgTouch struct {
	noCreate bool
	truncate bool
	perm     string
}

var cmdTouch = &cobra.Command{
	Use:   "touch",
	Short: "Create or modify a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := args[0]
		perm, err := format.ParseFilePerm(cfgTouch.perm)
		if err != nil {
			return err
		}

		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		flags := os.O_RDWR | os.O_CREATE
		if cfgTouch.noCreate {
			flags ^= os.O_CREATE
		}
		if cfgTouch.truncate {
			flags |= os.O_TRUNC
		}

		f, err := fs.OpenFile(targetPath, flags, os.FileMode(perm))
		if errors.Is(err, syscall.EISDIR) {
			// Stracing touch under linux, we can see opening a
			// file which is a directory will result in EISDIR,
			// but touch just ignore it. And we will do so.
			err = nil
		}
		if err != nil {
			return errors.Wrapf(err, "touch file %q", targetPath)
		}
		defer func() {
			if f != nil {
				_ = f.Close()
			}
		}()

		return nil
	},
}

func init() {
	cmdTouch.PersistentFlags().StringVar(
		&cfgTouch.perm, "perm",
		format.FilePerm(os.FileMode(0644)).String(),
		"permission of the file",
	)
	cmdTouch.PersistentFlags().BoolVarP(
		&cfgTouch.noCreate, "no-create", "c", false,
		"do not create any files",
	)
	cmdTouch.PersistentFlags().BoolVar(
		&cfgTouch.truncate, "truncate", false,
		"truncate the file",
	)
	rootCmd.AddCommand(cmdTouch)
}

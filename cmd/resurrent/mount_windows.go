package main

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/winfsp/go-winfsp"
	"github.com/winfsp/go-winfsp/gofs"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/adapter/winfspfs"
)

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
			gofs.New(&winfspfs.FS{FS: fs}), cfgMount.mountpoint,
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

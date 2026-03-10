package main

import (
	"log"
	"time"

	fuseFS "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/aegistudio/resurrent"
	fuseFSAdapter "github.com/aegistudio/resurrent/adapter/fusefs"
)

var cfgMount struct {
	debug bool
}

var cmdMount = &cobra.Command{
	Use:   "mount",
	Short: "Mount and serve the filesystem",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		grp, ctx := errgroup.WithContext(cmd.Context())
		defer func() { _ = grp.Wait() }()

		mountpoint := args[0]
		adaptFS := fuseFSAdapter.New(fs)
		defer adaptFS.Close()
		rootNode := adaptFS.RootNode()
		nodeFS := fuseFS.NewNodeFS(rootNode, &fuseFS.Options{})
		server, err := fuse.NewServer(
			nodeFS, mountpoint, &fuse.MountOptions{
				FsName:        "resurrent",
				Name:          "resurrent",
				DisableXAttrs: true,
				Debug:         cfgMount.debug,
			},
		)
		if err != nil {
			return errors.Wrap(err, "mount fs")
		}
		defer func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				err := server.Unmount()
				if err == nil {
					return
				}
				log.Printf("unmount %q: %v", mountpoint, err)
				<-ticker.C
			}
		}()

		grp.Go(func() error {
			server.Serve()
			return nil
		})
		select {
		case <-cmd.Context().Done():
			return nil
		case <-fs.Done():
			return fs.Close()
		case <-ctx.Done():
			return grp.Wait()
		}
	},
}

func init() {
	cmdMount.PersistentFlags().BoolVarP(
		&cfgMount.debug, "debug", "d", false,
		"Turn on debug output of filesystem",
	)
	rootCmd.AddCommand(cmdMount)
}

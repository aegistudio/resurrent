package main

import (
	"io/fs"
	"log"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/adapter/iofs"
)

var cfgUpload struct {
	engines     []string
	parallelism uint
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

		uploadOne := func(path string) error {
			log.Printf("uploading %q", path)
			for _, engine := range cfgUpload.engines {
				if err := rfs.Upload(path, engine); err != nil {
					return errors.Wrapf(
						err, "upload %q engine %q",
						path, engine,
					)
				}
			}
			log.Printf("uploaded %q", path)
			return nil
		}

		grp, ctx := errgroup.WithContext(cmd.Context())

		parallelism := int(cfgUpload.parallelism)
		if parallelism <= 0 {
			parallelism = max(1, runtime.GOMAXPROCS(-1))
		}
		pathCh := make(chan string, parallelism)
		for range parallelism {
			grp.Go(func() error {
				for {
					select {
					case path, ok := <-pathCh:
						if !ok {
							return nil
						}
						if err := uploadOne(path); err != nil {
							return err
						}
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			})
		}

		grp.Go(func() error {
			defer close(pathCh)
			adaptedIOFS := &iofs.FS{FS: rfs}
			slashRoot := filepath.ToSlash(root)
			return fs.WalkDir(adaptedIOFS, slashRoot, func(
				path string, d fs.DirEntry, err error,
			) error {
				if err != nil {
					return err
				}
				path = filepath.FromSlash(path)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case pathCh <- path:
				}
				return nil
			})
		})

		return grp.Wait()
	},
}

func init() {
	cmdUpload.PersistentFlags().StringArrayVarP(
		&cfgUpload.engines, "engine", "e", nil,
		"Specify the engine name(s) to upload",
	)
	cmdUpload.PersistentFlags().UintVarP(
		&cfgUpload.parallelism, "parallelism", "p", 0,
		"Number of parallelism to upload files",
	)
	rootCmd.AddCommand(cmdUpload)
}

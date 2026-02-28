package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
)

var cmdLs = &cobra.Command{
	Use:   "ls",
	Short: "List the content in a directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := args[0]
		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		stat, err := fs.Stat(targetPath)
		if err != nil {
			return errors.Wrapf(err, "stat %q", targetPath)
		}
		var dirents []os.FileInfo
		if stat.IsDir() {
			dirents, err = fs.ReadDir(targetPath)
			if err != nil {
				return errors.Wrapf(err, "list dir %q", targetPath)
			}
		} else {
			dirents = append(dirents, stat)
		}

		for _, dirent := range dirents {
			fmt.Printf(
				"%s %d %s %s\n",
				dirent.Mode(), dirent.Size(),
				dirent.ModTime().Format(time.RFC3339),
				dirent.Name(),
			)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(cmdLs)
}

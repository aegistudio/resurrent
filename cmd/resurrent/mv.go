package main

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
)

var cmdMv = &cobra.Command{
	Use:   "mv",
	Short: "Move the file at path to path",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath, targetPath := args[0], args[1]

		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		return fs.Rename(sourcePath, targetPath)
	},
}

func init() {
	rootCmd.AddCommand(cmdMv)
}

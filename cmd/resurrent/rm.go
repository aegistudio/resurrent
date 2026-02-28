package main

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
)

var cmdRm = &cobra.Command{
	Use:   "rm",
	Short: "Remove the file at path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := args[0]

		fs, err := resurrent.New(rootCfg.rootDir)
		if err != nil {
			return errors.Wrap(err, "new fs")
		}
		defer fs.Close()

		return fs.Remove(targetPath)
	},
}

func init() {
	rootCmd.AddCommand(cmdRm)
}

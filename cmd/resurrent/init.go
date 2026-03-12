package main

import (
	"github.com/spf13/cobra"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/format"
)

var cfgInit struct {
	caseSensitive bool
	inlineMaxSize int64
}

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize or upgrade resurrent filesystem",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath, err := getRootDir()
		if err != nil {
			return err
		}
		return resurrent.Init(
			targetPath,
			&format.FSConfig{
				Version: format.CurrentFSConfigVersion,
			},
		)
	},
}

func init() {
	cmdInit.PersistentFlags().BoolVarP(
		&cfgInit.caseSensitive,
		"case-sensitive", "i", true,
		"initialize a case sensitive filesystem",
	)
	cmdInit.PersistentFlags().Int64Var(
		&cfgInit.inlineMaxSize,
		"inline-max-size",
		int64(format.DefaultInlineMaxSize),
		"default size to inline file",
	)
	rootCmd.AddCommand(cmdInit)
}

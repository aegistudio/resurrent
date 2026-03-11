package main

import (
	"github.com/aegistudio/resurrent/migrate"
	"github.com/spf13/cobra"
)

var cfgMigrate struct {
	maxVersion uint64
}

var cmdMigrate = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate the filesystem",
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []migrate.Option
		if maxVer := cfgMigrate.maxVersion; maxVer != 0 {
			opts = append(opts, migrate.MaxVersion(maxVer))
		}
		return migrate.Migrate(
			cmd.Context(), rootCfg.rootDir, opts...,
		)
	},
}

func init() {
	cmdMigrate.PersistentFlags().Uint64Var(
		&cfgMigrate.maxVersion, "max-version", 0,
		"maximum version to migrate to, debug only",
	)
	rootCmd.AddCommand(cmdMigrate)
}

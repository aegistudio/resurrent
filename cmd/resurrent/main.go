package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	_ "github.com/aegistudio/resurrent/engine/host"
)

var rootCfg struct {
	rootDir string
}

var rootCmd = &cobra.Command{
	Use:   "resurrent",
	Short: "Two-Stage Snapshot Filesystem",
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&rootCfg.rootDir,
		"root-dir", "r", "",
		"path of the root directory",
	)
}

func getRootDir() (string, error) {
	if rootDir := rootCfg.rootDir; rootDir != "" {
		return rootDir, nil
	}
	result, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "get working dir")
	}
	return result, nil
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt,
	)
	defer cancel()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

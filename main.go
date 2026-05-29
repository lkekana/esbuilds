/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd.AddCommand(initCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "esbuilds [npm|pnpm] [args...]",
	Short: "Run npm/pnpm with auto-managed esbuild versions",
	Long: `esbuilds automatically builds and caches esbuild binaries based on 
your project's lockfile, then delegates to npm or pnpm with ESBUILD_BINARY_PATH set.`,
	RunE: loadEsbuild,
	Args: cobra.MinimumNArgs(1),
}

var initCmd = &cobra.Command{
	Use:   "init [bash|zsh|fish]",
	Short: "Initialize shell aliases for esbuilds",
	Long:  "Add aliases to your shell config so 'npm' and 'pnpm' automatically use esbuilds",
	Args:  cobra.ExactArgs(1),
	Run:   initShell,
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.esbuilds.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

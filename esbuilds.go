package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func loadEsbuild(cmd *cobra.Command, args []string) error {
	start := time.Now()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}

	pm := strings.ToLower(args[0])
	pmArgs := args[1:]
	if pm != "npm" && pm != "pnpm" {
		return fmt.Errorf("unknown package manager: %s (expected npm or pnpm)", pm)
	}

	esbuildRepo := filepath.Join(homeDir, "esbuild")
	version := findAndParseNearestLockfile(pm)
	binaryPath := ""
	binaryWasBuilt := false

	if version != "" {
		binaryPath = filepath.Join(homeDir, "esbuilds", version, "esbuild")

		// check is required version is already built and cached
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			if _, err := os.Stat(esbuildRepo); os.IsNotExist(err) {
				color.Red("esbuild repo not found at %s", esbuildRepo)
				binaryPath = ""
			} else {
				// fetch latest tags (new versions maybe)
				fetchCmd := exec.Command("git", "fetch", "--tags", "--quiet")
				fetchCmd.Dir = esbuildRepo
				_ = fetchCmd.Run()

				// checkout requested version
				checkoutCmd := exec.Command("git", "checkout", "-q", "-f", "v"+version)
				checkoutCmd.Dir = esbuildRepo
				checkoutCmd.Stdout = os.Stdout
				checkoutCmd.Stderr = os.Stderr

				if err := checkoutCmd.Run(); err != nil {
					// try version without leading "v*"
					checkoutCmd = exec.Command("git", "checkout", "-q", "-f", version)
					checkoutCmd.Dir = esbuildRepo
					checkoutCmd.Stdout = os.Stdout
					checkoutCmd.Stderr = os.Stderr
					if err := checkoutCmd.Run(); err != nil {
						return fmt.Errorf("failed to checkout esbuild version %s: %w", version, err)
					}
				}

				if binaryPath != "" {
					err := os.MkdirAll(filepath.Dir(binaryPath), 0755)
					if err != nil {
						return fmt.Errorf("error creating cache directory: %w", err)
					}

					// build directly into the cache directory
					tempBinary := binaryPath + ".tmp"
					buildCmd := exec.Command("go", "build", "-o", tempBinary, "./cmd/esbuild")
					buildCmd.Dir = esbuildRepo
					buildCmd.Stdout = os.Stdout
					buildCmd.Stderr = os.Stderr

					if err := buildCmd.Run(); err != nil {
						os.Remove(tempBinary)
						return fmt.Errorf("Error while building esbuild version %s via golang: %w", version, err)
					}

					binaryWasBuilt = true
					// rename temporary file
					if err := os.Rename(tempBinary, binaryPath); err != nil {
						return fmt.Errorf("failed to move built binary: %w", err)
					}
					_ = os.Chmod(binaryPath, 0755)

				}
			}
		}
	}

	newEnv := setupEnv(binaryPath)

	// find package manager binary in PATH
	pmPath, err := exec.LookPath(pm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s not found in PATH: %v\n", pm, err)
		os.Exit(1)
	}

	argv := append([]string{pm}, pmArgs...)
	elapsed := time.Since(start)

	if version != "" {
		if binaryWasBuilt {
			fmt.Printf("🚀 Starting %s with esbuild version %s (built & loaded in %s)\n", pm, version, elapsed)
		} else {
			fmt.Printf("🚀 Starting %s with esbuild version %s (cached, took %s)\n", pm, version, elapsed)
		}
	}

	// replace current process & hand everything over to the package manager
	// using syscall.Exec (Unix-only)
	if err := syscall.Exec(pmPath, argv, newEnv); err != nil {
		return fmt.Errorf("failed to exec %s: %w", pm, err)
	}
	return nil // unreachable, but required for error handling
}

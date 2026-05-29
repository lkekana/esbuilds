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
)

func main() {
	start := time.Now()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}

	pm, pmArgs := resolvePackageManager()
	if pm == "" {	
		if len(os.Args) > 1 {
			fmt.Fprintf(os.Stderr, "Unknown command or package manager: %s\n", os.Args[1])
		}
		fmt.Fprintln(os.Stderr, "Usage: esbuilds [npm|pnpm] [args...]")
		os.Exit(1)
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
				fetchCmd.Run()

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
						fmt.Fprintln(os.Stderr, color.RedString("Failed to checkout esbuild version %s", version))
						binaryPath = ""
					}
				}

				if binaryPath != "" {
					err := os.MkdirAll(filepath.Dir(binaryPath), 0755)
					if err != nil {
						fmt.Println("Error creating cache directory:", err)
						binaryPath = ""
					} else {
						// build directly into the cache directory
						tempBinary := binaryPath + ".tmp"
						buildCmd := exec.Command("go", "build", "-o", tempBinary, "./cmd/esbuild")
						buildCmd.Dir = esbuildRepo
						buildCmd.Stdout = os.Stdout
						buildCmd.Stderr = os.Stderr
						if err := buildCmd.Run(); err != nil {
							fmt.Fprintln(os.Stderr, color.RedString("Failed to build esbuild %s", version))
							os.Remove(tempBinary)
							binaryPath = ""
						} else {
							binaryWasBuilt = true
							// rename temporary file
							os.Rename(tempBinary, binaryPath)
							os.Chmod(binaryPath, 0755)
						}
					}
				}
			}
		}
	}

	// override ESBUILD_BINARY_PATH only if our cache has the binary
	var newEnv []string
	found := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "ESBUILD_BINARY_PATH=") {
			if binaryPath != "" {
				newEnv = append(newEnv, "ESBUILD_BINARY_PATH="+binaryPath)
			} else {
				newEnv = append(newEnv, e) // keep existing value if we don't have a new one
			}
			found = true
		} else {
			newEnv = append(newEnv, e)
		}
	}
	if binaryPath != "" && !found {
		newEnv = append(newEnv, "ESBUILD_BINARY_PATH="+binaryPath)
	}

	// replace current process & hand everything over to the package manager
	// using syscall.Exec (Unix-only)
	pmPath, err := exec.LookPath(pm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s not found in PATH: %v\n", pm, err)
		os.Exit(1)
	}
	argv := append([]string{pm}, pmArgs...)

	// syscall.Exec replaces the current process image with the package manager.
	// This call only returns if there's an error.
	elapsed := time.Since(start)
	if version != "" {
		if binaryWasBuilt {
			fmt.Printf("🚀 Starting %s with esbuild version %s (built & loaded in %s)\n", pm, version, elapsed)
		} else {
			fmt.Printf("🚀 Starting %s with esbuild version %s (cached, took %s)\n", pm, version, elapsed)
		}
	}

	if err := syscall.Exec(pmPath, argv, newEnv); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec %s: %v\n", pm, err)
		os.Exit(1)
	}
}

func resolvePackageManager() (string, []string) {
	if len(os.Args) > 1 {
		cmd := strings.ToLower(os.Args[1])
		if cmd == "npm" || cmd == "pnpm" {
			return cmd, os.Args[2:]
		}
	}
	return "", nil
}

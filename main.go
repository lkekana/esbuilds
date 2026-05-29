package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

func main() {
	start := time.Now()

	// Get terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // Fallback width
	}
	fmt.Println(strings.Repeat("%", width))


	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}

	esbuildRepo := filepath.Join(homeDir, "esbuild")
	version := ""

	// find nearest pnpm-lock.yaml
	lockfilePath := ""
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for {
			p := filepath.Join(dir, "pnpm-lock.yaml")
			if _, err := os.Stat(p); err == nil {
				lockfilePath = p
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break // Reached filesystem root
			}
			dir = parent
		}
	}

	// get esbuild version from lockfile
	if lockfilePath != "" {
		content, err := os.ReadFile(lockfilePath)
		if err == nil {
			// Find the exact package name "esbuild" avoiding things like "esbuild-plugin"
			reEsbuild := regexp.MustCompile(`(?m)(?:^|[/\s"'])esbuild(?:[@/:\s"']|$)`)
			loc := reEsbuild.FindStringIndex(string(content))
			if loc != nil {
				// Look at the next 300 characters for a semantic version (0.x.y)
				end := loc[1] + 300
				if end > len(content) {
					end = len(content)
				}
				snippet := string(content)[loc[1]:end]
				
				reVersion := regexp.MustCompile(`(0\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`)
				vMatch := reVersion.FindStringSubmatch(snippet)
				if len(vMatch) > 1 {
					version = vMatch[1]
				}
			}
		}
	}

	binaryPath := ""
	binaryWasBuilt := false
	if version != "" {
		binaryPath = filepath.Join(homeDir, "esbuilds", version, "esbuild")

		// check is required version is already built and cached
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			// fmt.Printf("🔧 esbuild version %s not found in cache. Building...\n", version)

			if _, err := os.Stat(esbuildRepo); os.IsNotExist(err) {
				// fmt.Printf("⚠️  esbuild repo not found at %s\n", esbuildRepo)
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
						// fmt.Printf( "⚠️  Failed to checkout esbuild version %s\n", version)
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
							// fmt.Printf("⚠️  Failed to build esbuild %s\n", version)
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

	// override ESBUILD_BINARY_PATH only if we our cache has the binary
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

	// replace current process & hand everything over to pnpm using syscall.Exec (Unix-only)
	pnpmPath, err := exec.LookPath("pnpm")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pnpm not found in PATH: %v\n", err)
		os.Exit(1)
	}
	argv := append([]string{"pnpm"}, os.Args[1:]...)
	
	// syscall.Exec replaces the current process image with pnpm.
	// This call only returns if there's an error.
	elapsed := time.Since(start)
	// fmt.Printf("🚀 Starting pnpm with esbuild version %s (took %s)\n", version, elapsed)
	if binaryWasBuilt {
		fmt.Printf("🚀 Starting pnpm with esbuild version %s (built & loaded in %s)\n", version, elapsed)
	} else if version != "" {
		fmt.Printf("🚀 Starting pnpm with esbuild version %s (cached, took %s)\n", version, elapsed)
	} else {
		fmt.Printf("🚀 Starting pnpm with esbuild version (default, took %s)\n", elapsed)
	}
	fmt.Println(strings.Repeat("%", width))

	if err := syscall.Exec(pnpmPath, argv, newEnv); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec pnpm: %v\n", err)
		os.Exit(1)
	}
}
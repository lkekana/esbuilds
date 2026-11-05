package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var binaryWasBuilt = false

func loadEsbuild(cmd *cobra.Command, args []string) error {
	start := time.Now()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}

	_, err = exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}

	pm := strings.ToLower(args[0])
	pmArgs := args[1:]
	if pm != "npm" && pm != "pnpm" {
		return fmt.Errorf("unknown package manager: %s (expected npm or pnpm)", pm)
	}

	isInstallCmd := false
	isHelpCmd := false
	firstCommand := ""
	for _, arg := range pmArgs {
		if arg == "--help" || arg == "-h" || arg == "help" {
			isHelpCmd = true
			break
		}
		if !strings.HasPrefix(arg, "-") {
			firstCommand = strings.ToLower(arg)
			break
		}
	}
	if firstCommand != "" {
		if firstCommand == "install" || firstCommand == "i" || firstCommand == "add" || firstCommand == "ci" || firstCommand == "update" || firstCommand == "up" || firstCommand == "upgrade" {
			isInstallCmd = true
		}
	}

	newEnv := os.Environ()

	// find package manager binary in PATH
	pmPath, err := exec.LookPath(pm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s not found in PATH: %v\n", pm, err)
		os.Exit(1)
	}

	if isInstallCmd {
		// inject --ignore-scripts, so we can patch the esbuild binaries before running postinstall scripts
		hasIgnoreScripts := false
		for _, arg := range pmArgs {
			if arg == "--ignore-scripts" {
				hasIgnoreScripts = true
				break
			}
		}
		runArgs := pmArgs
		if !hasIgnoreScripts {
			runArgs = append(runArgs, "--ignore-scripts")
		}

		installCmd := exec.Command(pmPath, runArgs...)
		installCmd.Stdin = os.Stdin
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Env = newEnv

		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("package manager install failed: %w", err)
		}

		print("\n")

		versions, err := getEsbuildVersions(pm)
		// print("🔍 Discovered esbuild versions: %v\n", versions)
		if err != nil {
			return fmt.Errorf("failed to get esbuild versions: %w", err)
		} else {
			// pre-build all discovered versions
			for _, v := range versions {
				cachedBinary := filepath.Join(homeDir, "esbuilds", v, "esbuild")
				if _, err := os.Stat(cachedBinary); os.IsNotExist(err) {
					fmt.Printf("⚙️  Building esbuild %s...\n", v)
					if err := buildSpecificVersion(v, homeDir); err != nil {
						return fmt.Errorf("failed to build esbuild version %s: %w", v, err)
					} else {
						binaryWasBuilt = true
					}
				}
			}
		}

		// patch the binaries in the virtual store
		if err := patchEsbuildBinaries(homeDir); err != nil {
			return fmt.Errorf("failed to patch esbuild binaries: %w", err)
		}

		// prevent `ERR_PNPM_IGNORED_BUILDS` during rebuild
		// allow esbuild builds in pnpm workspace config
		if pm == "pnpm" {
			err = ensureEsbuildBuildAllowed()
			if err != nil {
				return fmt.Errorf("failed to ensure allowBuilds for esbuild: %w", err)
			}
		}

		// run rebuild to trigger postinstall scripts
		rebuildCmd := exec.Command(pmPath, "rebuild")
		rebuildCmd.Stdin = os.Stdin
		rebuildCmd.Stdout = os.Stdout
		rebuildCmd.Stderr = os.Stderr
		rebuildCmd.Env = newEnv

		if err := rebuildCmd.Run(); err != nil {
			return fmt.Errorf("rebuild failed: %w", err)
		}

		print("\n")

		// sometimes the postinstall scripts will copy the cached binary to the node_modules
		// we patch again to keep a symlink instead (save space and avoid issues with incompatible binaries)
		// fmt.Println("🛠️  Patching again after post-install scripts...")
		if err := patchEsbuildBinaries(homeDir); err != nil {
			return fmt.Errorf("failed to patch esbuild binaries: %w", err)
		}

		elapsed := time.Since(start)
		fmt.Printf("🎉 esbuild installation complete! (took %s)\n", elapsed)
		return nil
	}

	// For non-install commands (e.g. pnpm run dev), just patch and execute
	if !isHelpCmd {
		if err := patchEsbuildBinaries(homeDir); err != nil {
			return fmt.Errorf("failed to patch esbuild binaries: %w", err)
		}
	}

	argv := append([]string{pm}, pmArgs...)
	elapsed := time.Since(start)
	if binaryWasBuilt {
		fmt.Printf("🚀 Starting %s (built & loaded in %s)\n", pm, elapsed)
	} else if !isHelpCmd {
		fmt.Printf("🚀 Starting %s (cached, took %s)\n", pm, elapsed)
	}

	// replace current process & hand everything over to the package manager
	// using syscall.Exec (Unix-only)
	if err := syscall.Exec(pmPath, argv, newEnv); err != nil {
		return fmt.Errorf("failed to exec %s: %w", pm, err)
	}
	return nil // unreachable, but required for error handling
}

func patchEsbuildBinaries(homeDir string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	nodeModulesDir := filepath.Join(cwd, "node_modules")
	if _, err := os.Stat(nodeModulesDir); os.IsNotExist(err) {
		return nil // Nothing to patch
	}

	var paths []string

	// find esbuild in pnpm virtual store
	// Matches: node_modules/.pnpm/@esbuild+darwin-x64@0.28.1/node_modules/@esbuild/darwin-x64/bin/esbuild
	pnpmPattern := filepath.Join(nodeModulesDir, ".pnpm", "@esbuild+*", "node_modules", "@esbuild", "*", "bin", "esbuild")
	if matches, _ := filepath.Glob(pnpmPattern); len(matches) > 0 {
		paths = append(paths, matches...)
	}

	// find esbuild in npm node_modules
	npmPattern := filepath.Join(nodeModulesDir, "@esbuild", "*", "bin", "esbuild")
	if matches, _ := filepath.Glob(npmPattern); len(matches) > 0 {
		paths = append(paths, matches...)
	}

	// check node_modules for npm nested dependencies (and fallback for any other weird structure)
	filepath.WalkDir(nodeModulesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Ignore errors like permission denied
		}
		if !d.IsDir() && d.Name() == "esbuild" && filepath.Base(filepath.Dir(path)) == "bin" {
			// Avoid duplicates

			// binDir := filepath.Dir(path)
			// pkgDir := filepath.Dir(binDir)
			// Ensure it's actually under an @esbuild scope
			// if filepath.Base(filepath.Dir(pkgDir)) == "@esbuild" {
				found := false
				for _, p := range paths {
					if p == path {
						found = true
						break
					}
				}
				if !found {
					paths = append(paths, path)
				}
			// }
		}
		return nil
	})

	patchedCount := 0
	for _, binPath := range paths {
		// try to extract version from the directory name (pnpm style)
		version := extractVersionFromPath(binPath)
		if version == "" {
			// fallback: read package.json of the esbuild packages in `node_modules` (npm style)
			pkgDir := filepath.Dir(filepath.Dir(binPath))
			version = readVersionFromPackageJson(pkgDir)
		}

		if version == "" {
			fmt.Printf("⚠️  Could not determine version for %s, skipping patch.\n", binPath)
			continue
		}

		cachedBinary := filepath.Join(homeDir, "esbuilds", version, "esbuild")

		// Ensure the binary is built
		if _, err := os.Stat(cachedBinary); os.IsNotExist(err) {
			// fmt.Printf("⚙️  Building esbuild %s...\n", version)
			err := buildSpecificVersion(version, homeDir)
			if err != nil {
				fmt.Printf("⚠️  Failed to build %s: %v\n", version, err)
				continue
			} else {
				binaryWasBuilt = true
			}
		}

		// Check if it's already a symlink pointing to our cache
		if target, err := os.Readlink(binPath); err == nil && target == cachedBinary {
			continue // Already patched
		}

		// Delete the existing broken/incompatible binary
		os.Remove(binPath)

		// create symlink (fallback to copy for Windows without Dev Mode)
		if err := os.Symlink(cachedBinary, binPath); err != nil {
			if copyErr := copyFile(cachedBinary, binPath); copyErr != nil {
				return fmt.Errorf("failed to symlink or copy %s -> %s: %w", binPath, cachedBinary, err)
			}
		}
		// fmt.Printf("🔗 Patched @esbuild %s\n (linked %s to %s)\n", version, binPath, cachedBinary)
		patchedCount++
	}

	// if patchedCount > 0 {
	// 	fmt.Printf("✅ Successfully patched %d esbuild binaries.\n", patchedCount)
	// }
	return nil
}

func extractVersionFromPath(binPath string) string {
	// Normalize path separators for consistent splitting
	parts := strings.Split(filepath.ToSlash(binPath), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "@esbuild+") {
			lastAt := strings.LastIndex(part, "@")
			if lastAt != -1 {
				return part[lastAt+1:]
			}
		}
		if strings.HasPrefix(part, "esbuild@") {
			return part[len("esbuild@"):]
		}
	}
	return ""
}

func readVersionFromPackageJson(pkgDir string) string {
	pkgJsonPath := filepath.Join(pkgDir, "package.json")
	content, err := os.ReadFile(pkgJsonPath)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return ""
	}
	// if strings.HasPrefix(pkg.Name, "@esbuild/") {
	// 	return pkg.Version
	// }
	// return ""
	return pkg.Version
}

func buildSpecificVersion(version string, homeDir string) error {
	esbuildRepo := filepath.Join(homeDir, "esbuild")

	if _, err := os.Stat(esbuildRepo); os.IsNotExist(err) {
		// clone the esbuild repo if it doesn't exist
		cloneCmd := exec.Command("git", "clone", "https://github.com/evanw/esbuild.git", esbuildRepo, "-q")
		cloneCmd.Stdout = os.Stdout
		cloneCmd.Stderr = os.Stderr
		if err := cloneCmd.Run(); err != nil {
			return fmt.Errorf("failed to clone esbuild repository: %w", err)
		}
	}

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

	binaryPath := filepath.Join(homeDir, "esbuilds", version, "esbuild")
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

	// rename temporary file
	if err := os.Rename(tempBinary, binaryPath); err != nil {
		return fmt.Errorf("failed to move built binary: %w", err)
	}
	return os.Chmod(binaryPath, 0755)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	if info, err := os.Stat(src); err == nil {
		os.Chmod(dst, info.Mode())
	}
	return nil
}

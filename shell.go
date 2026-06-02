package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func initShell(cmd *cobra.Command, args []string) {
	shellName := args[0]
	switch shellName {
	case "bash":
		initBash()
	case "zsh":
		initZsh()
	case "fish":
		initFish()
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s\nSupported shells are: bash, zsh, fish\n", shellName)
		os.Exit(1)
	}
}

func initBash() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return
	}

	// prefer .bash_profile, but check for .bashrc and .profile as well
	bashProfilePath := filepath.Join(homeDir, ".bash_profile")
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	profilePath := filepath.Join(homeDir, ".profile")

	var configFile string
	if _, err := os.Stat(bashProfilePath); err == nil {
		configFile = bashProfilePath
	} else if _, err := os.Stat(bashrcPath); err == nil {
		configFile = bashrcPath
	} else if _, err := os.Stat(profilePath); err == nil {
		configFile = profilePath
	} else {
		// use .bash_profile by default if none exist
		configFile = bashProfilePath
	}

	updateShellConfig(configFile, "bash")
}

func initZsh() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return
	}

	// Zsh typically uses .zshrc
	configFile := filepath.Join(homeDir, ".zshrc")
	updateShellConfig(configFile, "zsh")
}

func initFish() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return
	}

	// Fish uses ~/.config/fish/config.fish
	configFile := filepath.Join(homeDir, ".config", "fish", "config.fish")
	updateShellConfig(configFile, "fish")
}

func updateShellConfig(configFile, shellType string) {
	// Check if file contains aliases for esbuilds to avoid duplicates
	hasPnpmAlias := false
	hasNpmAlias := false
	if _, err := os.Stat(configFile); err == nil {
		file, err := os.Open(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening config file %s: %v\n", configFile, err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), `alias pnpm="esbuilds pnpm"`) {
				hasPnpmAlias = true
			}
			if strings.Contains(scanner.Text(), `alias npm="esbuilds npm"`) {
				hasNpmAlias = true
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", configFile, err)
			return
		}
	}

	// Skip if already configured
	if hasNpmAlias && hasPnpmAlias {
		fmt.Printf("✓ pnpm and npm aliases already present in %s\n", configFile)
		return
	}

	// Check if pnpm and npm commands exist in PATH
	hasPnpm := commandExists("pnpm")
	hasNpm := commandExists("npm")

	if !hasPnpm && !hasNpm {
		fmt.Fprintln(os.Stderr, "⚠️  Neither pnpm nor npm found in PATH. Aliases not added.")
		return
	}

	// Prepare alias lines based on shell syntax
	var aliases []string
	if hasPnpmAlias {
		if hasPnpm {
			aliases = append(aliases, `alias pnpm="esbuilds pnpm"`)
		}
	} else {
		fmt.Printf("✓ Found pnpm alias in %s. Skipping.\n", configFile)
	}

	if hasNpmAlias {
		if hasNpm {
			aliases = append(aliases, `alias npm="esbuilds npm"`)
		}
	} else {
		fmt.Printf("✓ Found npm alias in %s. Skipping.\n", configFile)
	}

	if len(aliases) == 0 {
		return
	}

	// Ensure directory exists (important for fish config)
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
		return
	}

	// Open file for appending (create if doesn't exist)
	err := appendAliases(configFile, aliases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", configFile, err)
		return
	}

	fmt.Printf("✓ Updated %s with esbuilds aliases\n", configFile)
}

func checkAliases(configFile string) (hasPnpm, hasNpm bool) {
	file, err := os.Open(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening config file %s: %v\n", configFile, err)
		return false, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `alias pnpm="esbuilds pnpm"`) {
			hasPnpm = true
		}
		if strings.Contains(line, `alias npm="esbuilds npm"`) {
			hasNpm = true
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", configFile, err)
	}
	return hasPnpm, hasNpm
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func appendAliases(configFile string, aliases []string) error {
	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening config file for writing %s: %v\n", configFile, err)
		return err
	}
	defer f.Close()

	// Add newline before appending if file has content
	if stat, err := os.Stat(configFile); err == nil && stat.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing newline to config file: %v\n", err)
			return err
		}
	}

	// Write alias lines
	for _, alias := range aliases {
		if _, err := f.WriteString(alias + "\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing alias to config file: %v\n", err)
			return err
		}
	}
	return nil
}

func setupEnv(binaryPath string) []string {
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
	return newEnv
}

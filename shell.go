package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func initShell() {
	var shellName string
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: esbuilds init [bash|zsh|fish]")
		return
	}

	if os.Args[2] == "bash" || os.Args[2] == "zsh" || os.Args[2] == "fish" {
		shellName = os.Args[2]
	} else {
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s\n. Supported shells are: bash, zsh, fish.\n", os.Args[2])
		return
	}

	if shellName == "bash" {
		initBash()
	} else if shellName == "zsh" {
		initZsh()
	} else if shellName == "fish" {
		initFish()
	} else {
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s\n. Supported shells are: bash, zsh, fish.\n", shellName)
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
		fmt.Printf("✓ pnpm and npm aliases already present in %s.\n", configFile)
		return
	}

	// Check if pnpm and npm commands exist in PATH
	_, err := exec.LookPath("pnpm")
	hasPnpm := err == nil
	_, err = exec.LookPath("npm")
	hasNpm := err == nil

	if !hasPnpm && !hasNpm {
		return
	}

	// Prepare alias lines based on shell syntax
	var lines []string
	if hasPnpmAlias {
		if hasPnpm {
			lines = append(lines, `alias pnpm="esbuilds pnpm"`)
		}
	} else {
		fmt.Printf("✓ Found pnpm alias in %s. Skipping.\n", configFile)
	}

	if hasNpmAlias {
		if hasNpm {
			lines = append(lines, `alias npm="esbuilds npm"`)
		}
	} else {
		fmt.Printf("✓ Found npm alias in %s. Skipping.\n", configFile)
	}

	if len(lines) == 0 {
		return
	}

	// Ensure parent directory exists (important for fish config)
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
		return
	}

	// Open file for appending (create if doesn't exist)
	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening config file for writing %s: %v\n", configFile, err)
		return
	}
	defer f.Close()

	// Add newline before appending if file has content
	if stat, err := os.Stat(configFile); err == nil && stat.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing newline to config file: %v\n", err)
			return
		}
	}

	// Write alias lines
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing alias to config file: %v\n", err)
			return
		}
	}

	fmt.Printf("✓ Updated %s with esbuilds aliases\n", configFile)
}

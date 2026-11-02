package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// $ pnpm config get allowBuilds --json
func getWorkspaceAllowedBuilds() (map[string]bool, error) {
	cmd := exec.Command("pnpm", "config", "get", "allowBuilds", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run '%s': %w", cmd.String(), err)
	}

	output = []byte(strings.TrimSpace(string(output)))
	if len(output) == 0 || string(output) == "null" || string(output) == "undefined" {
		return nil, nil
	}

	var results map[string]bool
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("failed to parse 'pnpm config get allowBuilds' output: %w", err)
	}

	return results, nil
}

// $ pnpm config set --location=project --json allowBuilds '{"esbuild": true}'
func setEsbuildAllowed(allowedBuilds map[string]bool) (error) {
	allowedBuilds["esbuild"] = true

	jsonData, err := json.Marshal(allowedBuilds)
	if err != nil {
		return fmt.Errorf("failed to marshal allowBuilds: %w", err)
	}

	cmd := exec.Command("pnpm", "config", "set", "--location=project", "--json", "allowBuilds", string(jsonData))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run '%s': %w", cmd.String(), err)
	}
	return nil
}

func ensureEsbuildBuildAllowed() error {
	allowedBuilds, err := getWorkspaceAllowedBuilds()
	if err != nil {
		return err
	}

	if allowedBuilds == nil {
		allowedBuilds = make(map[string]bool)
	}

	if allowedBuilds["esbuild"] {
		return nil
	}

	if err := setEsbuildAllowed(allowedBuilds); err != nil {
		return err
	}
	return nil
}
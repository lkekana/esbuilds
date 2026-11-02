package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type WhyResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func getEsbuildVersions(pm string) ([]string, error) {
	cmd := exec.Command(pm, "why", "esbuild", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run '%s why esbuild --json': %w", pm, err)
	}

	var results []WhyResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("failed to parse '%s why' output: %w", pm, err)
	}

	versionSet := make(map[string]bool)
	var versions []string

	for _, res := range results {
		// Filter for the exact "esbuild" package, ignoring aliases
		// if res.Name == "esbuild" && res.Version != "" {
		if res.Version != "" {
			if !versionSet[res.Version] {
				versionSet[res.Version] = true
				versions = append(versions, res.Version)
			}
		}
	}

	return versions, nil
}
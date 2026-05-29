package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type lockfileCandidate struct {
	filename string
	parse    func(content []byte) (string, bool)
}

var candidates = []lockfileCandidate{
	{
		filename: "pnpm-lock.yaml",
		parse: func(content []byte) (string, bool) {
			reEsbuild := regexp.MustCompile(`(?m)(?:^|[/\s"'])esbuild(?:[@/:\s"']|$)`)
			loc := reEsbuild.FindStringIndex(string(content))
			if loc == nil {
				return "", false
			}
			end := loc[1] + 300
			if end > len(content) {
				end = len(content)
			}
			snippet := string(content)[loc[1]:end]
			reVersion := regexp.MustCompile(`(0\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`)
			vMatch := reVersion.FindStringSubmatch(snippet)
			if len(vMatch) > 1 {
				return vMatch[1], true
			}
			return "", false
		},
	},
	{
		filename: "package-lock.json",
		parse: func(content []byte) (string, bool) {
			var lockfile struct {
				Packages map[string]struct {
					Version string `json:"version"`
				} `json:"packages"`
				Dependencies map[string]struct {
					Version string `json:"version"`
				} `json:"dependencies"`
			}
			if err := json.Unmarshal(content, &lockfile); err != nil {
				return "", false
			}
			// lockfile v3 (npm 7+)
			if pkg, ok := lockfile.Packages["node_modules/esbuild"]; ok && pkg.Version != "" {
				return pkg.Version, true
			}
			// lockfile v1 (npm 6 and earlier)
			if dep, ok := lockfile.Dependencies["esbuild"]; ok && dep.Version != "" {
				return dep.Version, true
			}
			return "", false
		},
	},
}

func findAndParseNearestLockfile(pm string) string {
	var candidate *lockfileCandidate
	if pm == "pnpm" {
		candidate = &candidates[0]
	} else if pm == "npm" {
		candidate = &candidates[1]
	}
	if candidate == nil {
		fmt.Printf("Unsupported package manager: %s\n", pm)
		return ""
	}

	version := ""
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for {
			p := filepath.Join(dir, candidate.filename)
			content, err := os.ReadFile(p)
			if err != nil {
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
				continue
			}
			if v, ok := candidate.parse(content); ok {
				version = v
				break
			}
			break
		}
	}
	return version
}

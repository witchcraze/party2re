package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	checkOnly := flag.Bool("check", false, "Only check if OpenAPI specs are synchronized without modifying files")
	flag.Parse()

	rootDir, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	docsPath := filepath.Join(rootDir, "docs", "api", "openapi.json")
	pkgPath := filepath.Join(rootDir, "internal", "api", "http", "openapi.json")

	docsStat, docsErr := os.Stat(docsPath)
	pkgStat, pkgErr := os.Stat(pkgPath)

	if docsErr != nil && pkgErr != nil {
		fmt.Fprintf(os.Stderr, "Error: neither %s nor %s exists\n", docsPath, pkgPath)
		os.Exit(1)
	}

	// Determine source file: default to docs/api/openapi.json, or pick the newer one
	sourcePath := docsPath
	if docsErr != nil {
		sourcePath = pkgPath
	} else if pkgErr == nil && pkgStat.ModTime().After(docsStat.ModTime()) {
		sourcePath = pkgPath
	}

	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading source OpenAPI spec %s: %v\n", sourcePath, err)
		os.Exit(1)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(sourceData, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "Error: source %s is not valid JSON: %v\n", sourcePath, err)
		os.Exit(1)
	}

	var specMap map[string]interface{}
	if err := json.Unmarshal(raw, &specMap); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing OpenAPI structure: %v\n", err)
		os.Exit(1)
	}

	version, _ := specMap["openapi"].(string)
	if !strings.HasPrefix(version, "3.1.") {
		fmt.Fprintf(os.Stderr, "Warning: expected OpenAPI version 3.1.x, got %q\n", version)
	}

	paths, _ := specMap["paths"].(map[string]interface{})
	pathCount := len(paths)

	formattedData, err := json.MarshalIndent(specMap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}
	formattedData = append(formattedData, '\n')

	// Check if already in sync
	docsCurrent, _ := os.ReadFile(docsPath)
	pkgCurrent, _ := os.ReadFile(pkgPath)

	inSync := bytes.Equal(bytes.TrimSpace(docsCurrent), bytes.TrimSpace(formattedData)) &&
		bytes.Equal(bytes.TrimSpace(pkgCurrent), bytes.TrimSpace(formattedData))

	if *checkOnly {
		if !inSync {
			fmt.Fprintf(os.Stderr, "Error: OpenAPI specs are out of sync or unformatted. Run 'make openapi-sync' to synchronize.\n")
			os.Exit(1)
		}
		fmt.Printf("OpenAPI specs are synchronized and formatted (%d endpoints).\n", pathCount)
		return
	}

	if err := os.WriteFile(docsPath, formattedData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", docsPath, err)
		os.Exit(1)
	}

	if err := os.WriteFile(pkgPath, formattedData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", pkgPath, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully synchronized and formatted OpenAPI %s specification (%d endpoints)\n", version, pathCount)
	fmt.Printf("  - %s\n", docsPath)
	fmt.Printf("  - %s\n", pkgPath)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find repository root containing go.mod")
}

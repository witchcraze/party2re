package architecture_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// whitelistedLegacyFileLimits defines pre-existing files exceeding 500 lines.
// No new files may be added to this map.
// Each entry records the exact baseline line count as of Issue #418.
// If any whitelisted file grows larger than its baseline, the test fails.
// When a file is refactored below 500 lines, it MUST be removed from this list.
var whitelistedLegacyFileLimits = map[string]int{
	"internal/api/http/handler.go":             1122,
	"internal/party/valkey_repository.go":      824,
	"internal/party/application.go":            706,
	"internal/contest/service.go":              645,
	"internal/api/http/combat.go":              576,
	"internal/database/delivery_repository.go": 541,
	"internal/monster/monster.go":              504,
}

const (
	maxProductionFileLines = 500
	maxMainFileLines       = 150
)

// countLines counts the number of lines in a byte slice matching wc -l / newline counting.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if !bytes.HasSuffix(data, []byte("\n")) {
		n++
	}
	return n
}

// evaluateFileSize checks if a file satisfies architecture line limits.
// Returns an error message if any rule is violated, or empty string if valid.
func evaluateFileSize(relPath string, lineCount int, whitelist map[string]int) string {
	cleanPath := filepath.ToSlash(relPath)

	// Rule 1: Application entrypoints (cmd/*/main.go) must be <= 150 lines
	if strings.HasPrefix(cleanPath, "cmd/") && filepath.Base(cleanPath) == "main.go" {
		if lineCount > maxMainFileLines {
			return fmt.Sprintf("entrypoint %s is %d lines, exceeding limit of %d lines (.agents/rules/03-architecture.md Section 9)",
				cleanPath, lineCount, maxMainFileLines)
		}
		return ""
	}

	// Rule 2: Whitelisted legacy files
	if baseline, isWhitelisted := whitelist[cleanPath]; isWhitelisted {
		if lineCount > baseline {
			return fmt.Sprintf("whitelisted legacy file %s grew from baseline %d lines to %d lines (ratchet regression)",
				cleanPath, baseline, lineCount)
		}
		if lineCount <= maxProductionFileLines {
			return fmt.Sprintf("whitelisted file %s is now %d lines (<= %d); please remove it from whitelistedLegacyFileLimits to ratchet down",
				cleanPath, lineCount, maxProductionFileLines)
		}
		return ""
	}

	// Rule 3: All other production Go files in internal/ and cmd/ must be <= 500 lines
	if lineCount > maxProductionFileLines {
		return fmt.Sprintf("production file %s is %d lines, exceeding limit of %d lines. Decompose into peer files per .agents/rules/03-architecture.md Section 9",
			cleanPath, lineCount, maxProductionFileLines)
	}

	return ""
}

func TestProductionFileSizeLimits(t *testing.T) {
	repoRoot := "../.."
	start := time.Now()

	var checkedCount int
	var violations []string

	targetDirs := []string{
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd"),
	}

	seenWhitelisted := make(map[string]bool)

	for _, dir := range targetDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Only inspect Go source files, skipping tests
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			relPath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			cleanRelPath := filepath.ToSlash(relPath)

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			lineCount := countLines(data)
			checkedCount++

			if _, ok := whitelistedLegacyFileLimits[cleanRelPath]; ok {
				seenWhitelisted[cleanRelPath] = true
			}

			if violation := evaluateFileSize(cleanRelPath, lineCount, whitelistedLegacyFileLimits); violation != "" {
				violations = append(violations, violation)
			}

			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk directory %s: %v", dir, err)
		}
	}

	for whitelistedPath := range whitelistedLegacyFileLimits {
		if !seenWhitelisted[whitelistedPath] {
			violations = append(violations, fmt.Sprintf("whitelisted file %s no longer exists; remove obsolete entry from whitelistedLegacyFileLimits", whitelistedPath))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Detected %d file size violation(s):\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}

	t.Logf("Checked %d production Go files in %s (violations: %d, whitelisted: %d)",
		checkedCount, time.Since(start), len(violations), len(whitelistedLegacyFileLimits))
}

func TestEvaluateFileSizeLogic(t *testing.T) {
	mockWhitelist := map[string]int{
		"internal/legacy/large.go": 750,
	}

	tests := []struct {
		name        string
		path        string
		lines       int
		whitelist   map[string]int
		expectError bool
		errorSubstr string
	}{
		{
			name:        "valid main.go",
			path:        "cmd/party2/main.go",
			lines:       128,
			whitelist:   mockWhitelist,
			expectError: false,
		},
		{
			name:        "oversized main.go",
			path:        "cmd/party2/main.go",
			lines:       151,
			whitelist:   mockWhitelist,
			expectError: true,
			errorSubstr: "exceeding limit of 150 lines",
		},
		{
			name:        "standard production file within limit",
			path:        "internal/dungeon/dungeon_step.go",
			lines:       350,
			whitelist:   mockWhitelist,
			expectError: false,
		},
		{
			name:        "oversized unlisted production file",
			path:        "internal/feature/new_feature.go",
			lines:       501,
			whitelist:   mockWhitelist,
			expectError: true,
			errorSubstr: "exceeding limit of 500 lines",
		},
		{
			name:        "whitelisted file within baseline",
			path:        "internal/legacy/large.go",
			lines:       720,
			whitelist:   mockWhitelist,
			expectError: false,
		},
		{
			name:        "whitelisted file grew beyond baseline",
			path:        "internal/legacy/large.go",
			lines:       751,
			whitelist:   mockWhitelist,
			expectError: true,
			errorSubstr: "ratchet regression",
		},
		{
			name:        "whitelisted file refactored below 500 lines should ratchet down",
			path:        "internal/legacy/large.go",
			lines:       480,
			whitelist:   mockWhitelist,
			expectError: true,
			errorSubstr: "please remove it from whitelistedLegacyFileLimits to ratchet down",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := evaluateFileSize(tc.path, tc.lines, tc.whitelist)
			if tc.expectError {
				if res == "" {
					t.Fatalf("expected error containing %q, got empty string", tc.errorSubstr)
				}
				if !strings.Contains(res, tc.errorSubstr) {
					t.Fatalf("expected error containing %q, got: %s", tc.errorSubstr, res)
				}
			} else {
				if res != "" {
					t.Fatalf("expected no error, got: %s", res)
				}
			}
		})
	}
}

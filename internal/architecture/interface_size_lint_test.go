package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// whitelistedLegacyInterfaceLimits defines pre-existing domain interfaces exceeding 10 direct methods.
// No new interfaces may be added to this map.
// Each entry records the exact baseline direct method count as of Issue #439.
// If any whitelisted interface grows larger than its baseline, the test fails.
// When an interface is refactored to <= 10 direct methods, it MUST be removed from this list.
var whitelistedLegacyInterfaceLimits = map[string]int{
	"ranking.Repository": 15,
	"party.Repository":   13, // 13 direct methods (14 including embedded AdventureLogRepository)
	"guild.Repository":   11,
}

const maxDirectInterfaceMethods = 10

type interfaceTarget struct {
	Key           string // e.g. "contest.ContestRepository"
	Pkg           string
	Name          string
	DirectMethods int
	FilePath      string
	Line          int
}

// countDirectInterfaceMethods counts directly declared methods on an interface,
// excluding embedded interfaces.
func countDirectInterfaceMethods(iface *ast.InterfaceType) int {
	if iface == nil || iface.Methods == nil {
		return 0
	}
	count := 0
	for _, field := range iface.Methods.List {
		if len(field.Names) > 0 {
			count++
		}
	}
	return count
}

// evaluateInterfaceSize checks if an interface satisfies direct method count limits.
// Returns an error message if any rule is violated, or empty string if valid.
func evaluateInterfaceSize(key string, directMethods int, whitelist map[string]int) string {
	if baseline, isWhitelisted := whitelist[key]; isWhitelisted {
		if directMethods > baseline {
			return fmt.Sprintf("whitelisted legacy interface %s grew from baseline %d direct methods to %d (ratchet regression)",
				key, baseline, directMethods)
		}
		if directMethods <= maxDirectInterfaceMethods {
			return fmt.Sprintf("whitelisted interface %s now has %d direct methods (<= %d); please remove it from whitelistedLegacyInterfaceLimits to ratchet down",
				key, directMethods, maxDirectInterfaceMethods)
		}
		return ""
	}

	if directMethods > maxDirectInterfaceMethods {
		return fmt.Sprintf("interface %s has %d direct methods, exceeding limit of %d. Decompose into composite sub-interfaces per .agents/rules/03-architecture.md Section 9",
			key, directMethods, maxDirectInterfaceMethods)
	}

	return ""
}

// collectDomainInterfaces scans internal/ domain packages for interface declarations.
func collectDomainInterfaces(repoRoot string) ([]interfaceTarget, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	fset := token.NewFileSet()
	var targets []interfaceTarget

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		cleanPath := filepath.ToSlash(path)
		// Skip internal/architecture, internal/api (HTTP handlers), and test utilities
		if strings.Contains(cleanPath, "internal/architecture") ||
			strings.Contains(cleanPath, "internal/api") ||
			strings.Contains(cleanPath, "internal/testutil") ||
			strings.Contains(cleanPath, "internal/database/testutil") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		node, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			return nil
		}

		pkgName := node.Name.Name

		for _, decl := range node.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				iface, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}

				pos := fset.Position(ts.Pos())
				key := fmt.Sprintf("%s.%s", pkgName, ts.Name.Name)
				targets = append(targets, interfaceTarget{
					Key:           key,
					Pkg:           pkgName,
					Name:          ts.Name.Name,
					DirectMethods: countDirectInterfaceMethods(iface),
					FilePath:      path,
					Line:          pos.Line,
				})
			}
		}
		return nil
	})

	return targets, err
}

func TestInterfaceSizeLimits(t *testing.T) {
	repoRoot := "../.."
	start := time.Now()

	targets, err := collectDomainInterfaces(repoRoot)
	if err != nil {
		t.Fatalf("failed to collect domain interfaces: %v", err)
	}

	seenWhitelisted := make(map[string]bool)
	var violations []string

	for _, target := range targets {
		if _, isWhitelisted := whitelistedLegacyInterfaceLimits[target.Key]; isWhitelisted {
			seenWhitelisted[target.Key] = true
		}

		if violation := evaluateInterfaceSize(target.Key, target.DirectMethods, whitelistedLegacyInterfaceLimits); violation != "" {
			relPath, _ := filepath.Rel(repoRoot, target.FilePath)
			violations = append(violations, fmt.Sprintf("%s (%s:%d)", violation, relPath, target.Line))
		}
	}

	for whitelistedKey := range whitelistedLegacyInterfaceLimits {
		if !seenWhitelisted[whitelistedKey] {
			violations = append(violations, fmt.Sprintf("whitelisted interface %s no longer exists; remove obsolete entry from whitelistedLegacyInterfaceLimits", whitelistedKey))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Detected %d interface size violation(s):\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}

	t.Logf("Checked %d domain interfaces in %s (violations: %d, whitelisted: %d)",
		len(targets), time.Since(start), len(violations), len(whitelistedLegacyInterfaceLimits))
}

func TestEvaluateInterfaceSizeLogic(t *testing.T) {
	mockWhitelist := map[string]int{
		"sample.LegacyRepository": 15,
	}

	tests := []struct {
		name          string
		key           string
		directMethods int
		whitelist     map[string]int
		expectError   bool
		errorSubstr   string
	}{
		{
			name:          "valid interface within limit",
			key:           "shop.Repository",
			directMethods: 6,
			whitelist:     mockWhitelist,
			expectError:   false,
		},
		{
			name:          "interface at exact limit",
			key:           "shop.Repository",
			directMethods: 10,
			whitelist:     mockWhitelist,
			expectError:   false,
		},
		{
			name:          "oversized unlisted interface",
			key:           "newmodule.Repository",
			directMethods: 11,
			whitelist:     mockWhitelist,
			expectError:   true,
			errorSubstr:   "exceeding limit of 10",
		},
		{
			name:          "whitelisted interface within baseline",
			key:           "sample.LegacyRepository",
			directMethods: 15,
			whitelist:     mockWhitelist,
			expectError:   false,
		},
		{
			name:          "whitelisted interface grew beyond baseline",
			key:           "sample.LegacyRepository",
			directMethods: 16,
			whitelist:     mockWhitelist,
			expectError:   true,
			errorSubstr:   "ratchet regression",
		},
		{
			name:          "whitelisted interface refactored below 10 should ratchet down",
			key:           "sample.LegacyRepository",
			directMethods: 5,
			whitelist:     mockWhitelist,
			expectError:   true,
			errorSubstr:   "please remove it from whitelistedLegacyInterfaceLimits to ratchet down",
		},
		{
			name:          "whitelisted interface refactored to exactly 10 should ratchet down",
			key:           "sample.LegacyRepository",
			directMethods: 10,
			whitelist:     mockWhitelist,
			expectError:   true,
			errorSubstr:   "please remove it from whitelistedLegacyInterfaceLimits to ratchet down",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := evaluateInterfaceSize(tc.key, tc.directMethods, tc.whitelist)
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

func TestInterfaceSizeEmbeddingsExcluded(t *testing.T) {
	// Synthetic test verifying that embedded interfaces do not count towards direct methods.
	src := `package sample

type Reader interface {
	Read() error
}

type Writer interface {
	Write() error
}

type ReadWriter interface {
	Reader
	Writer
	Flush() error
}
`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		t.Fatalf("failed to parse test source: %v", err)
	}

	for _, decl := range node.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			iface := ts.Type.(*ast.InterfaceType)
			direct := countDirectInterfaceMethods(iface)

			switch ts.Name.Name {
			case "Reader", "Writer":
				if direct != 1 {
					t.Errorf("%s: expected 1 direct method, got %d", ts.Name.Name, direct)
				}
			case "ReadWriter":
				// ReadWriter embeds Reader & Writer, but only declares 1 direct method (Flush).
				if direct != 1 {
					t.Errorf("ReadWriter: expected 1 direct method, got %d", direct)
				}
			}
		}
	}
}

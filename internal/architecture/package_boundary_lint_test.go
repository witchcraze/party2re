package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const internalPrefix = "github.com/witchcraze/party2re/internal/"

// sharedFoundationPackages defines packages that provide universal cross-cutting contracts
// and may be legitimately imported by domain feature packages.
var sharedFoundationPackages = map[string]bool{
	"internal/id":         true, // Centralized cryptographic ID generation
	"internal/pagination": true, // Keyset and page pagination primitives
	"internal/validation": true, // Text and value format validation
	"internal/economy":    true, // TransactionRunner and single-character transaction orchestration
	"internal/logging":    true, // Structured logging with credential masking
	"internal/ratelimit":  true, // Generic rate limiters
	"internal/town":       true, // Pure town value objects
	"internal/depot":      true, // Universal depot item storage and capacity system
	"internal/battle":     true, // Battle Adapter service (ParticipantBuilder and ApplyPostBattleResult)
}

// compositionRoots defines packages responsible for system wiring and infrastructure assembly.
// These packages are permitted to import both domain features and database persistence layers.
var compositionRoots = map[string]bool{
	"internal/api/http":            true,
	"internal/database":            true,
	"internal/database/testutil":   true,
	"internal/architecture":        true,
	"internal/testutil":            true,
	"internal/testutil/valkeytest": true,
}

// permittedCrossFeatureCouplings defines the strictly limited set of documented domain relationships
// between specific feature packages. No new entries may be added without formal architecture review.
var permittedCrossFeatureCouplings = map[string]map[string]bool{
	"internal/party":  {"internal/adventure": true},                      // Multi-player party adventure 10-floor crawl loop
	"internal/boss":   {"internal/party": true},                          // 4-player party recruitment for King sealing battles
	"internal/gvg":    {"internal/guild": true},                          // Guild battle room validation and guild standings
	"internal/god":    {"internal/casino": true, "internal/guild": true}, // God wishes (WishCoin50000 and WishGuildPoint1000)
	"internal/battle": {"internal/custom_skill": true},                   // Battle Adapter equips custom skill gems
}

// getPackagePath extracts the logical package path (e.g. "internal/casino" or "internal/core/character")
// from a clean relative file path.
func getPackagePath(cleanPath string) string {
	parts := strings.Split(cleanPath, "/")
	if len(parts) >= 3 && parts[1] == "core" {
		return "internal/core/" + parts[2]
	}
	if len(parts) >= 3 && parts[1] == "api" {
		return "internal/api/" + parts[2]
	}
	if len(parts) >= 2 {
		return "internal/" + parts[1]
	}
	return ""
}

// getTargetPackage extracts the internal package identifier from an import path.
func getTargetPackage(importPath string) string {
	if !strings.HasPrefix(importPath, internalPrefix) {
		return ""
	}
	rel := strings.TrimPrefix(importPath, "github.com/witchcraze/party2re/")
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && parts[1] == "core" {
		return "internal/core/" + parts[2]
	}
	if len(parts) >= 3 && parts[1] == "api" {
		return "internal/api/" + parts[2]
	}
	if len(parts) >= 2 {
		return "internal/" + parts[1]
	}
	return rel
}

type boundaryViolation struct {
	FilePath string
	Line     int
	Message  string
}

// detectPackageBoundaryViolations inspects a Go source file for package boundary violations:
// 1. Direct imports of internal/database in domain/feature packages.
// 2. Core packages importing feature packages.
// 3. Feature packages importing peer feature packages without explicit authorization.
func detectPackageBoundaryViolations(fset *token.FileSet, filePath string, src any, relPath string) ([]boundaryViolation, error) {
	node, err := parser.ParseFile(fset, filePath, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	cleanRelPath := filepath.ToSlash(relPath)
	if cleanRelPath == "" {
		cleanRelPath = filepath.ToSlash(filePath)
	}

	srcPkg := getPackagePath(cleanRelPath)
	if srcPkg == "" || compositionRoots[srcPkg] {
		return nil, nil
	}

	var violations []boundaryViolation

	for _, imp := range node.Imports {
		rawPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}

		targetPkg := getTargetPackage(rawPath)
		if targetPkg == "" || targetPkg == srcPkg {
			continue
		}

		line := fset.Position(imp.Pos()).Line

		// Rule 1: No direct database imports in domain packages
		if targetPkg == "internal/database" {
			violations = append(violations, boundaryViolation{
				FilePath: cleanRelPath,
				Line:     line,
				Message: fmt.Sprintf("package %s directly imports internal/database (.agents/rules/03-architecture.md §4 & §10); "+
					"persistence must be accessed through domain repository interfaces", srcPkg),
			})
			continue
		}

		// Rule 2: Core cannot import feature packages
		if strings.HasPrefix(srcPkg, "internal/core") {
			if !strings.HasPrefix(targetPkg, "internal/core") && targetPkg != "internal/id" {
				violations = append(violations, boundaryViolation{
					FilePath: cleanRelPath,
					Line:     line,
					Message: fmt.Sprintf("core package %s imports feature package %s (.agents/rules/03-architecture.md §3); "+
						"core must remain small and self-contained", srcPkg, targetPkg),
				})
			}
			continue
		}

		// Rule 3: Cross-feature imports
		// Imports of Core packages and shared foundation packages are always permitted
		if strings.HasPrefix(targetPkg, "internal/core") || sharedFoundationPackages[targetPkg] {
			continue
		}

		// Whitelisted cross-feature coupling
		if permittedCrossFeatureCouplings[srcPkg] != nil && permittedCrossFeatureCouplings[srcPkg][targetPkg] {
			continue
		}

		violations = append(violations, boundaryViolation{
			FilePath: cleanRelPath,
			Line:     line,
			Message: fmt.Sprintf("forbidden cross-feature import: package %s imports peer feature package %s (.agents/rules/03-architecture.md §4); "+
				"features must not directly depend on another feature's private implementation", srcPkg, targetPkg),
		})
	}

	return violations, nil
}

// TestProhibitPackageBoundaryViolations scans all production Go source files under internal/
// and asserts that modular monolith boundaries are strictly maintained.
func TestProhibitPackageBoundaryViolations(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}

	internalDir := filepath.Join(repoRoot, "internal")
	fset := token.NewFileSet()

	var violations []string

	err = filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		cleanPath := filepath.ToSlash(path)
		if strings.Contains(cleanPath, "/testutil/") {
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)
		cleanRelPath := filepath.ToSlash(relPath)

		detected, parseErr := detectPackageBoundaryViolations(fset, path, nil, cleanRelPath)
		if parseErr != nil {
			t.Errorf("failed to parse %s: %v", path, parseErr)
			return nil
		}

		for _, v := range detected {
			violations = append(violations, fmt.Sprintf("%s:%d: %s", v.FilePath, v.Line, v.Message))
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found modular monolith package boundary violations (.agents/rules/03-architecture.md §4):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestProhibitPackageBoundaryViolations_SelfTest verifies the accuracy of package boundary detection.
func TestProhibitPackageBoundaryViolations_SelfTest(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name        string
		filePath    string
		src         string
		wantViolate bool
		wantLine    int
		wantSubstr  string
	}{
		{
			name:     "ValidFeatureImports",
			filePath: "internal/casino/prizes.go",
			src: `package casino
import (
	"context"
	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)`,
			wantViolate: false,
		},
		{
			name:     "InvalidDirectDatabaseImport",
			filePath: "internal/casino/service.go",
			src: `package casino
import (
	"context"
	"github.com/witchcraze/party2re/internal/database"
)`,
			wantViolate: true,
			wantLine:    4,
			wantSubstr:  "directly imports internal/database",
		},
		{
			name:     "InvalidCrossFeatureImport",
			filePath: "internal/casino/game.go",
			src: `package casino
import (
	"context"
	"github.com/witchcraze/party2re/internal/alchemy"
)`,
			wantViolate: true,
			wantLine:    4,
			wantSubstr:  "forbidden cross-feature import",
		},
		{
			name:     "PermittedWhitelistedCrossFeatureImport",
			filePath: "internal/gvg/match.go",
			src: `package gvg
import (
	"context"
	"github.com/witchcraze/party2re/internal/guild"
)`,
			wantViolate: false,
		},
		{
			name:     "InvalidCoreImportingFeature",
			filePath: "internal/core/item/catalog.go",
			src: `package item
import (
	"github.com/witchcraze/party2re/internal/shop"
)`,
			wantViolate: true,
			wantLine:    3,
			wantSubstr:  "core package internal/core/item imports feature package internal/shop",
		},
		{
			name:     "ValidCompositionRootDatabaseImport",
			filePath: "internal/api/http/handler.go",
			src: `package http
import (
	"github.com/witchcraze/party2re/internal/database"
)`,
			wantViolate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := detectPackageBoundaryViolations(fset, tt.filePath, tt.src, tt.filePath)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if !tt.wantViolate {
				if len(violations) != 0 {
					t.Errorf("expected 0 violations, got %v", violations)
				}
				return
			}

			if len(violations) != 1 {
				t.Fatalf("expected 1 violation, got %d (%v)", len(violations), violations)
			}
			if violations[0].Line != tt.wantLine {
				t.Errorf("expected violation on line %d, got %d", tt.wantLine, violations[0].Line)
			}
			if !strings.Contains(violations[0].Message, tt.wantSubstr) {
				t.Errorf("expected message to contain %q, got %q", tt.wantSubstr, violations[0].Message)
			}
		})
	}
}

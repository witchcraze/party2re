package progression_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type progressionViolation struct {
	file    string
	line    int
	field   string
	message string
}

// Whitelisted packages/directories where direct progression mutations or entity mappings are permitted.
var allowedProgressionPaths = map[string]bool{
	"internal/core/progression": true,
	"internal/core/character":   true,
	"internal/core/battle":      true, // battle outcome reward definitions
	"internal/database":         true, // database SQL mappers & row scanning
}

func checkFileProgressionRules(fset *token.FileSet, node *ast.File, filename string) []progressionViolation {
	var violations []progressionViolation

	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.IncDecStmt:
			if sel, ok := stmt.X.(*ast.SelectorExpr); ok {
				if isProgressionField(sel.Sel.Name) {
					pos := fset.Position(stmt.Pos())
					violations = append(violations, progressionViolation{
						file:    filename,
						line:    pos.Line,
						field:   sel.Sel.Name,
						message: "direct mutation of character progression field '" + sel.Sel.Name + "' is prohibited; use progression.ApplyExperience instead",
					})
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					if isProgressionField(sel.Sel.Name) {
						pos := fset.Position(stmt.Pos())
						violations = append(violations, progressionViolation{
							file:    filename,
							line:    pos.Line,
							field:   sel.Sel.Name,
							message: "direct mutation of character progression field '" + sel.Sel.Name + "' is prohibited; use progression.ApplyExperience instead",
						})
					}
				}
			}
		}
		return true
	})

	return violations
}

func isProgressionField(name string) bool {
	return name == "Experience" || name == "Level"
}

func isWhitelistedPath(relPath string) bool {
	// Normalize path with forward slashes
	normalized := filepath.ToSlash(relPath)
	for allowed := range allowedProgressionPaths {
		if strings.HasPrefix(normalized, allowed) {
			return true
		}
	}
	return false
}

// TestProgressionFieldMutationAST scans all feature packages under internal/
// to mechanically ensure that character progression fields (Experience, Level)
// are never mutated directly outside of canonical core helpers.
func TestProgressionFieldMutationAST(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checkedFiles := 0

	err = filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		if isWhitelistedPath(relPath) {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", path, err)
		}

		checkedFiles++
		violations := checkFileProgressionRules(fset, node, relPath)
		for _, v := range violations {
			t.Errorf("[%s:%d] %s", v.file, v.line, v.message)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if checkedFiles == 0 {
		t.Fatalf("no production Go files were checked under %s", internalDir)
	}

	t.Logf("Successfully verified canonical progression usage across %d feature files", checkedFiles)
}

// TestProgressionFieldMutationAST_Detection verifies that the linter correctly
// detects both positive valid constructs and negative prohibited mutations.
func TestProgressionFieldMutationAST_Detection(t *testing.T) {
	fset := token.NewFileSet()

	testCases := []struct {
		name        string
		code        string
		expectError bool
		field       string
	}{
		{
			name: "Valid: using progression.ApplyExperience",
			code: `package feature
import "github.com/witchcraze/party2re/internal/core/progression"
func ApplyReward(c *Character, exp int) error {
	_, err := progression.ApplyExperience(c, exp)
	return err
}`,
			expectError: false,
		},
		{
			name: "Valid: modifying non-progression fields (Money)",
			code: `package feature
func RewardMoney(c *Character, gold int) {
	c.Money += gold
}`,
			expectError: false,
		},
		{
			name: "Violation: direct Experience increment",
			code: `package feature
func BadReward(c *Character, exp int) {
	c.Experience += exp
}`,
			expectError: true,
			field:       "Experience",
		},
		{
			name: "Violation: direct Experience assignment",
			code: `package feature
func BadSetExp(c *Character) {
	c.Experience = 1000
}`,
			expectError: true,
			field:       "Experience",
		},
		{
			name: "Violation: direct Level increment",
			code: `package feature
func BadLevelUp(c *Character) {
	c.Level++
}`,
			expectError: true,
			field:       "Level",
		},
		{
			name: "Violation: direct Level assignment",
			code: `package feature
func BadSetLevel(c *Character) {
	c.Level = 99
}`,
			expectError: true,
			field:       "Level",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := parser.ParseFile(fset, "mock.go", tc.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			violations := checkFileProgressionRules(fset, node, "mock.go")
			if tc.expectError {
				if len(violations) == 0 {
					t.Fatalf("expected violation for %s, got none", tc.field)
				}
				matched := false
				for _, v := range violations {
					if v.field == tc.field && strings.Contains(v.message, "use progression.ApplyExperience instead") {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("expected violation message containing 'use progression.ApplyExperience instead', got: %+v", violations)
				}
			} else {
				if len(violations) > 0 {
					t.Fatalf("expected 0 violations, got: %+v", violations)
				}
			}
		})
	}
}

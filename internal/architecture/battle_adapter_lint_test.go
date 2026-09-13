package architecture_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Allowed packages that may define or orchestrate raw battle participant creation from characters.
var allowedBattleAdapterDirs = map[string]bool{
	"internal/core/battle":  true,
	"internal/battle":       true,
	"internal/architecture": true,
	"internal/testutil":     true,
}

type battleAdapterViolation struct {
	file    string
	line    int
	message string
}

func checkFileBattleAdapterRules(fset *token.FileSet, node *ast.File, filename string) []battleAdapterViolation {
	var violations []battleAdapterViolation

	var battlePkgIdent string
	isDotImport := false
	for _, imp := range node.Imports {
		pathVal := strings.Trim(imp.Path.Value, `"`)
		if pathVal == "github.com/witchcraze/party2re/internal/core/battle" {
			if imp.Name != nil {
				if imp.Name.Name == "." {
					isDotImport = true
				} else {
					battlePkgIdent = imp.Name.Name
				}
			} else {
				battlePkgIdent = "battle"
			}
			break
		}
	}

	isProhibitedFunc := func(name string) bool {
		return name == "NewParticipantFromCharacter" || name == "NewParticipantFromCharacterWithHP"
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Selector call: e.g., corebattle.NewParticipantFromCharacter(...) or builder.FromCharacter(...)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isProhibitedFunc(sel.Sel.Name) {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if (battlePkgIdent != "" && ident.Name == battlePkgIdent) ||
						ident.Name == "corebattle" || ident.Name == "battle" {
						pos := fset.Position(call.Pos())
						violations = append(violations, battleAdapterViolation{
							file:    filename,
							line:    pos.Line,
							message: fmt.Sprintf("direct call to %s.%s is prohibited in feature packages; use battle.ParticipantBuilder (internal/battle) or battle.BuildParticipantFromData to include equipment stats, passives, and combat items (.agents/rules/03-architecture.md §11)", ident.Name, sel.Sel.Name),
						})
						return true
					}
				}
			}

			// Prohibit builder.FromCharacter(...) in non-whitelisted packages when core/battle is imported
			if (battlePkgIdent != "" || isDotImport) && sel.Sel.Name == "FromCharacter" {
				pos := fset.Position(call.Pos())
				violations = append(violations, battleAdapterViolation{
					file:    filename,
					line:    pos.Line,
					message: "direct call to FromCharacter is prohibited in feature packages; use battle.ParticipantBuilder (internal/battle) or battle.BuildParticipantFromData to include equipment stats, passives, and combat items (.agents/rules/03-architecture.md §11)",
				})
				return true
			}
		}

		// Dot import or direct function call: e.g., NewParticipantFromCharacter(...)
		if isDotImport {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				if isProhibitedFunc(ident.Name) {
					pos := fset.Position(call.Pos())
					violations = append(violations, battleAdapterViolation{
						file:    filename,
						line:    pos.Line,
						message: fmt.Sprintf("direct call to %s is prohibited in feature packages; use battle.ParticipantBuilder (internal/battle) or battle.BuildParticipantFromData to include equipment stats, passives, and combat items (.agents/rules/03-architecture.md §11)", ident.Name),
					})
					return true
				}
			}
		}

		return true
	})

	return violations
}

// TestBattleAdapterLint_NoDirectParticipantFromCharacterInFeatures scans all production Go code
// across the repository to ensure no feature package directly invokes corebattle.NewParticipantFromCharacter
// or corebattle.NewParticipantFromCharacterWithHP, enforcing .agents/rules/03-architecture.md §11.
func TestBattleAdapterLint_NoDirectParticipantFromCharacterInFeatures(t *testing.T) {
	repoRoot := "../.."
	fset := token.NewFileSet()
	var allViolations []battleAdapterViolation

	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			if rel == ".git" || rel == "vendor" || rel == ".cache" {
				return filepath.SkipDir
			}
			if allowedBattleAdapterDirs[rel] {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)
		relPath = filepath.ToSlash(relPath)
		for allowed := range allowedBattleAdapterDirs {
			if strings.HasPrefix(relPath, allowed+"/") {
				return nil
			}
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		node, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}

		violations := checkFileBattleAdapterRules(fset, node, relPath)
		allViolations = append(allViolations, violations...)
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk repository directory: %v", err)
	}

	if len(allViolations) > 0 {
		var buf bytes.Buffer
		buf.WriteString("Found direct corebattle participant construction calls in feature packages:\n")
		for _, v := range allViolations {
			buf.WriteString(fmt.Sprintf("%s:%d: %s\n", filepath.ToSlash(v.file), v.line, v.message))
		}
		buf.WriteString("\nAccording to .agents/rules/03-architecture.md §11:\n" +
			"- Combat features MUST route character participant creation through battle.ParticipantBuilder (internal/battle) or battle.BuildParticipantFromData.\n" +
			"- Direct calls to corebattle.NewParticipantFromCharacter, NewParticipantFromCharacterWithHP, or FromCharacter bypass equipment calculation and passives, creating naked combatants.\n")
		t.Errorf("%s", buf.String())
	}
}

// TestBattleAdapterLint_DetectsDirectParticipantConstructionViolation verifies that synthetic code
// directly calling prohibited participant constructors is detected as a violation.
func TestBattleAdapterLint_DetectsDirectParticipantConstructionViolation(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectCount int
	}{
		{
			name: "direct corebattle.NewParticipantFromCharacter call",
			code: `package testpkg
import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) corebattle.Participant {
	return corebattle.NewParticipantFromCharacter(char)
}`,
			expectCount: 1,
		},
		{
			name: "direct corebattle.NewParticipantFromCharacterWithHP call",
			code: `package testpkg
import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) corebattle.Participant {
	return corebattle.NewParticipantFromCharacterWithHP(char, 50)
}`,
			expectCount: 1,
		},
		{
			name: "aliased import direct call",
			code: `package testpkg
import (
	cb "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) cb.Participant {
	return cb.NewParticipantFromCharacter(char)
}`,
			expectCount: 1,
		},
		{
			name: "default package import direct call",
			code: `package testpkg
import (
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) battle.Participant {
	return battle.NewParticipantFromCharacterWithHP(char, 100)
}`,
			expectCount: 1,
		},
		{
			name: "dot import direct call",
			code: `package testpkg
import (
	. "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) Participant {
	return NewParticipantFromCharacter(char)
}`,
			expectCount: 1,
		},
		{
			name: "chained FromCharacter builder call",
			code: `package testpkg
import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)
func TestOp(char corecharacter.Character) (corebattle.Participant, error) {
	return corebattle.NewParticipantBuilder(char.ID).FromCharacter(char).Build()
}`,
			expectCount: 1,
		},
		{
			name: "compliant code using battle.ParticipantBuilder",
			code: `package testpkg
import (
	"context"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)
type Service struct {
	builder battle.ParticipantBuilder
}
func (s *Service) StartCombat(ctx context.Context, charID string) (corebattle.Participant, error) {
	return s.builder.BuildParticipant(ctx, charID)
}`,
			expectCount: 0,
		},
		{
			name: "compliant code constructing monster via MustNewParticipant",
			code: `package testpkg
import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)
func CreateMonster() corebattle.Participant {
	return corebattle.MustNewParticipant("slime", 50, 10, 5)
}`,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, "synthetic.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse synthetic test code: %v", err)
			}

			violations := checkFileBattleAdapterRules(fset, node, "synthetic.go")
			if len(violations) != tt.expectCount {
				t.Errorf("expected %d violation(s), got %d: %+v", tt.expectCount, len(violations), violations)
			}
		})
	}
}

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

// Allowed packages that may define or orchestrate raw database transactions.
var allowedTxInfraDirs = map[string]bool{
	"internal/database":     true,
	"internal/economy":      true,
	"internal/architecture": true,
	"internal/testutil":     true,
}

type txRunnerViolation struct {
	file    string
	line    int
	message string
}

func checkFileTxRunnerRules(fset *token.FileSet, node *ast.File, filename string) []txRunnerViolation {
	var violations []txRunnerViolation

	var dbPkgIdent string
	for _, imp := range node.Imports {
		pathVal := strings.Trim(imp.Path.Value, `"`)
		if pathVal == "github.com/witchcraze/party2re/internal/database" {
			if imp.Name != nil {
				dbPkgIdent = imp.Name.Name
			} else {
				dbPkgIdent = "database"
			}
			break
		}
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "RunInTx" {
			return true
		}

		if ident, ok := sel.X.(*ast.Ident); ok {
			if dbPkgIdent != "" && ident.Name == dbPkgIdent {
				pos := fset.Position(call.Pos())
				violations = append(violations, txRunnerViolation{
					file:    filename,
					line:    pos.Line,
					message: "direct call to database.RunInTx is prohibited in feature packages; route through economy.TransactionRunner or repository methods (.agents/rules/03-architecture.md §10)",
				})
				return true
			}
			if ident.Name == "database" || ident.Name == "db" {
				pos := fset.Position(call.Pos())
				violations = append(violations, txRunnerViolation{
					file:    filename,
					line:    pos.Line,
					message: "direct call to RunInTx on database handle is prohibited in feature packages; route through economy.TransactionRunner (.agents/rules/03-architecture.md §10)",
				})
				return true
			}
		}

		return true
	})

	return violations
}

// TestTxRunnerLint_NoDirectDatabaseRunInTx scans all feature packages in internal/
// to ensure no feature service directly invokes database.RunInTx or raw db transactions,
// enforcing .agents/rules/03-architecture.md §10.
func TestTxRunnerLint_NoDirectDatabaseRunInTx(t *testing.T) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	var allViolations []txRunnerViolation

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			if allowedTxInfraDirs[rel] {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)
		relPath = filepath.ToSlash(relPath)
		for infra := range allowedTxInfraDirs {
			if strings.HasPrefix(relPath, infra+"/") {
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

		violations := checkFileTxRunnerRules(fset, node, relPath)
		allViolations = append(allViolations, violations...)
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if len(allViolations) > 0 {
		var buf bytes.Buffer
		buf.WriteString("Found direct database transaction calls in feature packages:\n")
		for _, v := range allViolations {
			buf.WriteString(fmt.Sprintf("%s:%d: %s\n", filepath.ToSlash(v.file), v.line, v.message))
		}
		buf.WriteString("\nAccording to .agents/rules/03-architecture.md §10, feature operations mutating currencies,\n" +
			"inventories, or multi-resource domain states MUST route through economy.TransactionRunner (ExecuteTransaction / economy.Run[T]).\n")
		t.Errorf("%s", buf.String())
	}
}

// TestTxRunnerLint_DetectsDirectDatabaseRunInTxViolation verifies that synthetic code
// directly calling database.RunInTx or aliased database.RunInTx is detected as a violation.
func TestTxRunnerLint_DetectsDirectDatabaseRunInTxViolation(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectCount int
	}{
		{
			name: "direct database.RunInTx call",
			code: `package testpkg
import (
	"context"
	"database/sql"
	"github.com/witchcraze/party2re/internal/database"
)
func TestOperation(ctx context.Context, db *sql.DB) error {
	return database.RunInTx(ctx, db, func(txCtx context.Context) error {
		return nil
	})
}`,
			expectCount: 1,
		},
		{
			name: "aliased database.RunInTx call",
			code: `package testpkg
import (
	"context"
	"database/sql"
	partydb "github.com/witchcraze/party2re/internal/database"
)
func TestOperation(ctx context.Context, db *sql.DB) error {
	return partydb.RunInTx(ctx, db, func(txCtx context.Context) error {
		return nil
	})
}`,
			expectCount: 1,
		},
		{
			name: "direct call on identifier named db",
			code: `package testpkg
import "context"
type dbHandle interface {
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}
func TestOperation(ctx context.Context, db dbHandle) error {
	return db.RunInTx(ctx, func(context.Context) error {
		return nil
	})
}`,
			expectCount: 1,
		},
		{
			name: "compliant code without database.RunInTx",
			code: `package testpkg
import "context"
type Service struct{}
func (s *Service) DoSomething(ctx context.Context) error {
	return nil
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

			violations := checkFileTxRunnerRules(fset, node, "synthetic.go")
			if len(violations) != tt.expectCount {
				t.Errorf("expected %d violation(s), got %d: %+v", tt.expectCount, len(violations), violations)
			}
		})
	}
}

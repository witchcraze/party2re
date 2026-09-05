package database_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Allowed infrastructure files in internal/database that can define transaction infrastructure.
var allowedInfraFiles = map[string]bool{
	"database.go": true,
	"tx.go":       true,
	"testing.go":  true,
}

// Banned direct SQL execution methods on raw db handles (e.g. r.db.ExecContext).
var bannedDirectDBMethods = map[string]bool{
	"Exec":            true,
	"ExecContext":     true,
	"Query":           true,
	"QueryContext":    true,
	"QueryRow":        true,
	"QueryRowContext": true,
	"Begin":           true,
	"BeginTx":         true,
}

type txViolation struct {
	file    string
	line    int
	message string
}

func checkFileTxRules(fset *token.FileSet, node *ast.File, filename string) []txViolation {
	var violations []txViolation

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name

		// Check Rule 1: Banned Begin / BeginTx calls in repository code
		if methodName == "Begin" || methodName == "BeginTx" {
			pos := fset.Position(call.Pos())
			violations = append(violations, txViolation{
				file:    filename,
				line:    pos.Line,
				message: "direct Begin/BeginTx call is banned in database repositories; use database.RunInTx or ambient transactions",
			})
			return true
		}

		// Check Rule 2: Direct unmanaged queries on r.db (e.g. r.db.ExecContext, repo.db.QueryContext)
		// which bypass ExecutorFromContext(ctx, r.db).
		if bannedDirectDBMethods[methodName] {
			if innerSel, ok := sel.X.(*ast.SelectorExpr); ok {
				if innerSel.Sel.Name == "db" {
					pos := fset.Position(call.Pos())
					violations = append(violations, txViolation{
						file:    filename,
						line:    pos.Line,
						message: "direct r.db." + methodName + " call bypasses ambient transaction context; use ExecutorFromContext(ctx, r.db)." + methodName,
					})
					return true
				}
			}
		}

		return true
	})

	return violations
}

func hasTxKeywords(src []byte) bool {
	return bytes.Contains(src, []byte("Begin")) ||
		bytes.Contains(src, []byte("Exec")) ||
		bytes.Contains(src, []byte("Query"))
}

func TestAmbientTransactionPropagationAST(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read database directory: %v", err)
	}

	fset := token.NewFileSet()
	checkedFiles := 0
	parsedFiles := 0
	start := time.Now()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		if allowedInfraFiles[entry.Name()] {
			continue
		}

		checkedFiles++

		filePath := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", filePath, err)
		}

		// Fast path: files without SQL execution or transaction method candidates cannot contain violations
		if !hasTxKeywords(src) {
			continue
		}

		parsedFiles++
		node, err := parser.ParseFile(fset, filePath, src, parser.AllErrors)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", filePath, err)
		}

		violations := checkFileTxRules(fset, node, entry.Name())
		for _, v := range violations {
			t.Errorf("[%s:%d] %s", v.file, v.line, v.message)
		}
	}

	if checkedFiles == 0 {
		t.Fatalf("no repository files were checked in %s", dir)
	}
	t.Logf("Successfully verified ambient transaction propagation rules across %d repository files (%d parsed) in %s", checkedFiles, parsedFiles, time.Since(start))
}

func TestAmbientTransactionPropagationAST_ViolationDetection(t *testing.T) {
	fset := token.NewFileSet()

	testCases := []struct {
		name        string
		code        string
		expectError bool
		errSnippet  string
	}{
		{
			name: "Valid repository method using ExecutorFromContext",
			code: `package database
import "context"
func (r *Repo) Find(ctx context.Context, id string) error {
	return ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, "SELECT 1").Scan()
}`,
			expectError: false,
		},
		{
			name: "Violation: direct r.db.ExecContext call",
			code: `package database
import "context"
func (r *Repo) Update(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE foo SET bar = 1")
	return err
}`,
			expectError: true,
			errSnippet:  "direct r.db.ExecContext call bypasses ambient transaction context",
		},
		{
			name: "Violation: direct r.db.QueryContext call",
			code: `package database
import "context"
func (r *Repo) List(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM foo")
	return err
}`,
			expectError: true,
			errSnippet:  "direct r.db.QueryContext call bypasses ambient transaction context",
		},
		{
			name: "Violation: direct r.db.QueryRowContext call",
			code: `package database
import "context"
func (r *Repo) Get(ctx context.Context) error {
	return r.db.QueryRowContext(ctx, "SELECT 1").Scan()
}`,
			expectError: true,
			errSnippet:  "direct r.db.QueryRowContext call bypasses ambient transaction context",
		},
		{
			name: "Violation: raw BeginTx call",
			code: `package database
import "context"
func (r *Repo) DoWork(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	return err
}`,
			expectError: true,
			errSnippet:  "direct Begin/BeginTx call is banned",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := parser.ParseFile(fset, "dummy.go", tc.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			violations := checkFileTxRules(fset, node, "dummy.go")
			if tc.expectError {
				if len(violations) == 0 {
					t.Fatalf("expected violation, got none")
				}
				matched := false
				for _, v := range violations {
					if strings.Contains(v.message, tc.errSnippet) {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("expected violation containing %q, got: %+v", tc.errSnippet, violations)
				}
			} else {
				if len(violations) > 0 {
					t.Fatalf("expected 0 violations, got: %+v", violations)
				}
			}
		})
	}
}

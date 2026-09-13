package http_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type rootContextViolation struct {
	line   int
	fnName string
}

// detectRootContextInSource parses Go source content and returns violations where
// context.Background() or context.TODO() is called.
func detectRootContextInSource(fset *token.FileSet, filename string, src any) ([]rootContextViolation, error) {
	node, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}

	var contextPkgIdent string
	isDotImport := false

	for _, imp := range node.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if importPath == "context" {
			if imp.Name != nil {
				if imp.Name.Name == "." {
					isDotImport = true
				} else {
					contextPkgIdent = imp.Name.Name
				}
			} else {
				contextPkgIdent = "context"
			}
			break
		}
	}

	// If "context" package is not imported, context.Background/TODO cannot be called.
	if contextPkgIdent == "" && !isDotImport {
		return nil, nil
	}

	var violations []rootContextViolation

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isDotImport {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				if ident.Name == "Background" || ident.Name == "TODO" {
					violations = append(violations, rootContextViolation{
						line:   fset.Position(call.Pos()).Line,
						fnName: ident.Name + "()",
					})
					return true
				}
			}
		}

		if contextPkgIdent != "" {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Background" || sel.Sel.Name == "TODO" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == contextPkgIdent {
						violations = append(violations, rootContextViolation{
							line:   fset.Position(call.Pos()).Line,
							fnName: fmt.Sprintf("%s.%s()", contextPkgIdent, sel.Sel.Name),
						})
					}
				}
			}
		}

		return true
	})

	return violations, nil
}

// TestProhibitRootContextInHTTPHandlers scans all production Go source files in internal/api/http/
// and asserts that none call context.Background() or context.TODO().
// HTTP handlers and middlewares must always observe r.Context() or pass incoming ctx to preserve
// cancellation, deadlines, authentication session identities, and ambient transaction scopes.
func TestProhibitRootContextInHTTPHandlers(t *testing.T) {
	httpDir := "."
	fset := token.NewFileSet()

	entries, err := os.ReadDir(httpDir)
	if err != nil {
		t.Fatalf("failed to read http directory: %v", err)
	}

	var violationMsgs []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(httpDir, entry.Name())
		violations, parseErr := detectRootContextInSource(fset, filePath, nil)
		if parseErr != nil {
			t.Errorf("failed to parse %s: %v", filePath, parseErr)
			continue
		}

		for _, v := range violations {
			violationMsgs = append(violationMsgs, fmt.Sprintf("%s:%d (%s)", entry.Name(), v.line, v.fnName))
		}
	}

	if len(violationMsgs) > 0 {
		t.Errorf("found forbidden root context invocation in HTTP presentation layer:\n  %s\n\n"+
			"HTTP handlers and middlewares must never instantiate detached root contexts.\n"+
			"Detached contexts discard client disconnect cancellations, request deadlines, auth session metadata, and ambient transaction scopes.\n"+
			"Use r.Context() or propagate the incoming ctx instead.\n"+
			"See .agents/rules/03-architecture.md Section 5.",
			strings.Join(violationMsgs, "\n  "))
	}
}

// TestProhibitRootContextInHTTPHandlers_SelfTest verifies that the linter accurately detects
// context.Background() and context.TODO() calls across various import formats.
func TestProhibitRootContextInHTTPHandlers_SelfTest(t *testing.T) {
	fset := token.NewFileSet()

	validSource := `package foo
import (
	"context"
	"net/http"
)
func handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = ctx
}
`
	violations, err := detectRootContextInSource(fset, "valid.go", validSource)
	if err != nil {
		t.Fatalf("unexpected error parsing valid source: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for valid source, got %v", violations)
	}

	invalidBackgroundSource := `package bar
import (
	"context"
	"net/http"
)
func handle(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	_ = ctx
}
`
	violations, err = detectRootContextInSource(fset, "invalid_bg.go", invalidBackgroundSource)
	if err != nil {
		t.Fatalf("unexpected error parsing invalid source: %v", err)
	}
	if len(violations) != 1 || violations[0].line != 7 || violations[0].fnName != "context.Background()" {
		t.Errorf("expected violation on line 7 for context.Background(), got %v", violations)
	}

	invalidTODOSource := `package baz
import (
	"context"
	"net/http"
)
func handle(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO()
	_ = ctx
}
`
	violations, err = detectRootContextInSource(fset, "invalid_todo.go", invalidTODOSource)
	if err != nil {
		t.Fatalf("unexpected error parsing invalid source: %v", err)
	}
	if len(violations) != 1 || violations[0].line != 7 || violations[0].fnName != "context.TODO()" {
		t.Errorf("expected violation on line 7 for context.TODO(), got %v", violations)
	}

	invalidAliasedSource := `package qux
import (
	c "context"
	"net/http"
)
func handle(w http.ResponseWriter, r *http.Request) {
	ctx := c.Background()
	_ = ctx
}
`
	violations, err = detectRootContextInSource(fset, "invalid_alias.go", invalidAliasedSource)
	if err != nil {
		t.Fatalf("unexpected error parsing aliased source: %v", err)
	}
	if len(violations) != 1 || violations[0].line != 7 || violations[0].fnName != "c.Background()" {
		t.Errorf("expected violation on line 7 for c.Background(), got %v", violations)
	}

	invalidDotImportSource := `package quux
import (
	. "context"
	"net/http"
)
func handle(w http.ResponseWriter, r *http.Request) {
	ctx := Background()
	_ = ctx
}
`
	violations, err = detectRootContextInSource(fset, "invalid_dot.go", invalidDotImportSource)
	if err != nil {
		t.Fatalf("unexpected error parsing dot import source: %v", err)
	}
	if len(violations) != 1 || violations[0].line != 7 || violations[0].fnName != "Background()" {
		t.Errorf("expected violation on line 7 for Background(), got %v", violations)
	}

	validCustomCallSource := `package corge
type customCtx struct{}
func (c customCtx) Background() string { return "" }
func handle() {
	var c customCtx
	_ = c.Background()
}
`
	violations, err = detectRootContextInSource(fset, "valid_custom.go", validCustomCallSource)
	if err != nil {
		t.Fatalf("unexpected error parsing custom call source: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for custom call source, got %v", violations)
	}
}

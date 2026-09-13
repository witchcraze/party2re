package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// detectTimeSleepInSource parses Go source content and returns line numbers where raw time.Sleep is called.
func detectTimeSleepInSource(fset *token.FileSet, filename string, src any) ([]int, error) {
	node, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}

	var timePkgIdent string
	isDotImport := false

	for _, imp := range node.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if importPath == "time" {
			if imp.Name != nil {
				if imp.Name.Name == "." {
					isDotImport = true
				} else {
					timePkgIdent = imp.Name.Name
				}
			} else {
				timePkgIdent = "time"
			}
			break
		}
	}

	// If "time" package is not imported, time.Sleep cannot be called.
	if timePkgIdent == "" && !isDotImport {
		return nil, nil
	}

	var lines []int

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isDotImport {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "Sleep" {
				lines = append(lines, fset.Position(call.Pos()).Line)
				return true
			}
		}

		if timePkgIdent != "" {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Sleep" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == timePkgIdent {
						lines = append(lines, fset.Position(call.Pos()).Line)
					}
				}
			}
		}

		return true
	})

	return lines, nil
}

// TestProhibitRawTimeSleep scans all production Go source files under internal/ and cmd/
// and asserts that none call raw time.Sleep.
// Backend services must support cooperative cancellation via context.Context.
func TestProhibitRawTimeSleep(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string

	targetDirs := []string{
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd"),
	}

	for _, dir := range targetDirs {
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Skip non-Go files and test files
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			cleanPath := filepath.ToSlash(path)

			// Skip test utility directories
			if strings.Contains(cleanPath, "/testutil/") || strings.Contains(cleanPath, "/database/testutil/") {
				return nil
			}

			lines, parseErr := detectTimeSleepInSource(fset, path, nil)
			if parseErr != nil {
				t.Errorf("failed to parse %s: %v", path, parseErr)
				return nil
			}

			for _, line := range lines {
				relPath, _ := filepath.Rel(repoRoot, path)
				violations = append(violations, relPath+":"+strconv.Itoa(line))
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk directory %s: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		t.Errorf("found raw time.Sleep calls in production packages (must use context-aware delays with ctx.Done()):\n  %s\n\n"+
			"Cooperative cancellation via context.Context is required to prevent blocking graceful shutdown and leaking goroutines.\n"+
			"Use select with ctx.Done() and time.After(d) or time.NewTicker / time.NewTimer instead.\n"+
			"See .agents/rules/03-architecture.md Section 12.",
			strings.Join(violations, "\n  "))
	}
}

// TestProhibitRawTimeSleep_SelfTest verifies that the linter accurately detects raw time.Sleep calls.
func TestProhibitRawTimeSleep_SelfTest(t *testing.T) {
	fset := token.NewFileSet()

	validSource := `package foo
import (
	"context"
	"time"
)
func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
`
	lines, err := detectTimeSleepInSource(fset, "valid.go", validSource)
	if err != nil {
		t.Fatalf("unexpected error parsing valid source: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 violations for valid source, got %v", lines)
	}

	invalidSource := `package bar
import "time"
func sleep() {
	time.Sleep(10 * time.Millisecond)
}
`
	lines, err = detectTimeSleepInSource(fset, "invalid.go", invalidSource)
	if err != nil {
		t.Fatalf("unexpected error parsing invalid source: %v", err)
	}
	if len(lines) != 1 || lines[0] != 4 {
		t.Errorf("expected violation on line 4, got %v", lines)
	}

	invalidAliasedSource := `package baz
import t "time"
func sleep() {
	t.Sleep(5 * time.Millisecond)
}
`
	lines, err = detectTimeSleepInSource(fset, "invalid_aliased.go", invalidAliasedSource)
	if err != nil {
		t.Fatalf("unexpected error parsing aliased source: %v", err)
	}
	if len(lines) != 1 || lines[0] != 4 {
		t.Errorf("expected violation on line 4, got %v", lines)
	}

	invalidDotImportSource := `package qux
import . "time"
func sleep() {
	Sleep(5 * time.Millisecond)
}
`
	lines, err = detectTimeSleepInSource(fset, "invalid_dot.go", invalidDotImportSource)
	if err != nil {
		t.Fatalf("unexpected error parsing dot import source: %v", err)
	}
	if len(lines) != 1 || lines[0] != 4 {
		t.Errorf("expected violation on line 4, got %v", lines)
	}

	validCustomSleepSource := `package custom
type Sleeper struct{}
func (s Sleeper) Sleep(d int) {}
func doSleep(s Sleeper) {
	s.Sleep(10)
}
`
	lines, err = detectTimeSleepInSource(fset, "custom_sleep.go", validCustomSleepSource)
	if err != nil {
		t.Fatalf("unexpected error parsing custom sleep source: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 violations for custom sleep source, got %v", lines)
	}
}

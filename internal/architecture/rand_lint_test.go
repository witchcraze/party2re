package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// detectMathRandInSource parses Go source content and returns line numbers where "math/rand" is imported.
func detectMathRandInSource(fset *token.FileSet, filename string, src any) ([]int, error) {
	node, err := parser.ParseFile(fset, filename, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var lines []int
	for _, imp := range node.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if importPath == "math/rand" {
			lines = append(lines, fset.Position(imp.Pos()).Line)
		}
	}
	return lines, nil
}

// TestProhibitDirectMathRand scans all production Go source files under internal/
// and asserts that none directly import legacy "math/rand".
// Production code must instead import the centralized, concurrency-safe RNG package:
// "github.com/witchcraze/party2re/internal/core/random".
func TestProhibitDirectMathRand(t *testing.T) {
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
		// Skip non-Go files and test files
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		cleanPath := filepath.ToSlash(path)

		// Skip test utility directories
		if strings.Contains(cleanPath, "/testutil/") || strings.Contains(cleanPath, "/database/testutil/") {
			return nil
		}

		// The random implementation itself is exempt (wraps math/rand/v2)
		if strings.Contains(cleanPath, "internal/core/random/") {
			return nil
		}

		lines, parseErr := detectMathRandInSource(fset, path, nil)
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
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found direct imports of legacy \"math/rand\" in production packages (use \"github.com/witchcraze/party2re/internal/core/random\" instead):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestProhibitDirectMathRand_SelfTest verifies that the linter accurately detects "math/rand" imports.
func TestProhibitDirectMathRand_SelfTest(t *testing.T) {
	fset := token.NewFileSet()

	validSource := `package foo
import (
	"fmt"
	"github.com/witchcraze/party2re/internal/core/random"
)
`
	lines, err := detectMathRandInSource(fset, "valid.go", validSource)
	if err != nil {
		t.Fatalf("unexpected error parsing valid source: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 violations for valid source, got %v", lines)
	}

	invalidSource := `package bar
import (
	"fmt"
	"math/rand"
)
`
	lines, err = detectMathRandInSource(fset, "invalid.go", invalidSource)
	if err != nil {
		t.Fatalf("unexpected error parsing invalid source: %v", err)
	}
	if len(lines) != 1 || lines[0] != 4 {
		t.Errorf("expected violation on line 4, got %v", lines)
	}
}

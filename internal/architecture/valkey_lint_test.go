package architecture_test

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

// Registered production Valkey namespaces
var validProdNamespaces = []string{
	"party2:session:",
	"party2:player:",
	"party2:maintenance:",
	"party2:scheduled:",
	"party2:ratelimit:",
	"party2:ranking:",
	"party2:party:",
}

// Required keys documented in SSOT docs/architecture/valkey-keyspace.md
var requiredDocumentedKeys = []string{
	"party2:session:",
	"party2:player:sessions:",
	"party2:maintenance:status",
	"party2:scheduled:pending",
	"party2:scheduled:action:",
	"party2:scheduled:lock:",
	"party2:scheduled:actor:",
	"party2:ratelimit:",
	"party2:ranking:snapshot:",
	"party2:ranking:refresh",
	"party2:party:lobby:",
	"party2:party:lobbies",
	"party2:party:character:",
	"party2:party:ready:",
}

func TestValkeyKeyspaceDocExistsAndCoversKeys(t *testing.T) {
	repoRoot := "../.."
	docPath := filepath.Join(repoRoot, "docs", "architecture", "valkey-keyspace.md")

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("docs/architecture/valkey-keyspace.md does not exist: %v", err)
	}

	content := string(data)
	if len(strings.TrimSpace(content)) == 0 {
		t.Fatalf("docs/architecture/valkey-keyspace.md is empty")
	}

	for _, key := range requiredDocumentedKeys {
		if !strings.Contains(content, key) {
			t.Errorf("docs/architecture/valkey-keyspace.md does not document required key pattern %q", key)
		}
	}
}

func TestValkeyNoBannedKeysCommandInProd(t *testing.T) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checkedFiles := 0
	parsedFiles := 0
	start := time.Now()

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		checkedFiles++

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Fast path: files without "Keys" cannot invoke banned .Keys() calls
		if !bytes.Contains(src, []byte("Keys")) {
			return nil
		}

		parsedFiles++
		node, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			t.Fatalf("failed to parse %s: %v", path, parseErr)
		}

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check for .Keys() calls on Valkey builder
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Keys" {
					pos := fset.Position(call.Pos())
					t.Errorf("banned Valkey KEYS command call detected at %s:%d (use Set indexing instead of KEYS *)", path, pos.Line)
				}
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	t.Logf("Valkey banned KEYS linter verified %d production files (%d parsed) in %s", checkedFiles, parsedFiles, time.Since(start))
}

func TestValkeyKeyPrefixTaxonomy(t *testing.T) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checkedFiles := 0
	parsedFiles := 0
	start := time.Now()

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		checkedFiles++
		isTestFile := strings.HasSuffix(path, "_test.go")

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Fast path: files without "party2:" cannot contain Valkey keyspace string literals
		if !bytes.Contains(src, []byte("party2:")) {
			return nil
		}

		parsedFiles++
		node, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			t.Fatalf("failed to parse %s: %v", path, parseErr)
		}

		ast.Inspect(node, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			val := strings.Trim(lit.Value, "\"")
			if !strings.HasPrefix(val, "party2:") || val == "party2:" {
				return true
			}

			// Ignore MariaDB DSN strings like "party2:party2@tcp..."
			if strings.Contains(val, "@tcp(") {
				return true
			}

			// Test files may use "party2:test:"
			if isTestFile && strings.HasPrefix(val, "party2:test:") {
				return true
			}

			// Verify against registered production namespaces
			matched := false
			for _, prefix := range validProdNamespaces {
				if strings.HasPrefix(val, prefix) {
					matched = true
					break
				}
			}

			if !matched {
				pos := fset.Position(lit.Pos())
				t.Errorf("unregistered Valkey key prefix %q at %s:%d (must conform to taxonomy in docs/architecture/valkey-keyspace.md)", val, path, pos.Line)
			}

			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	t.Logf("Valkey key taxonomy linter verified %d Go files (%d parsed) in %s", checkedFiles, parsedFiles, time.Since(start))
}

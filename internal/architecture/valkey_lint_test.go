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
	"party2:dungeon:",
	"party2:challenge:",
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

func TestValkeyNoBannedCommandsInLua(t *testing.T) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checkedFiles := 0
	parsedFiles := 0
	start := time.Now()

	bannedLuaCommands := []string{"KEYS", "SCAN", "FLUSHDB", "FLUSHALL"}

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		checkedFiles++

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Fast path: files without "redis.call" cannot contain Valkey Lua command executions
		if !bytes.Contains(src, []byte("redis.call")) {
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

			strVal := lit.Value
			if !strings.Contains(strVal, "redis.call") {
				return true
			}

			upper := strings.ToUpper(strVal)
			for _, cmd := range bannedLuaCommands {
				// Detect patterns like redis.call('KEYS' or redis.call("KEYS" or redis.call('SCAN'
				if strings.Contains(upper, "REDIS.CALL('"+cmd+"'") ||
					strings.Contains(upper, "REDIS.CALL(\""+cmd+"\"") ||
					strings.Contains(upper, "REDIS.CALL('"+cmd+" ") ||
					strings.Contains(upper, "REDIS.CALL(\""+cmd+" ") {
					pos := fset.Position(lit.Pos())
					t.Errorf("banned Valkey command %s in Lua script detected at %s:%d (prohibited by .agents/rules/05-database-and-caching.md Section 3.5)", cmd, path, pos.Line)
				}
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	t.Logf("Valkey Lua banned command linter verified %d files (%d parsed) in %s", checkedFiles, parsedFiles, time.Since(start))
}

func TestValkeyKeyspaceDocCoversLuaAndHashTags(t *testing.T) {
	repoRoot := "../.."
	docPath := filepath.Join(repoRoot, "docs", "architecture", "valkey-keyspace.md")

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("docs/architecture/valkey-keyspace.md does not exist: %v", err)
	}

	content := string(data)

	requiredSectionsAndTerms := []string{
		"Hash Tag",
		"Lua Script Registry",
		"party_add_member",
		"party_remove_member",
		"party_update_member_ready",
		"party_update_party",
		"dungeon_step",
		"challenge_advance_round",
	}

	for _, term := range requiredSectionsAndTerms {
		if !strings.Contains(content, term) {
			t.Errorf("docs/architecture/valkey-keyspace.md does not contain required Lua/HashTag specification term %q", term)
		}
	}
}

func TestValkeyKeyspaceDocCoversTTLScoredZSetPattern(t *testing.T) {
	repoRoot := "../.."
	docPath := filepath.Join(repoRoot, "docs", "architecture", "valkey-keyspace.md")

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("docs/architecture/valkey-keyspace.md does not exist: %v", err)
	}

	content := string(data)

	requiredTerms := []string{
		"TTL-Scored Sorted Set with Lazy Purging",
		"ZREMRANGEBYSCORE",
		"WRONGTYPE",
		"Candidate D: In-Progress Run Buffers",
	}

	for _, term := range requiredTerms {
		if !strings.Contains(content, term) {
			t.Errorf("docs/architecture/valkey-keyspace.md does not contain required TTL-scored ZSet pattern term %q", term)
		}
	}
}

func TestTransientRunStateDocCoversLuaAndPersistenceBoundaries(t *testing.T) {
	repoRoot := "../.."
	docPath := filepath.Join(repoRoot, "docs", "architecture", "transient-run-state.md")

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("docs/architecture/transient-run-state.md does not exist: %v", err)
	}

	content := string(data)
	if len(strings.TrimSpace(content)) == 0 {
		t.Fatalf("docs/architecture/transient-run-state.md is empty")
	}

	requiredTerms := []string{
		"Candidate D",
		"dungeon_step",
		"challenge_advance_round",
		"Cluster Hash Tag",
		"Two-Phase Settlement",
		"Crash Recovery",
		"In-Memory Fallback Parity",
		"party2:dungeon:{char:<character_id>}:state",
		"party2:challenge:{char:<character_id>}:session",
	}

	for _, term := range requiredTerms {
		if !strings.Contains(content, term) {
			t.Errorf("docs/architecture/transient-run-state.md does not contain required architectural term %q", term)
		}
	}
}

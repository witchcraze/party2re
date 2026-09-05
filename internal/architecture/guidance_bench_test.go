package architecture_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkGuidanceLayerLinter(b *testing.B) {
	repoRoot := "../.."
	modulesDir := filepath.Join(repoRoot, ".arch", "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		b.Fatalf("failed to read .arch/modules: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			filePath := filepath.Join(modulesDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				b.Fatalf("failed to read %s: %v", filePath, err)
			}

			var doc ModuleDoc
			if err := json.Unmarshal(data, &doc); err != nil {
				b.Fatalf("invalid json in %s: %v", filePath, err)
			}

			for _, dep := range doc.Dependencies {
				if dep.SourceRef != "" {
					parts := strings.Split(dep.SourceRef, "#")
					if len(parts) == 2 {
						targetFullPath := filepath.Join(repoRoot, parts[0])
						src, err := os.ReadFile(targetFullPath)
						if err == nil && bytes.Contains(src, []byte(strings.Split(parts[1], ".")[len(strings.Split(parts[1], "."))-1])) {
							_, _ = parseArchitectureFileCached(targetFullPath, src)
						}
					}
				}
			}
		}
	}
}

func BenchmarkValkeyLinter(b *testing.B) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fset := token.NewFileSet()
		_ = filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			src, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			if !bytes.Contains(src, []byte("Keys")) {
				return nil
			}

			node, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				return nil
			}

			ast.Inspect(node, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						_ = sel.Sel.Name == "Keys"
					}
				}
				return true
			})
			return nil
		})
	}
}

package architecture_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type ModuleDoc struct {
	Module           string       `json:"module"`
	Entrypoint       string       `json:"entrypoint"`
	Dependencies     []Dependency `json:"dependencies"`
	TransactionFlows []TxFlow     `json:"transaction_flows"`
}

type Dependency struct {
	Module    string `json:"module"`
	SourceRef string `json:"source_ref"`
}

type TxFlow struct {
	Flow      string `json:"flow"`
	SourceRef string `json:"source_ref"`
}

func TestArchitectureGuidanceSymbols(t *testing.T) {
	// Locate repository root
	repoRoot := "../.."
	modulesDir := filepath.Join(repoRoot, ".arch", "modules")

	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip(".arch/modules does not exist")
		}
		t.Fatalf("failed to read .arch/modules: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(modulesDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", filePath, err)
		}

		var doc ModuleDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("invalid json in %s: %v", filePath, err)
		}

		t.Run(entry.Name(), func(t *testing.T) {
			// Collect all source_ref symbols
			var refs []string
			for _, dep := range doc.Dependencies {
				if dep.SourceRef != "" {
					refs = append(refs, dep.SourceRef)
				}
			}
			for _, flow := range doc.TransactionFlows {
				if flow.SourceRef != "" {
					refs = append(refs, flow.SourceRef)
				}
			}

			for _, ref := range refs {
				parts := strings.Split(ref, "#")
				if len(parts) != 2 {
					t.Errorf("invalid source_ref format %q (expected file.go#Symbol)", ref)
					continue
				}

				targetRelPath := parts[0]
				targetSymbol := parts[1]
				targetFullPath := filepath.Join(repoRoot, targetRelPath)

				verifySymbolExists(t, targetFullPath, targetSymbol)
			}
		})
	}
}

func verifySymbolExists(t *testing.T, fullPath, symbol string) {
	t.Helper()

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fullPath, nil, 0)
	if err != nil {
		t.Errorf("failed to parse Go file %s: %v", fullPath, err)
		return
	}

	// Target can be a Type (e.g. CharacterRepository) or Struct.Method (e.g. Service.OrderMeal)
	symbolParts := strings.Split(symbol, ".")
	var targetReceiver string
	var targetName string

	if len(symbolParts) == 2 {
		targetReceiver = symbolParts[0]
		targetName = symbolParts[1]
	} else {
		targetName = symbolParts[0]
	}

	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.TypeSpec:
			// Match interface or struct name
			if targetReceiver == "" && decl.Name.Name == targetName {
				found = true
				return false
			}
		case *ast.FuncDecl:
			if decl.Name.Name == targetName {
				if targetReceiver == "" {
					// Standalone function
					if decl.Recv == nil {
						found = true
						return false
					}
				} else if decl.Recv != nil && len(decl.Recv.List) > 0 {
					// Method with receiver (e.g. *Service or Service)
					recvType := decl.Recv.List[0].Type
					if star, ok := recvType.(*ast.StarExpr); ok {
						if ident, ok := star.X.(*ast.Ident); ok && ident.Name == targetReceiver {
							found = true
							return false
						}
					} else if ident, ok := recvType.(*ast.Ident); ok && ident.Name == targetReceiver {
						found = true
						return false
					}
				}
			}
		}
		return true
	})

	if !found {
		t.Errorf("symbol %q not found in %s", symbol, fullPath)
	}
}

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
	Module                string                `json:"module"`
	Entrypoint            string                `json:"entrypoint"`
	Dependencies          []Dependency          `json:"dependencies"`
	TransactionFlows      []TxFlow              `json:"transaction_flows"`
	TransactionBoundaries []TransactionBoundary `json:"transaction_boundaries"`
}

type Dependency struct {
	Module    string `json:"module"`
	SourceRef string `json:"source_ref"`
}

type TxFlow struct {
	Flow      string `json:"flow"`
	TxMode    string `json:"tx_mode"`
	SourceRef string `json:"source_ref"`
}

type TransactionBoundary struct {
	Method          string `json:"method"`
	SourceRef       string `json:"source_ref"`
	TransactionType string `json:"transaction_type"`
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
			for _, boundary := range doc.TransactionBoundaries {
				if boundary.SourceRef != "" {
					refs = append(refs, boundary.SourceRef)
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

			// Verify transaction boundary RunInTx presence
			for _, boundary := range doc.TransactionBoundaries {
				if boundary.TransactionType == "RunInTx" && boundary.SourceRef != "" {
					verifyRunInTxBoundary(t, repoRoot, boundary.SourceRef)
				}
			}
			for _, flow := range doc.TransactionFlows {
				if flow.TxMode == "RunInTx" && flow.SourceRef != "" {
					verifyRunInTxBoundary(t, repoRoot, flow.SourceRef)
				}
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

func verifyRunInTxBoundary(t *testing.T, repoRoot, sourceRef string) {
	t.Helper()

	parts := strings.Split(sourceRef, "#")
	if len(parts) != 2 {
		t.Errorf("invalid source_ref format %q (expected file.go#Symbol)", sourceRef)
		return
	}

	targetRelPath := parts[0]
	targetSymbol := parts[1]
	fullPath := filepath.Join(repoRoot, targetRelPath)

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fullPath, nil, 0)
	if err != nil {
		t.Errorf("failed to parse Go file %s: %v", fullPath, err)
		return
	}

	symbolParts := strings.Split(targetSymbol, ".")
	var targetReceiver string
	var targetName string

	if len(symbolParts) == 2 {
		targetReceiver = symbolParts[0]
		targetName = symbolParts[1]
	} else {
		targetName = symbolParts[0]
	}

	var targetFunc *ast.FuncDecl

	ast.Inspect(node, func(n ast.Node) bool {
		decl, ok := n.(*ast.FuncDecl)
		if !ok || decl.Name.Name != targetName {
			return true
		}

		if targetReceiver == "" {
			if decl.Recv == nil {
				targetFunc = decl
				return false
			}
		} else if decl.Recv != nil && len(decl.Recv.List) > 0 {
			recvType := decl.Recv.List[0].Type
			if star, ok := recvType.(*ast.StarExpr); ok {
				if ident, ok := star.X.(*ast.Ident); ok && ident.Name == targetReceiver {
					targetFunc = decl
					return false
				}
			} else if ident, ok := recvType.(*ast.Ident); ok && ident.Name == targetReceiver {
				targetFunc = decl
				return false
			}
		}
		return true
	})

	if targetFunc == nil {
		t.Errorf("transaction boundary func/method %q not found in %s", targetSymbol, fullPath)
		return
	}

	if targetFunc.Body == nil {
		t.Errorf("transaction boundary func/method %q has no body in %s", targetSymbol, fullPath)
		return
	}

	if !hasRunInTxCall(targetFunc.Body) {
		t.Errorf("transaction boundary method %q in %s is declared with RunInTx, but contains no RunInTx call in AST body", targetSymbol, fullPath)
	}
}

func hasRunInTxCall(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "RunInTx" || fun.Name == "runInTx" {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			if fun.Sel.Name == "RunInTx" || fun.Sel.Name == "runInTx" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func TestHasRunInTxCall_AST_Detection(t *testing.T) {
	srcWithIdent := `package test
func DoTx() error {
	return RunInTx(ctx, db, func(txCtx context.Context) error { return nil })
}`
	srcWithSelector := `package test
func (s *Service) DoTx() error {
	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error { return nil })
}`
	srcWithInternal := `package test
func (s *Service) DoTx() error {
	return s.runInTx(ctx, func(txCtx context.Context) error { return nil })
}`
	srcWithoutTx := `package test
func (s *Service) NonTx() error {
	return s.repo.Save(ctx)
}`

	fset := token.NewFileSet()

	tests := []struct {
		name     string
		src      string
		expected bool
	}{
		{"ident RunInTx", srcWithIdent, true},
		{"selector RunInTx", srcWithSelector, true},
		{"internal runInTx", srcWithInternal, true},
		{"non-transactional", srcWithoutTx, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.ParseFile(fset, "", tt.src, 0)
			if err != nil {
				t.Fatalf("failed to parse test source: %v", err)
			}
			var body *ast.BlockStmt
			ast.Inspect(node, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					body = fn.Body
					return false
				}
				return true
			})

			actual := hasRunInTxCall(body)
			if actual != tt.expected {
				t.Errorf("hasRunInTxCall() = %v, expected %v", actual, tt.expected)
			}
		})
	}
}

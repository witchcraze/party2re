package http_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type jsonDecodeViolation struct {
	file string
	line int
	desc string
}

func TestAST_NoIgnoredJSONDecodingInHTTPHandlers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to glob go files: %v", err)
	}

	var violations []jsonDecodeViolation
	fset := token.NewFileSet()

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}

		node, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", file, err)
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// 1. Detect assignments to blank identifier: `_ = ...`
			if assign, ok := n.(*ast.AssignStmt); ok {
				for _, lhs := range assign.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
						for _, rhs := range assign.Rhs {
							if isDecoderCall(rhs) {
								pos := fset.Position(assign.Pos())
								violations = append(violations, jsonDecodeViolation{
									file: file,
									line: pos.Line,
									desc: "ignored JSON decoder result using blank identifier `_ =`",
								})
							}
						}
					}
				}
			}

			// 2. Detect bare expression statements discarding decodeJSON / decodeOptionalJSON boolean result
			if exprStmt, ok := n.(*ast.ExprStmt); ok {
				if isDecoderCall(exprStmt.X) {
					pos := fset.Position(exprStmt.Pos())
					violations = append(violations, jsonDecodeViolation{
						file: file,
						line: pos.Line,
						desc: "discarded JSON decoder return value without checking result",
					})
				}
			}

			return true
		})
	}

	if len(violations) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("found %d unhandled/ignored JSON decode violation(s) in internal/api/http:\n", len(violations)))
		for _, v := range violations {
			sb.WriteString(fmt.Sprintf("  %s:%d: %s\n", v.file, v.line, v.desc))
		}
		t.Fatal(sb.String())
	}
}

// isDecoderCall checks if an expression is a call to decodeJSON, decodeOptionalJSON, or (*json.Decoder).Decode
func isDecoderCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}

	// Case A: call to `decodeJSON(...)` or `decodeOptionalJSON(...)`
	if ident, ok := call.Fun.(*ast.Ident); ok {
		if ident.Name == "decodeJSON" || ident.Name == "decodeOptionalJSON" {
			return true
		}
	}

	// Case B: call to `dec.Decode(...)` or `json.NewDecoder(...).Decode(...)`
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if sel.Sel.Name == "Decode" {
			return true
		}
	}

	return false
}

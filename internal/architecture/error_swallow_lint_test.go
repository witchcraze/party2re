package architecture_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type errorSwallowViolation struct {
	file     string
	line     int
	receiver string
	method   string
	message  string
}

// isRepoReceiver returns true if the receiver represents a database repository,
// persistent store, or transaction-bound aggregate gateway.
func isRepoReceiver(receiver string) bool {
	lower := strings.ToLower(receiver)
	if strings.Contains(lower, "repo") ||
		strings.Contains(lower, "store") ||
		strings.Contains(lower, "database") ||
		strings.Contains(lower, "updater") ||
		strings.Contains(lower, "creator") ||
		strings.Contains(lower, "saver") ||
		strings.HasSuffix(lower, "characters") ||
		strings.HasSuffix(lower, "inventories") ||
		strings.HasSuffix(lower, "depots") ||
		strings.HasSuffix(lower, "adventures") ||
		strings.HasSuffix(lower, "contests") ||
		strings.HasSuffix(lower, "standings") ||
		strings.HasSuffix(lower, "quests") ||
		strings.HasSuffix(lower, "guilds") ||
		strings.HasSuffix(lower, "accounts") ||
		strings.HasSuffix(lower, "parcels") ||
		strings.HasSuffix(lower, "auctions") ||
		strings.HasSuffix(lower, "listings") ||
		strings.HasSuffix(lower, "orders") ||
		strings.HasSuffix(lower, "sales") ||
		strings.HasSuffix(lower, "plots") ||
		strings.HasSuffix(lower, "monsters") ||
		strings.HasSuffix(lower, "parties") {
		return true
	}
	return false
}

// isStorageMutation returns true for container mutations that must check capacity/existence.
func isStorageMutation(receiver string, method string) bool {
	lowerRec := strings.ToLower(receiver)
	if lowerRec == "inv" || lowerRec == "newinv" || lowerRec == "depot" || lowerRec == "dep" || lowerRec == "box" {
		if method == "Add" || method == "AddItem" || method == "RemoveItem" || method == "ConsumeItem" {
			return true
		}
	}
	return false
}

// isStateMutationMethod returns true if the method is a critical state mutation
// that must never have its error return ignored or swallowed.
func isStateMutationMethod(method string) bool {
	switch method {
	case "AddMoney", "DeductMoney", "SpendMoney",
		"AddSmallMedals", "DeductSmallMedals",
		"AddCrystal", "DeductCrystal",
		"AddItem", "RemoveItem", "ConsumeItem",
		"RecordMatchSettlement":
		return true
	}
	return false
}

func isCriticalCall(receiver string, method string) bool {
	return isRepoReceiver(receiver) || isStorageMutation(receiver, method) || isStateMutationMethod(method)
}

func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	default:
		return fmt.Sprintf("%T", e)
	}
}

func hasErrorSwallowIgnoreDirective(comments []*ast.CommentGroup, fset *token.FileSet, line int) bool {
	for _, cg := range comments {
		startLine := fset.Position(cg.Pos()).Line
		endLine := fset.Position(cg.End()).Line
		if (startLine <= line && endLine >= line) || endLine == line-1 {
			for _, c := range cg.List {
				if strings.Contains(c.Text, "lint:ignore error-swallow") {
					return true
				}
			}
		}
	}
	return false
}

func checkFileErrorSwallowRules(fset *token.FileSet, node *ast.File, filename string) []errorSwallowViolation {
	var violations []errorSwallowViolation

	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if len(stmt.Lhs) == 0 {
				return true
			}
			lastLhs := stmt.Lhs[len(stmt.Lhs)-1]
			ident, ok := lastLhs.(*ast.Ident)
			if !ok || ident.Name != "_" {
				return true
			}

			var call *ast.CallExpr
			if len(stmt.Rhs) == 1 {
				call, _ = stmt.Rhs[0].(*ast.CallExpr)
			} else if len(stmt.Rhs) == len(stmt.Lhs) {
				call, _ = stmt.Rhs[len(stmt.Rhs)-1].(*ast.CallExpr)
			}
			if call == nil {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			receiver := exprToString(sel.X)
			method := sel.Sel.Name

			if !isCriticalCall(receiver, method) {
				return true
			}

			line := fset.Position(stmt.Pos()).Line
			if hasErrorSwallowIgnoreDirective(node.Comments, fset, line) {
				return true
			}

			violations = append(violations, errorSwallowViolation{
				file:     filename,
				line:     line,
				receiver: receiver,
				method:   method,
				message:  fmt.Sprintf("silent error suppression on persistence/storage call '%s.%s' is prohibited; propagate error or annotate with //lint:ignore error-swallow <reason>", receiver, method),
			})

		case *ast.ExprStmt:
			call, ok := stmt.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver := exprToString(sel.X)
			method := sel.Sel.Name

			if !isCriticalCall(receiver, method) {
				return true
			}

			line := fset.Position(stmt.Pos()).Line
			if hasErrorSwallowIgnoreDirective(node.Comments, fset, line) {
				return true
			}

			violations = append(violations, errorSwallowViolation{
				file:     filename,
				line:     line,
				receiver: receiver,
				method:   method,
				message:  fmt.Sprintf("unhandled error from persistence/storage call '%s.%s' in expression statement; check and propagate error", receiver, method),
			})
		}

		return true
	})

	return violations
}

// TestErrorSwallowLint verifies that no production package in internal/ silently suppresses
// error returns from database repository and persistent store calls using blank assignments or ignored statements.
func TestErrorSwallowLint(t *testing.T) {
	repoRoot := "../.."
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	var allViolations []errorSwallowViolation

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			if rel == "internal/architecture" || rel == "internal/testutil" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)
		relPath = filepath.ToSlash(relPath)

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		node, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}

		violations := checkFileErrorSwallowRules(fset, node, relPath)
		allViolations = append(allViolations, violations...)
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if len(allViolations) > 0 {
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("Found %d silent error suppression violation(s) across internal/:\n", len(allViolations)))
		for _, v := range allViolations {
			buf.WriteString(fmt.Sprintf("%s:%d: %s\n", v.file, v.line, v.message))
		}
		buf.WriteString("\nAccording to .agents/rules/01-development-workflow.md and docs/development/ast-linters.md:\n" +
			"- Database repository and persistent store calls must never discard errors via blank identifiers ('_ =', '_, _ =', 'val, _ :=') or unassigned expression statements.\n" +
			"- All errors must be propagated to caller or handled within transactional rollback boundaries.\n" +
			"- If a call is strictly best-effort or compensatory (e.g. defer rollback), annotate with '//lint:ignore error-swallow <reason>'.\n")
		t.Errorf("%s", buf.String())
	}
}

// TestErrorSwallowLint_UnitTests tests the AST checker logic with synthetic code snippets.
func TestErrorSwallowLint_UnitTests(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectCount int
	}{
		{
			name: "blank assignment to repo.Save",
			code: `package testpkg
func foo(s *Service) {
	_ = s.repo.Save(ctx, item)
}`,
			expectCount: 1,
		},
		{
			name: "multi-value blank assignment to repo.DeductBetAndCreditPayout",
			code: `package testpkg
func foo(s *Service) {
	_, _ = s.repo.DeductBetAndCreditPayout(txCtx, id, 0, pot)
}`,
			expectCount: 1,
		},
		{
			name: "multi-value assignment to repo.GetRecord discarding error",
			code: `package testpkg
func foo(s *Service) {
	rec, _ := s.repo.GetRecord(ctx, id)
}`,
			expectCount: 1,
		},
		{
			name: "bare expr stmt on repo call",
			code: `package testpkg
func foo(s *Service) {
	s.partyRepo.DeleteParty(txCtx, id)
}`,
			expectCount: 1,
		},
		{
			name: "in-memory query method is NOT flagged (zero noise)",
			code: `package testpkg
func foo(c *Character) {
	_ = c.CalculateAttack()
}`,
			expectCount: 0,
		},
		{
			name: "currency mutation method on character struct is flagged",
			code: `package testpkg
func foo(c *Character) {
	_ = c.AddMoney(100)
}`,
			expectCount: 1,
		},
		{
			name: "stdlib strconv is NOT flagged (zero noise)",
			code: `package testpkg
import "strconv"
func foo(s string) {
	val, _ := strconv.Atoi(s)
}`,
			expectCount: 0,
		},
		{
			name: "multi-value assignment properly capturing error is NOT flagged",
			code: `package testpkg
func foo(s *Service) {
	_, err := s.repo.FindByID(ctx, id)
	if err != nil { panic(err) }
}`,
			expectCount: 0,
		},
		{
			name: "inline lint:ignore directive bypasses",
			code: `package testpkg
func foo(s *Service) {
	_ = s.repo.DeleteRoom(ctx, id) //lint:ignore error-swallow best effort cleanup
}`,
			expectCount: 0,
		},
		{
			name: "preceding lint:ignore directive bypasses",
			code: `package testpkg
func foo(s *Service) {
	//lint:ignore error-swallow best effort cleanup
	_ = s.repo.DeleteRoom(ctx, id)
}`,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, "test.go", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			violations := checkFileErrorSwallowRules(fset, node, "test.go")
			if len(violations) != tt.expectCount {
				t.Fatalf("expected %d violations, got %d: %+v", tt.expectCount, len(violations), violations)
			}
		})
	}
}

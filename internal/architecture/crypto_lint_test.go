package architecture_test

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

const (
	minBcryptCost = 12
	bcryptPkgPath = "golang.org/x/crypto/bcrypt"
)

// whitelistedInsecureDigestPaths defines files permitted to import deprecated digests
// strictly for non-security legacy checksums or interoperability.
// No entries should be added without formal security and architecture review.
var whitelistedInsecureDigestPaths = map[string]string{
	// Currently zero exceptions.
}

type bcryptViolation struct {
	Line   int
	Reason string
}

type digestViolation struct {
	Line       int
	ImportPath string
	Reason     string
}

// collectFileConstants inspects the AST for top-level constant declarations and records their expressions.
func collectFileConstants(node *ast.File) map[string]ast.Expr {
	consts := make(map[string]ast.Expr)
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		var lastValues []ast.Expr
		for _, spec := range genDecl.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if len(valSpec.Values) > 0 {
				lastValues = valSpec.Values
			}
			for i, name := range valSpec.Names {
				if i < len(lastValues) {
					consts[name.Name] = lastValues[i]
				}
			}
		}
	}
	return consts
}

func formatSelector(sel *ast.SelectorExpr) string {
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name
	}
	return "expr"
}

// evaluateBcryptCost checks if an AST expression evaluates to a statically verifiable int >= minBcryptCost (12).
func evaluateBcryptCost(expr ast.Expr, consts map[string]ast.Expr, depth int) (int, bool, string) {
	if depth > 5 {
		return 0, false, "recursion limit exceeded evaluating constant"
	}

	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.INT {
			return 0, false, fmt.Sprintf("bcrypt cost must be an integer, got %s literal %s", v.Kind, v.Value)
		}
		cost, err := strconv.Atoi(v.Value)
		if err != nil {
			return 0, false, fmt.Sprintf("invalid integer literal %q: %v", v.Value, err)
		}
		if cost < minBcryptCost {
			return cost, true, fmt.Sprintf("bcrypt cost %d is below required minimum %d (.agents/rules/06-security.md Section 5)", cost, minBcryptCost)
		}
		return cost, true, ""

	case *ast.UnaryExpr:
		if v.Op == token.SUB {
			return 0, false, "negative bcrypt cost is invalid"
		}
		if v.Op == token.ADD {
			return evaluateBcryptCost(v.X, consts, depth+1)
		}
		return 0, false, fmt.Sprintf("unsupported unary operator %s on cost", v.Op)

	case *ast.Ident:
		constExpr, ok := consts[v.Name]
		if !ok {
			return 0, false, fmt.Sprintf("bcrypt cost %q is not a statically verifiable constant in the file (must be defined as const >= %d)", v.Name, minBcryptCost)
		}
		return evaluateBcryptCost(constExpr, consts, depth+1)

	case *ast.SelectorExpr:
		switch v.Sel.Name {
		case "DefaultCost":
			return 10, true, fmt.Sprintf("bcrypt.DefaultCost (10) is below required minimum %d (.agents/rules/06-security.md Section 5)", minBcryptCost)
		case "MinCost":
			return 4, true, fmt.Sprintf("bcrypt.MinCost (4) is below required minimum %d (.agents/rules/06-security.md Section 5)", minBcryptCost)
		case "MaxCost":
			return 31, true, ""
		default:
			return 0, false, fmt.Sprintf("unsupported selector %s.%s: bcrypt cost must be a statically verifiable constant >= %d", formatSelector(v), v.Sel.Name, minBcryptCost)
		}

	default:
		return 0, false, fmt.Sprintf("bcrypt cost must be a statically verifiable integer constant >= %d, got %T", minBcryptCost, expr)
	}
}

// detectBcryptCostViolations parses Go source content and detects any call to bcrypt.GenerateFromPassword
// whose cost parameter is < minBcryptCost (12) or cannot be statically verified as a constant >= 12.
func detectBcryptCostViolations(fset *token.FileSet, filename string, src any) ([]bcryptViolation, error) {
	node, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}

	var bcryptIdent string
	isDotImport := false

	for _, imp := range node.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if importPath == bcryptPkgPath {
			if imp.Name != nil {
				if imp.Name.Name == "." {
					isDotImport = true
				} else if imp.Name.Name != "_" {
					bcryptIdent = imp.Name.Name
				}
			} else {
				bcryptIdent = "bcrypt"
			}
			break
		}
	}

	if bcryptIdent == "" && !isDotImport {
		return nil, nil
	}

	consts := collectFileConstants(node)
	var violations []bcryptViolation

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		isGenerateFromPassword := false
		if isDotImport {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "GenerateFromPassword" {
				isGenerateFromPassword = true
			}
		} else if bcryptIdent != "" {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "GenerateFromPassword" {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == bcryptIdent {
					isGenerateFromPassword = true
				}
			}
		}

		if !isGenerateFromPassword {
			return true
		}

		line := fset.Position(call.Pos()).Line
		if len(call.Args) < 2 {
			violations = append(violations, bcryptViolation{
				Line:   line,
				Reason: "GenerateFromPassword call has fewer than 2 arguments",
			})
			return true
		}

		costExpr := call.Args[1]
		_, _, reason := evaluateBcryptCost(costExpr, consts, 0)
		if reason != "" {
			violations = append(violations, bcryptViolation{
				Line:   line,
				Reason: reason,
			})
		}

		return true
	})

	return violations, nil
}

// detectInsecureDigestImports parses Go source content and detects deprecated hash imports (crypto/md5, crypto/sha1).
func detectInsecureDigestImports(fset *token.FileSet, filename string, src any, relPath string, whitelist map[string]string) ([]digestViolation, error) {
	node, err := parser.ParseFile(fset, filename, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	cleanRelPath := filepath.ToSlash(relPath)
	if cleanRelPath == "" {
		cleanRelPath = filepath.ToSlash(filename)
	}

	var violations []digestViolation
	for _, imp := range node.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}

		if importPath == "crypto/md5" || importPath == "crypto/sha1" {
			if _, ok := whitelist[cleanRelPath]; ok {
				continue
			}
			line := fset.Position(imp.Pos()).Line
			violations = append(violations, digestViolation{
				Line:       line,
				ImportPath: importPath,
				Reason:     fmt.Sprintf("use of deprecated digest algorithm %q is prohibited (.agents/rules/06-security.md Section 5)", importPath),
			})
		}
	}

	return violations, nil
}

// TestProhibitInsecureCryptographicPolicy scans all production Go source files under internal/ and cmd/
// to verify compliance with .agents/rules/06-security.md Section 5:
// 1. Password hashing via bcrypt MUST use cost >= 12 (statically verified).
// 2. Insecure digests (crypto/md5, crypto/sha1) are strictly prohibited unless explicitly whitelisted.
func TestProhibitInsecureCryptographicPolicy(t *testing.T) {
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
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			cleanPath := filepath.ToSlash(path)
			if strings.Contains(cleanPath, "/testutil/") || strings.Contains(cleanPath, "/database/testutil/") {
				return nil
			}

			relPath, _ := filepath.Rel(repoRoot, path)
			cleanRelPath := filepath.ToSlash(relPath)

			// 1. Check bcrypt cost violations
			bcryptViolations, parseErr := detectBcryptCostViolations(fset, path, nil)
			if parseErr != nil {
				t.Errorf("failed to parse %s for bcrypt cost: %v", path, parseErr)
				return nil
			}
			for _, v := range bcryptViolations {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", cleanRelPath, v.Line, v.Reason))
			}

			// 2. Check insecure digest imports
			digestViolations, parseErr := detectInsecureDigestImports(fset, path, nil, cleanRelPath, whitelistedInsecureDigestPaths)
			if parseErr != nil {
				t.Errorf("failed to parse %s for digest imports: %v", path, parseErr)
				return nil
			}
			for _, v := range digestViolations {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", cleanRelPath, v.Line, v.Reason))
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk directory %s: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		t.Errorf("found cryptographic security policy violations (.agents/rules/06-security.md Section 5):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestProhibitInsecureCryptographicPolicy_SelfTest validates static analysis against synthetic Go code.
func TestProhibitInsecureCryptographicPolicy_SelfTest(t *testing.T) {
	fset := token.NewFileSet()

	t.Run("BcryptCost", func(t *testing.T) {
		tests := []struct {
			name        string
			src         string
			wantViolate bool
			wantLine    int
			wantReason  string
		}{
			{
				name: "ValidConst12",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
const bcryptCost = 12
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), bcryptCost)
}`,
				wantViolate: false,
			},
			{
				name: "ValidLiteral14",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), 14)
}`,
				wantViolate: false,
			},
			{
				name: "ValidMaxCost",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), bcrypt.MaxCost)
}`,
				wantViolate: false,
			},
			{
				name: "ValidAliasedImport",
				src: `package foo
import b "golang.org/x/crypto/bcrypt"
const cost = 12
func hash(p string) ([]byte, error) {
	return b.GenerateFromPassword([]byte(p), cost)
}`,
				wantViolate: false,
			},
			{
				name: "InvalidLiteralBelow12",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), 10)
}`,
				wantViolate: true,
				wantLine:    4,
				wantReason:  "below required minimum 12",
			},
			{
				name: "InvalidConstBelow12",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
const fastCost = 4
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), fastCost)
}`,
				wantViolate: true,
				wantLine:    5,
				wantReason:  "below required minimum 12",
			},
			{
				name: "InvalidDefaultCost",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
}`,
				wantViolate: true,
				wantLine:    4,
				wantReason:  "DefaultCost",
			},
			{
				name: "InvalidMinCost",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), bcrypt.MinCost)
}`,
				wantViolate: true,
				wantLine:    4,
				wantReason:  "MinCost",
			},
			{
				name: "InvalidDynamicVariable",
				src: `package foo
import "golang.org/x/crypto/bcrypt"
func hash(p string, cost int) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), cost)
}`,
				wantViolate: true,
				wantLine:    4,
				wantReason:  "not a statically verifiable constant",
			},
			{
				name: "InvalidDotImport",
				src: `package foo
import . "golang.org/x/crypto/bcrypt"
func hash(p string) ([]byte, error) {
	return GenerateFromPassword([]byte(p), 10)
}`,
				wantViolate: true,
				wantLine:    4,
				wantReason:  "below required minimum 12",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				violations, err := detectBcryptCostViolations(fset, tt.name+".go", tt.src)
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if !tt.wantViolate {
					if len(violations) != 0 {
						t.Errorf("expected 0 violations, got %v", violations)
					}
					return
				}
				if len(violations) != 1 {
					t.Fatalf("expected 1 violation, got %d (%v)", len(violations), violations)
				}
				if violations[0].Line != tt.wantLine {
					t.Errorf("expected line %d, got %d", tt.wantLine, violations[0].Line)
				}
				if !strings.Contains(violations[0].Reason, tt.wantReason) {
					t.Errorf("expected reason to contain %q, got %q", tt.wantReason, violations[0].Reason)
				}
			})
		}
	})

	t.Run("InsecureDigestImports", func(t *testing.T) {
		tests := []struct {
			name        string
			src         string
			whitelist   map[string]string
			wantViolate bool
			wantLine    int
			wantImport  string
		}{
			{
				name: "MD5Import",
				src: `package foo
import (
	"crypto/md5"
	"fmt"
)`,
				wantViolate: true,
				wantLine:    3,
				wantImport:  "crypto/md5",
			},
			{
				name: "SHA1Import",
				src: `package foo
import (
	"crypto/sha1"
	"fmt"
)`,
				wantViolate: true,
				wantLine:    3,
				wantImport:  "crypto/sha1",
			},
			{
				name: "WhitelistedMD5",
				src: `package foo
import (
	"crypto/md5"
)`,
				whitelist:   map[string]string{"internal/foo/whitelisted.go": "legacy digest"},
				wantViolate: false,
			},
			{
				name: "SecureCryptoRandAndSHA256",
				src: `package foo
import (
	"crypto/rand"
	"crypto/sha256"
)`,
				wantViolate: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				filename := "internal/foo/" + tt.name + ".go"
				if tt.name == "WhitelistedMD5" {
					filename = "internal/foo/whitelisted.go"
				}
				violations, err := detectInsecureDigestImports(fset, filename, tt.src, filename, tt.whitelist)
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if !tt.wantViolate {
					if len(violations) != 0 {
						t.Errorf("expected 0 violations, got %v", violations)
					}
					return
				}
				if len(violations) != 1 {
					t.Fatalf("expected 1 violation, got %d (%v)", len(violations), violations)
				}
				if violations[0].Line != tt.wantLine {
					t.Errorf("expected line %d, got %d", tt.wantLine, violations[0].Line)
				}
				if violations[0].ImportPath != tt.wantImport {
					t.Errorf("expected import %q, got %q", tt.wantImport, violations[0].ImportPath)
				}
			})
		}
	})
}

package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// htmlTagRegex matches common HTML tags (e.g. <b>, <br>, <font>, <span>, <div>, etc.)
var htmlTagRegex = regexp.MustCompile(`(?i)</?(?:b|br|font|span|div|i|u|p|a|table|tr|td|th)\b[^>]*>`)

// TestProhibitHTMLInDomainPackages scans all production Go source files under internal/
// (excluding transport layer internal/api/http and test utilities) and asserts that
// no HTML presentation tags are hardcoded in string literals (.agents/rules/03-architecture.md §5).
func TestProhibitHTMLInDomainPackages(t *testing.T) {
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		cleanPath := filepath.ToSlash(path)

		// Transport layer (HTTP) and test utilities are exempt
		if strings.Contains(cleanPath, "/api/http/") || strings.Contains(cleanPath, "/testutil/") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(node, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				val = lit.Value
			}

			if match := htmlTagRegex.FindString(val); match != "" {
				pos := fset.Position(lit.Pos())
				relPath, _ := filepath.Rel(repoRoot, path)
				violations = append(violations, relPath+":"+strconv.Itoa(pos.Line)+": contains prohibited HTML markup: "+match)
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found %d presentation leakage violations in domain packages:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// TestProhibitPresentationFieldsInDomainModels verifies that domain service result structs
// do not declare presentation fields unless annotated as authored or persisted data.
func TestProhibitPresentationFieldsInDomainModels(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}
	violations, err := presentationFieldViolations(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("found %d prohibited presentation field violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

func presentationFieldViolations(repoRoot string) ([]string, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	fset := token.NewFileSet()

	var violations []string

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
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

		// Transport layer (HTTP), architecture linter, and test utilities are exempt
		if strings.Contains(cleanPath, "/api/http/") ||
			strings.Contains(cleanPath, "/architecture/") ||
			strings.Contains(cleanPath, "/testutil/") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		pkgName := node.Name.Name
		typeNames := map[*ast.StructType]string{}

		ast.Inspect(node, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					typeNames[st] = ts.Name.Name
				}
				return true
			}
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			typeName := typeNames[st]
			if typeName == "" {
				typeName = "(anonymous)"
			}

			for _, f := range st.Fields.List {
				jsonName := ""
				if f.Tag != nil {
					jsonName = strings.Split(reflect.StructTag(stringLiteral(f.Tag)).Get("json"), ",")[0]
				}
				names := f.Names
				if len(names) == 0 {
					// Embedded fields have no Names; their field name comes from the type.
					embedded := f.Type
					if pointer, ok := embedded.(*ast.StarExpr); ok {
						embedded = pointer.X
					}
					switch generic := embedded.(type) {
					case *ast.IndexExpr:
						embedded = generic.X
					case *ast.IndexListExpr:
						embedded = generic.X
					}
					switch embedded := embedded.(type) {
					case *ast.Ident:
						names = []*ast.Ident{embedded}
					case *ast.SelectorExpr:
						names = []*ast.Ident{embedded.Sel}
					}
				}
				for _, name := range names {
					if name.Name == "NPCMessage" || name.Name == "NPCSpeech" || name.Name == "Dialogue" || name.Name == "Message" || jsonName == "message" {
						if (name.Name == "Message" || jsonName == "message") && presentationDataField(f) {
							continue
						}
						key := pkgName + "." + typeName + "." + name.Name
						pos := fset.Position(name.Pos())
						relPath, _ := filepath.Rel(repoRoot, path)
						violations = append(violations, fmt.Sprintf("%s:%d: prohibited presentation field %s; move formatting to HTTP or annotate authored/persisted data with //lint:ignore presentation <reason>",
							relPath, pos.Line, key))
					}
				}
			}
			return true
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return violations, nil
}

func presentationDataField(field *ast.Field) bool {
	const directive = "//lint:ignore presentation "
	for _, group := range []*ast.CommentGroup{field.Doc, field.Comment} {
		if group != nil {
			for _, comment := range group.List {
				if reason, ok := strings.CutPrefix(comment.Text, directive); ok && strings.TrimSpace(reason) != "" {
					return true
				}
			}
		}
	}
	return false
}

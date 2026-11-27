package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// legacyProhibitedChars are the characters of the internal/validation profiles.
const legacyProhibitedChars = `,;"'&<>@＠\/`

var (
	charSetFuncs = map[string]bool{"ContainsAny": true, "IndexAny": true, "LastIndexAny": true, "ContainsRune": true, "IndexRune": true, "Trim": true, "TrimLeft": true, "TrimRight": true}
	regexpClass  = regexp.MustCompile(`\[[^\]]*\]`)
)

// prohibitedCount counts distinct legacy prohibited characters in s; a set
// holds two or more, while a single ＠ is ordinary command parsing.
func prohibitedCount(s string) int {
	n := 0
	for _, r := range legacyProhibitedChars {
		if strings.ContainsRune(s, r) {
			n++
		}
	}
	return n
}

// detectLocalProhibitedSets reports lines where a file builds its own
// prohibited-character set: a strings character-set call or regexp character
// class over a local literal, or a switch case listing such runes.
// shortcut: a literal handed to a feature helper that calls strings itself is
// not traced; extend the walk when such a helper appears.
func detectLocalProhibitedSets(fset *token.FileSet, filename string, src any) ([]int, error) {
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	literals := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		if spec, ok := n.(*ast.ValueSpec); ok {
			for i, name := range spec.Names {
				if i < len(spec.Values) {
					literals[name.Name] = localText(spec.Values[i], nil)
				}
			}
		}
		return true
	})
	var lines []int
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			isRegexp := ok && pkg.Name == "regexp" && (sel.Sel.Name == "MustCompile" || sel.Sel.Name == "Compile")
			if !isRegexp && !(ok && pkg.Name == "strings" && charSetFuncs[sel.Sel.Name]) {
				return true
			}
			for _, arg := range node.Args {
				sets := []string{localText(arg, literals)}
				if isRegexp {
					sets = regexpClass.FindAllString(sets[0], -1)
				}
				for _, set := range sets {
					if prohibitedCount(set) >= 2 {
						lines = append(lines, fset.Position(arg.Pos()).Line)
					}
				}
			}
		case *ast.CaseClause:
			runes := ""
			for _, expr := range node.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.CHAR {
					if r, err := strconv.Unquote(lit.Value); err == nil {
						runes += r
					}
				}
			}
			if prohibitedCount(runes) >= 2 {
				lines = append(lines, fset.Position(node.Pos()).Line)
			}
		}
		return true
	})
	return lines, nil
}

// localText returns the file-local literal text of expr: string literals,
// identifiers bound to them and concatenations including them. References to
// other packages, such as validation profiles, contribute nothing.
func localText(expr ast.Expr, literals map[string]string) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if s, err := strconv.Unquote(e.Value); err == nil && (e.Kind == token.STRING || e.Kind == token.CHAR) {
			return s
		}
	case *ast.Ident:
		return literals[e.Name]
	case *ast.BinaryExpr:
		return localText(e.X, literals) + localText(e.Y, literals)
	case *ast.ParenExpr:
		return localText(e.X, literals)
	}
	return ""
}

// TestProhibitLocalProhibitedCharacterSets keeps legacy prohibited-character
// sets in the internal/validation profiles (.agents/rules/06-security.md §6).
func TestProhibitLocalProhibitedCharacterSets(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var violations []string
	for _, dir := range []string{"internal", "cmd"} {
		err = filepath.Walk(filepath.Join(repoRoot, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			clean := filepath.ToSlash(path)
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(clean, "/internal/validation/") {
				return nil
			}
			lines, parseErr := detectLocalProhibitedSets(fset, path, nil)
			if parseErr != nil {
				t.Errorf("failed to parse %s: %v", path, parseErr)
				return nil
			}
			for _, line := range lines {
				rel, _ := filepath.Rel(repoRoot, path)
				violations = append(violations, rel+":"+strconv.Itoa(line))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		t.Errorf("feature-local prohibited-character sets; use the internal/validation profiles:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestProhibitLocalProhibitedCharacterSets_SelfTest(t *testing.T) {
	fset := token.NewFileSet()
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"validation profile", `package p; func f(s string) bool { return strings.ContainsAny(s, validation.TitleChars+validation.FullwidthAt+"\t") }`, 0},
		{"single fullwidth at", `package p; func f(s string) bool { return strings.Contains(s, "＠") || strings.ContainsRune(s, '＠') }`, 0},
		{"whitespace set", `package p; func f(s string) bool { return strings.ContainsAny(s, " \t\r\n　") }`, 0},
		{"unrelated regexp", "package p; var r = regexp.MustCompile(`<[A-Za-z!/?]`)", 0},
		{"error message", `package p; var e = errors.New("name contains , ; < >")`, 0},
		{"ContainsAny literal", `package p; func f(s string) bool { return strings.ContainsAny(s, ";<>") }`, 1},
		{"ContainsRune set", `package p; func f(r rune) bool { return strings.ContainsRune(",;<>＠", r) }`, 1},
		{"concatenated literal", `package p; func f(s string) bool { return strings.ContainsAny(s, validation.MarkupChars+"@＠") }`, 1},
		{"named constant", `package p; const forbidden = "&<>"; func f(s string) bool { return strings.IndexAny(s, forbidden) >= 0 }`, 1},
		{"regexp class", "package p; var r = regexp.MustCompile(`[,;\\\"'&<>]`)", 1},
		{"rune switch", `package p; func f(r rune) bool { switch r { case ',', ';', '<': return true }; return false }`, 1},
	} {
		lines, err := detectLocalProhibitedSets(fset, tc.name+".go", tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(lines) != tc.want {
			t.Errorf("%s: got violations at %v, want %d", tc.name, lines, tc.want)
		}
	}
}

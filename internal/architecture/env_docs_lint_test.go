package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	envDocsReadme         = "README.md"
	envDocsOptional       = "docs/development/environment-variables.md"
	envDocsTableHeading   = "### 実行環境変数"
	envDocsComposeHeading = "### Docker Compose での起動例"
	envDocsIgnore         = "//lint:ignore env-docs"
)

var (
	envDocsTableRow = regexp.MustCompile("^\\| `([A-Z][A-Z0-9_]*)`")
	envDocsName     = regexp.MustCompile("`([A-Z][A-Z0-9_]*)`")
)

// envRead is an environment variable read found in production code.
type envRead struct {
	Name     string
	Location string
}

// collectEnvReadsFromSource returns reads resolved to a name and diagnostics for unresolved ones.
func collectEnvReadsFromSource(path string, src any) ([]envRead, []string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	ignoredLines := map[int]bool{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, envDocsIgnore+" ") {
				line := fset.Position(comment.Pos()).Line
				ignoredLines[line] = true
				ignoredLines[line+1] = true
			}
		}
	}

	var reads []envRead
	var problems []string
	for _, decl := range file.Decls {
		defaults := literalAssignments(decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isEnvReadCall(call) || len(call.Args) != 1 {
				return true
			}
			pos := fset.Position(call.Pos())
			location := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
			name := ""
			switch arg := call.Args[0].(type) {
			case *ast.BasicLit:
				name = stringLiteral(arg)
			case *ast.Ident:
				name = defaults[arg.Name]
			}
			switch {
			case name != "":
				reads = append(reads, envRead{Name: name, Location: location})
			case !ignoredLines[pos.Line]:
				problems = append(problems, fmt.Sprintf("%s: environment variable name cannot be resolved; use a string literal or add %q", location, envDocsIgnore+" <reason>"))
			}
			return true
		})
	}
	return reads, problems, nil
}

// literalAssignments maps identifiers to string literals assigned to them, such as a default key.
func literalAssignments(decl ast.Decl) map[string]string {
	assigned := map[string]string{}
	ast.Inspect(decl, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, isIdent := lhs.(*ast.Ident)
			lit, isLit := assign.Rhs[i].(*ast.BasicLit)
			if isIdent && isLit {
				assigned[ident.Name] = stringLiteral(lit)
			}
		}
		return true
	})
	return assigned
}

func isEnvReadCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os"
}

func stringLiteral(lit *ast.BasicLit) string {
	if lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}

func collectProductionEnvReads(repoRoot string) ([]envRead, []string, error) {
	var reads []envRead
	var problems []string
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.Walk(filepath.Join(repoRoot, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if path == filepath.Join(repoRoot, "internal", "testutil") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fileReads, fileProblems, err := collectEnvReadsFromSource(path, nil)
			if err != nil {
				return err
			}
			reads = append(reads, fileReads...)
			problems = append(problems, fileProblems...)
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return reads, problems, nil
}

// markdownSection returns the text from heading up to the next heading of the same or higher level.
func markdownSection(markdown, heading string) string {
	_, rest, found := strings.Cut(markdown, heading+"\n")
	if !found {
		return ""
	}
	inFence := false
	var lines []string
	for line := range strings.SplitSeq(rest, "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence && (strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")) {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// envTable holds the variables a document table lists.
type envTable struct {
	Rows      map[string]string // primary variable name to its row text
	Mentioned map[string]bool   // every backticked name, including aliases
}

func parseEnvTable(markdown string) envTable {
	table := envTable{Rows: map[string]string{}, Mentioned: map[string]bool{}}
	for line := range strings.SplitSeq(markdown, "\n") {
		row := envDocsTableRow.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		table.Rows[row[1]] = line
		for _, match := range envDocsName.FindAllStringSubmatch(line, -1) {
			table.Mentioned[match[1]] = true
		}
	}
	return table
}

// findEnvDocProblems compares the variables read by code with the README and optional-variable documents.
func findEnvDocProblems(reads []envRead, readme, optionalDoc string) []string {
	required := parseEnvTable(markdownSection(readme, envDocsTableHeading))
	optional := parseEnvTable(optionalDoc)
	compose := markdownSection(readme, envDocsComposeHeading)

	var problems []string
	readNames := map[string]bool{}
	for _, read := range reads {
		readNames[read.Name] = true
		if !required.Mentioned[read.Name] && !optional.Mentioned[read.Name] {
			problems = append(problems, fmt.Sprintf("%s: %s is not documented; add it to %s, or to the %s table in %s if required", read.Location, read.Name, envDocsOptional, envDocsTableHeading, envDocsReadme))
		}
	}
	for _, table := range []struct {
		doc   string
		table envTable
	}{{envDocsReadme, required}, {envDocsOptional, optional}} {
		for name := range table.table.Mentioned {
			if !readNames[name] {
				problems = append(problems, fmt.Sprintf("%s: %s is documented but production code does not read it", table.doc, name))
			}
		}
	}
	for name, row := range required.Rows {
		if !strings.Contains(row, "必須") {
			problems = append(problems, fmt.Sprintf("%s: optional variable %s belongs in %s", envDocsReadme, name, envDocsOptional))
		}
		if !strings.Contains(compose, name+":") {
			problems = append(problems, fmt.Sprintf("%s: required variable %s is missing from the Compose example", envDocsReadme, name))
		}
		if _, duplicated := optional.Rows[name]; duplicated {
			problems = append(problems, fmt.Sprintf("%s: %s is also listed in %s; keep one entry", envDocsOptional, name, envDocsReadme))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestEnvironmentVariablesAreDocumented(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	reads, problems, err := collectProductionEnvReads(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(reads) == 0 {
		t.Fatal("no environment variable reads found; the scan is broken")
	}
	readme, err := os.ReadFile(filepath.Join(repoRoot, envDocsReadme))
	if err != nil {
		t.Fatal(err)
	}
	optionalDoc, err := os.ReadFile(filepath.Join(repoRoot, envDocsOptional))
	if err != nil {
		t.Fatal(err)
	}
	problems = append(problems, findEnvDocProblems(reads, string(readme), string(optionalDoc))...)
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestCollectEnvReadsFromSource(t *testing.T) {
	const src = `package p

import "os"

func literal() { _ = os.Getenv("LITERAL"); _, _ = os.LookupEnv("LOOKUP") }

func withDefault(key string) string {
	if key == "" {
		key = "DEFAULTED"
	}
	return os.Getenv(key)
}

func dynamic(key string) string { return os.Getenv(key) }

func ignored(key string) string {
	//lint:ignore env-docs caller-supplied key in tests
	return os.Getenv(key)
}
`
	reads, problems, err := collectEnvReadsFromSource("p.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, read := range reads {
		names = append(names, read.Name)
	}
	if got := strings.Join(names, ","); got != "LITERAL,LOOKUP,DEFAULTED" {
		t.Errorf("reads = %s", got)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "p.go:14") {
		t.Errorf("expected only the dynamic read at p.go:14, got %v", problems)
	}
}

func TestFindEnvDocProblems(t *testing.T) {
	const compose = envDocsComposeHeading + "\n\n```yaml\n# comment\n      REQ: value\n```\n"
	readme := func(rows string) string {
		return envDocsTableHeading + "\n\n" + rows + "\n\n" + compose
	}
	const requiredRow = "| `REQ` | **必須** | Alias `REQ_ALIAS` | `x` |"
	reads := []envRead{{Name: "REQ", Location: "a.go:1"}, {Name: "REQ_ALIAS", Location: "a.go:2"}, {Name: "OPT", Location: "b.go:3"}}

	for _, tc := range []struct {
		name     string
		reads    []envRead
		readme   string
		optional string
		want     string
	}{
		{name: "complete", reads: reads, readme: readme(requiredRow), optional: "| `OPT` | Optional | `1` |"},
		{name: "undocumented read names the location", reads: reads, readme: readme(requiredRow), want: "b.go:3: OPT is not documented"},
		{name: "undocumented alias", reads: reads, readme: readme("| `REQ` | **必須** | x | `x` |"), optional: "| `OPT` | Optional | `1` |", want: "a.go:2: REQ_ALIAS is not documented"},
		{name: "stale entry", reads: reads, readme: readme(requiredRow), optional: "| `OPT` | Optional | `1` |\n| `OLD` | Gone | `1` |", want: "OLD is documented but production code does not read it"},
		{name: "stale alias", reads: reads[:2], readme: readme(requiredRow), optional: "| `REQ_ALIAS` | x | `1` |\n| `OPT` | x | `1` |", want: "OPT is documented but production code does not read it"},
		{name: "optional row in README", reads: []envRead{{Name: "REQ", Location: "a.go:1"}, {Name: "OPT", Location: "b.go:3"}}, readme: readme("| `REQ` | **必須** | x | `x` |\n| `OPT` | 任意 | x | `1` |"), want: "optional variable OPT belongs in"},
		{name: "required missing from Compose", reads: reads, readme: envDocsTableHeading + "\n\n" + requiredRow + "\n", optional: "| `OPT` | x | `1` |", want: "required variable REQ is missing from the Compose example"},
		{name: "listed in both documents", reads: reads, readme: readme(requiredRow), optional: "| `REQ` | x | `1` |\n| `OPT` | x | `1` |", want: "REQ is also listed in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := findEnvDocProblems(tc.reads, tc.readme, tc.optional)
			joined := strings.Join(problems, "\n")
			if tc.want == "" {
				if len(problems) != 0 {
					t.Errorf("expected no problems, got:\n%s", joined)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("expected %q in:\n%s", tc.want, joined)
			}
		})
	}
}

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
	envDocsFile            = "docs/development/environment-variables.md"
	envDocsRequiredHeading = "## Required variables"
	envDocsOptionalHeading = "## Optional variables"
	envDocsRequiredMarker  = "**Required**"
	envDocsComposeFile     = "README.md"
	envDocsComposeHeading  = "### Docker Compose での起動例"
	envDocsIgnore          = "//lint:ignore env-docs"
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

// findEnvDocProblems compares the variables read by code with the variable tables and the Compose example.
func findEnvDocProblems(reads []envRead, envDoc, composeDoc string) []string {
	required := parseEnvTable(markdownSection(envDoc, envDocsRequiredHeading))
	optional := parseEnvTable(markdownSection(envDoc, envDocsOptionalHeading))
	compose := markdownSection(composeDoc, envDocsComposeHeading)

	var problems []string
	readNames := map[string]bool{}
	for _, read := range reads {
		readNames[read.Name] = true
		if !required.Mentioned[read.Name] && !optional.Mentioned[read.Name] {
			problems = append(problems, fmt.Sprintf("%s: %s is not documented; add it to %q or %q in %s", read.Location, read.Name, envDocsOptionalHeading, envDocsRequiredHeading, envDocsFile))
		}
	}
	for _, table := range []envTable{required, optional} {
		for name := range table.Mentioned {
			if !readNames[name] {
				problems = append(problems, fmt.Sprintf("%s: %s is documented but production code does not read it", envDocsFile, name))
			}
		}
	}
	for name, row := range required.Rows {
		if !strings.Contains(row, envDocsRequiredMarker) {
			problems = append(problems, fmt.Sprintf("%s: optional variable %s belongs in %q", envDocsFile, name, envDocsOptionalHeading))
		}
		if !strings.Contains(compose, name+":") {
			problems = append(problems, fmt.Sprintf("%s: required variable %s is missing from the Compose example", envDocsComposeFile, name))
		}
		if _, duplicated := optional.Rows[name]; duplicated {
			problems = append(problems, fmt.Sprintf("%s: %s is listed in both %q and %q; keep one entry", envDocsFile, name, envDocsRequiredHeading, envDocsOptionalHeading))
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
	envDoc, err := os.ReadFile(filepath.Join(repoRoot, envDocsFile))
	if err != nil {
		t.Fatal(err)
	}
	composeDoc, err := os.ReadFile(filepath.Join(repoRoot, envDocsComposeFile))
	if err != nil {
		t.Fatal(err)
	}
	problems = append(problems, findEnvDocProblems(reads, string(envDoc), string(composeDoc))...)
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
	envDoc := func(required, optional string) string {
		return "# Environment Variables\n\n" + envDocsRequiredHeading + "\n\n" + required + "\n\n" + envDocsOptionalHeading + "\n\n" + optional + "\n"
	}
	const requiredRow = "| `REQ` | **Required** | Alias `REQ_ALIAS` | `x` |"
	const optionalRow = "| `OPT` | Optional | `1` |"
	reads := []envRead{{Name: "REQ", Location: "a.go:1"}, {Name: "REQ_ALIAS", Location: "a.go:2"}, {Name: "OPT", Location: "b.go:3"}}

	for _, tc := range []struct {
		name    string
		reads   []envRead
		envDoc  string
		compose string
		want    string
	}{
		{name: "complete", reads: reads, envDoc: envDoc(requiredRow, optionalRow), compose: compose},
		{name: "undocumented read names the location", reads: reads, envDoc: envDoc(requiredRow, ""), compose: compose, want: "b.go:3: OPT is not documented"},
		{name: "undocumented alias", reads: reads, envDoc: envDoc("| `REQ` | **Required** | x | `x` |", optionalRow), compose: compose, want: "a.go:2: REQ_ALIAS is not documented"},
		{name: "stale entry", reads: reads, envDoc: envDoc(requiredRow, optionalRow+"\n| `OLD` | Gone | `1` |"), compose: compose, want: "OLD is documented but production code does not read it"},
		{name: "stale alias", reads: reads[:2], envDoc: envDoc(requiredRow, "| `REQ_ALIAS` | x | `1` |\n"+optionalRow), compose: compose, want: "OPT is documented but production code does not read it"},
		{name: "optional row in required table", reads: []envRead{{Name: "REQ", Location: "a.go:1"}, {Name: "OPT", Location: "b.go:3"}}, envDoc: envDoc("| `REQ` | **Required** | x | `x` |\n| `OPT` | Optional | x | `1` |", ""), compose: compose, want: "optional variable OPT belongs in"},
		{name: "required missing from Compose", reads: reads, envDoc: envDoc(requiredRow, optionalRow), compose: envDocsComposeHeading + "\n", want: "required variable REQ is missing from the Compose example"},
		{name: "listed in both tables", reads: reads, envDoc: envDoc(requiredRow, "| `REQ` | x | `1` |\n"+optionalRow), compose: compose, want: "REQ is listed in both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := findEnvDocProblems(tc.reads, tc.envDoc, tc.compose)
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

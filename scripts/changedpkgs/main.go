// Command changedpkgs reads changed repository-relative paths from stdin and
// prints the go test patterns they can affect, one per line. Anything it cannot
// map selects the full suite, with the reason on stderr.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const fullSuite = "./..."

// scannedRoots are top-level directories outside any package whose files only
// matter to code naming them in a string literal. Other non-package paths, such
// as go.mod, compose.yaml or migrations/, select the full suite.
var scannedRoots = []string{".agents", ".arch", ".github", ".githooks", "assets", "docs"}

// selectionLogic lists paths that define this selection, whose impact it cannot judge.
var selectionLogic = []string{"scripts/changedpkgs/", "scripts/check_changed.sh"}

// goPackage merges a package with its test variants from `go list -test`.
type goPackage struct {
	importPath string
	dir        string   // relative to the repository root
	deps       []string // transitive imports, including those of its tests
	sources    []string
}

func main() {
	if err := run(os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "changedpkgs:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out, diag io.Writer) error {
	var changed []string
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			changed = append(changed, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	packages, err := loadPackages()
	if err != nil {
		return err
	}
	literals, err := indexPathLiterals(packages)
	if err != nil {
		return err
	}
	patterns, reasons := selectPatterns(changed, packages, literals)
	for _, reason := range reasons {
		fmt.Fprintln(diag, "==> [check-changed] Full suite:", reason)
	}
	for _, pattern := range patterns {
		fmt.Fprintln(out, pattern)
	}
	return nil
}

// loadPackages lists every package. A test binary's Deps already contain the
// transitive imports of its test files, so no separate graph walk is needed.
func loadPackages() ([]*goPackage, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("go", "list", "-test", "-json=ImportPath,Dir,ForTest,Deps,GoFiles,TestGoFiles,XTestGoFiles", "./...")
	cmd.Stderr = os.Stderr
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	byImportPath := map[string]*goPackage{}
	var packages []*goPackage
	decoder := json.NewDecoder(output)
	for {
		var entry struct {
			ImportPath, Dir, ForTest                 string
			Deps, GoFiles, TestGoFiles, XTestGoFiles []string
		}
		if err := decoder.Decode(&entry); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		owner := entry.ForTest
		if owner == "" {
			owner = strings.TrimSuffix(entry.ImportPath, ".test")
		}
		pkg := byImportPath[owner]
		if pkg == nil {
			pkg = &goPackage{importPath: owner}
			byImportPath[owner] = pkg
			packages = append(packages, pkg)
		}
		for _, dep := range entry.Deps {
			name, _, _ := strings.Cut(dep, " [") // "x [y.test]" is x recompiled for y's test.
			pkg.deps = append(pkg.deps, name)
		}
		if entry.ImportPath == owner {
			if pkg.dir, err = filepath.Rel(root, entry.Dir); err != nil {
				return nil, err
			}
			for _, file := range slices.Concat(entry.GoFiles, entry.TestGoFiles, entry.XTestGoFiles) {
				pkg.sources = append(pkg.sources, filepath.Join(entry.Dir, file))
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	return packages, nil
}

// indexPathLiterals maps each slash-separated segment of the string literals in
// Go source to the packages using it. Tests name the files they read this way,
// for example filepath.Join(root, "docs", "api", "openapi.json").
func indexPathLiterals(packages []*goPackage) (map[string][]string, error) {
	index := map[string][]string{}
	for _, pkg := range packages {
		seen := map[string]bool{}
		for _, source := range pkg.sources {
			file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				for _, segment := range strings.Split(value, "/") {
					if !seen[segment] {
						seen[segment] = true
						index[segment] = append(index[segment], pkg.importPath)
					}
				}
				return true
			})
		}
	}
	return index, nil
}

// selectPatterns returns the packages to test, or the full suite and its reasons.
func selectPatterns(changed []string, packages []*goPackage, literals map[string][]string) ([]string, []string) {
	byDir := map[string]string{}
	for _, pkg := range packages {
		byDir[pkg.dir] = pkg.importPath
	}

	seeds := map[string]bool{}
	var reasons []string
	for _, file := range changed {
		owners, reason := ownersOf(file, byDir, literals)
		if reason != "" {
			reasons = append(reasons, reason)
		}
		for _, owner := range owners {
			seeds[owner] = true
		}
	}
	if len(reasons) > 0 {
		return []string{fullSuite}, reasons
	}

	var patterns []string
	for _, pkg := range packages {
		if seeds[pkg.importPath] || slices.ContainsFunc(pkg.deps, func(dep string) bool { return seeds[dep] }) {
			patterns = append(patterns, "./"+pkg.dir)
		}
	}
	slices.Sort(patterns)
	return patterns, nil
}

// ownersOf returns the packages directly affected by file, or why they are unknown.
func ownersOf(file string, byDir map[string]string, literals map[string][]string) ([]string, string) {
	for _, logic := range selectionLogic {
		if strings.HasPrefix(file, logic) {
			return nil, file + " is part of the selection logic"
		}
	}

	if strings.HasSuffix(file, ".go") {
		if owner, ok := byDir[path.Dir(file)]; ok {
			return []string{owner}, ""
		}
		return nil, file + " is not in a current package"
	}

	// go:embed patterns cannot use "..", so an embedded file's owner is always an
	// ancestor directory. This also covers embedded files that were deleted.
	owners := slices.Clone(literals[path.Base(file)])
	inPackage := false
	for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
		if owner, ok := byDir[dir]; ok {
			owners = append(owners, owner)
			inPackage = true
		}
	}

	root, _, nested := strings.Cut(file, "/")
	switch {
	case inPackage, !nested && strings.HasSuffix(file, ".md"):
		return owners, ""
	case nested && slices.Contains(scannedRoots, root):
		return append(owners, literals[root]...), ""
	}
	return nil, file + " cannot be mapped to packages"
}

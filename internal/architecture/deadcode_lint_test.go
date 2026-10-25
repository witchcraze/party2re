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
	"time"
)

// knownLegacyOrphanedMethods was previously used to document legacy orphaned methods.
// All legacy orphaned methods have been eliminated (#428). This map must remain empty (Issue #438).
// Adding any entry to this map is permanently prohibited and guarded by TestDeadCodeZeroWhitelistRatchet.
var knownLegacyOrphanedMethods = map[string]string{}

// verifyZeroWhitelist returns an error if the whitelist contains any entries.
func verifyZeroWhitelist(whitelist map[string]string) error {
	if len(whitelist) > 0 {
		return fmt.Errorf("knownLegacyOrphanedMethods must remain empty (found %d entries); whitelisting is permanently prohibited (Issue #438)", len(whitelist))
	}
	return nil
}

type methodTarget struct {
	Key      string // e.g. "casino.Service.PlayIndianPokerRound"
	Pkg      string
	Type     string
	Method   string
	FilePath string
	Line     int
}

// collectTargetMethods inspects internal/ production code and returns exported methods
// on Service structs and Repository interfaces.
func collectTargetMethods(repoRoot string) ([]methodTarget, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	fset := token.NewFileSet()
	var targets []methodTarget

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip internal/architecture and test utilities
		if strings.Contains(path, "internal/architecture") || strings.Contains(path, "internal/testutil") || strings.Contains(path, "internal/database/testutil") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		node, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			return nil
		}

		pkgName := node.Name.Name

		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				// Methods on *Service or Service
				if d.Recv == nil || len(d.Recv.List) == 0 || !d.Name.IsExported() {
					continue
				}
				var recvName string
				switch t := d.Recv.List[0].Type.(type) {
				case *ast.StarExpr:
					if id, ok := t.X.(*ast.Ident); ok {
						recvName = id.Name
					}
				case *ast.Ident:
					recvName = t.Name
				}
				if recvName == "Service" {
					pos := fset.Position(d.Pos())
					key := fmt.Sprintf("%s.%s.%s", pkgName, recvName, d.Name.Name)
					targets = append(targets, methodTarget{
						Key:      key,
						Pkg:      pkgName,
						Type:     recvName,
						Method:   d.Name.Name,
						FilePath: path,
						Line:     pos.Line,
					})
				}

			case *ast.GenDecl:
				// Exported methods on Repository interfaces
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					typeName := ts.Name.Name
					if !strings.HasSuffix(typeName, "Repository") && !strings.HasSuffix(typeName, "Repo") && typeName != "Repository" {
						continue
					}
					iface, ok := ts.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}
					for _, field := range iface.Methods.List {
						if len(field.Names) == 0 || !field.Names[0].IsExported() {
							continue
						}
						mName := field.Names[0].Name
						pos := fset.Position(field.Pos())
						key := fmt.Sprintf("%s.%s.%s", pkgName, typeName, mName)
						targets = append(targets, methodTarget{
							Key:      key,
							Pkg:      pkgName,
							Type:     typeName,
							Method:   mName,
							FilePath: path,
							Line:     pos.Line,
						})
					}
				}
			}
		}
		return nil
	})

	return targets, err
}

// hasCaller checks if method is called or referenced in repoFiles outside its declaration.
func hasCaller(target methodTarget, repoFiles map[string][]byte) bool {
	methodBytes := []byte(target.Method)
	fset := token.NewFileSet()

	for filePath, src := range repoFiles {
		// Fast path: skip file if method name does not occur in byte stream
		if !bytes.Contains(src, methodBytes) {
			continue
		}

		node, err := parser.ParseFile(fset, filePath, src, 0)
		if err != nil {
			continue
		}

		found := false
		ast.Inspect(node, func(n ast.Node) bool {
			if found {
				return false
			}
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != target.Method {
				return true
			}
			pos := fset.Position(sel.Pos())
			// Ignore the definition site itself
			if pos.Filename == target.FilePath && pos.Line == target.Line {
				return true
			}
			found = true
			return false
		})

		if found {
			return true
		}
	}

	return false
}

// collectRepoFiles walks repoRoot and reads all .go files into memory,
// skipping ignored directories (.git, vendor, .cache).
func collectRepoFiles(repoRoot string) (map[string][]byte, error) {
	repoFiles := make(map[string][]byte)
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			if rel == ".git" || rel == "vendor" || rel == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		repoFiles[path] = data
		return nil
	})
	return repoFiles, err
}

func findOrphanedMethods(repoRoot string, targets []methodTarget, repoFiles map[string][]byte) []string {
	var unhandledOrphans []string
	for _, target := range targets {
		if hasCaller(target, repoFiles) {
			continue
		}

		relPath, _ := filepath.Rel(repoRoot, target.FilePath)
		unhandledOrphans = append(unhandledOrphans, fmt.Sprintf("%s (%s:%d)", target.Key, relPath, target.Line))
	}
	return unhandledOrphans
}

func TestDeadCodeOrphanedMethods(t *testing.T) {
	repoRoot := "../.."
	start := time.Now()

	// Issue #438: Strict zero-tolerance ratchet. Whitelist must remain empty.
	if err := verifyZeroWhitelist(knownLegacyOrphanedMethods); err != nil {
		t.Errorf("zero-whitelist ratchet violation: %v", err)
	}

	targets, err := collectTargetMethods(repoRoot)
	if err != nil {
		t.Fatalf("failed to collect target methods: %v", err)
	}

	// Pre-load all .go files across repository into memory once for fast byte matching
	repoFiles, err := collectRepoFiles(repoRoot)
	if err != nil {
		t.Fatalf("failed to read repository files: %v", err)
	}

	unhandledOrphans := findOrphanedMethods(repoRoot, targets, repoFiles)

	if len(unhandledOrphans) > 0 {
		t.Errorf("Detected %d orphaned method(s) with zero callers across repository:\n  - %s\nMethods must be referenced or deleted (prohibited by .agents/rules/01-development-workflow.md Section 4).",
			len(unhandledOrphans), strings.Join(unhandledOrphans, "\n  - "))
	}

	t.Logf("Dead code linter checked %d methods across %d Go files in %s (orphans: %d, known: %d)",
		len(targets), len(repoFiles), time.Since(start), len(unhandledOrphans), len(knownLegacyOrphanedMethods))
}

func TestDeadCodeZeroWhitelistRatchet(t *testing.T) {
	if err := verifyZeroWhitelist(knownLegacyOrphanedMethods); err != nil {
		t.Fatalf("ratchet violation: %v", err)
	}
}

func TestDeadCodeZeroWhitelistRatchetRejectsAdditions(t *testing.T) {
	mockWhitelist := map[string]string{
		"sample.Service.LegacyMethod": "prohibited bypass entry",
	}
	if err := verifyZeroWhitelist(mockWhitelist); err == nil {
		t.Fatal("expected verifyZeroWhitelist to return error for non-empty whitelist, got nil")
	}
}

func TestDeadCodeDetectorDetectsOrphan(t *testing.T) {
	// Synthetic unit test validating that hasCaller correctly distinguishes between
	// orphaned methods and called methods for Service structs.
	target := methodTarget{
		Key:      "sample.Service.DoAction",
		Pkg:      "sample",
		Type:     "Service",
		Method:   "DoAction",
		FilePath: "sample.go",
		Line:     10,
	}

	// Case 1: Only the declaration exists
	declOnlyFiles := map[string][]byte{
		"sample.go": []byte("package sample\n\nfunc (s *Service) DoAction() {}\n"),
	}
	if hasCaller(target, declOnlyFiles) {
		t.Error("expected hasCaller to return false when only declaration exists")
	}

	// Case 2: A caller exists in another file
	withCallerFiles := map[string][]byte{
		"sample.go": []byte("package sample\n\nfunc (s *Service) DoAction() {}\n"),
		"main.go":   []byte("package main\n\nfunc run(s *Service) { s.DoAction() }\n"),
	}
	if !hasCaller(target, withCallerFiles) {
		t.Error("expected hasCaller to return true when caller exists")
	}

	// Case 3: Repository interface method with no callers
	repoTarget := methodTarget{
		Key:      "sample.ItemRepository.FindByID",
		Pkg:      "sample",
		Type:     "ItemRepository",
		Method:   "FindByID",
		FilePath: "sample_repo.go",
		Line:     5,
	}
	repoDeclOnlyFiles := map[string][]byte{
		"sample_repo.go": []byte("package sample\n\ntype ItemRepository interface {\n\tFindByID(id string) error\n}\n"),
	}
	if hasCaller(repoTarget, repoDeclOnlyFiles) {
		t.Error("expected hasCaller to return false when only interface method declaration exists")
	}

	// Case 4: Repository interface method with caller
	repoWithCallerFiles := map[string][]byte{
		"sample_repo.go": []byte("package sample\n\ntype ItemRepository interface {\n\tFindByID(id string) error\n}\n"),
		"service.go":     []byte("package sample\n\nfunc (s *Service) Get(r ItemRepository, id string) { r.FindByID(id) }\n"),
	}
	if !hasCaller(repoTarget, repoWithCallerFiles) {
		t.Error("expected hasCaller to return true when interface caller exists")
	}
}

func TestDeadCodeRepoWalkExcludesIgnoredDirs(t *testing.T) {
	tmpDir := t.TempDir()

	dirs := []string{
		filepath.Join(tmpDir, "internal", "sample"),
		filepath.Join(tmpDir, "cmd", "app"),
		filepath.Join(tmpDir, "scripts"),
		filepath.Join(tmpDir, ".cache"),
		filepath.Join(tmpDir, ".git"),
		filepath.Join(tmpDir, "vendor"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	serviceSrc := `package sample

type Service struct{}

func (s *Service) CalledInCmd() {}
func (s *Service) CalledInScript() {}
func (s *Service) CalledInTest() {}
func (s *Service) CacheHidden() {}
func (s *Service) RealOrphan() {}

type ItemRepository interface {
	CalledRepoMethod() error
	RealOrphanRepoMethod() error
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "sample", "service.go"), []byte(serviceSrc), 0644); err != nil {
		t.Fatalf("failed to write service.go: %v", err)
	}

	cmdSrc := `package main

import "sample"

func run(s *sample.Service, r sample.ItemRepository) {
	s.CalledInCmd()
	_ = r.CalledRepoMethod()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "cmd", "app", "main.go"), []byte(cmdSrc), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	scriptSrc := `package main

import "sample"

func runScript(s *sample.Service) {
	s.CalledInScript()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "scripts", "worker.go"), []byte(scriptSrc), 0644); err != nil {
		t.Fatalf("failed to write worker.go: %v", err)
	}

	testSrc := `package sample_test

import (
	"sample"
	"testing"
)

func TestSample(t *testing.T) {
	var s sample.Service
	s.CalledInTest()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "sample", "service_test.go"), []byte(testSrc), 0644); err != nil {
		t.Fatalf("failed to write service_test.go: %v", err)
	}

	// Ignored directory contents:
	// 1. .cache/probe.go referencing CacheHidden: must NOT count as caller
	cacheSrc := `package cache

import "sample"

func cached(s *sample.Service) {
	s.CacheHidden()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".cache", "probe.go"), []byte(cacheSrc), 0644); err != nil {
		t.Fatalf("failed to write cache probe.go: %v", err)
	}

	// 2. Dangling symlinks in .cache, .git, vendor: deterministic reproduction of vanished build artifacts
	for _, dir := range []string{".cache", ".git", "vendor"} {
		symlinkPath := filepath.Join(tmpDir, dir, "dangling.go")
		if err := os.Symlink("already-removed.go", symlinkPath); err != nil {
			t.Logf("symlink not supported on this filesystem: %v", err)
		}
	}

	targets, err := collectTargetMethods(tmpDir)
	if err != nil {
		t.Fatalf("collectTargetMethods failed: %v", err)
	}
	if len(targets) != 7 {
		t.Fatalf("expected 7 target methods, got %d", len(targets))
	}

	repoFiles, err := collectRepoFiles(tmpDir)
	if err != nil {
		t.Fatalf("collectRepoFiles failed on directory with ignored dirs: %v", err)
	}

	// Verify ignored directories are completely absent from collected files
	for filePath := range repoFiles {
		rel, _ := filepath.Rel(tmpDir, filePath)
		relSlash := filepath.ToSlash(rel)
		if strings.HasPrefix(relSlash, ".cache/") || strings.HasPrefix(relSlash, ".git/") || strings.HasPrefix(relSlash, "vendor/") {
			t.Errorf("repoFiles contains file from ignored directory: %s", relSlash)
		}
	}

	orphans := findOrphanedMethods(tmpDir, targets, repoFiles)

	orphanSet := make(map[string]bool)
	for _, o := range orphans {
		orphanSet[o] = true
	}

	// Must detect RealOrphan, CacheHidden, RealOrphanRepoMethod
	for _, expected := range []string{"RealOrphan", "CacheHidden", "RealOrphanRepoMethod"} {
		found := false
		for o := range orphanSet {
			if strings.Contains(o, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be detected as orphan, but it was not; detected orphans: %v", expected, orphans)
		}
	}

	// Must NOT detect CalledInCmd, CalledInScript, CalledInTest, CalledRepoMethod
	for _, notExpected := range []string{"CalledInCmd", "CalledInScript", "CalledInTest", "CalledRepoMethod"} {
		for o := range orphanSet {
			if strings.Contains(o, notExpected) {
				t.Errorf("expected %s to have a caller, but detected as orphan: %s", notExpected, o)
			}
		}
	}
}

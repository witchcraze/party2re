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

// knownLegacyOrphanedMethods documents existing methods without active callers
// that are preserved for backward compatibility or scheduled for cleanup in dedicated domain issues (e.g. #287).
// No new orphaned methods may be added to this list.
var knownLegacyOrphanedMethods = map[string]string{
	"casino.Service.PlayIndianPokerRound":                 "superseded by PlayIndianPokerAction in PR #397; cleanup tracked in #287",
	"casino.Service.SetTransactionProvider":               "unused transaction setter; cleanup tracked in #287",
	"helper.Service.SetRandomSource":                      "unused test setter; cleanup tracked in #287",
	"boss.Service.GetCharacterRecord":                     "unused record query; cleanup tracked in #287",
	"guild.Service.GetByCharacter":                        "unused query; cleanup tracked in #287",
	"guild.Service.Disband":                               "superseded; cleanup tracked in #287",
	"medal.Service.GetAchievementCatalog":                 "unused catalog query; cleanup tracked in #287",
	"notification.Service.PruneExpired":                   "unused background pruning method; cleanup tracked in #287",
	"contest.ContestRepository.GetRoundByNumberForUpdate": "unused repo method; cleanup tracked in #287",
	"contest.ContestRepository.FindPhotoByIDForUpdate":    "unused repo method; cleanup tracked in #287",
	"contest.ContestRepository.ListVotesByRound":          "unused repo method; cleanup tracked in #287",
	"contest.ContestRepository.CountEntriesByRound":       "unused repo method; cleanup tracked in #287",
	"challenge.Repository.SaveRecord":                     "unused repo method; cleanup tracked in #287",
	"collection.Repository.GetMonsterBookCount":           "unused repo method; cleanup tracked in #287",
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

func TestDeadCodeOrphanedMethods(t *testing.T) {
	repoRoot := "../.."
	start := time.Now()

	targets, err := collectTargetMethods(repoRoot)
	if err != nil {
		t.Fatalf("failed to collect target methods: %v", err)
	}

	// Pre-load all .go files across repository into memory once for fast byte matching
	repoFiles := make(map[string][]byte)
	err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "vendor") || strings.Contains(path, ".git") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		repoFiles[path] = data
		return nil
	})
	if err != nil {
		t.Fatalf("failed to read repository files: %v", err)
	}

	var unhandledOrphans []string

	for _, target := range targets {
		if hasCaller(target, repoFiles) {
			continue
		}

		// Check if known legacy orphan
		if _, known := knownLegacyOrphanedMethods[target.Key]; known {
			continue
		}

		relPath, _ := filepath.Rel(repoRoot, target.FilePath)
		unhandledOrphans = append(unhandledOrphans, fmt.Sprintf("%s (%s:%d)", target.Key, relPath, target.Line))
	}

	if len(unhandledOrphans) > 0 {
		t.Errorf("Detected %d orphaned method(s) with zero callers across repository:\n  - %s\nMethods must be referenced or deleted (prohibited by .agents/rules/01-development-workflow.md Section 4).",
			len(unhandledOrphans), strings.Join(unhandledOrphans, "\n  - "))
	}

	t.Logf("Dead code linter checked %d methods across %d Go files in %s (orphans: %d, known: %d)",
		len(targets), len(repoFiles), time.Since(start), len(unhandledOrphans), len(knownLegacyOrphanedMethods))
}

func TestDeadCodeDetectorDetectsOrphan(t *testing.T) {
	// Synthetic unit test validating that hasCaller correctly distinguishes between
	// orphaned methods and called methods.
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
}

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

// knownLegacyUnusedConstants documents existing top-level constants without active references
// preserved for schema completeness, future domain expansion, or scheduled for cleanup in domain issues.
// No new unused constants may be added to this map without explicit review;
// use //lint:ignore <reason> directly on the definition for intentional schema compatibility.
var knownLegacyUnusedConstants = map[string]string{
	"replay.CombatTypeGvG":                       "reserved for future GvG combat replay recording",
	"replay.CombatTypeDungeon":                   "reserved for future dungeon exploration replay recording",
	"fleamarket.MaxGoldCap":                      "flea market balance constant; candidate for cleanup",
	"notification.MaxCategoryLen":                "schema constraint constant for news categories",
	"notification.MaxAuthorLen":                  "schema constraint constant for news author",
	"notification.MaxLinkLen":                    "schema constraint constant for news link",
	"notification.CategoryMaintenance":           "pre-defined news category",
	"notification.CategoryEvent":                 "pre-defined news category",
	"notification.CategoryMilestone":             "pre-defined news category",
	"notification.NotificationCategoryGuild":     "pre-defined notification category",
	"notification.NotificationCategoryAdventure": "pre-defined notification category",
	"notification.NotificationCategoryGift":      "pre-defined notification category",
	"notification.NotificationCategoryReward":    "pre-defined notification category",
}

// knownLegacyUnusedStructFields documents existing DTO/API struct fields without active readers/writers.
// No new unused struct fields may be added without review; use //lint:ignore on the field for OpenAPI compatibility.
var knownLegacyUnusedStructFields = map[string]string{}

type constTarget struct {
	Key       string // e.g. "fleamarket.MaxGoldCap"
	Pkg       string
	Name      string
	FilePath  string
	Line      int
	Exported  bool
	HasOptOut bool
}

type fieldTarget struct {
	Key        string // e.g. "http.CreateAPITokenRequest.Name"
	Pkg        string
	StructName string
	FieldName  string
	FilePath   string
	Line       int
	Exported   bool
	HasOptOut  bool
}

func hasOptOutComment(comments ...*ast.CommentGroup) bool {
	for _, cg := range comments {
		if cg == nil {
			continue
		}
		for _, c := range cg.List {
			if strings.Contains(c.Text, "lint:ignore") || strings.Contains(c.Text, "nolint") {
				return true
			}
		}
	}
	return false
}

// collectTargetConstants traverses internal/ production Go files and returns top-level const definitions.
func collectTargetConstants(fileASTs map[string]*ast.File, fset *token.FileSet) []constTarget {
	var targets []constTarget

	for path, node := range fileASTs {
		if !strings.HasPrefix(path, "internal/") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		if strings.HasPrefix(path, "internal/architecture") ||
			strings.HasPrefix(path, "internal/testutil") ||
			strings.HasPrefix(path, "internal/database/testutil") {
			continue
		}

		pkgName := node.Name.Name
		for _, decl := range node.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}

			blockOptOut := hasOptOutComment(gd.Doc)

			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				specOptOut := blockOptOut || hasOptOutComment(vs.Doc, vs.Comment)

				for _, name := range vs.Names {
					if name.Name == "_" {
						continue
					}
					pos := fset.Position(name.Pos())
					key := fmt.Sprintf("%s.%s", pkgName, name.Name)
					targets = append(targets, constTarget{
						Key:       key,
						Pkg:       pkgName,
						Name:      name.Name,
						FilePath:  path,
						Line:      pos.Line,
						Exported:  name.IsExported(),
						HasOptOut: specOptOut,
					})
				}
			}
		}
	}

	return targets
}

// collectTargetDTOFields inspects struct definitions in internal/api/http and domain DTOs.
func collectTargetDTOFields(fileASTs map[string]*ast.File, fset *token.FileSet) []fieldTarget {
	var targets []fieldTarget

	for path, node := range fileASTs {
		if !strings.HasPrefix(path, "internal/") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		if strings.HasPrefix(path, "internal/architecture") ||
			strings.HasPrefix(path, "internal/testutil") ||
			strings.HasPrefix(path, "internal/database/testutil") {
			continue
		}

		pkgName := node.Name.Name
		isHTTP := strings.HasPrefix(path, "internal/api/http")

		for _, decl := range node.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}

			blockOptOut := hasOptOutComment(gd.Doc)

			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				typeName := ts.Name.Name
				isDTO := isHTTP ||
					strings.HasSuffix(typeName, "Request") ||
					strings.HasSuffix(typeName, "Response") ||
					strings.HasSuffix(typeName, "DTO") ||
					strings.HasSuffix(typeName, "Params") ||
					strings.HasSuffix(typeName, "Input") ||
					strings.HasSuffix(typeName, "Output")

				if !isDTO {
					continue
				}

				typeOptOut := blockOptOut || hasOptOutComment(ts.Doc, ts.Comment)

				for _, field := range st.Fields.List {
					fieldOptOut := typeOptOut || hasOptOutComment(field.Doc, field.Comment)

					for _, fName := range field.Names {
						if fName.Name == "_" {
							continue
						}
						pos := fset.Position(fName.Pos())
						key := fmt.Sprintf("%s.%s.%s", pkgName, typeName, fName.Name)
						targets = append(targets, fieldTarget{
							Key:        key,
							Pkg:        pkgName,
							StructName: typeName,
							FieldName:  fName.Name,
							FilePath:   path,
							Line:       pos.Line,
							Exported:   fName.IsExported(),
							HasOptOut:  fieldOptOut,
						})
					}
				}
			}
		}
	}

	return targets
}

// hasConstReference checks if a constant is referenced anywhere in repoFiles outside its declaration.
func hasConstReference(target constTarget, repoFiles map[string][]byte, fileASTs map[string]*ast.File, fset *token.FileSet) bool {
	nameBytes := []byte(target.Name)
	targetDir := filepath.Dir(target.FilePath)

	for p, data := range repoFiles {
		if !bytes.Contains(data, nameBytes) {
			continue
		}

		// Unexported constants cannot be referenced outside their package directory
		if !target.Exported && filepath.Dir(p) != targetDir {
			continue
		}

		astNode := fileASTs[p]
		if astNode == nil {
			continue
		}

		found := false
		ast.Inspect(astNode, func(n ast.Node) bool {
			if found {
				return false
			}

			id, ok := n.(*ast.Ident)
			if !ok || id.Name != target.Name {
				return true
			}

			pos := fset.Position(id.Pos())
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

// hasFieldReference checks if a struct field is read or written anywhere in repoFiles outside its declaration.
func hasFieldReference(target fieldTarget, repoFiles map[string][]byte, fileASTs map[string]*ast.File, fset *token.FileSet) bool {
	fieldBytes := []byte(target.FieldName)

	for p, data := range repoFiles {
		if !bytes.Contains(data, fieldBytes) {
			continue
		}

		astNode := fileASTs[p]
		if astNode == nil {
			continue
		}

		found := false
		ast.Inspect(astNode, func(n ast.Node) bool {
			if found {
				return false
			}

			// Case 1: SelectorExpr (e.g. req.FieldName or resp.FieldName)
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == target.FieldName {
				pos := fset.Position(sel.Sel.Pos())
				if pos.Filename == target.FilePath && pos.Line == target.Line {
					return true
				}
				found = true
				return false
			}

			// Case 2: KeyValueExpr in composite literal (e.g. MyDTO{ FieldName: val })
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == target.FieldName {
					pos := fset.Position(id.Pos())
					if pos.Filename == target.FilePath && pos.Line == target.Line {
						return true
					}
					found = true
					return false
				}
			}

			return true
		})

		if found {
			return true
		}
	}

	return false
}

// loadRepoFiles reads and parses all .go files across the repository once.
func loadRepoFiles(repoRoot string, fset *token.FileSet) (map[string][]byte, map[string]*ast.File, error) {
	repoFiles := make(map[string][]byte)
	fileASTs := make(map[string]*ast.File)

	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
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
		// Store normalized relative path
		relPath, _ := filepath.Rel(repoRoot, path)
		cleanPath := filepath.ToSlash(relPath)

		repoFiles[cleanPath] = data
		node, parseErr := parser.ParseFile(fset, cleanPath, data, parser.ParseComments)
		if parseErr == nil {
			fileASTs[cleanPath] = node
		}
		return nil
	})

	return repoFiles, fileASTs, err
}

func TestUnusedDefinitions(t *testing.T) {
	repoRoot := "../.."
	start := time.Now()
	fset := token.NewFileSet()

	repoFiles, fileASTs, err := loadRepoFiles(repoRoot, fset)
	if err != nil {
		t.Fatalf("failed to load repository files: %v", err)
	}

	t.Run("Constants", func(t *testing.T) {
		constTargets := collectTargetConstants(fileASTs, fset)
		var unhandledConsts []string

		for _, target := range constTargets {
			if target.HasOptOut {
				continue
			}
			if _, known := knownLegacyUnusedConstants[target.Key]; known {
				continue
			}

			if hasConstReference(target, repoFiles, fileASTs, fset) {
				continue
			}

			unhandledConsts = append(unhandledConsts, fmt.Sprintf("%s (%s:%d)", target.Key, target.FilePath, target.Line))
		}

		if len(unhandledConsts) > 0 {
			t.Errorf("Detected %d unused top-level constant(s) with zero references across repository:\n  - %s\nConstants must be referenced or deleted, or annotated with //lint:ignore <reason> (.agents/rules/01-development-workflow.md Section 4).",
				len(unhandledConsts), strings.Join(unhandledConsts, "\n  - "))
		}

		t.Logf("Checked %d top-level constants across %d Go files (unused: %d, legacy exempt: %d)",
			len(constTargets), len(repoFiles), len(unhandledConsts), len(knownLegacyUnusedConstants))
	})

	t.Run("DTOStructFields", func(t *testing.T) {
		fieldTargets := collectTargetDTOFields(fileASTs, fset)
		var unhandledFields []string

		for _, target := range fieldTargets {
			if target.HasOptOut {
				continue
			}
			if _, known := knownLegacyUnusedStructFields[target.Key]; known {
				continue
			}

			if hasFieldReference(target, repoFiles, fileASTs, fset) {
				continue
			}

			unhandledFields = append(unhandledFields, fmt.Sprintf("%s (%s:%d)", target.Key, target.FilePath, target.Line))
		}

		if len(unhandledFields) > 0 {
			t.Errorf("Detected %d unreferenced DTO/API struct field(s) with zero reads/writes across repository:\n  - %s\nFields must be referenced or deleted, or annotated with //lint:ignore <reason> for OpenAPI/wire compatibility (.agents/rules/01-development-workflow.md Section 4).",
				len(unhandledFields), strings.Join(unhandledFields, "\n  - "))
		}

		t.Logf("Checked %d DTO/API struct fields across %d Go files (unused: %d, legacy exempt: %d)",
			len(fieldTargets), len(repoFiles), len(unhandledFields), len(knownLegacyUnusedStructFields))
	})

	elapsed := time.Since(start)
	t.Logf("TestUnusedDefinitions completed in %v (< 2.0s budget)", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("TestUnusedDefinitions took %v, exceeding the 2.0s limit", elapsed)
	}
}

func TestUnusedDefinitionsDetectsUnusedConst(t *testing.T) {
	fset := token.NewFileSet()

	// Case 1: Unreferenced constant
	declSrc := []byte("package sample\n\nconst UnusedConst = 42\n")
	fileA, err := parser.ParseFile(fset, "sample/sample.go", declSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	repoFiles := map[string][]byte{"sample/sample.go": declSrc}
	fileASTs := map[string]*ast.File{"sample/sample.go": fileA}

	target := constTarget{
		Key:      "sample.UnusedConst",
		Pkg:      "sample",
		Name:     "UnusedConst",
		FilePath: "sample/sample.go",
		Line:     3,
		Exported: true,
	}

	if hasConstReference(target, repoFiles, fileASTs, fset) {
		t.Error("expected hasConstReference to return false for unreferenced constant")
	}

	// Case 2: Reference exists in another file
	callSrc := []byte("package other\n\nimport \"sample\"\n\nvar x = sample.UnusedConst\n")
	fileB, err := parser.ParseFile(fset, "other/other.go", callSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	repoFiles["other/other.go"] = callSrc
	fileASTs["other/other.go"] = fileB

	if !hasConstReference(target, repoFiles, fileASTs, fset) {
		t.Error("expected hasConstReference to return true when reference exists")
	}
}

func TestUnusedDefinitionsDetectsUnusedField(t *testing.T) {
	fset := token.NewFileSet()

	// Case 1: Struct with unreferenced field
	dtoSrc := []byte("package sample\n\ntype MyDTO struct {\n\tUnusedField string\n}\n")
	fileA, err := parser.ParseFile(fset, "sample/dto.go", dtoSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	repoFiles := map[string][]byte{"sample/dto.go": dtoSrc}
	fileASTs := map[string]*ast.File{"sample/dto.go": fileA}

	target := fieldTarget{
		Key:        "sample.MyDTO.UnusedField",
		Pkg:        "sample",
		StructName: "MyDTO",
		FieldName:  "UnusedField",
		FilePath:   "sample/dto.go",
		Line:       4,
		Exported:   true,
	}

	if hasFieldReference(target, repoFiles, fileASTs, fset) {
		t.Error("expected hasFieldReference to return false for unreferenced field")
	}

	// Case 2: Referenced via SelectorExpr (e.g. dto.UnusedField)
	readSrc := []byte("package other\n\nfunc read(d *MyDTO) string { return d.UnusedField }\n")
	fileB, err := parser.ParseFile(fset, "other/read.go", readSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	repoFiles["other/read.go"] = readSrc
	fileASTs["other/read.go"] = fileB

	if !hasFieldReference(target, repoFiles, fileASTs, fset) {
		t.Error("expected hasFieldReference to return true when SelectorExpr reference exists")
	}

	// Case 3: Referenced via KeyValueExpr in composite literal (e.g. MyDTO{ UnusedField: "val" })
	delete(repoFiles, "other/read.go")
	delete(fileASTs, "other/read.go")

	litSrc := []byte("package other\n\nvar d = MyDTO{ UnusedField: \"val\" }\n")
	fileC, err := parser.ParseFile(fset, "other/lit.go", litSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	repoFiles["other/lit.go"] = litSrc
	fileASTs["other/lit.go"] = fileC

	if !hasFieldReference(target, repoFiles, fileASTs, fset) {
		t.Error("expected hasFieldReference to return true when KeyValueExpr reference exists")
	}
}

func TestUnusedDefinitionsRespectsOptOut(t *testing.T) {
	fset := token.NewFileSet()

	src := []byte(`package sample

//lint:ignore unused testing block opt-out
const (
	IgnoredByBlock = 1
)

const (
	//lint:ignore unused testing spec doc opt-out
	IgnoredBySpecDoc = 2
	IgnoredByInlineComment = 3 //lint:ignore unused testing inline comment
	NotIgnored = 4
)

//lint:ignore unused testing struct block opt-out
type IgnoredStructDTO struct {
	FieldA string
}

type RegularDTO struct {
	//lint:ignore unused testing field doc opt-out
	IgnoredField string
	IgnoredInlineField int //lint:ignore unused testing field inline opt-out
	ActiveField bool
}
`)

	node, err := parser.ParseFile(fset, "internal/sample/sample.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fileASTs := map[string]*ast.File{"internal/sample/sample.go": node}

	constTargets := collectTargetConstants(fileASTs, fset)
	constMap := make(map[string]bool)
	for _, ct := range constTargets {
		constMap[ct.Name] = ct.HasOptOut
	}

	if !constMap["IgnoredByBlock"] {
		t.Error("expected IgnoredByBlock to have HasOptOut = true")
	}
	if !constMap["IgnoredBySpecDoc"] {
		t.Error("expected IgnoredBySpecDoc to have HasOptOut = true")
	}
	if !constMap["IgnoredByInlineComment"] {
		t.Error("expected IgnoredByInlineComment to have HasOptOut = true")
	}
	if constMap["NotIgnored"] {
		t.Error("expected NotIgnored to have HasOptOut = false")
	}

	fieldTargets := collectTargetDTOFields(fileASTs, fset)
	fieldMap := make(map[string]bool)
	for _, ft := range fieldTargets {
		fieldMap[ft.StructName+"."+ft.FieldName] = ft.HasOptOut
	}

	if !fieldMap["IgnoredStructDTO.FieldA"] {
		t.Error("expected IgnoredStructDTO.FieldA to have HasOptOut = true")
	}
	if !fieldMap["RegularDTO.IgnoredField"] {
		t.Error("expected RegularDTO.IgnoredField to have HasOptOut = true")
	}
	if !fieldMap["RegularDTO.IgnoredInlineField"] {
		t.Error("expected RegularDTO.IgnoredInlineField to have HasOptOut = true")
	}
	if fieldMap["RegularDTO.ActiveField"] {
		t.Error("expected RegularDTO.ActiveField to have HasOptOut = false")
	}
}

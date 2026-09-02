package core_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type coreViolation struct {
	file    string
	line    int
	field   string
	message string
}

// Whitelisted packages/directories where direct Core field mutations or entity mappings are permitted.
var (
	allowedProgressionPaths = map[string]bool{
		"internal/core/progression": true,
		"internal/core/character":   true,
		"internal/core/battle":      true, // battle outcome reward definitions
		"internal/database":         true, // database SQL mappers & row scanning
	}

	allowedCurrencyPaths = map[string]bool{
		"internal/core/character": true,
		"internal/core/battle":    true, // battle outcome reward definitions
		"internal/database":       true, // database SQL mappers & row scanning
	}

	allowedJobPaths = map[string]bool{
		"internal/core/job": true,
		"internal/database": true, // database SQL mappers & row scanning
	}

	allowedInventoryPaths = map[string]bool{
		"internal/core/inventory": true,
		"internal/depot":          true, // depot storage items (Depot.Items)
		"internal/database":       true, // database SQL mappers & row scanning
	}

	allowedEquipmentPaths = map[string]bool{
		"internal/core/equipment": true,
		"internal/custom_skill":   true, // custom skill loadout slots (Loadout.Slots)
		"internal/database":       true, // database SQL mappers & row scanning
	}
)

func isPathAllowed(relPath string, allowedMap map[string]bool) bool {
	normalized := filepath.ToSlash(relPath)
	for allowed := range allowedMap {
		if strings.HasPrefix(normalized, allowed) {
			return true
		}
	}
	return false
}

func checkFileCoreRules(fset *token.FileSet, node *ast.File, filename string) []coreViolation {
	var violations []coreViolation

	isAllowedProgression := isPathAllowed(filename, allowedProgressionPaths)
	isAllowedCurrency := isPathAllowed(filename, allowedCurrencyPaths)
	isAllowedJob := isPathAllowed(filename, allowedJobPaths)
	isAllowedInventory := isPathAllowed(filename, allowedInventoryPaths)
	isAllowedEquipment := isPathAllowed(filename, allowedEquipmentPaths)

	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.IncDecStmt:
			if sel, ok := stmt.X.(*ast.SelectorExpr); ok {
				fieldName := sel.Sel.Name
				pos := fset.Position(stmt.Pos())

				// Progression (Experience, Level)
				if (fieldName == "Experience" || fieldName == "Level") && !isAllowedProgression {
					violations = append(violations, coreViolation{
						file:    filename,
						line:    pos.Line,
						field:   fieldName,
						message: "direct mutation of character progression field '" + fieldName + "' is prohibited; use progression.ApplyExperience or progression.Rebirth instead",
					})
				}

				// Currency (Money, SmallMedals)
				if fieldName == "Money" && !isAllowedCurrency {
					violations = append(violations, coreViolation{
						file:    filename,
						line:    pos.Line,
						field:   fieldName,
						message: "direct mutation of character currency field 'Money' is prohibited; use char.AddMoney or char.DeductMoney instead",
					})
				}
				if fieldName == "SmallMedals" && !isAllowedCurrency {
					violations = append(violations, coreViolation{
						file:    filename,
						line:    pos.Line,
						field:   fieldName,
						message: "direct mutation of character medal field 'SmallMedals' is prohibited; use char.AddSmallMedals or char.DeductSmallMedals instead",
					})
				}

				// Job (CurrentJobID, MasteredJobs)
				if (fieldName == "CurrentJobID" || fieldName == "MasteredJobs") && !isAllowedJob {
					violations = append(violations, coreViolation{
						file:    filename,
						line:    pos.Line,
						field:   fieldName,
						message: "direct mutation of job state field '" + fieldName + "' is prohibited; use CharacterJob.ChangeTo or CharacterJob.Master instead",
					})
				}
			}

		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				// 1. Direct selector assignment (e.g. c.Experience = ..., c.Money += ..., inv.Items = ...)
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					fieldName := sel.Sel.Name
					pos := fset.Position(stmt.Pos())

					// Progression
					if (fieldName == "Experience" || fieldName == "Level") && !isAllowedProgression {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation of character progression field '" + fieldName + "' is prohibited; use progression.ApplyExperience or progression.Rebirth instead",
						})
					}

					// Currency
					if fieldName == "Money" && !isAllowedCurrency {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation of character currency field 'Money' is prohibited; use char.AddMoney or char.DeductMoney instead",
						})
					}
					if fieldName == "SmallMedals" && !isAllowedCurrency {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation of character medal field 'SmallMedals' is prohibited; use char.AddSmallMedals or char.DeductSmallMedals instead",
						})
					}

					// Job
					if (fieldName == "CurrentJobID" || fieldName == "MasteredJobs") && !isAllowedJob {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation of job state field '" + fieldName + "' is prohibited; use CharacterJob.ChangeTo or CharacterJob.Master instead",
						})
					}

					// Inventory Items assignment
					if fieldName == "Items" && !isAllowedInventory {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation or slicing of Inventory.Items is prohibited; use Inventory.Add, Consume, or Update instead",
						})
					}

					// Equipment Slots assignment
					if fieldName == "Slots" && !isAllowedEquipment {
						violations = append(violations, coreViolation{
							file:    filename,
							line:    pos.Line,
							field:   fieldName,
							message: "direct mutation of Equipment.Slots is prohibited; use Equipment.Equip or Unequip instead",
						})
					}
				}

				// 2. Index expression assignment (e.g. inv.Items[i] = ..., equip.Slots[slot] = ...)
				if indexExpr, ok := lhs.(*ast.IndexExpr); ok {
					if innerSel, ok := indexExpr.X.(*ast.SelectorExpr); ok {
						fieldName := innerSel.Sel.Name
						pos := fset.Position(stmt.Pos())

						if fieldName == "Items" && !isAllowedInventory {
							violations = append(violations, coreViolation{
								file:    filename,
								line:    pos.Line,
								field:   fieldName,
								message: "direct indexed mutation of Inventory.Items is prohibited; use Inventory.Add, Consume, or Update instead",
							})
						}

						if fieldName == "Slots" && !isAllowedEquipment {
							violations = append(violations, coreViolation{
								file:    filename,
								line:    pos.Line,
								field:   fieldName,
								message: "direct indexed mutation of Equipment.Slots is prohibited; use Equipment.Equip or Unequip instead",
							})
						}
					}
				}
			}
		}
		return true
	})

	return violations
}

// TestCoreDomainInvariantsAST scans all production Go source files under internal/
// to mechanically verify that critical Core domain invariants (Progression, Currency,
// Job state, Inventory, Equipment) are never mutated directly outside their canonical Core helpers.
func TestCoreDomainInvariantsAST(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checkedFiles := 0

	err = filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", path, err)
		}

		checkedFiles++
		violations := checkFileCoreRules(fset, node, relPath)
		for _, v := range violations {
			t.Errorf("[%s:%d] %s", v.file, v.line, v.message)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	if checkedFiles == 0 {
		t.Fatalf("no production Go files were checked under %s", internalDir)
	}

	t.Logf("Successfully verified Core domain invariant encapsulation across %d production files", checkedFiles)
}

// TestCoreDomainInvariantsAST_Detection verifies that the linter correctly
// catches violations on synthetic AST snippets across all invariant categories.
func TestCoreDomainInvariantsAST_Detection(t *testing.T) {
	fset := token.NewFileSet()

	testCases := []struct {
		name        string
		path        string
		code        string
		expectError bool
		field       string
		errSnippet  string
	}{
		// 1. Progression
		{
			name: "Valid Progression: ApplyExperience",
			path: "internal/feature/service.go",
			code: `package feature
import "github.com/witchcraze/party2re/internal/core/progression"
func Reward(c *Character, exp int) error {
	_, err := progression.ApplyExperience(c, exp)
	return err
}`,
			expectError: false,
		},
		{
			name: "Violation: Experience direct increment",
			path: "internal/feature/service.go",
			code: `package feature
func Reward(c *Character, exp int) {
	c.Experience += exp
}`,
			expectError: true,
			field:       "Experience",
			errSnippet:  "use progression.ApplyExperience",
		},
		{
			name: "Violation: Level direct increment",
			path: "internal/feature/service.go",
			code: `package feature
func LevelUp(c *Character) {
	c.Level++
}`,
			expectError: true,
			field:       "Level",
			errSnippet:  "use progression.ApplyExperience",
		},

		// 2. Currency (Money & SmallMedals)
		{
			name: "Valid Currency: AddMoney and DeductMoney",
			path: "internal/feature/service.go",
			code: `package feature
func Pay(c *Character, amount int) error {
	if err := c.DeductMoney(amount); err != nil {
		return err
	}
	return c.AddMoney(amount)
}`,
			expectError: false,
		},
		{
			name: "Violation: Money direct deduction",
			path: "internal/feature/service.go",
			code: `package feature
func Pay(c *Character, amount int) {
	c.Money -= amount
}`,
			expectError: true,
			field:       "Money",
			errSnippet:  "use char.AddMoney or char.DeductMoney",
		},
		{
			name: "Violation: Money direct assignment",
			path: "internal/feature/service.go",
			code: `package feature
func ResetMoney(c *Character) {
	c.Money = 0
}`,
			expectError: true,
			field:       "Money",
			errSnippet:  "use char.AddMoney or char.DeductMoney",
		},
		{
			name: "Violation: SmallMedals direct increment",
			path: "internal/feature/service.go",
			code: `package feature
func AwardMedals(c *Character, medals int) {
	c.SmallMedals += medals
}`,
			expectError: true,
			field:       "SmallMedals",
			errSnippet:  "use char.AddSmallMedals or char.DeductSmallMedals",
		},

		// 3. Job State
		{
			name: "Valid Job: ChangeTo and Master",
			path: "internal/feature/service.go",
			code: `package feature
func Change(cj *CharacterJob, def Definition) error {
	cj.Master("starter")
	return cj.ChangeTo(def, 10, "male")
}`,
			expectError: false,
		},
		{
			name: "Violation: CurrentJobID direct assignment",
			path: "internal/feature/service.go",
			code: `package feature
func HackJob(cj *CharacterJob) {
	cj.CurrentJobID = "paladin"
}`,
			expectError: true,
			field:       "CurrentJobID",
			errSnippet:  "use CharacterJob.ChangeTo",
		},

		// 4. Inventory Items
		{
			name: "Valid Inventory: Add, Consume, Update",
			path: "internal/feature/service.go",
			code: `package feature
func Manage(inv *Inventory, inst Instance) error {
	_ = inv.Add(inst)
	_ = inv.Update(inst)
	return inv.Consume(inst.ID, 1)
}`,
			expectError: false,
		},
		{
			name: "Violation: Inventory.Items append",
			path: "internal/feature/service.go",
			code: `package feature
func Bypass(inv *Inventory, inst Instance) {
	inv.Items = append(inv.Items, inst)
}`,
			expectError: true,
			field:       "Items",
			errSnippet:  "use Inventory.Add, Consume, or Update",
		},
		{
			name: "Violation: Inventory.Items indexed mutation",
			path: "internal/feature/service.go",
			code: `package feature
func ModifyItem(inv *Inventory, inst Instance) {
	inv.Items[0] = inst
}`,
			expectError: true,
			field:       "Items",
			errSnippet:  "use Inventory.Add, Consume, or Update",
		},

		// 5. Equipment Slots
		{
			name: "Valid Equipment: Equip and Unequip",
			path: "internal/feature/service.go",
			code: `package feature
func EquipItem(eq *Equipment, owned Ownership, def Definition, instID string) error {
	_, err := eq.Equip(owned, def, instID)
	return err
}`,
			expectError: false,
		},
		{
			name: "Violation: Equipment.Slots direct indexed mutation",
			path: "internal/feature/service.go",
			code: `package feature
func ForceEquip(eq *Equipment, slot Slot, instID string) {
	eq.Slots[slot] = instID
}`,
			expectError: true,
			field:       "Slots",
			errSnippet:  "use Equipment.Equip or Unequip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := parser.ParseFile(fset, tc.path, tc.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			violations := checkFileCoreRules(fset, node, tc.path)
			if tc.expectError {
				if len(violations) == 0 {
					t.Fatalf("expected violation for field %q, got none", tc.field)
				}
				matched := false
				for _, v := range violations {
					if v.field == tc.field && strings.Contains(v.message, tc.errSnippet) {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("expected violation containing %q for field %q, got: %+v", tc.errSnippet, tc.field, violations)
				}
			} else {
				if len(violations) > 0 {
					t.Fatalf("expected 0 violations, got: %+v", violations)
				}
			}
		})
	}
}

package database_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// LockRank represents the numeric level in the global pessimistic lock hierarchy.
// Defined in .agents/rules/05-database-and-caching.md to mechanically eliminate database deadlocks.
type LockRank int

const (
	// Rank 0: Shared peer-to-peer / contest / reservation entities that serialize concurrent contenders.
	RankSharedPeerEntity LockRank = 0

	// Rank 1: Player account primary entity.
	RankPlayer LockRank = 1

	// Rank 2: Character primary entity. Multiple characters must be locked ascending by ID.
	RankCharacter LockRank = 2

	// Rank 3: Inventory items and equipment slots.
	RankInventory LockRank = 3

	// Rank 4: Character jobs and skill masteries.
	RankJob LockRank = 4

	// Rank 5: Depots and storage items.
	RankDepot LockRank = 5

	// Rank 6: Bank accounts and transfers.
	RankBank LockRank = 6

	// Rank 7: Guilds and guild membership. Multiple guilds must be locked ascending by ID.
	RankGuild LockRank = 7

	// Rank 8: Secondary domain feature records (monsters, achievements, points, farm plots).
	RankSecondaryFeature LockRank = 8
)

func (r LockRank) String() string {
	switch r {
	case RankSharedPeerEntity:
		return "Rank 0 (Shared Peer Entity)"
	case RankPlayer:
		return "Rank 1 (Player)"
	case RankCharacter:
		return "Rank 2 (Character)"
	case RankInventory:
		return "Rank 3 (Inventory/Equipment)"
	case RankJob:
		return "Rank 4 (Job/Mastery)"
	case RankDepot:
		return "Rank 5 (Depot)"
	case RankBank:
		return "Rank 6 (Bank)"
	case RankGuild:
		return "Rank 7 (Guild)"
	case RankSecondaryFeature:
		return "Rank 8 (Secondary Feature Record)"
	default:
		return fmt.Sprintf("Rank %d (Unknown)", int(r))
	}
}

type lockCall struct {
	rank        LockRank
	description string
	pos         token.Position
}

type lockViolation struct {
	file        string
	prevCall    lockCall
	currentCall lockCall
	message     string
}

func classifyLockCall(recvName, methodName, relPath string) (LockRank, string, bool) {
	if !strings.Contains(methodName, "ForUpdate") {
		return 0, "", false
	}

	r := strings.ToLower(recvName)
	m := strings.ToLower(methodName)
	normPath := filepath.ToSlash(relPath)

	// Rank 0: Shared peer-to-peer, contest rounds, or contention entities
	if strings.Contains(m, "listing") || strings.Contains(m, "parcel") ||
		strings.Contains(m, "party") || strings.Contains(m, "round") ||
		strings.Contains(m, "auction") || strings.Contains(m, "entry") {
		return RankSharedPeerEntity, fmt.Sprintf("%s.%s", recvName, methodName), true
	}
	if strings.Contains(r, "party") || strings.Contains(r, "contest") {
		return RankSharedPeerEntity, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 1: Player account
	if strings.Contains(r, "player") || strings.Contains(m, "player") {
		return RankPlayer, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 2: Character
	if strings.Contains(r, "char") || strings.Contains(r, "character") ||
		(strings.Contains(normPath, "internal/character") && (m == "findforupdate" || m == "findbynameforupdate")) {
		return RankCharacter, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 3: Inventory / Equipment
	if strings.Contains(r, "inv") || strings.Contains(r, "inventory") ||
		strings.Contains(r, "inventories") || strings.Contains(r, "equip") {
		return RankInventory, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 4: Job
	if strings.Contains(r, "job") {
		return RankJob, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 5: Depot
	if strings.Contains(r, "depot") {
		return RankDepot, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 6: Bank
	if strings.Contains(r, "bank") {
		return RankBank, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 7: Guild
	if strings.Contains(r, "guild") {
		return RankGuild, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Rank 8: Secondary domain feature records
	if strings.Contains(r, "monster") || strings.Contains(r, "achievement") ||
		strings.Contains(r, "blackmarket") || strings.Contains(r, "point") ||
		strings.Contains(r, "farm") || strings.Contains(m, "achievement") ||
		strings.Contains(m, "point") {
		return RankSecondaryFeature, fmt.Sprintf("%s.%s", recvName, methodName), true
	}

	// Default unknown lock call to secondary feature rank
	return RankSecondaryFeature, fmt.Sprintf("%s.%s", recvName, methodName), true
}

func analyzeASTFileLocks(fset *token.FileSet, fileNode *ast.File, relPath string) []lockViolation {
	var violations []lockViolation

	ast.Inspect(fileNode, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				violations = append(violations, checkDirectBlockLocks(fset, fn.Body, relPath)...)
			}
		case *ast.FuncLit:
			if fn.Body != nil {
				violations = append(violations, checkDirectBlockLocks(fset, fn.Body, relPath)...)
			}
		}
		return true
	})

	return violations
}

func checkDirectBlockLocks(fset *token.FileSet, body *ast.BlockStmt, relPath string) []lockViolation {
	var violations []lockViolation
	var lockCalls []lockCall

	ast.Inspect(body, func(n ast.Node) bool {
		// Do not traverse into nested function literals; each closure has its own scope
		if fnLit, ok := n.(*ast.FuncLit); ok && fnLit.Body != body {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name
		var recvName string
		switch x := sel.X.(type) {
		case *ast.Ident:
			recvName = x.Name
		case *ast.SelectorExpr:
			recvName = x.Sel.Name
		}

		rank, desc, isLock := classifyLockCall(recvName, methodName, relPath)
		if !isLock {
			return true
		}

		pos := fset.Position(call.Pos())
		lockCalls = append(lockCalls, lockCall{
			rank:        rank,
			description: desc,
			pos:         pos,
		})
		return true
	})

	if len(lockCalls) <= 1 {
		return violations
	}

	for i := 0; i < len(lockCalls)-1; i++ {
		prev := lockCalls[i]
		curr := lockCalls[i+1]

		if curr.rank < prev.rank {
			msg := fmt.Sprintf("database lock hierarchy inversion detected in %s:\n"+
				"  Acquired %s (%s) at line %d\n"+
				"  AFTER %s (%s) at line %d.\n"+
				"  Locks MUST be acquired in ascending rank order (Rank 0 -> Rank 8) to prevent deadlocks.",
				relPath, curr.description, curr.rank, curr.pos.Line, prev.description, prev.rank, prev.pos.Line)

			violations = append(violations, lockViolation{
				file:        relPath,
				prevCall:    prev,
				currentCall: curr,
				message:     msg,
			})
		}
	}

	return violations
}

func getRepoRootDir(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine runtime caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func runLockHierarchyLint(internalDir, rootDir string) (int, []lockViolation, error) {
	fset := token.NewFileSet()
	var totalViolations []lockViolation
	checkedFiles := 0

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

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			relPath = path
		}

		checkedFiles++

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", relPath, err)
		}

		// Fast path: files without "ForUpdate" cannot contain pessimistic lock acquisitions
		if !bytes.Contains(src, []byte("ForUpdate")) {
			return nil
		}

		fileNode, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("failed to parse file %s: %w", relPath, err)
		}

		violations := analyzeASTFileLocks(fset, fileNode, relPath)
		totalViolations = append(totalViolations, violations...)
		return nil
	})

	return checkedFiles, totalViolations, err
}

func TestLockHierarchyInvariants(t *testing.T) {
	start := time.Now()
	rootDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to resolve root dir: %v", err)
	}

	internalDir := filepath.Join(rootDir, "internal")

	checkedFiles, totalViolations, err := runLockHierarchyLint(internalDir, rootDir)
	if err != nil {
		t.Fatalf("failed to walk internal directory: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("Lock hierarchy linter verified %d production Go files in %s", checkedFiles, elapsed)

	// Liberal dead-man safety bound strictly to catch infinite loops or deadlocks in AST traversal.
	// Performance benchmarks and regression tracking are handled via BenchmarkLockHierarchyLinter.
	if elapsed > 5*time.Second {
		t.Errorf("lock hierarchy linter took %s (exceeds 5s dead-man safety bound, possible infinite loop or deadlock)", elapsed)
	}

	if len(totalViolations) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d lock hierarchy inversion(s):\n", len(totalViolations)))
		for i, v := range totalViolations {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, v.message))
		}
		t.Fatal(sb.String())
	}
}

func BenchmarkLockHierarchyLinter(b *testing.B) {
	rootDir, err := filepath.Abs("../..")
	if err != nil {
		b.Fatalf("failed to resolve root dir: %v", err)
	}
	internalDir := filepath.Join(rootDir, "internal")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := runLockHierarchyLint(internalDir, rootDir)
		if err != nil {
			b.Fatalf("runLockHierarchyLint failed: %v", err)
		}
	}
}

func BenchmarkLockEvaluationStatement(b *testing.B) {
	fset := token.NewFileSet()
	code := `package test
func (s *Service) Transfer(ctx context.Context) {
	s.charRepo.GetByIDForUpdate(ctx, id1)
	s.invRepo.GetByIDForUpdate(ctx, item1)
	s.bankRepo.GetByIDForUpdate(ctx, acc1)
}
`
	fileNode, err := parser.ParseFile(fset, "test.go", code, 0)
	if err != nil {
		b.Fatalf("failed to parse test code: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = analyzeASTFileLocks(fset, fileNode, "test.go")
	}
}

func TestLockHierarchyInversionDetections(t *testing.T) {
	tests := []struct {
		name              string
		code              string
		expectViolations  int
		expectedSubstrMsg string
	}{
		{
			name: "Inversion: Inventory before Character (Issue #357 Shop Sale Bug)",
			code: `package testpkg
import "context"
func (s *Service) BadSell(ctx context.Context, charID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		inv, _ := s.inventories.FindByCharacterIDForUpdate(txCtx, charID)
		char, _ := s.characters.FindByIDForUpdate(txCtx, charID)
		_ = inv
		_ = char
		return nil
	})
}`,
			expectViolations:  1,
			expectedSubstrMsg: "Acquired characters.FindByIDForUpdate (Rank 2 (Character))",
		},
		{
			name: "Inversion: Bank before Character",
			code: `package testpkg
import "context"
func (s *Service) BadWithdraw(ctx context.Context, charID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		bank, _ := s.bankRepo.FindByIDForUpdate(txCtx, charID)
		char, _ := s.charRepo.FindByIDForUpdate(txCtx, charID)
		_ = bank
		_ = char
		return nil
	})
}`,
			expectViolations:  1,
			expectedSubstrMsg: "Acquired charRepo.FindByIDForUpdate (Rank 2 (Character))",
		},
		{
			name: "Inversion: Secondary Feature before Inventory",
			code: `package testpkg
import "context"
func (s *Service) BadTrade(ctx context.Context, charID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		pts, _ := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, charID)
		inv, _ := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, charID)
		_ = pts
		_ = inv
		return nil
	})
}`,
			expectViolations:  1,
			expectedSubstrMsg: "Acquired inventoryRepo.FindByCharacterIDForUpdate (Rank 3 (Inventory/Equipment))",
		},
		{
			name: "Valid Sequence: Shared Listing -> Character -> Inventory (Flea Market)",
			code: `package testpkg
import "context"
func (s *Service) ValidPurchase(ctx context.Context, listingID, buyerID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		listing, _ := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		buyer, _ := s.charRepo.FindByIDForUpdate(txCtx, buyerID)
		inv, _ := s.invRepo.FindByCharacterIDForUpdate(txCtx, buyerID)
		_ = listing
		_ = buyer
		_ = inv
		return nil
	})
}`,
			expectViolations: 0,
		},
		{
			name: "Valid Sequence: Multiple Characters in Ascending Order -> Inventory",
			code: `package testpkg
import "context"
func (s *Service) ValidTransfer(ctx context.Context, id1, id2 string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		c1, _ := s.charRepo.FindByIDForUpdate(txCtx, id1)
		c2, _ := s.charRepo.FindByIDForUpdate(txCtx, id2)
		inv, _ := s.invRepo.FindByCharacterIDForUpdate(txCtx, id1)
		_ = c1
		_ = c2
		_ = inv
		return nil
	})
}`,
			expectViolations: 0,
		},
		{
			name: "Valid Sequence: Character -> Inventory -> Secondary Points (Blackmarket)",
			code: `package testpkg
import "context"
func (s *Service) ValidSacrifice(ctx context.Context, charID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		char, _ := s.characterRepo.FindByIDForUpdate(txCtx, charID)
		inv, _ := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, charID)
		pts, _ := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, charID)
		_ = char
		_ = inv
		_ = pts
		return nil
	})
}`,
			expectViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			fileNode, err := parser.ParseFile(fset, "mock.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			violations := analyzeASTFileLocks(fset, fileNode, "mock.go")
			if len(violations) != tt.expectViolations {
				t.Fatalf("expected %d violations, got %d: %+v", tt.expectViolations, len(violations), violations)
			}

			if tt.expectViolations > 0 && tt.expectedSubstrMsg != "" {
				found := false
				for _, v := range violations {
					if strings.Contains(v.message, tt.expectedSubstrMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation message containing %q, got: %s", tt.expectedSubstrMsg, violations[0].message)
				}
			}
		})
	}
}

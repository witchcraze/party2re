package collection_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var errInjectedLegendFailure = errors.New("simulated legend induction failure")
var errInjectedRepoFailure = errors.New("simulated repo persistence failure")

// TestCompletionInduction_LegendFailureNotTreatedAsSuccess verifies Acceptance Criterion 1:
// 4種の完成で殿堂失敗を成功扱いにしない
// For all 4 collection milestone types (monster book, weapon, armor, item),
// if Hall of Fame legend induction fails, the operation MUST return an error,
// the milestone MUST NOT be marked completed, and news MUST NOT be published.
func TestCompletionInduction_LegendFailureNotTreatedAsSuccess(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name          string
		setupSvc      func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error)
		triggerRecord func(svc *collection.Service) error
		kind          string
		legendCat     string
	}{
		{
			name: "monster_book",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 1, 10,
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service) error {
				return svc.RecordMonsterDefeat(ctx, "char-test", collection.DefeatedMonsterRecord{
					MonsterID:   "mon-001",
					MonsterName: "Slime",
					Habitat:     "Plains",
				})
			},
			kind:      "monster_book",
			legendCat: "comp_mon",
		},
		{
			name: "weapon",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 10,
					collection.WithTotalWeapons(1),
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service) error {
				return svc.RecordItemDiscovered(ctx, "char-test", "wea-001", "Iron Sword", "weapon")
			},
			kind:      "weapon",
			legendCat: "comp_wea",
		},
		{
			name: "armor",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 10,
					collection.WithTotalArmors(1),
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service) error {
				return svc.RecordItemDiscovered(ctx, "char-test", "arm-001", "Bronze Shield", "armor")
			},
			kind:      "armor",
			legendCat: "comp_arm",
		},
		{
			name: "item",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 1,
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service) error {
				return svc.RecordItemDiscovered(ctx, "char-test", "ite-001", "Herb", "item")
			},
			kind:      "item",
			legendCat: "comp_ite",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockCollectionRepo{}
			pub := &mockNewsPublisher{}
			legend := &mockLegendInductor{
				recordErr: errInjectedLegendFailure,
			}

			svc, err := tc.setupSvc(repo, pub, legend)
			if err != nil {
				t.Fatalf("setup service failed: %v", err)
			}

			// Trigger completion with failing legend inductor
			recordErr := tc.triggerRecord(svc)
			if recordErr == nil {
				t.Fatalf("expected error when legend inductor fails, but got nil")
			}
			if !errors.Is(recordErr, errInjectedLegendFailure) {
				t.Errorf("expected error wrapping errInjectedLegendFailure, got: %v", recordErr)
			}

			// Verify milestone was NOT marked completed in repository
			completed, err := repo.IsCompleted(ctx, "char-test", tc.kind)
			if err != nil {
				t.Fatalf("IsCompleted check failed: %v", err)
			}
			if completed {
				t.Errorf("milestone %s MUST NOT be marked completed when legend induction failed", tc.kind)
			}

			// Verify news was NOT published
			articles := pub.getArticles()
			if len(articles) != 0 {
				t.Errorf("expected 0 news articles, got %d", len(articles))
			}

			// Verify no successful inductions recorded
			inductions := legend.getInductions()
			if len(inductions) != 0 {
				t.Errorf("expected 0 successful inductions, got %d", len(inductions))
			}
		})
	}
}

// TestCompletionInduction_ReprocessingRecoversMissingLegendOnce verifies Acceptance Criterion 2:
// 再処理で欠落した殿堂を一度だけ保存できる
// When legend induction initially fails, re-executing the discovery/defeat:
// 1. Inducts into Hall of Fame exactly once.
// 2. Marks milestone completed.
// 3. Publishes news exactly once.
// 4. Subsequent defeats/discoveries do NOT duplicate inductions or news broadcasts.
func TestCompletionInduction_ReprocessingRecoversMissingLegendOnce(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name          string
		setupSvc      func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error)
		triggerRecord func(svc *collection.Service, id string) error
		kind          string
		legendCat     string
	}{
		{
			name: "monster_book",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 1, 10,
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service, id string) error {
				return svc.RecordMonsterDefeat(ctx, "char-test", collection.DefeatedMonsterRecord{
					MonsterID:   id,
					MonsterName: "Monster " + id,
					Habitat:     "Dungeon",
				})
			},
			kind:      "monster_book",
			legendCat: "comp_mon",
		},
		{
			name: "weapon",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 10,
					collection.WithTotalWeapons(1),
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service, id string) error {
				return svc.RecordItemDiscovered(ctx, "char-test", id, "Weapon "+id, "weapon")
			},
			kind:      "weapon",
			legendCat: "comp_wea",
		},
		{
			name: "armor",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 10,
					collection.WithTotalArmors(1),
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service, id string) error {
				return svc.RecordItemDiscovered(ctx, "char-test", id, "Armor "+id, "armor")
			},
			kind:      "armor",
			legendCat: "comp_arm",
		},
		{
			name: "item",
			setupSvc: func(repo collection.Repository, pub collection.NewsPublisher, legend collection.LegendInductor) (*collection.Service, error) {
				return collection.NewService(repo, 10, 1,
					collection.WithNewsPublisher(pub),
					collection.WithLegendInductor(legend),
				)
			},
			triggerRecord: func(svc *collection.Service, id string) error {
				return svc.RecordItemDiscovered(ctx, "char-test", id, "Item "+id, "item")
			},
			kind:      "item",
			legendCat: "comp_ite",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockCollectionRepo{}
			pub := &mockNewsPublisher{}
			// Fail once, then succeed on subsequent attempts
			legend := &mockLegendInductor{
				recordErr:     errInjectedLegendFailure,
				failCountdown: 1,
				hasCountdown:  true,
			}

			svc, err := tc.setupSvc(repo, pub, legend)
			if err != nil {
				t.Fatalf("setup service failed: %v", err)
			}

			// Step 1: Initial attempt hits completion but fails legend inductor
			err1 := tc.triggerRecord(svc, "elem-1")
			if err1 == nil {
				t.Fatalf("step 1: expected failure, got nil")
			}
			if !errors.Is(err1, errInjectedLegendFailure) {
				t.Errorf("step 1: expected errInjectedLegendFailure, got: %v", err1)
			}
			isComp1, _ := repo.IsCompleted(ctx, "char-test", tc.kind)
			if isComp1 {
				t.Errorf("step 1: milestone MUST NOT be completed")
			}

			// Step 2: Reprocessing - re-execute discovery with working legend inductor
			err2 := tc.triggerRecord(svc, "elem-1")
			if err2 != nil {
				t.Fatalf("step 2 (reprocessing): expected success, got: %v", err2)
			}

			// Verify milestone is now completed
			isComp2, _ := repo.IsCompleted(ctx, "char-test", tc.kind)
			if !isComp2 {
				t.Errorf("step 2: expected milestone %s to be completed", tc.kind)
			}

			// Verify legend was inducted exactly once
			inductions := legend.getInductions()
			if len(inductions) != 1 {
				t.Fatalf("step 2: expected exactly 1 legend induction, got %d", len(inductions))
			}
			if inductions[0].Category != tc.legendCat || inductions[0].CharacterID != "char-test" {
				t.Errorf("step 2: unexpected induction: %+v", inductions[0])
			}

			// Verify news was published exactly once
			articles := pub.getArticles()
			if len(articles) != 1 {
				t.Fatalf("step 2: expected exactly 1 news article, got %d", len(articles))
			}

			// Step 3: Subsequent discovery/defeat of the same kind
			err3 := tc.triggerRecord(svc, "elem-2")
			if err3 != nil {
				t.Fatalf("step 3: expected success, got: %v", err3)
			}

			// Verify legend is STILL inducted exactly once (no duplicate)
			inductionsAfter := legend.getInductions()
			if len(inductionsAfter) != 1 {
				t.Errorf("step 3: expected still 1 induction, got %d", len(inductionsAfter))
			}

			// Verify news was NOT published again (still 1 article)
			articlesAfter := pub.getArticles()
			if len(articlesAfter) != 1 {
				t.Errorf("step 3: expected still 1 news article, got %d", len(articlesAfter))
			}
		})
	}
}

// TestCompletionInduction_RecoverMissingLegends verifies Acceptance Criterion 3:
// 既存の完成済み・殿堂欠落データの回復手順を定義する
// Tests the RecoverMissingLegends service method for repairing existing completed data
// where Hall of Fame records were lost due to prior persistence errors.
func TestCompletionInduction_RecoverMissingLegends(t *testing.T) {
	ctx := context.Background()

	t.Run("recovers_all_completed_kinds_idempotently", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		pub := &mockNewsPublisher{}
		legend := &mockLegendInductor{}

		svc, err := collection.NewService(repo, 180, 141,
			collection.WithNewsPublisher(pub),
			collection.WithLegendInductor(legend),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		// Simulate existing database state where milestones are already marked completed
		// but Hall of Fame entries were never written
		repo.completions = map[string]bool{
			"char-hero:monster_book": true,
			"char-hero:weapon":       true,
			"char-hero:armor":        true,
			"char-hero:item":         true,
		}

		// Initial legend inductions: empty
		if len(legend.getInductions()) != 0 {
			t.Fatalf("expected 0 inductions initially")
		}

		// Run recovery
		if err := svc.RecoverMissingLegends(ctx, "char-hero"); err != nil {
			t.Fatalf("RecoverMissingLegends failed: %v", err)
		}

		// Verify all 4 categories were inducted
		inductions := legend.getInductions()
		if len(inductions) != 4 {
			t.Fatalf("expected 4 recovered inductions, got %d", len(inductions))
		}

		expectedCats := map[string]bool{
			"comp_mon": false,
			"comp_wea": false,
			"comp_arm": false,
			"comp_ite": false,
		}
		for _, ind := range inductions {
			if ind.CharacterID != "char-hero" {
				t.Errorf("expected CharacterID char-hero, got %s", ind.CharacterID)
			}
			expectedCats[ind.Category] = true
		}
		for cat, found := range expectedCats {
			if !found {
				t.Errorf("missing recovered category %s", cat)
			}
		}

		// Re-run recovery: verify idempotent (no duplicate induction calls)
		legend.inductions = nil
		if err := svc.RecoverMissingLegends(ctx, "char-hero"); err != nil {
			t.Fatalf("second RecoverMissingLegends failed: %v", err)
		}
		if len(legend.getInductions()) != 4 {
			t.Errorf("expected 4 calls on idempotent re-run, got %d", len(legend.getInductions()))
		}
	})

	t.Run("partial_completions_only_recovers_completed_ones", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		legend := &mockLegendInductor{}

		svc, err := collection.NewService(repo, 180, 141,
			collection.WithLegendInductor(legend),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		// Only monster_book and item are marked completed
		repo.completions = map[string]bool{
			"char-mage:monster_book": true,
			"char-mage:item":         true,
		}

		if err := svc.RecoverMissingLegends(ctx, "char-mage"); err != nil {
			t.Fatalf("RecoverMissingLegends failed: %v", err)
		}

		inductions := legend.getInductions()
		if len(inductions) != 2 {
			t.Fatalf("expected 2 inductions, got %d", len(inductions))
		}
		cats := map[string]bool{}
		for _, ind := range inductions {
			cats[ind.Category] = true
		}
		if !cats["comp_mon"] || !cats["comp_ite"] {
			t.Errorf("expected comp_mon and comp_ite, got: %+v", cats)
		}
	})

	t.Run("error_propagation_on_legend_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		legend := &mockLegendInductor{
			recordErr: errInjectedLegendFailure,
		}

		svc, err := collection.NewService(repo, 180, 141,
			collection.WithLegendInductor(legend),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		repo.completions = map[string]bool{
			"char-err:monster_book": true,
		}

		err = svc.RecoverMissingLegends(ctx, "char-err")
		if err == nil {
			t.Fatalf("expected error from RecoverMissingLegends, got nil")
		}
		if !errors.Is(err, errInjectedLegendFailure) {
			t.Errorf("expected errInjectedLegendFailure, got: %v", err)
		}
	})

	t.Run("invalid_character_id", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		svc, _ := collection.NewService(repo, 180, 141)
		err := svc.RecoverMissingLegends(ctx, "")
		if !errors.Is(err, collection.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
	})
}

// TestCompletionInduction_RepositoryFailures verifies Acceptance Criterion 4:
// 保存失敗
// Verifies all repository error conditions are properly propagated and never swallowed.
func TestCompletionInduction_RepositoryFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("GetMonsterBookCount_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			getMonsterCountErr: errInjectedRepoFailure,
		}
		svc, _ := collection.NewService(repo, 10, 10)
		err := svc.RecordMonsterDefeat(ctx, "char-1", collection.DefeatedMonsterRecord{
			MonsterID:   "m1",
			MonsterName: "M",
		})
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})

	t.Run("GetItemCollectionCount_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			getItemCountErr: errInjectedRepoFailure,
		}
		svc, _ := collection.NewService(repo, 10, 10)
		err := svc.RecordItemDiscovered(ctx, "char-1", "i1", "Item", "item")
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})

	t.Run("GetWeaponCollection_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			getItemCollectionErr: errInjectedRepoFailure,
		}
		svc, _ := collection.NewService(repo, 10, 10)
		err := svc.RecordItemDiscovered(ctx, "char-1", "w1", "Weapon", "weapon")
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})

	t.Run("GetArmorCollection_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			getItemCollectionErr: errInjectedRepoFailure,
		}
		svc, _ := collection.NewService(repo, 10, 10)
		err := svc.RecordItemDiscovered(ctx, "char-1", "a1", "Armor", "armor")
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})

	t.Run("MarkCompleted_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			markCompletedErr: errInjectedRepoFailure,
		}
		legend := &mockLegendInductor{}
		svc, _ := collection.NewService(repo, 1, 10, collection.WithLegendInductor(legend))
		err := svc.RecordMonsterDefeat(ctx, "char-1", collection.DefeatedMonsterRecord{
			MonsterID:   "m1",
			MonsterName: "M",
		})
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})

	t.Run("IsCompleted_failure", func(t *testing.T) {
		repo := &mockCollectionRepo{
			isCompletedErr: errInjectedRepoFailure,
		}
		svc, _ := collection.NewService(repo, 1, 10)
		err := svc.RecordMonsterDefeat(ctx, "char-1", collection.DefeatedMonsterRecord{
			MonsterID:   "m1",
			MonsterName: "M",
		})
		if !errors.Is(err, errInjectedRepoFailure) {
			t.Errorf("expected errInjectedRepoFailure, got %v", err)
		}
	})
}

// TestCompletionInduction_ConcurrentCompletion verifies Acceptance Criterion 4:
// 並行完成
// Simulates concurrent goroutines triggering completion simultaneously.
// Verifies 0 deadlocks, thread safety, exactly 1 legend induction, and exactly 1 news broadcast.
func TestCompletionInduction_ConcurrentCompletion(t *testing.T) {
	ctx := context.Background()

	t.Run("concurrent_monster_book_completion", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		pub := &mockNewsPublisher{}
		legend := &mockLegendInductor{}

		svc, err := collection.NewService(repo, 1, 10,
			collection.WithNewsPublisher(pub),
			collection.WithLegendInductor(legend),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		errs := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = svc.RecordMonsterDefeat(ctx, "char-concurrent", collection.DefeatedMonsterRecord{
					MonsterID:   fmt.Sprintf("mon-%d", idx),
					MonsterName: "Monster",
					Habitat:     "Arena",
				})
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("goroutine %d failed: %v", i, err)
			}
		}

		// Exactly 1 Hall of Fame induction
		inductions := legend.getInductions()
		if len(inductions) != 1 {
			t.Errorf("expected exactly 1 legend induction under concurrency, got %d", len(inductions))
		}

		// Exactly 1 news article broadcast
		articles := pub.getArticles()
		if len(articles) != 1 {
			t.Errorf("expected exactly 1 news broadcast under concurrency, got %d", len(articles))
		}

		// Repository milestone is completed
		completed, err := repo.IsCompleted(ctx, "char-concurrent", "monster_book")
		if err != nil || !completed {
			t.Errorf("expected milestone to be completed, got %v, err=%v", completed, err)
		}
	})

	t.Run("concurrent_weapon_collection_completion", func(t *testing.T) {
		repo := &mockCollectionRepo{}
		pub := &mockNewsPublisher{}
		legend := &mockLegendInductor{}

		svc, err := collection.NewService(repo, 10, 10,
			collection.WithTotalWeapons(1),
			collection.WithNewsPublisher(pub),
			collection.WithLegendInductor(legend),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		errs := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = svc.RecordItemDiscovered(ctx, "char-concurrent", fmt.Sprintf("wea-%d", idx), "Sword", "weapon")
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("goroutine %d failed: %v", i, err)
			}
		}

		inductions := legend.getInductions()
		if len(inductions) != 1 {
			t.Errorf("expected exactly 1 weapon induction, got %d", len(inductions))
		}
		articles := pub.getArticles()
		if len(articles) != 1 {
			t.Errorf("expected exactly 1 news article, got %d", len(articles))
		}
	})
}

// TestCompletionInduction_NormalSuccessAllFourKinds verifies Acceptance Criterion 4:
// 正常系
// Verifies successful completion induction across all 4 categories with proper legend categories,
// character name resolution, and news broadcasts.
func TestCompletionInduction_NormalSuccessAllFourKinds(t *testing.T) {
	ctx := context.Background()

	repo := &mockCollectionRepo{}
	pub := &mockNewsPublisher{}
	legend := &mockLegendInductor{}
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-hero": {ID: "char-hero", Name: "伝説の勇者"},
		},
	}

	svc, err := collection.NewService(repo, 1, 1,
		collection.WithTotalWeapons(1),
		collection.WithTotalArmors(1),
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Monster Book completion
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", collection.DefeatedMonsterRecord{
		MonsterID:   "mon-king",
		MonsterName: "King Dragon",
	}); err != nil {
		t.Fatalf("defeat failed: %v", err)
	}

	// 2. Weapon completion
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "wea-excalibur", "Excalibur", "weapon"); err != nil {
		t.Fatalf("weapon discovery failed: %v", err)
	}

	// 3. Armor completion
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "arm-aegis", "Aegis", "armor"); err != nil {
		t.Fatalf("armor discovery failed: %v", err)
	}

	// 4. Item completion
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "ite-elixir", "Elixir", "item"); err != nil {
		t.Fatalf("item discovery failed: %v", err)
	}

	// Verify all 4 categories were inducted
	inductions := legend.getInductions()
	if len(inductions) != 4 {
		t.Fatalf("expected 4 legend inductions, got %d", len(inductions))
	}
	expectedCats := []string{"comp_mon", "comp_wea", "comp_arm", "comp_ite"}
	for i, expected := range expectedCats {
		if inductions[i].Category != expected {
			t.Errorf("induction %d: expected category %s, got %s", i, expected, inductions[i].Category)
		}
		if inductions[i].CharacterID != "char-hero" {
			t.Errorf("induction %d: expected char-hero, got %s", i, inductions[i].CharacterID)
		}
	}

	// Verify all 4 news articles were broadcast with character name
	articles := pub.getArticles()
	if len(articles) != 4 {
		t.Fatalf("expected 4 news articles, got %d", len(articles))
	}
	expectedTitles := []string{
		"伝説の勇者がモンスターブックをコンプリートしました！",
		"伝説の勇者が武器図鑑をコンプリートしました！",
		"伝説の勇者が防具図鑑をコンプリートしました！",
		"伝説の勇者がアイテム図鑑をコンプリートしました！",
	}
	for i, expected := range expectedTitles {
		if articles[i].Content != expected {
			t.Errorf("article %d: expected content %q, got %q", i, expected, articles[i].Content)
		}
		if articles[i].Category != "collection" {
			t.Errorf("article %d: expected category collection, got %q", i, articles[i].Category)
		}
	}
}

package adventure

import (
	"context"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type chronicleRepositoryStub struct {
	adventures      []Adventure
	aggregatedStats AggregatedStats
}

func (r *chronicleRepositoryStub) Save(_ context.Context, _ Adventure) error {
	return nil
}

func (r *chronicleRepositoryStub) FindByID(_ context.Context, _ string) (Adventure, error) {
	return Adventure{}, ErrNotFound
}

func (r *chronicleRepositoryStub) ClaimAndApply(_ context.Context, _ Adventure, _ corecharacter.Character) error {
	return nil
}

func (r *chronicleRepositoryStub) ListByCharacterID(_ context.Context, characterID string, limit, offset int) ([]Adventure, int, error) {
	var matched []Adventure
	for _, a := range r.adventures {
		if a.CharacterID == characterID {
			matched = append(matched, a)
		}
	}
	total := len(matched)
	if offset >= total {
		return []Adventure{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (r *chronicleRepositoryStub) GetAggregatedStats(_ context.Context, characterID string) (AggregatedStats, error) {
	return r.aggregatedStats, nil
}

func TestNormalizePagination(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{name: "default when non-positive", limit: 0, offset: 0, wantLimit: 20, wantOffset: 0},
		{name: "negative values", limit: -5, offset: -10, wantLimit: 20, wantOffset: 0},
		{name: "valid within bounds", limit: 50, offset: 10, wantLimit: 50, wantOffset: 10},
		{name: "clamp excessive limit", limit: 500, offset: 20, wantLimit: 100, wantOffset: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := NormalizePagination(tt.limit, tt.offset)
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Fatalf("NormalizePagination(%d, %d) = (%d, %d), want (%d, %d)",
					tt.limit, tt.offset, gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestListHistory(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	stages, err := NewStageCatalog([]Stage{
		{ID: "stage-01", Name: "平原", MinLevel: 1, MonsterIDs: []string{"mon-01"}, Duration: time.Minute},
		{ID: "stage-02", Name: "森林", MinLevel: 2, MonsterIDs: []string{"mon-02"}, Duration: time.Minute},
	})
	if err != nil {
		t.Fatalf("create stage catalog: %v", err)
	}

	monsters, err := NewMonsterCatalog([]Monster{
		{ID: "mon-01", Name: "スライム", HP: 10, MP: 0, Attack: 2, Defense: 1, Agility: 1, ExperienceReward: 10, GoldReward: 5},
		{ID: "mon-02", Name: "ゴブリン", HP: 20, MP: 0, Attack: 4, Defense: 2, Agility: 2, ExperienceReward: 20, GoldReward: 10},
	})
	if err != nil {
		t.Fatalf("create monster catalog: %v", err)
	}

	charStub := &characterRepositoryStub{
		value: corecharacter.Character{ID: "char-1", PlayerID: "p1", Name: "Hero", Level: 5},
	}

	advs := []Adventure{
		{
			ID:          "adv-1",
			CharacterID: "char-1",
			Type:        "stage-01",
			StageID:     "stage-01",
			MonsterID:   "mon-01",
			StartedAt:   now,
			AvailableAt: now.Add(time.Minute),
			BattleResult: corebattle.Result{
				Outcome:  corebattle.OutcomeWin,
				WinnerID: "char-1",
				LoserID:  "mon-01",
				Turns:    3,
				Reward: corebattle.Reward{
					Experience:       10,
					Currency:         5,
					ItemDefinitionID: "item-herb",
					ItemQuantity:     1,
				},
			},
			Resolved: true,
			Claimed:  true,
		},
		{
			ID:          "adv-2",
			CharacterID: "char-1",
			Type:        "stage-02",
			StageID:     "stage-02",
			MonsterID:   "mon-02",
			StartedAt:   now.Add(time.Hour),
			AvailableAt: now.Add(time.Hour + time.Minute),
			BattleResult: corebattle.Result{
				Outcome:  corebattle.OutcomeWin,
				WinnerID: "mon-02",
				LoserID:  "char-1",
				Turns:    5,
			},
			Resolved: true,
			Claimed:  true,
		},
	}

	repo := &chronicleRepositoryStub{adventures: advs}
	service, err := NewServiceWithCatalogs(repo, charStub, nil, stages, monsters, battleResolverStub{}, nil, nil, &testClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()

	t.Run("empty character ID fails", func(t *testing.T) {
		_, err := service.ListHistory(ctx, "", 20, 0)
		if err == nil {
			t.Fatal("expected error for empty character ID")
		}
	})

	t.Run("character not found fails", func(t *testing.T) {
		_, err := service.ListHistory(ctx, "nonexistent", 20, 0)
		if err == nil {
			t.Fatal("expected error for nonexistent character")
		}
	})

	t.Run("retrieve enriched history with pagination", func(t *testing.T) {
		res, err := service.ListHistory(ctx, "char-1", 10, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Total != 2 {
			t.Fatalf("expected total 2, got %d", res.Total)
		}
		if len(res.Adventures) != 2 {
			t.Fatalf("expected 2 adventures, got %d", len(res.Adventures))
		}

		first := res.Adventures[0]
		if first.ID != "adv-1" || first.StageName != "平原" || first.MonsterName != "スライム" {
			t.Fatalf("expected enriched first entry, got %+v", first)
		}
		if first.Outcome != corebattle.OutcomeWin || first.BattleTurns != 3 || first.RewardExperience != 10 || first.RewardCurrency != 5 || first.RewardItemID != "item-herb" {
			t.Fatalf("unexpected battle/reward fields in first entry: %+v", first)
		}

		second := res.Adventures[1]
		if second.ID != "adv-2" || second.StageName != "森林" || second.MonsterName != "ゴブリン" {
			t.Fatalf("expected enriched second entry, got %+v", second)
		}
		if second.Outcome != corebattle.OutcomeWin || second.BattleTurns != 5 {
			t.Fatalf("unexpected battle fields in second entry: %+v", second)
		}
	})
}

func TestGetChronicle(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	stages, err := NewStageCatalog([]Stage{
		{ID: "stage-01", Name: "平原", MinLevel: 1, MonsterIDs: []string{"mon-01"}, Duration: time.Minute},
		{ID: "stage-02", Name: "森林", MinLevel: 2, MonsterIDs: []string{"mon-02"}, Duration: time.Minute},
	})
	if err != nil {
		t.Fatalf("create stage catalog: %v", err)
	}

	charStub := &characterRepositoryStub{
		value: corecharacter.Character{ID: "char-1", PlayerID: "p1", Name: "Hero", Level: 5},
	}

	stats := AggregatedStats{
		TotalAdventures: 100,
		TotalVictories:  80,
		TotalDefeats:    15,
		TotalDraws:      5,
		TotalTurns:      350,
		TotalExpEarned:  2400,
		TotalGoldEarned: 1200,
		StageStats: []StageStatData{
			{StageID: "stage-01", TotalAttempts: 60, ClearCount: 50},
			{StageID: "stage-02", TotalAttempts: 40, ClearCount: 30},
		},
	}

	repo := &chronicleRepositoryStub{aggregatedStats: stats}
	service, err := NewServiceWithCatalogs(repo, charStub, nil, stages, nil, battleResolverStub{}, nil, nil, &testClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()

	t.Run("empty character ID fails", func(t *testing.T) {
		_, err := service.GetChronicle(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty character ID")
		}
	})

	t.Run("aggregate chronicle computation and milestone unlocking", func(t *testing.T) {
		chronicle, err := service.GetChronicle(ctx, "char-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if chronicle.CharacterID != "char-1" {
			t.Fatalf("expected character_id char-1, got %s", chronicle.CharacterID)
		}
		if chronicle.TotalAdventures != 100 || chronicle.TotalVictories != 80 || chronicle.TotalDefeats != 15 || chronicle.TotalDraws != 5 {
			t.Fatalf("unexpected counts in chronicle: %+v", chronicle)
		}
		if chronicle.WinRate != 0.8 {
			t.Fatalf("expected win rate 0.8, got %f", chronicle.WinRate)
		}
		if chronicle.TotalTurns != 350 || chronicle.TotalExpEarned != 2400 || chronicle.TotalGoldEarned != 1200 {
			t.Fatalf("unexpected aggregates in chronicle: %+v", chronicle)
		}

		if len(chronicle.Stages) != 2 {
			t.Fatalf("expected 2 stage stats, got %d", len(chronicle.Stages))
		}
		if chronicle.Stages[0].StageName != "平原" || chronicle.Stages[0].ClearCount != 50 {
			t.Fatalf("unexpected first stage stat: %+v", chronicle.Stages[0])
		}

		// 80 victories: try_mode (50) unlocked, image_setting (100) locked
		var tryModeUnlocked, imageSettingUnlocked bool
		for _, m := range chronicle.Milestones {
			if m.Key == "try_mode" {
				tryModeUnlocked = m.Unlocked
			}
			if m.Key == "image_setting" {
				imageSettingUnlocked = m.Unlocked
			}
		}
		if !tryModeUnlocked {
			t.Fatal("expected try_mode milestone to be unlocked at 80 victories")
		}
		if imageSettingUnlocked {
			t.Fatal("expected image_setting milestone to be locked at 80 victories")
		}
	})
}

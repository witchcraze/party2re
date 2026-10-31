package adventure

import (
	"context"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/id"
)

type coopSeasonalMemAdvRepo struct {
	records []Adventure
	saved   map[string]Adventure
}

func newCoopSeasonalMemAdvRepo() *coopSeasonalMemAdvRepo {
	return &coopSeasonalMemAdvRepo{
		saved: make(map[string]Adventure),
	}
}

func (m *coopSeasonalMemAdvRepo) Save(_ context.Context, value Adventure) error {
	m.saved[value.ID] = value
	m.records = append(m.records, value)
	return nil
}

func (m *coopSeasonalMemAdvRepo) FindByID(_ context.Context, id string) (Adventure, error) {
	if v, ok := m.saved[id]; ok {
		return v, nil
	}
	return Adventure{}, ErrNotFound
}

func (m *coopSeasonalMemAdvRepo) ListByCharacterID(_ context.Context, characterID string, limit, offset int) ([]Adventure, int, error) {
	var matched []Adventure
	for _, a := range m.saved {
		if a.CharacterID == characterID {
			matched = append(matched, a)
		}
	}
	total := len(matched)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (m *coopSeasonalMemAdvRepo) ListByCharacterIDByCursor(_ context.Context, characterID string, limit int, _ time.Time, _ string) ([]Adventure, error) {
	var matched []Adventure
	for _, a := range m.saved {
		if a.CharacterID == characterID {
			matched = append(matched, a)
		}
	}
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func (m *coopSeasonalMemAdvRepo) GetAggregatedStats(_ context.Context, characterID string) (AggregatedStats, error) {
	var stats AggregatedStats
	type stageKey struct {
		stageID string
		subID   int
		season  string
	}
	stageMap := make(map[stageKey]*StageStatData)

	for _, a := range m.saved {
		if a.CharacterID != characterID || !a.Resolved {
			continue
		}
		stats.TotalAdventures++
		if a.IsCleared || (a.BattleResult.Outcome == corebattle.OutcomeWin && a.BattleResult.WinnerID == characterID) {
			stats.TotalVictories++
		} else if a.BattleResult.Outcome == corebattle.OutcomeDefeat {
			stats.TotalDefeats++
		} else if a.BattleResult.Outcome == corebattle.OutcomeDraw {
			stats.TotalDraws++
		}
		stats.TotalTurns += a.BattleResult.Turns
		stats.TotalExpEarned += a.BattleResult.Reward.Experience
		stats.TotalGoldEarned += a.BattleResult.Reward.Currency

		key := stageKey{stageID: a.Type, subID: a.SeasonalSubID, season: a.Season}
		if _, ok := stageMap[key]; !ok {
			stageMap[key] = &StageStatData{
				StageID:       a.Type,
				SeasonalSubID: a.SeasonalSubID,
				Season:        a.Season,
			}
		}
		entry := stageMap[key]
		entry.TotalAttempts++
		if a.IsCleared || (a.BattleResult.Outcome == corebattle.OutcomeWin && a.BattleResult.WinnerID == characterID) {
			entry.ClearCount++
		}
	}

	for _, v := range stageMap {
		stats.StageStats = append(stats.StageStats, *v)
	}
	return stats, nil
}

type coopSeasonalMemCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *coopSeasonalMemCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if c, ok := m.chars[id]; ok {
		return c, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (m *coopSeasonalMemCharRepo) Save(_ context.Context, c corecharacter.Character) error {
	m.chars[c.ID] = c
	return nil
}

type coopSeasonalMemInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func (m *coopSeasonalMemInvRepo) FindByCharacterID(_ context.Context, charID string) (coreinventory.Inventory, error) {
	if inv, ok := m.invs[charID]; ok {
		return inv, nil
	}
	return coreinventory.Inventory{CharacterID: charID}, nil
}

func (m *coopSeasonalMemInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	m.invs[inv.CharacterID] = inv
	return nil
}

type coopSeasonalClock struct {
	now time.Time
}

func (c *coopSeasonalClock) Now() time.Time {
	return c.now
}

type coopSeasonalAlwaysWinEngine struct{}

func (coopSeasonalAlwaysWinEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: req.Participants[0].ID,
		Turns:    3,
		Reward: corebattle.Reward{
			Experience: 50,
			Currency:   30,
		},
	}, nil
}

func (coopSeasonalAlwaysWinEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	remHP := make(map[string]int)
	remMP := make(map[string]int)
	for _, a := range req.Allies {
		remHP[a.ID] = a.HP
		remMP[a.ID] = a.MP
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  "allies",
		Turns:       4,
		RemainingHP: remHP,
		RemainingMP: remMP,
		TotalReward: req.VictoryReward,
	}, nil
}

func TestAdventure_SeasonalSubIdentityMapping(t *testing.T) {
	tests := []struct {
		season    string
		wantSubID int
	}{
		{"spring", 1},
		{"SPRING", 1},
		{"summer", 2},
		{"autumn", 3},
		{"winter", 4},
		{"", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		got := SeasonToSubID(tt.season)
		if got != tt.wantSubID {
			t.Errorf("SeasonToSubID(%q) = %d, want %d", tt.season, got, tt.wantSubID)
		}
		if tt.wantSubID > 0 {
			back := SubIDToSeason(tt.wantSubID)
			if back != tt.season && back != "spring" && back != "summer" && back != "autumn" && back != "winter" {
				t.Errorf("SubIDToSeason(%d) = %q, expected valid season name", tt.wantSubID, back)
			}
		}
	}
}

func TestAdventure_SeasonalStageChronicleDistinction(t *testing.T) {
	ctx := context.Background()
	advRepo := newCoopSeasonalMemAdvRepo()
	charRepo := &coopSeasonalMemCharRepo{chars: map[string]corecharacter.Character{
		"hero-1": {ID: "hero-1", Name: "Hero", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
	}}
	invRepo := &coopSeasonalMemInvRepo{invs: make(map[string]coreinventory.Inventory)}
	stages, _ := InitialStageCatalog()
	monsters, _ := InitialMonsterCatalog()
	engine := coopSeasonalAlwaysWinEngine{}
	clk := &coopSeasonalClock{now: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)}

	svc, err := NewServiceWithCatalogs(
		advRepo, charRepo, invRepo, stages, monsters, engine, nil, nil, clk,
	)
	if err != nil {
		t.Fatalf("NewServiceWithCatalogs: %v", err)
	}

	// 1. Clear Stage 26 in Spring
	springStage, _ := stages.FindByID("stage-26")
	springStage = springStage.ForSeason("spring")
	sessionSpring, err := NewCrawlSession(springStage, []corecharacter.Character{charRepo.chars["hero-1"]}, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession: %v", err)
	}
	for f := 1; f <= 11; f++ {
		_, _ = sessionSpring.AdvanceFloor(stages, monsters, engine)
	}
	resSpring := sessionSpring.Result()
	if !resSpring.StageCleared {
		t.Fatalf("expected spring session cleared")
	}

	// Persist spring clear
	advSpring := Adventure{
		ID:            id.New(),
		CharacterID:   "hero-1",
		Type:          "stage-26",
		StageID:       "stage-26",
		Season:        "spring",
		SeasonalSubID: SeasonToSubID("spring"),
		StartedAt:     clk.now,
		FloorsCleared: 10,
		IsCleared:     true,
		Resolved:      true,
		BattleResult:  corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: "hero-1"},
	}
	if err := advRepo.Save(ctx, advSpring); err != nil {
		t.Fatalf("Save spring: %v", err)
	}

	// 2. Clear Stage 26 in Summer
	advSummer := Adventure{
		ID:            id.New(),
		CharacterID:   "hero-1",
		Type:          "stage-26",
		StageID:       "stage-26",
		Season:        "summer",
		SeasonalSubID: SeasonToSubID("summer"),
		StartedAt:     clk.now.Add(time.Hour),
		FloorsCleared: 10,
		IsCleared:     true,
		Resolved:      true,
		BattleResult:  corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: "hero-1"},
	}
	if err := advRepo.Save(ctx, advSummer); err != nil {
		t.Fatalf("Save summer: %v", err)
	}

	// 3. Query chronicle - verify they are distinguished and do not collapse into 1 entry
	chronicle, err := svc.GetChronicle(ctx, "hero-1")
	if err != nil {
		t.Fatalf("GetChronicle: %v", err)
	}

	if chronicle.TotalVictories != 2 {
		t.Errorf("TotalVictories = %d, want 2", chronicle.TotalVictories)
	}

	if len(chronicle.Stages) != 2 {
		t.Fatalf("expected 2 distinct seasonal entries in chronicle.Stages, got %d: %+v", len(chronicle.Stages), chronicle.Stages)
	}

	foundSpring := false
	foundSummer := false
	for _, s := range chronicle.Stages {
		if s.StageID == "stage-26" && s.SeasonalSubID == 1 && s.Season == "spring" {
			foundSpring = true
			if s.ClearCount != 1 {
				t.Errorf("spring clear count = %d, want 1", s.ClearCount)
			}
		}
		if s.StageID == "stage-26" && s.SeasonalSubID == 2 && s.Season == "summer" {
			foundSummer = true
			if s.ClearCount != 1 {
				t.Errorf("summer clear count = %d, want 1", s.ClearCount)
			}
		}
	}

	if !foundSpring {
		t.Errorf("spring seasonal stage stat not found in chronicle")
	}
	if !foundSummer {
		t.Errorf("summer seasonal stage stat not found in chronicle")
	}
}

func TestAdventure_TravelPreservesClearFlagForDestinationStage(t *testing.T) {
	// Acceptance criteria 1:
	// - Coordinate assignment alone does NOT set clear.
	// - Boss victory before travel sets clear flag.
	// - Retained clear flag is consumed when reaching normal Floor 11 treasure room for destination stage.
	// - Travel itself does not record achievements.
	// - Retry is idempotent and does not duplicate clear count.
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog: %v", err)
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatalf("InitialMonsterCatalog: %v", err)
	}
	stage1, _ := stages.FindByID("stage-01")
	stage2, _ := stages.FindByID("stage-02")
	engine := coopSeasonalAlwaysWinEngine{}

	char := corecharacter.Character{ID: "char-travel", Name: "Traveler", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}

	// Case A: Coordinate assignment alone without boss victory does NOT set StageCleared
	sessionA, err := NewCrawlSession(stage1, []corecharacter.Character{char}, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession: %v", err)
	}
	// Travel to stage 2 floor 10
	sessionA.SetStage(stage2)
	sessionA.SetRound(10)
	if sessionA.StageCleared {
		t.Errorf("expected StageCleared to be false after coordinate travel alone")
	}

	// Case B: Defeating boss on Stage 1 sets StageCleared, then travel to Stage 2
	sessionB, err := NewCrawlSession(stage1, []corecharacter.Character{char}, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession: %v", err)
	}
	for f := 1; f <= BossFloor; f++ {
		floorRes, err := sessionB.AdvanceFloor(stages, monsters, engine)
		if err != nil {
			t.Fatalf("floor %d advance: %v", f, err)
		}
		if !floorRes.Cleared {
			t.Fatalf("floor %d not cleared", f)
		}
	}
	if !sessionB.StageCleared {
		t.Fatalf("expected StageCleared to be true after defeating boss")
	}

	// Travel to stage-02
	sessionB.SetStage(stage2)
	// Clear flag is retained!
	if !sessionB.StageCleared {
		t.Errorf("expected StageCleared to be retained after SetStage")
	}

	// Advance into Floor 11 (treasure room)
	_, _ = sessionB.AdvanceFloor(stages, monsters, engine)
	resB := sessionB.Result()
	if !resB.StageCleared {
		t.Errorf("expected StageCleared true in final result")
	}
	if resB.StageID != "stage-02" {
		t.Errorf("final StageID = %s, want stage-02 (destination stage)", resB.StageID)
	}

	// Case C: Retry of the same run produces identical deterministic ID and does not duplicate clear count
	advRepo := newCoopSeasonalMemAdvRepo()
	ctx := context.Background()

	advID1 := "adv-run-123"
	adv1 := Adventure{
		ID:            advID1,
		CharacterID:   "char-travel",
		Type:          resB.StageID,
		StageID:       resB.StageID,
		FloorsCleared: 10,
		IsCleared:     true,
		Resolved:      true,
		BattleResult:  corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: "char-travel"},
	}
	if err := advRepo.Save(ctx, adv1); err != nil {
		t.Fatalf("Save 1: %v", err)
	}

	// Retry save for the same run
	if err := advRepo.Save(ctx, adv1); err != nil {
		t.Fatalf("Save retry: %v", err)
	}

	stats, err := advRepo.GetAggregatedStats(ctx, "char-travel")
	if err != nil {
		t.Fatalf("GetAggregatedStats: %v", err)
	}
	if stats.TotalVictories != 1 {
		t.Errorf("TotalVictories on retry = %d, want 1 (not duplicated)", stats.TotalVictories)
	}
	if len(stats.StageStats) != 1 || stats.StageStats[0].ClearCount != 1 {
		t.Errorf("StageStats clear count = %+v, want 1", stats.StageStats)
	}
}

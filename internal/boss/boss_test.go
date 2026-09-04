package boss_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockBossRepo struct {
	records      map[string]boss.CharacterBossRecord
	histories    map[string][]boss.BossChallengeHistory
	charRepo     *mockCharRepo
	savedChars   map[string]corecharacter.Character
	awardedItems map[string][]coreitem.Instance
}

func newMockBossRepo() *mockBossRepo {
	return &mockBossRepo{
		records:      make(map[string]boss.CharacterBossRecord),
		histories:    make(map[string][]boss.BossChallengeHistory),
		savedChars:   make(map[string]corecharacter.Character),
		awardedItems: make(map[string][]coreitem.Instance),
	}
}

func (m *mockBossRepo) GetOrCreateRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error) {
	rec, ok := m.records[characterID]
	if !ok {
		now := time.Now().UTC()
		rec = boss.CharacterBossRecord{
			CharacterID:          characterID,
			HighestTierCleared:   0,
			TotalBossDefeats:     0,
			DailyAttemptsUsed:    0,
			DailyAttemptsResetAt: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		m.records[characterID] = rec
	}
	return rec, nil
}

func (m *mockBossRepo) RecordChallenge(
	ctx context.Context,
	history boss.BossChallengeHistory,
	record boss.CharacterBossRecord,
	character corecharacter.Character,
	rewardItem *coreitem.Instance,
) error {
	m.records[record.CharacterID] = record
	m.histories[record.CharacterID] = append([]boss.BossChallengeHistory{history}, m.histories[record.CharacterID]...)
	m.savedChars[character.ID] = character
	if m.charRepo != nil {
		m.charRepo.chars[character.ID] = character
	}
	if rewardItem != nil {
		m.awardedItems[character.ID] = append(m.awardedItems[character.ID], *rewardItem)
	}
	return nil
}

func (m *mockBossRepo) GetHistory(ctx context.Context, characterID string, limit int) ([]boss.BossChallengeHistory, error) {
	list := m.histories[characterID]
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockBossRepo) GetLeaderboard(ctx context.Context, limit int) ([]boss.BossLeaderboardEntry, error) {
	entries := make([]boss.BossLeaderboardEntry, 0)
	for _, rec := range m.records {
		entries = append(entries, boss.BossLeaderboardEntry{
			CharacterID:        rec.CharacterID,
			HighestTierCleared: rec.HighestTierCleared,
			TotalBossDefeats:   rec.TotalBossDefeats,
			FirstClearedAt:     rec.FirstClearedAt,
		})
	}
	return entries, nil
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, boss.ErrCharacterNotFound
	}
	return c, nil
}

func createTestChar(id string, level, hp, attack, defense int) corecharacter.Character {
	return corecharacter.Character{
		ID:    id,
		Name:  "Hero_" + id,
		Level: level,
		Stats: corecharacter.Stats{
			HP:      hp,
			MaxHP:   hp,
			Attack:  attack,
			Defense: defense,
			Agility: 50,
		},
		Money: 1000,
	}
}

func TestBossDailyAttemptReset(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	rec := boss.CharacterBossRecord{
		CharacterID:          "char1",
		DailyAttemptsUsed:    3,
		DailyAttemptsResetAt: time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC),
	}

	today := time.Now().UTC()
	rec.ResetDailyAttemptsIfExpired(today)

	if rec.DailyAttemptsUsed != 0 {
		t.Errorf("expected DailyAttemptsUsed to reset to 0, got %d", rec.DailyAttemptsUsed)
	}
}

func TestListBosses_LockUnlockAndPrereq(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"low_level":  createTestChar("low_level", 10, 100, 20, 20),
			"mid_level":  createTestChar("mid_level", 30, 300, 70, 50),
			"high_level": createTestChar("high_level", 99, 1000, 300, 200),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Low level character (Level 10) - Tier 1 requires Lv 15 -> locked
	statuses, err := service.ListBosses(ctx, "low_level")
	if err != nil {
		t.Fatalf("ListBosses error = %v", err)
	}
	if len(statuses) != 11 {
		t.Errorf("expected 11 boss encounters, got %d", len(statuses))
	}
	if statuses[0].IsUnlocked {
		t.Errorf("expected tier 1 to be locked for lv 10 character")
	}

	// 2. Mid level character (Level 30, cleared 0) -> Tier 1 unlocked, Tier 2 locked by prereq
	statuses, err = service.ListBosses(ctx, "mid_level")
	if err != nil {
		t.Fatal(err)
	}
	if !statuses[0].IsUnlocked {
		t.Errorf("expected tier 1 to be unlocked for lv 30 character")
	}
	if statuses[1].IsUnlocked {
		t.Errorf("expected tier 2 to be locked before clearing tier 1")
	}
}

func TestChallengeBoss_LevelGateAndPrerequisiteGate(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"low": createTestChar("low", 10, 100, 20, 20),
			"mid": createTestChar("mid", 30, 300, 70, 50),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Level Requirement Not Met
	_, err = service.ChallengeBoss(ctx, "low", "king-01")
	if !errors.Is(err, boss.ErrLevelRequirementNotMet) {
		t.Errorf("expected ErrLevelRequirementNotMet, got %v", err)
	}

	// 2. Prerequisite Requirement Not Met (attempting Tier 2 without clearing Tier 1)
	_, err = service.ChallengeBoss(ctx, "mid", "king-02")
	if !errors.Is(err, boss.ErrPrerequisiteNotMet) {
		t.Errorf("expected ErrPrerequisiteNotMet, got %v", err)
	}
}

func TestChallengeBoss_DailyAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"mid": createTestChar("mid", 30, 500, 150, 100),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// Exhaust 3 attempts
	for i := 0; i < 3; i++ {
		_, err := service.ChallengeBoss(ctx, "mid", "king-01")
		if err != nil {
			t.Fatalf("attempt %d failed: %v", i+1, err)
		}
	}

	// 4th attempt should fail with ErrDailyAttemptsExhausted
	_, err = service.ChallengeBoss(ctx, "mid", "king-01")
	if !errors.Is(err, boss.ErrDailyAttemptsExhausted) {
		t.Errorf("expected ErrDailyAttemptsExhausted, got %v", err)
	}
}

func TestChallengeBoss_VictoryFirstClearRewards(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": createTestChar("hero", 30, 1000, 300, 100),
		},
	}
	bossRepo.charRepo = charRepo
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. First Clear Victory against King 1 (Base: 300 EXP, 500 Gold; FirstClearBonus: 500 EXP, 1000 Gold)
	res, err := service.ChallengeBoss(ctx, "hero", "king-01")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin || res.BattleResult.WinnerID != "hero" {
		t.Fatalf("expected victory for hero, got outcome %v, winner %s", res.Outcome, res.BattleResult.WinnerID)
	}
	if !res.IsFirstClear {
		t.Errorf("expected first clear to be true")
	}
	if res.ExperienceReward != 800 { // 300 + 500
		t.Errorf("expected 800 EXP, got %d", res.ExperienceReward)
	}
	if res.GoldReward != 1500 { // 500 + 1000
		t.Errorf("expected 1500 Gold, got %d", res.GoldReward)
	}
	if res.SmallMedalsReward != 2 { // 1 + 1
		t.Errorf("expected 2 SmallMedals, got %d", res.SmallMedalsReward)
	}
	if res.ItemRewardID != "potion" {
		t.Errorf("expected potion drop, got %s", res.ItemRewardID)
	}
	if res.UpdatedRecord.HighestTierCleared != 1 {
		t.Errorf("expected HighestTierCleared=1, got %d", res.UpdatedRecord.HighestTierCleared)
	}
	if res.UpdatedRecord.TotalBossDefeats != 1 {
		t.Errorf("expected TotalBossDefeats=1, got %d", res.UpdatedRecord.TotalBossDefeats)
	}

	// 2. Repeat clear against King 1 (No FirstClearBonus: 300 EXP, 500 Gold, 1 Medal)
	res2, err := service.ChallengeBoss(ctx, "hero", "king-01")
	if err != nil {
		t.Fatalf("second challenge failed: %v", err)
	}
	if res2.IsFirstClear {
		t.Errorf("expected repeat clear to not be first clear")
	}
	if res2.ExperienceReward != 300 {
		t.Errorf("expected 300 EXP on repeat, got %d", res2.ExperienceReward)
	}
	if res2.GoldReward != 500 {
		t.Errorf("expected 500 Gold on repeat, got %d", res2.GoldReward)
	}
	if res2.SmallMedalsReward != 1 {
		t.Errorf("expected 1 SmallMedal on repeat, got %d", res2.SmallMedalsReward)
	}
	if res2.UpdatedRecord.TotalBossDefeats != 2 {
		t.Errorf("expected TotalBossDefeats=2, got %d", res2.UpdatedRecord.TotalBossDefeats)
	}

	savedChar := bossRepo.savedChars["hero"]
	if savedChar.SmallMedals != 3 { // 2 + 1
		t.Errorf("expected 3 total small medals on saved character, got %d", savedChar.SmallMedals)
	}

	// 3. Verify history and leaderboard
	history, err := service.GetHistory(ctx, "hero", 10)
	if err != nil || len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d (err: %v)", len(history), err)
	}

	leaderboard, err := service.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) != 1 {
		t.Errorf("expected 1 leaderboard entry, got %d", len(leaderboard))
	}
}

func TestChallengeBoss_Defeat(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"weakling": createTestChar("weakling", 20, 10, 5, 2),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.ChallengeBoss(ctx, "weakling", "king-01")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.BattleResult.WinnerID == "weakling" {
		t.Fatalf("expected defeat, got weakling victory")
	}
	if res.ExperienceReward != 0 || res.GoldReward != 0 {
		t.Errorf("expected 0 rewards on defeat, got EXP=%d, Gold=%d", res.ExperienceReward, res.GoldReward)
	}
	if res.UpdatedRecord.TotalBossDefeats != 0 {
		t.Errorf("expected TotalBossDefeats=0, got %d", res.UpdatedRecord.TotalBossDefeats)
	}
	if res.UpdatedRecord.DailyAttemptsUsed != 1 {
		t.Errorf("expected DailyAttemptsUsed=1, got %d", res.UpdatedRecord.DailyAttemptsUsed)
	}
}

func TestChallengeBoss_VictoryHook(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"strong-hero": createTestChar("strong-hero", 50, 1000, 200, 100),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	var hookedCharID string
	var hookedBossID string
	var hookedTier int
	service.SetVictoryHook(func(ctx context.Context, characterID string, bossID string, tier int) error {
		hookedCharID = characterID
		hookedBossID = bossID
		hookedTier = tier
		return nil
	})

	res, err := service.ChallengeBoss(ctx, "strong-hero", "king-01")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got outcome %v", res.Outcome)
	}
	if hookedCharID != "strong-hero" {
		t.Errorf("expected hookedCharID strong-hero, got %s", hookedCharID)
	}
	if hookedBossID != "king-01" {
		t.Errorf("expected hookedBossID king-01, got %s", hookedBossID)
	}
	if hookedTier != 1 {
		t.Errorf("expected hookedTier 1, got %d", hookedTier)
	}
}

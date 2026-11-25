package boss_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/party"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockBossRepo struct {
	settlements  map[string]*boss.SealingSettlement
	unsealedAt   *time.Time
	unsealErr    error
	records      map[string]boss.CharacterBossRecord
	histories    map[string][]boss.BossChallengeHistory
	savedChars   map[string]corecharacter.Character
	awardedItems map[string][]coreitem.Instance
	charRepo     *mockCharRepo
}

func newMockBossRepo() *mockBossRepo {
	return &mockBossRepo{
		records:      make(map[string]boss.CharacterBossRecord),
		histories:    make(map[string][]boss.BossChallengeHistory),
		savedChars:   make(map[string]corecharacter.Character),
		awardedItems: make(map[string][]coreitem.Instance),
	}
}

func (m *mockBossRepo) LockSealingSettlement(_ context.Context, roomID string) (*boss.SealingSettlement, error) {
	return m.settlements[roomID], nil
}
func (m *mockBossRepo) SaveSealingSettlement(_ context.Context, receipt boss.SealingSettlement) error {
	if m.settlements == nil {
		m.settlements = make(map[string]*boss.SealingSettlement)
	}
	m.settlements[receipt.Lobby.Party.ID] = &receipt
	return nil
}

func (m *mockBossRepo) GetUnsealedAt(_ context.Context) (*time.Time, error) {
	if m.unsealErr != nil {
		return nil, m.unsealErr
	}
	if m.unsealedAt == nil {
		return nil, nil
	}
	t := *m.unsealedAt
	return &t, nil
}

func (m *mockBossRepo) SaveUnsealedAt(_ context.Context, unsealedAt time.Time) error {
	t := unsealedAt.UTC()
	m.unsealedAt = &t
	return nil
}

func (m *mockBossRepo) GetOrCreateRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error) {
	rec, ok := m.records[characterID]
	if !ok {
		now := time.Now().UTC()
		rec = boss.CharacterBossRecord{
			CharacterID:        characterID,
			HighestTierCleared: 0,
			TotalBossDefeats:   0,
			CreatedAt:          now,
			UpdatedAt:          now,
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

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	if m.chars == nil {
		m.chars = make(map[string]corecharacter.Character)
	}
	m.chars[char.ID] = char
	return nil
}

type mockPartyRepo struct {
	parties map[string]party.Party
	members map[string][]party.Member
	deleted map[string]bool
}

func newMockPartyRepo() *mockPartyRepo {
	return &mockPartyRepo{
		parties: make(map[string]party.Party),
		members: make(map[string][]party.Member),
		deleted: make(map[string]bool),
	}
}

func (m *mockPartyRepo) WithPartyAdventureLock(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func (m *mockPartyRepo) GetPartyForUpdate(_ context.Context, id string) (party.Party, error) {
	p, ok := m.parties[id]
	if !ok || m.deleted[id] {
		return party.Party{}, boss.ErrPartyNotFound
	}
	return p, nil
}

func (m *mockPartyRepo) GetMembers(_ context.Context, partyID string) ([]party.Member, error) {
	if m.deleted[partyID] {
		return nil, boss.ErrPartyNotFound
	}
	return m.members[partyID], nil
}

func (m *mockPartyRepo) CleanupParty(ctx context.Context, state party.LobbyState) error {
	return m.DeleteParty(ctx, state.Party.ID)
}

func (m *mockPartyRepo) DeleteParty(_ context.Context, id string) error {
	m.deleted[id] = true
	return nil
}

func (m *mockPartyRepo) UpdateParty(_ context.Context, p party.Party) error {
	m.parties[p.ID] = p
	return nil
}

func (m *mockPartyRepo) OpenSystemRoom(_ context.Context, p party.Party) (bool, error) {
	for id, other := range m.parties {
		if other.StageID == p.StageID && !m.deleted[id] {
			return false, nil
		}
	}
	m.parties[p.ID] = p
	return true, nil
}

type mockNewsPublisher struct {
	published []string
}

func (m *mockNewsPublisher) PublishNews(_ context.Context, _, _, content, _ string, _ time.Time) error {
	m.published = append(m.published, content)
	return nil
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestListBosses_LockUnlock verifies that stages are locked/unlocked per
// the authentic need_join conditions (level gate) from the king*.cgi catalog.
func TestListBosses_LockUnlock(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"low_level":  createTestChar("low_level", 10, 100, 20, 20),
			"mid_level":  createTestChar("mid_level", 30, 500, 70, 50),
			"high_level": createTestChar("high_level", 99, 1000, 300, 200),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// Level 10 character — king1 requires MaxHP >= 400, should be locked.
	statuses, err := service.ListBosses(ctx, "low_level")
	if err != nil {
		t.Fatalf("ListBosses error = %v", err)
	}
	if len(statuses) != 11 {
		t.Errorf("expected 11 boss encounters (king1-10 + king99), got %d", len(statuses))
	}
	if statuses[0].IsUnlocked {
		t.Errorf("expected king1 to be locked for lv 10 character (MaxHP too low)")
	}

	// Level 30 character (hp=500, MaxHP>=400) — king1 should be unlocked; king2 locked (prereq: clear king1).
	statuses, err = service.ListBosses(ctx, "mid_level")
	if err != nil {
		t.Fatal(err)
	}
	if !statuses[0].IsUnlocked {
		t.Errorf("expected king1 to be unlocked for lv 30 character with MaxHP=500")
	}
	// king2 also has hp_400_o condition; a character with MaxHP=500 satisfies it.
	// The legacy vs_king system has no inter-stage prerequisite — each stage's access
	// is governed solely by its own need_join condition.
	if !statuses[1].IsUnlocked {
		t.Errorf("expected king2 to be unlocked for character meeting hp_400_o condition")
	}
}

// TestChallengeBoss_NeedJoinNotMet verifies that ChallengeBoss returns
// ErrNeedJoinNotMet when the character's level is too low.
func TestChallengeBoss_NeedJoinNotMet(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"low": createTestChar("low", 10, 100, 20, 20),
		},
	}
	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChallengeBoss(ctx, "low", "king1")
	if !errors.Is(err, boss.ErrNeedJoinNotMet) {
		t.Errorf("expected ErrNeedJoinNotMet, got %v", err)
	}
}

// TestChallengeBoss_ExhaustedCharacter verifies that a character with
// Tired >= 100 cannot enter a sealing battle.
func TestChallengeBoss_ExhaustedCharacter(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	char := createTestChar("tired", 30, 300, 70, 50)
	char.Tired = 100
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"tired": char}}

	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChallengeBoss(ctx, "tired", "king1")
	if !errors.Is(err, boss.ErrCharacterExhausted) {
		t.Errorf("expected ErrCharacterExhausted, got %v", err)
	}
}

// TestChallengeBoss_Victory verifies that a strong character wins against
// king-01, gains HeroCount, and accumulates history.
func TestChallengeBoss_Victory(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			// king1 boss (破壊神) has HP=150000, Attack=600, Defense=300.
			// Use a supremely powerful character to guarantee victory.
			"hero": createTestChar("hero", 99, 500000, 999999, 99999),
		},
	}
	bossRepo.charRepo = charRepo

	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.ChallengeBoss(ctx, "hero", "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got outcome %v", res.Outcome)
	}
	if res.HeroCountGained != 1 {
		t.Errorf("expected HeroCountGained=1, got %d", res.HeroCountGained)
	}
	if res.RewardExp <= 0 {
		t.Errorf("expected positive RewardExp, got %d", res.RewardExp)
	}

	// Second win accumulates TotalBossDefeats.
	_, err = service.ChallengeBoss(ctx, "hero", "king1")
	if err != nil {
		t.Fatalf("second ChallengeBoss failed: %v", err)
	}

	history, err := service.GetHistory(ctx, "hero", 10)
	if err != nil || len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d (err: %v)", len(history), err)
	}

	leaderboard, err := service.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) != 1 {
		t.Errorf("expected 1 leaderboard entry, got %d", len(leaderboard))
	}
}

// TestChallengeBoss_WithNowFunc_WeekdayOrb verifies that injecting WithNowFunc
// correctly determines the day-of-week orb based on JST time during boss victory.
func TestChallengeBoss_WithNowFunc_WeekdayOrb(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": createTestChar("hero", 99, 500000, 999999, 99999),
		},
	}
	bossRepo.charRepo = charRepo

	// Custom boss stage with empty TreasureItemIDs so the day-of-week orb is the only candidate.
	customStage := boss.BossStage{
		ID:         "test-king-orb",
		Name:       "Test Orb Boss",
		LeaderName: "Orb Master",
		Speed:      10,
		MaxMembers: 1,
		Bosses: []boss.BossMonster{
			{
				Name:    "Boss Minion",
				HP:      10,
				MaxHP:   10,
				Attack:  1,
				Defense: 1,
			},
		},
		TreasureItemIDs: nil, // Only the day-of-week orb will be in candidates
	}

	// 2026-09-27 23:00:00 UTC is Monday 08:00:00 JST -> should drop item-060
	jstMonday := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)

	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{}, customStage)
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithNowFunc(func() time.Time {
		return jstMonday
	}))

	res, err := service.ChallengeBoss(ctx, "hero", "test-king-orb")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got %v", res.Outcome)
	}
	if res.RewardItemID != "item-060" {
		t.Errorf("expected JST Monday orb 'item-060', got '%s'", res.RewardItemID)
	}
}

// TestChallengeBoss_Defeat verifies that a weak character loses without gaining rewards.
func TestChallengeBoss_Defeat(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			// MaxHP=500 satisfies king1's hp_400_o condition; attack=1 ensures defeat.
			"weakling": createTestChar("weakling", 20, 500, 1, 1),
		},
	}
	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.ChallengeBoss(ctx, "weakling", "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome == corebattle.OutcomeWin {
		t.Fatal("expected defeat, got victory")
	}
	if res.HeroCountGained != 0 {
		t.Errorf("expected HeroCountGained=0 on defeat, got %d", res.HeroCountGained)
	}
	if res.RewardExp != 0 || res.RewardGold != 0 {
		t.Errorf("expected zero rewards on defeat, got EXP=%d Gold=%d", res.RewardExp, res.RewardGold)
	}
	if got := charRepo.chars["weakling"]; got.Stats.HP != 0 {
		t.Errorf("expected defeated weakling HP=0, got %d", got.Stats.HP)
	}
}

// TestService_GetCharacterRecord verifies empty-ID guard and record creation.
func TestService_GetCharacterRecord(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("empty character ID returns error", func(t *testing.T) {
		_, err := service.GetCharacterRecord(ctx, "")
		if !errors.Is(err, boss.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("valid character retrieves or creates record", func(t *testing.T) {
		rec, err := service.GetCharacterRecord(ctx, "char-rec-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.CharacterID != "char-rec-1" {
			t.Errorf("expected character ID char-rec-1, got %s", rec.CharacterID)
		}
	})
}

// TestChallengeBoss_TiredPenalty verifies that entry costs +20 Tired.
func TestChallengeBoss_TiredPenalty(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charInitial := createTestChar("hero2", 30, 1000, 300, 100)
	charInitial.Tired = 0
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"hero2": charInitial}}
	bossRepo.charRepo = charRepo

	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChallengeBoss(ctx, "hero2", "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	saved, ok := charRepo.chars["hero2"]
	if !ok {
		t.Fatal("saved character not found")
	}
	// Entry cost is +20; a win may also have additional changes, but Tired should be at least 20.
	if saved.Tired < 20 {
		t.Errorf("expected Tired >= 20 after battle, got %d", saved.Tired)
	}
}

type stubPartyBattleEngine struct {
	resolveFn func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

func (e stubPartyBattleEngine) Resolve(_ corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{Outcome: corebattle.OutcomeWin}, nil
}

func (e stubPartyBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	if e.resolveFn != nil {
		return e.resolveFn(req)
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  "allies",
		Turns:       3,
		TotalReward: req.VictoryReward,
	}, nil
}

// TestStartSealingBattle_PartyVictoryAndResealing verifies authentic 4-player party
// sealing battle (vs_king.cgi): all members gain +1 HeroCount, entry fatigue +20%,
// server news announcement, celebration banquet hook, and party disbandment.
func TestStartSealingBattle_PartyVictoryAndResealing(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"leader": createTestChar("leader", 50, 600, 300, 200),
		"m1":     createTestChar("m1", 50, 600, 300, 200),
		"m2":     createTestChar("m2", 50, 600, 300, 200),
		"m3":     createTestChar("m3", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}
	bossRepo.charRepo = charRepo

	partyRepo := newMockPartyRepo()
	pID := "party-seal-1"
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "leader",
		StageID:           "king1",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king1"),
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "leader", CharacterName: "Hero_leader", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "m1", CharacterName: "Hero_m1", ReadyState: true},
		{PartyID: pID, CharacterID: "m2", CharacterName: "Hero_m2", ReadyState: true},
		{PartyID: pID, CharacterID: "m3", CharacterName: "Hero_m3", ReadyState: true},
	}

	newsPub := &mockNewsPublisher{}
	banquetCalled := false
	var banquetBossID, banquetLeaderID string

	engine := stubPartyBattleEngine{}
	service, err := boss.NewService(bossRepo, charRepo, engine)
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(
		boss.WithPartyRepository(partyRepo),
		boss.WithNewsPublisher(newsPub),
	)

	service.SetVictoryBanquetHook(func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error {
		banquetCalled = true
		banquetBossID = bossID
		banquetLeaderID = slayerID
		return nil
	})

	res, err := service.StartSealingBattle(ctx, pID, "leader")
	if err != nil {
		t.Fatalf("StartSealingBattle failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got %v", res.Outcome)
	}
	if res.HeroCountGained != 1 {
		t.Errorf("expected HeroCountGained=1, got %d", res.HeroCountGained)
	}

	// 1. All 4 members must have HeroCount incremented from 0 to 1
	for _, cID := range []string{"leader", "m1", "m2", "m3"} {
		c := charRepo.chars[cID]
		if c.HeroCount != 1 {
			t.Errorf("expected character %s HeroCount=1, got %d", cID, c.HeroCount)
		}
		if c.Tired != 0 {
			t.Errorf("sealing settlement charged %s entry fatigue: Tired=%d", cID, c.Tired)
		}
	}

	// 2. News announcement broadcast
	if len(newsPub.published) == 0 {
		t.Errorf("expected news announcement to be published")
	} else if wantPrefix := "勇者"; len(newsPub.published[0]) < len(wantPrefix) {
		t.Errorf("unexpected news content: %s", newsPub.published[0])
	}

	// 3. Victory banquet hook triggered
	if !banquetCalled {
		t.Errorf("expected celebration banquet hook to be invoked")
	}
	if banquetBossID != "king1" || banquetLeaderID != "leader" {
		t.Errorf("unexpected banquet hook args: bossID=%s, leaderID=%s", banquetBossID, banquetLeaderID)
	}

	// 4. Party deleted/disbanded after sealing
	if !partyRepo.deleted[pID] {
		t.Errorf("expected party to be deleted after sealing battle")
	}
}

// TestStartSealingBattle_WithNowFunc_WeekdayOrb verifies that party sealing battle
// awards the correct JST weekday orb when WithNowFunc is provided.
func TestStartSealingBattle_WithNowFunc_WeekdayOrb(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"lead": createTestChar("lead", 50, 2000, 500, 300),
			"mem":  createTestChar("mem", 50, 2000, 500, 300),
		},
	}
	bossRepo.charRepo = charRepo

	pID := "test-party-orb"
	partyRepo := newMockPartyRepo()
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "lead",
		StageID:           "party-orb-stage",
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "lead", CharacterName: "Hero_lead", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "mem", CharacterName: "Hero_mem", ReadyState: true},
	}

	customStage := boss.BossStage{
		ID:         "party-orb-stage",
		Name:       "Party Orb Stage",
		LeaderName: "Orb King",
		Speed:      10,
		MaxMembers: 2,
		Bosses: []boss.BossMonster{
			{
				Name:    "Minion",
				HP:      10,
				MaxHP:   10,
				Attack:  1,
				Defense: 1,
			},
		},
		TreasureItemIDs: nil, // Only orb will be added to candidates
	}

	// 2026-09-27 23:00:00 UTC = 2026-09-28 08:00:00 JST (Monday) -> item-060
	jstMonday := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)

	room := partyRepo.parties[pID]
	room.Kind, room.Enemies = party.KindSealing, boss.RosterForTest(customStage)
	partyRepo.parties[pID] = room
	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{}, customStage)
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(
		boss.WithPartyRepository(partyRepo),
		boss.WithNowFunc(func() time.Time {
			return jstMonday
		}),
	)

	res, err := service.StartSealingBattle(ctx, pID, "lead")
	if err != nil {
		t.Fatalf("StartSealingBattle failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got %v", res.Outcome)
	}
	if res.RewardItemID != "item-060" {
		t.Errorf("expected JST Monday orb 'item-060', got '%s'", res.RewardItemID)
	}
}

// TestStartSealingBattle_DejonBanishmentInParty verifies that when a boss casts
// Dejon (デジョン), the affected fallen participant incurs +30% Tired penalty.
func TestStartSealingBattle_DejonBanishmentInParty(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"leader": createTestChar("leader", 50, 600, 300, 200),
		"victim": createTestChar("victim", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}
	bossRepo.charRepo = charRepo

	partyRepo := newMockPartyRepo()
	pID := "party-dejon-1"
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "leader",
		StageID:           "king1",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king1"),
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "leader", CharacterName: "Hero_leader", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "victim", CharacterName: "Hero_victim", ReadyState: true},
	}

	// Engine simulates battle where "victim" gets banished by Dejon
	engine := stubPartyBattleEngine{
		resolveFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			return corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				WinnerSide:  "allies",
				Turns:       2,
				TotalReward: req.VictoryReward,
				RemainingHP: map[string]int{
					"leader": 400,
					"victim": 0,
				},
				BanishedIDs: map[string]bool{
					"victim": true,
				},
			}, nil
		},
	}

	service, err := boss.NewService(bossRepo, charRepo, engine)
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithPartyRepository(partyRepo))

	res, err := service.StartSealingBattle(ctx, pID, "leader")
	if err != nil {
		t.Fatalf("StartSealingBattle failed: %v", err)
	}

	if len(res.BanishedMemberIDs) != 1 || res.BanishedMemberIDs[0] != "victim" {
		t.Errorf("expected banished member 'victim', got %v", res.BanishedMemberIDs)
	}

	// victim: entry cost +20 + dejon banishment +30 = 50 Tired, HP = 0
	victim := charRepo.chars["victim"]
	if victim.Tired != 30 {
		t.Errorf("expected victim Tired=30 (dejon only; entry is charged at join), got %d", victim.Tired)
	}
	if victim.Stats.HP != 0 {
		t.Errorf("expected fallen victim HP=0, got %d", victim.Stats.HP)
	}

	if leader := charRepo.chars["leader"]; leader.Tired != 0 {
		t.Errorf("settlement charged the survivor entry fatigue: Tired=%d", leader.Tired)
	}
}

// TestStartSealingBattle_Validations verifies that any room member, ready or
// not, may start a sealing room's battle, while outsiders and ordinary lobbies cannot.
func TestStartSealingBattle_Validations(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"joiner":   createTestChar("joiner", 50, 600, 300, 200),
		"other":    createTestChar("other", 50, 600, 300, 200),
		"outsider": createTestChar("outsider", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}
	bossRepo.charRepo = charRepo

	partyRepo := newMockPartyRepo()
	pID := "party-val-1"
	partyRepo.parties[pID] = party.Party{ID: pID, Kind: party.KindSealing, StageID: "king1", Enemies: catalogRoster("king1"), Status: party.StatusRecruiting}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "joiner", CharacterName: "Hero_joiner"},
		{PartyID: pID, CharacterID: "other", CharacterName: "Hero_other"},
	}
	partyRepo.parties["ordinary"] = party.Party{ID: "ordinary", LeaderCharacterID: "outsider", StageID: "king1", Status: party.StatusRecruiting}
	partyRepo.members["ordinary"] = []party.Member{{PartyID: "ordinary", CharacterID: "outsider", IsLeader: true, ReadyState: true}}

	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithPartyRepository(partyRepo))

	if _, err := service.StartSealingBattle(ctx, "ordinary", "outsider"); !errors.Is(err, boss.ErrBossNotFound) {
		t.Errorf("ordinary lobby started a sealing battle: %v", err)
	}
	if _, err := service.StartSealingBattle(ctx, pID, "outsider"); !errors.Is(err, boss.ErrNotRoomMember) {
		t.Errorf("expected ErrNotRoomMember for an outsider, got %v", err)
	}
	if _, err := service.StartSealingBattle(ctx, pID, "other"); err != nil {
		t.Errorf("a room member who is not ready could not start: %v", err)
	}
}

// TestNoDailyAttemptsRestriction verifies that the fictional daily 3-entry solo raid
// limit has been completely removed: a character can challenge the boss multiple times
// without being blocked by daily attempt counters.
func TestNoDailyAttemptsRestriction(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	char := createTestChar("repeater", 99, 500000, 999999, 99999)
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"repeater": char}}
	bossRepo.charRepo = charRepo

	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	// Execute 5 consecutive challenges (previously blocked at 4th attempt with ErrDailyAttemptsExhausted)
	for i := 0; i < 5; i++ {
		// Reset Tired to allow next entry (entry cost is 20)
		c := charRepo.chars["repeater"]
		c.Tired = 0
		charRepo.chars["repeater"] = c

		res, err := service.ChallengeBoss(ctx, "repeater", "king1")
		if err != nil {
			t.Fatalf("challenge %d failed unexpectedly: %v", i+1, err)
		}
		if res.Outcome != corebattle.OutcomeWin {
			t.Fatalf("challenge %d expected win, got %v", i+1, res.Outcome)
		}
	}

	rec, err := bossRepo.GetOrCreateRecord(ctx, "repeater")
	if err != nil {
		t.Fatal(err)
	}
	if rec.TotalBossDefeats != 5 {
		t.Errorf("expected 5 boss defeats, got %d", rec.TotalBossDefeats)
	}
}

// TestCatalog_CapacityParity verifies that stages king1 through king10 have MaxMembers == 6,
// and king99 retains MaxMembers == 4 per legacy CGI specification.
func TestCatalog_CapacityParity(t *testing.T) {
	for _, stage := range boss.DefaultBossCatalog() {
		if stage.ID == "king99" {
			if stage.MaxMembers != 4 {
				t.Errorf("expected stage %s MaxMembers=4, got %d", stage.ID, stage.MaxMembers)
			}
		} else {
			if stage.MaxMembers != 6 {
				t.Errorf("expected stage %s MaxMembers=6, got %d", stage.ID, stage.MaxMembers)
			}
		}
	}
}

// TestStartSealingBattle_CapacityEnforcement verifies that a 6-player party can challenge king1,
// but a 5-player party attempting king99 (max 4) is rejected with ErrPartyNotReady.
func TestStartSealingBattle_CapacityEnforcement(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := make(map[string]corecharacter.Character)
	members6 := make([]party.Member, 6)
	for i := 0; i < 6; i++ {
		cID := fmt.Sprintf("hero-%d", i)
		chars[cID] = createTestChar(cID, 50, 600, 300, 200)
		members6[i] = party.Member{
			PartyID:       "party-6",
			CharacterID:   cID,
			CharacterName: "Hero_" + cID,
			ReadyState:    true,
			IsLeader:      i == 0,
		}
	}
	charRepo := &mockCharRepo{chars: chars}
	partyRepo := newMockPartyRepo()

	// 1. 6-player party on king1 succeeds
	pID6 := "party-6"
	partyRepo.parties[pID6] = party.Party{
		ID:                pID6,
		LeaderCharacterID: "hero-0",
		StageID:           "king1",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king1"),
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID6] = members6

	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithPartyRepository(partyRepo))

	res, err := service.StartSealingBattle(ctx, pID6, "hero-0")
	if err != nil {
		t.Fatalf("expected 6-player party to challenge king1 successfully, got err: %v", err)
	}
	if res.Outcome != corebattle.OutcomeWin {
		t.Errorf("expected win, got %v", res.Outcome)
	}

	// 2. 5-player party on king99 exceeds capacity (max 4) and is rejected
	pID5 := "party-5"
	members5 := members6[:5]
	for i := range members5 {
		members5[i].PartyID = pID5
	}
	partyRepo.parties[pID5] = party.Party{
		ID:                pID5,
		LeaderCharacterID: "hero-0",
		StageID:           "king99",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king99", boss.UnsealerForTest(chars["hero-0"])),
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID5] = members5

	_, err = service.StartSealingBattle(ctx, pID5, "hero-0")
	if !errors.Is(err, boss.ErrPartyNotReady) {
		t.Errorf("expected ErrPartyNotReady for 5-player party on king99 (max 4), got: %v", err)
	}
}

// TestStartSealingBattle_ClearTimeCrystals verifies that crystal multipliers use the persisted unseal
// timestamp (ignoring party.CreatedAt), default to 1x when not unsealed, apply per-enemy truncation,
// and restart when re-unsealed.
func TestStartSealingBattle_ClearTimeCrystals(t *testing.T) {
	ctx := context.Background()
	fakeNow := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

	runAt := func(t *testing.T, unsealedAt *time.Time, partyCreatedAt time.Time) int {
		t.Helper()
		bossRepo := newMockBossRepo()
		if unsealedAt != nil {
			u := *unsealedAt
			bossRepo.unsealedAt = &u
		}

		chars := map[string]corecharacter.Character{
			"c1": createTestChar("c1", 50, 600, 300, 200),
			"c2": createTestChar("c2", 50, 600, 300, 200),
		}
		charRepo := &mockCharRepo{chars: chars}
		partyRepo := newMockPartyRepo()

		pID := "party-crystal-test"
		partyRepo.parties[pID] = party.Party{
			ID:                pID,
			LeaderCharacterID: "c1",
			StageID:           "king1",
			Kind:              party.KindSealing,
			Enemies:           catalogRoster("king1"),
			Status:            party.StatusRecruiting,
			CreatedAt:         partyCreatedAt,
		}
		partyRepo.members[pID] = []party.Member{
			{PartyID: pID, CharacterID: "c1", CharacterName: "Hero_c1", ReadyState: true, IsLeader: true},
			{PartyID: pID, CharacterID: "c2", CharacterName: "Hero_c2", ReadyState: true},
		}

		service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
		if err != nil {
			t.Fatal(err)
		}
		service.Configure(
			boss.WithPartyRepository(partyRepo),
			boss.WithNowFunc(func() time.Time { return fakeNow }),
		)

		res, err := service.StartSealingBattle(ctx, pID, "c1")
		if err != nil {
			t.Fatalf("StartSealingBattle failed: %v", err)
		}
		if charRepo.chars["c1"].Crystal != res.RewardCrystals || charRepo.chars["c2"].Crystal != res.RewardCrystals {
			t.Fatalf("credited crystals mismatch: c1=%d c2=%d want=%d",
				charRepo.chars["c1"].Crystal, charRepo.chars["c2"].Crystal, res.RewardCrystals)
		}
		return res.RewardCrystals
	}

	at := func(d time.Duration) *time.Time {
		ts := fakeNow.Add(-d)
		return &ts
	}

	// Not unsealed -> 1.0x even when party was created 5s ago
	if got := runAt(t, nil, fakeNow.Add(-5*time.Second)); got != 70 {
		t.Errorf("not unsealed: got %d crystals, want 70", got)
	}

	// Boundaries ignore party.CreatedAt (set far in past 2h ago or fresh 5s ago)
	cases := []struct {
		name      string
		elapsed   time.Duration
		createdAt time.Time
		want      int
	}{
		{"600s (3x)", 600 * time.Second, fakeNow.Add(-2 * time.Hour), 210},
		{"601s (2x)", 601 * time.Second, fakeNow.Add(-5 * time.Second), 140},
		{"1800s (2x)", 1800 * time.Second, fakeNow.Add(-2 * time.Hour), 140},
		{"1801s (1.5x per enemy)", 1801 * time.Second, fakeNow.Add(-5 * time.Second), 102},
		{"3600s (1.5x per enemy)", 3600 * time.Second, fakeNow.Add(-2 * time.Hour), 102},
		{"3601s (1x)", 3601 * time.Second, fakeNow.Add(-5 * time.Second), 70},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runAt(t, at(tc.elapsed), tc.createdAt); got != tc.want {
				t.Errorf("%s: got %d crystals, want %d", tc.name, got, tc.want)
			}
		})
	}

	// Re-unseal restarts the clock: expired window becomes 3x again
	t.Run("re-unseal restarts clock", func(t *testing.T) {
		bossRepo := newMockBossRepo()
		expired := fakeNow.Add(-3601 * time.Second)
		_ = bossRepo.SaveUnsealedAt(ctx, expired)
		if got, _ := bossRepo.GetUnsealedAt(ctx); got == nil || !got.Equal(expired) {
			t.Fatalf("unexpected unsealedAt: %v", got)
		}
		restarted := fakeNow.Add(-120 * time.Second)
		_ = bossRepo.SaveUnsealedAt(ctx, restarted)

		chars := map[string]corecharacter.Character{
			"c1": createTestChar("c1", 50, 600, 300, 200),
		}
		charRepo := &mockCharRepo{chars: chars}
		partyRepo := newMockPartyRepo()
		partyRepo.parties["p-re"] = party.Party{
			ID: "p-re", LeaderCharacterID: "c1", StageID: "king1", Kind: party.KindSealing, Enemies: catalogRoster("king1"),
			Status: party.StatusRecruiting, CreatedAt: expired,
		}
		partyRepo.members["p-re"] = []party.Member{
			{PartyID: "p-re", CharacterID: "c1", ReadyState: true, IsLeader: true},
		}
		service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
		if err != nil {
			t.Fatal(err)
		}
		service.Configure(
			boss.WithPartyRepository(partyRepo),
			boss.WithNowFunc(func() time.Time { return fakeNow }),
		)
		res, err := service.StartSealingBattle(ctx, "p-re", "c1")
		if err != nil {
			t.Fatal(err)
		}
		if res.RewardCrystals != 210 {
			t.Errorf("restarted unseal: got %d crystals, want 210", res.RewardCrystals)
		}
	})

	t.Run("explicit clock reader and read error aborts", func(t *testing.T) {
		altClock := newMockBossRepo()
		ts := fakeNow.Add(-1801 * time.Second)
		altClock.unsealedAt = &ts

		bossRepo := newMockBossRepo()
		charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"c1": createTestChar("c1", 50, 600, 300, 200)}}
		partyRepo := newMockPartyRepo()
		partyRepo.parties["p-alt"] = party.Party{
			ID: "p-alt", LeaderCharacterID: "c1", StageID: "king1", Kind: party.KindSealing, Enemies: catalogRoster("king1"),
			Status: party.StatusRecruiting, CreatedAt: fakeNow,
		}
		partyRepo.members["p-alt"] = []party.Member{
			{PartyID: "p-alt", CharacterID: "c1", CharacterName: "Hero_c1", ReadyState: true, IsLeader: true},
		}
		service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
		if err != nil {
			t.Fatal(err)
		}
		service.Configure(
			boss.WithPartyRepository(partyRepo),
			boss.WithUnsealClockReader(altClock),
			boss.WithNowFunc(func() time.Time { return fakeNow }),
		)
		res, err := service.StartSealingBattle(ctx, "p-alt", "c1")
		if err != nil {
			t.Fatal(err)
		}
		if res.RewardCrystals != 102 {
			t.Errorf("expected 102 crystals from configured UnsealClockReader, got %d", res.RewardCrystals)
		}

		altClock.unsealErr = errors.New("clock db failure")
		partyRepo.parties["p-err"] = party.Party{
			ID: "p-err", LeaderCharacterID: "c1", StageID: "king1", Kind: party.KindSealing, Enemies: catalogRoster("king1"),
			Status: party.StatusRecruiting, CreatedAt: fakeNow,
		}
		partyRepo.members["p-err"] = []party.Member{
			{PartyID: "p-err", CharacterID: "c1", CharacterName: "Hero_c1", ReadyState: true, IsLeader: true},
		}
		if _, err := service.StartSealingBattle(ctx, "p-err", "c1"); err == nil {
			t.Fatal("expected error when GetUnsealedAt fails")
		}
	})
}

type mockBossMonsterDefeatRecorder struct {
	calls           []string
	records         []boss.DefeatedMonsterRecord
	recordsByCharID map[string][]boss.DefeatedMonsterRecord
}

func (m *mockBossMonsterDefeatRecorder) RecordMonsterDefeat(_ context.Context, charID string, record boss.DefeatedMonsterRecord) error {
	m.calls = append(m.calls, charID+":"+record.MonsterID+":"+record.MonsterName+":"+record.Habitat)
	m.records = append(m.records, record)
	if m.recordsByCharID == nil {
		m.recordsByCharID = make(map[string][]boss.DefeatedMonsterRecord)
	}
	m.recordsByCharID[charID] = append(m.recordsByCharID[charID], record)
	return nil
}

func TestStartSealingBattle_RecordMonsterDefeat(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"c1": createTestChar("c1", 50, 600, 300, 200),
		"c2": createTestChar("c2", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}
	partyRepo := newMockPartyRepo()

	pID := "party-defeat-test"
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "c1",
		StageID:           "king1",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king1"),
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "c1", CharacterName: "Hero_c1", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "c2", CharacterName: "Hero_c2", ReadyState: true},
	}

	recorder := &mockBossMonsterDefeatRecorder{}
	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(
		boss.WithPartyRepository(partyRepo),
		boss.WithMonsterDefeatRecorder(recorder),
	)

	_, err = service.StartSealingBattle(ctx, pID, "c1")
	if err != nil {
		t.Fatalf("StartSealingBattle failed: %v", err)
	}

	// king1 has 7 bosses, 2 party members -> 14 calls total
	if len(recorder.calls) != 14 {
		t.Fatalf("expected 14 recorder calls, got %d: %v", len(recorder.calls), recorder.calls)
	}

	// First call should be c1 against Red Stone (mon/190.gif -> monster-190)
	expectedFirst := "c1:monster-190:レッドストーン:封印戦"
	if recorder.calls[0] != expectedFirst {
		t.Errorf("expected call 0 = %q, got %q", expectedFirst, recorder.calls[0])
	}
	if len(recorder.records) != 14 {
		t.Fatalf("expected 14 records, got %d", len(recorder.records))
	}
	r0 := recorder.records[0]
	if r0.MonsterID != "monster-190" || r0.MonsterName != "レッドストーン" || r0.Habitat != "封印戦" ||
		r0.Icon != "mon/190.gif" || r0.HP != 50 || r0.MP != 999 || r0.Attack != 400 || r0.Defense != 9000 || r0.Agility != 3000 ||
		r0.ExperienceReward != 1000 || r0.GoldReward != 2000 {
		t.Errorf("unexpected boss record snapshot: %+v", r0)
	}
}

func TestChallengeBoss_RecordMonsterDefeat(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"hero-solo": createTestChar("hero-solo", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}

	recorder := &mockBossMonsterDefeatRecorder{}
	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithMonsterDefeatRecorder(recorder))

	_, err = service.ChallengeBoss(ctx, "hero-solo", "king99")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	// king99 clone defeat should record clone defeat
	if len(recorder.calls) != 1 {
		t.Fatalf("expected 1 recorder call for king99, got %d: %v", len(recorder.calls), recorder.calls)
	}
	expected := "hero-solo:king99-clone:影:封印戦"
	if recorder.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, recorder.calls[0])
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recorder.records))
	}
	r0 := recorder.records[0]
	if r0.MonsterID != "king99-clone" || r0.MonsterName != "影" || r0.Habitat != "封印戦" ||
		r0.Icon != "king99-clone" || r0.HP != 600*50 || r0.Attack != 300*2 || r0.Defense != 200*2 ||
		r0.ExperienceReward != 1500 || r0.GoldReward != 750 {
		t.Errorf("unexpected king99 record snapshot: %+v", r0)
	}
}

func TestChallengeBoss_King99_EncounterSnapshot_LevelUp(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	// Hero starts at Level 50, HP 600, Attack 300, Defense 200, Agility 50, JobLevel 0.
	// Set Experience such that gaining 1500 EXP triggers a level up.
	hero := createTestChar("hero-lvlup", 50, 600, 300, 200)
	nextExp50, err := progression.ExperienceForNextLevel(50)
	if err != nil {
		t.Fatal(err)
	}
	hero.Experience = nextExp50 - 1

	chars := map[string]corecharacter.Character{
		"hero-lvlup": hero,
	}
	charRepo := &mockCharRepo{chars: chars}

	recorder := &mockBossMonsterDefeatRecorder{}
	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithMonsterDefeatRecorder(recorder))

	_, err = service.ChallengeBoss(ctx, "hero-lvlup", "king99")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	// Verify hero leveled up after battle
	savedHero := charRepo.chars["hero-lvlup"]
	if savedHero.Level <= 50 {
		t.Fatalf("expected hero to level up beyond 50, got level %d", savedHero.Level)
	}

	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recorder.records))
	}
	r0 := recorder.records[0]
	// Max MP 0 clones to MP 0 (stage/king99.cgi); there is no fallback pool.
	expectedStrong := int(float64(600*50) + float64(300*2) + float64(200*2)*0.5 + float64(50*2))
	if r0.MonsterID != "king99-clone" || r0.MonsterName != "影" || r0.Habitat != "封印戦" ||
		r0.HP != 600*50 || r0.Attack != 300*2 || r0.Defense != 200*2 || r0.Agility != 50*2 ||
		r0.ExperienceReward != 1500 || r0.GoldReward != 750 || r0.Strong != expectedStrong {
		t.Errorf("king99 record mismatch: got %+v, expected HP=%d Atk=%d Def=%d Agi=%d Exp=1500 Gold=750 Strong=%d",
			r0, 600*50, 300*2, 200*2, 50*2, expectedStrong)
	}
}

// TestStartSealingBattle_King99_FightsTheUnsealingPartysClones verifies that a
// king99 room fights the clones stored at unseal (NPC companions included), not
// its joiners, and pays their EXP, gold, crystals and records (stage/king99.cgi).
func TestStartSealingBattle_King99_FightsTheUnsealingPartysClones(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	nextExp50, err := progression.ExperienceForNextLevel(50)
	if err != nil {
		t.Fatal(err)
	}
	c1 := createTestChar("c1", 50, 600, 300, 200)
	c1.JobLevel = 10
	c1.Experience = nextExp50 - 1
	c2 := createTestChar("c2", 30, 400, 150, 100) // joined later; never cloned
	unsealer := createTestChar("u3", 40, 500, 200, 100)
	npc := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "npc", Name: "@スライム", MaxHP: 100, Attack: 10, Defense: 5, Agility: 3, ExperienceReward: 4, GoldReward: 2}, NPC: true}

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"c1": c1, "c2": c2}}
	bossRepo.charRepo = charRepo
	partyRepo := newMockPartyRepo()
	pID := "party-king99-test"
	partyRepo.parties[pID] = party.Party{
		ID: pID, Kind: party.KindSealing, StageID: "king99", Status: party.StatusRecruiting, CreatedAt: time.Now().UTC(),
		Enemies: catalogRoster("king99", boss.UnsealerForTest(unsealer), boss.UnsealerForTest(c1), npc),
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "c1", CharacterName: "Hero_c1"},
		{PartyID: pID, CharacterID: "c2", CharacterName: "Hero_c2"},
	}

	recorder := &mockBossMonsterDefeatRecorder{}
	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithPartyRepository(partyRepo), boss.WithMonsterDefeatRecorder(recorder))

	res, err := service.StartSealingBattle(ctx, pID, "c2")
	if err != nil {
		t.Fatalf("StartSealingBattle failed: %v", err)
	}
	// EXP (lv+job_lv)*30 per clone: 40*30 + 60*30 + 4*30; gold int(lv/2)*30: 20*30 + 25*30 + 2*30.
	if res.RewardExp != 1200+1800+120 || res.RewardGold != 600+750+60 || res.RewardCrystals != 3*20 {
		t.Errorf("rewards = %d EXP, %d gold, %d crystals; want 3120, 1410, 60", res.RewardExp, res.RewardGold, res.RewardCrystals)
	}
	if charRepo.chars["c1"].Level <= 50 {
		t.Fatalf("expected c1 to level up beyond 50, got %d", charRepo.chars["c1"].Level)
	}
	for _, id := range []string{"c1", "c2"} {
		records := recorder.recordsByCharID[id]
		if len(records) != 3 {
			t.Fatalf("expected a record per clone for %s, got %+v", id, records)
		}
		if r := records[0]; r.MonsterID != "king99-clone" || r.HP != 500*50 || r.Attack != 200*2 || r.ExperienceReward != 1200 ||
			r.Strong != int(float64(500*50)+float64(200*2)+float64(100*2)*0.5+float64(50*2)) {
			t.Errorf("%s: first clone record should copy the unsealer u3, got %+v", id, r)
		}
		if r := records[2]; r.HP != 100*50 || r.GoldReward != 60 {
			t.Errorf("%s: NPC companion clone record mismatch: %+v", id, r)
		}
	}
}

func TestBoss_RealEngine_SynergyMultiplier(t *testing.T) {
	ctx := context.Background()

	// 1. Party Sealing Battle with 4 players using real corebattle.Engine{}
	bossRepo := newMockBossRepo()
	chars := map[string]corecharacter.Character{
		"lead": createTestChar("lead", 99, 500000, 999999, 99999),
		"m1":   createTestChar("m1", 99, 500000, 999999, 99999),
		"m2":   createTestChar("m2", 99, 500000, 999999, 99999),
		"m3":   createTestChar("m3", 99, 500000, 999999, 99999),
	}
	charRepo := &mockCharRepo{chars: chars}
	bossRepo.charRepo = charRepo

	partyRepo := newMockPartyRepo()
	pID := "party-synergy-boss"
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "lead",
		StageID:           "king1",
		Kind:              party.KindSealing,
		Enemies:           catalogRoster("king1"),
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "lead", CharacterName: "Hero_lead", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "m1", CharacterName: "Hero_m1", ReadyState: true},
		{PartyID: pID, CharacterID: "m2", CharacterName: "Hero_m2", ReadyState: true},
		{PartyID: pID, CharacterID: "m3", CharacterName: "Hero_m3", ReadyState: true},
	}

	partyService, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}
	partyService.Configure(boss.WithPartyRepository(partyRepo))

	res, err := partyService.StartSealingBattle(ctx, pID, "lead")
	if err != nil {
		t.Fatalf("StartSealingBattle with real engine failed: %v", err)
	}
	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got %v", res.Outcome)
	}
	if res.BattleResult.BonusPercent != 30 {
		t.Errorf("4-player boss battle expected BonusPercent=30, got %d", res.BattleResult.BonusPercent)
	}
	// Verify base vs total reward: TotalReward = BaseReward * 130 / 100
	expectedExp := res.BattleResult.BaseReward.Experience * 130 / 100
	expectedGold := res.BattleResult.BaseReward.Currency * 130 / 100
	if res.BattleResult.TotalReward.Experience != expectedExp {
		t.Errorf("expected TotalReward.Experience=%d, got %d", expectedExp, res.BattleResult.TotalReward.Experience)
	}
	if res.BattleResult.TotalReward.Currency != expectedGold {
		t.Errorf("expected TotalReward.Currency=%d, got %d", expectedGold, res.BattleResult.TotalReward.Currency)
	}

	// 2. Solo Boss Challenge with 1 player using real corebattle.Engine{}
	soloChars := map[string]corecharacter.Character{
		"solo": createTestChar("solo", 99, 500000, 999999, 99999),
	}
	soloCharRepo := &mockCharRepo{chars: soloChars}
	soloBossRepo := newMockBossRepo()
	soloBossRepo.charRepo = soloCharRepo

	soloService, err := boss.NewService(soloBossRepo, soloCharRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	soloRes, err := soloService.ChallengeBoss(ctx, "solo", "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss with real engine failed: %v", err)
	}
	if soloRes.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got %v", soloRes.Outcome)
	}
	if soloRes.BattleResult.BonusPercent != 0 {
		t.Errorf("solo boss battle expected BonusPercent=0, got %d", soloRes.BattleResult.BonusPercent)
	}
	if soloRes.BattleResult.TotalReward.Experience != soloRes.BattleResult.BaseReward.Experience {
		t.Errorf("solo boss battle TotalReward.Experience (%d) != BaseReward (%d)",
			soloRes.BattleResult.TotalReward.Experience, soloRes.BattleResult.BaseReward.Experience)
	}
}

func catalogRoster(stageID string, unsealers ...corebattle.LifetimeActor) []corebattle.LifetimeActor {
	for _, st := range boss.DefaultBossCatalog() {
		if st.ID == stageID {
			return boss.RosterForTest(st, unsealers...)
		}
	}
	panic("unknown stage " + stageID)
}

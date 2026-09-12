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
	"github.com/witchcraze/party2re/internal/party"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockBossRepo struct {
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

func (m *mockPartyRepo) DeleteParty(_ context.Context, id string) error {
	m.deleted[id] = true
	return nil
}

func (m *mockPartyRepo) UpdateParty(_ context.Context, p party.Party) error {
	m.parties[p.ID] = p
	return nil
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
}

// TestChallengeBoss_VictoryHook verifies the victory hook is called with correct args.
func TestChallengeBoss_VictoryHook(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"strong-hero": createTestChar("strong-hero", 99, 500000, 999999, 99999),
		},
	}
	service, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
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

	res, err := service.ChallengeBoss(ctx, "strong-hero", "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory, got outcome %v", res.Outcome)
	}
	if hookedCharID != "strong-hero" {
		t.Errorf("expected hookedCharID strong-hero, got %s", hookedCharID)
	}
	if hookedBossID != "king1" {
		t.Errorf("expected hookedBossID king-01, got %s", hookedBossID)
	}
	if hookedTier != 1 {
		t.Errorf("expected hookedTier 1, got %d", hookedTier)
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
		if c.Tired < 20 {
			t.Errorf("expected character %s Tired >= 20, got %d", cID, c.Tired)
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

	// victim: entry cost +20 + dejon banishment +30 = 50 Tired
	victim := charRepo.chars["victim"]
	if victim.Tired != 50 {
		t.Errorf("expected victim Tired=50 (20 entry + 30 dejon), got %d", victim.Tired)
	}

	// leader: entry cost only +20 Tired
	leader := charRepo.chars["leader"]
	if leader.Tired != 20 {
		t.Errorf("expected leader Tired=20, got %d", leader.Tired)
	}
}

// TestStartSealingBattle_Validations verifies guard conditions:
// non-leader cannot start, and all members must be ready.
func TestStartSealingBattle_Validations(t *testing.T) {
	ctx := context.Background()
	bossRepo := newMockBossRepo()

	chars := map[string]corecharacter.Character{
		"leader": createTestChar("leader", 50, 600, 300, 200),
		"member": createTestChar("member", 50, 600, 300, 200),
	}
	charRepo := &mockCharRepo{chars: chars}

	partyRepo := newMockPartyRepo()
	pID := "party-val-1"
	partyRepo.parties[pID] = party.Party{
		ID:                pID,
		LeaderCharacterID: "leader",
		StageID:           "king1",
		Status:            party.StatusRecruiting,
	}
	partyRepo.members[pID] = []party.Member{
		{PartyID: pID, CharacterID: "leader", CharacterName: "Hero_leader", ReadyState: true, IsLeader: true},
		{PartyID: pID, CharacterID: "member", CharacterName: "Hero_member", ReadyState: false},
	}

	service, err := boss.NewService(bossRepo, charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	service.Configure(boss.WithPartyRepository(partyRepo))

	// 1. Non-leader cannot start sealing battle
	_, err = service.StartSealingBattle(ctx, pID, "member")
	if !errors.Is(err, boss.ErrNotPartyLeader) {
		t.Errorf("expected ErrNotPartyLeader, got %v", err)
	}

	// 2. Member not ready
	_, err = service.StartSealingBattle(ctx, pID, "leader")
	if !errors.Is(err, boss.ErrPartyNotReady) {
		t.Errorf("expected ErrPartyNotReady, got %v", err)
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

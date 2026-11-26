package party

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type mockPartyRepository struct {
	parties              map[string]Party
	members              map[string]map[string]Member // partyID -> characterID -> Member
	memberParty          map[string]string            // characterID -> partyID
	logs                 []PartyAdventureLog
	saveLogErr           error
	savePartyErr         error
	updatePartyErr       error
	updatePartyCallCount int
	failUpdatePartyOn    int // if non-zero, fail on this N-th call
	deletePartyErr       error
	advLocksMu           sync.Mutex
	advLocks             map[string]string
}

func newMockPartyRepository() *mockPartyRepository {
	return &mockPartyRepository{
		parties:     make(map[string]Party),
		members:     make(map[string]map[string]Member),
		memberParty: make(map[string]string),
		advLocks:    make(map[string]string),
	}
}

func (r *mockPartyRepository) WithPartyAdventureLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error {
	if partyID == "" {
		return fn(ctx)
	}
	if isPartyAdventureLocked(ctx, partyID) {
		return fn(ctx)
	}

	r.advLocksMu.Lock()
	if r.advLocks == nil {
		r.advLocks = make(map[string]string)
	}
	if _, locked := r.advLocks[partyID]; locked {
		r.advLocksMu.Unlock()
		return ErrPartyNotRecruiting
	}
	r.advLocks[partyID] = "locked"
	r.advLocksMu.Unlock()

	defer func() {
		r.advLocksMu.Lock()
		delete(r.advLocks, partyID)
		r.advLocksMu.Unlock()
	}()

	lockedCtx := withPartyAdventureLockedContext(ctx, partyID)
	return fn(lockedCtx)
}

func (r *mockPartyRepository) SaveParty(_ context.Context, p Party) error {
	if r.savePartyErr != nil {
		return r.savePartyErr
	}
	r.parties[p.ID] = p
	return nil
}

func (r *mockPartyRepository) GetParty(_ context.Context, id string) (Party, error) {
	p, ok := r.parties[id]
	if !ok {
		return Party{}, ErrNotFound
	}
	return p, nil
}

func (r *mockPartyRepository) GetPartyForUpdate(_ context.Context, id string) (Party, error) {
	return r.GetParty(context.Background(), id)
}

func (r *mockPartyRepository) ListParties(_ context.Context, status string, limit, offset int) ([]PartySummary, int, error) {
	var list []PartySummary
	for _, p := range r.parties {
		if status != "" && p.Status != status {
			continue
		}
		memCount := len(r.members[p.ID])
		list = append(list, PartySummary{
			ID:                p.ID,
			Name:              p.Name,
			LeaderCharacterID: p.LeaderCharacterID,
			StageID:           p.StageID,
			Speed:             p.Speed,
			CurrentMembers:    memCount,
			MaxMembers:        p.MaxMembers,
			HasPassword:       p.PasswordHash != "",
			Status:            p.Status,
			CreatedAt:         p.CreatedAt,
		})
	}
	total := len(list)
	if offset >= len(list) {
		return []PartySummary{}, total, nil
	}
	end := offset + limit
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end], total, nil
}

func (r *mockPartyRepository) UpdateParty(_ context.Context, p Party) error {
	r.updatePartyCallCount++
	if r.failUpdatePartyOn > 0 && r.updatePartyCallCount == r.failUpdatePartyOn {
		return r.updatePartyErr
	}
	if r.failUpdatePartyOn == 0 && r.updatePartyErr != nil {
		return r.updatePartyErr
	}
	r.parties[p.ID] = p
	return nil
}

func (r *mockPartyRepository) DeleteParty(_ context.Context, id string) error {
	if r.deletePartyErr != nil {
		return r.deletePartyErr
	}
	delete(r.parties, id)
	for charID := range r.members[id] {
		delete(r.memberParty, charID)
	}
	delete(r.members, id)
	return nil
}

func (r *mockPartyRepository) AddMember(_ context.Context, m Member) error {
	if _, ok := r.members[m.PartyID]; !ok {
		r.members[m.PartyID] = make(map[string]Member)
	}
	m.BattleActor = cloneBattleActor(m.BattleActor)
	r.members[m.PartyID][m.CharacterID] = m
	r.memberParty[m.CharacterID] = m.PartyID
	return nil
}

func (r *mockPartyRepository) GetMembers(_ context.Context, partyID string) ([]Member, error) {
	mMap, ok := r.members[partyID]
	if !ok {
		return []Member{}, nil
	}
	var list []Member
	for _, m := range mMap {
		m.BattleActor = cloneBattleActor(m.BattleActor)
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsLeader != list[j].IsLeader {
			return list[i].IsLeader
		}
		return list[i].JoinedAt.Before(list[j].JoinedAt)
	})
	return list, nil
}

func (r *mockPartyRepository) GetMember(_ context.Context, partyID, characterID string) (Member, error) {
	mMap, ok := r.members[partyID]
	if !ok {
		return Member{}, ErrCharacterNotInParty
	}
	m, ok := mMap[characterID]
	if !ok {
		return Member{}, ErrCharacterNotInParty
	}
	return m, nil
}

func (r *mockPartyRepository) GetActivePartyByCharacter(_ context.Context, characterID string) (Party, Member, error) {
	partyID, ok := r.memberParty[characterID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	p, ok := r.parties[partyID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	m := r.members[partyID][characterID]
	return p, m, nil
}

func (r *mockPartyRepository) RemoveMember(_ context.Context, partyID, characterID string) error {
	mMap := r.members[partyID]
	delete(mMap, characterID)
	delete(r.memberParty, characterID)
	if p, ok := r.parties[partyID]; ok && p.LeaderCharacterID == characterID {
		var next Member
		for _, m := range mMap {
			if next.CharacterID == "" || m.JoinedAt.Before(next.JoinedAt) {
				next = m
			}
		}
		if next.CharacterID != "" {
			next.IsLeader = true
			mMap[next.CharacterID] = next
			p.LeaderCharacterID = next.CharacterID
			r.parties[partyID] = p
		}
	}
	return nil
}

func (r *mockPartyRepository) CountMembers(_ context.Context, partyID string) (int, error) {
	return len(r.members[partyID]), nil
}

func (r *mockPartyRepository) SaveAdventureLog(_ context.Context, log PartyAdventureLog) error {
	if r.saveLogErr != nil {
		return r.saveLogErr
	}
	r.logs = append(r.logs, log)
	return nil
}

func (r *mockPartyRepository) GetAdventureLogsByPartyID(_ context.Context, partyID string) ([]PartyAdventureLog, error) {
	var result []PartyAdventureLog
	for _, l := range r.logs {
		if l.PartyID == partyID {
			result = append(result, l)
		}
	}
	return result, nil
}

type mockCharacterRepository struct {
	chars     map[string]corecharacter.Character
	updateErr error
}

func newMockCharacterRepository() *mockCharacterRepository {
	return &mockCharacterRepository{
		chars: make(map[string]corecharacter.Character),
	}
}

func (r *mockCharacterRepository) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *mockCharacterRepository) FindByIDForUpdate(_ context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(context.Background(), id)
}

func (r *mockCharacterRepository) Update(_ context.Context, value corecharacter.Character) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.chars[value.ID] = value
	return nil
}

type mockTransactionProvider struct {
	rollbacks int
	commits   int
}

func (p *mockTransactionProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := fn(ctx); err != nil {
		p.rollbacks++
		return err
	}
	p.commits++
	return nil
}

type mockInventoryRepository struct {
	inventories map[string]coreinventory.Inventory
}

func (r *mockInventoryRepository) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterIDForUpdate(context.Background(), characterID)
}

func (r *mockInventoryRepository) FindByCharacterIDForUpdate(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := r.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		r.inventories[characterID] = inv
	}
	return inv, nil
}

func (r *mockInventoryRepository) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.inventories[inv.CharacterID] = inv
	return nil
}

type mockStageProvider struct {
	stages      map[string]adventure.Stage
	reqJobLevel map[string]int
}

func (p *mockStageProvider) FindByID(id string) (adventure.Stage, error) {
	s, ok := p.stages[id]
	if !ok {
		return adventure.Stage{}, adventure.ErrStageNotFound
	}
	return s, nil
}

func (p *mockStageProvider) CanAccessStage(c corecharacter.Character, stageID string) error {
	if _, ok := p.stages[stageID]; !ok {
		return adventure.ErrStageNotFound
	}
	if req := p.reqJobLevel[stageID]; req > 0 && c.JobLevel < req {
		return adventure.ErrJobLevelRequirementNotMet
	}
	return nil
}

type mockMonsterProvider struct {
	monsters map[string]adventure.Monster
}

func (p *mockMonsterProvider) FindByID(id string) (adventure.Monster, error) {
	m, ok := p.monsters[id]
	if !ok {
		return adventure.Monster{}, adventure.ErrMonsterNotFound
	}
	return m, nil
}

type mockNewsPublisher struct {
	published []string
}

func (m *mockNewsPublisher) PublishNews(_ context.Context, category, title, content, author string, publishedAt time.Time) error {
	m.published = append(m.published, title)
	return nil
}

func setupTestServiceWithEngine(
	t *testing.T,
	engine BattleEngine,
	opts ...Option,
) (*Service, *mockPartyRepository, *mockCharacterRepository, *mockNewsPublisher) {
	t.Helper()
	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	news := &mockNewsPublisher{}

	stages := &mockStageProvider{
		stages: map[string]adventure.Stage{
			"forest": {
				ID:         "forest",
				Name:       "はじまりの森",
				MonsterIDs: []string{"slime", "goblin"},
			},
		},
	}

	monsters := &mockMonsterProvider{
		monsters: map[string]adventure.Monster{
			"slime": {
				ID:               "slime",
				Name:             "スライム",
				HP:               20,
				Attack:           8,
				Defense:          2,
				ExperienceReward: 40,
				GoldReward:       20,
				DropItemIDs:      []string{"potion"},
			},
			"goblin": {
				ID:               "goblin",
				Name:             "ゴブリン",
				HP:               30,
				Attack:           12,
				Defense:          4,
				ExperienceReward: 60,
				GoldReward:       40,
			},
		},
	}

	if engine == nil {
		engine = fixedBattleEngine{outcome: battle.OutcomeWin}
	}

	serviceOpts := []Option{
		WithNewsPublisher(news),
		WithRNG(func(n int) int { return 0 }),
	}
	serviceOpts = append(serviceOpts, opts...)

	svc, err := NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		engine,
		serviceOpts...,
	)
	if err != nil {
		t.Fatal(err)
	}

	return svc, partyRepo, charRepo, news
}

func setupTestService(t *testing.T, opts ...Option) (*Service, *mockPartyRepository, *mockCharacterRepository, *mockNewsPublisher) {
	t.Helper()
	return setupTestServiceWithEngine(t, fixedBattleEngine{outcome: battle.OutcomeWin}, opts...)
}

func TestPartyService_CreateAndGet(t *testing.T) {
	svc, _, charRepo, news := setupTestService(t)

	// Setup leader character
	leader := corecharacter.Character{
		ID:    "char-leader",
		Name:  "LeaderHero",
		JobID: "warrior",
		Level: 10,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 20, Defense: 10},
	}
	charRepo.chars[leader.ID] = leader

	// 1. Successful party creation
	req := CreatePartyRequest{
		Name:       "勇者パーティ",
		StageID:    "forest",
		Password:   "secret",
		Speed:      3,
		MaxMembers: 4,
	}
	detail, err := svc.CreateParty(context.Background(), leader.ID, req)
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	if detail.Party.Name != "勇者パーティ" || detail.Party.LeaderCharacterID != leader.ID {
		t.Fatalf("unexpected party details: %+v", detail.Party)
	}
	if len(detail.Members) != 1 || !detail.Members[0].IsLeader {
		t.Fatalf("expected leader member, got: %+v", detail.Members)
	}
	if len(news.published) != 1 {
		t.Fatalf("expected news broadcast, got %d", len(news.published))
	}

	// 2. Fetch created party
	fetched, err := svc.GetParty(context.Background(), detail.Party.ID)
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if fetched.Party.ID != detail.Party.ID || len(fetched.Members) != 1 {
		t.Fatalf("unexpected fetched party: %+v", fetched)
	}

	// 3. Duplicate party creation rejection (already in party)
	if _, err := svc.CreateParty(context.Background(), leader.ID, req); !errors.Is(err, ErrAlreadyInParty) {
		t.Fatalf("expected ErrAlreadyInParty, got %v", err)
	}

	// 4. Invalid party name (empty, whitespace, prohibited characters, too long)
	for _, invalid := range []string{"", "   ", "has space", "全角　空白", "bad@name", "＠冒険", "bad/name", strings.Repeat("あ", 51)} {
		if _, err := svc.CreateParty(context.Background(), "char-other", CreatePartyRequest{Speed: 18, Name: invalid}); !errors.Is(err, ErrInvalidPartyName) {
			t.Fatalf("expected ErrInvalidPartyName for %q, got %v", invalid, err)
		}
	}

	// 5. Stage not found
	if _, err := svc.CreateParty(context.Background(), "char-other", CreatePartyRequest{Speed: 18, Name: "Party", StageID: "invalid-stage"}); !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("expected ErrStageNotFound, got %v", err)
	}
}

func TestPartyService_JoinAndLeave(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	// Setup Leader & Members
	leader := corecharacter.Character{
		ID:    "char-leader",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	m1 := corecharacter.Character{
		ID:    "char-m1",
		Name:  "Member1",
		Level: 10,
		Stats: corecharacter.Stats{HP: 40, MaxHP: 40},
	}
	mLowLevel := corecharacter.Character{
		ID:    "char-low",
		Name:  "LowLevel",
		Level: 2,
		Stats: corecharacter.Stats{HP: 20, MaxHP: 20},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1
	charRepo.chars[mLowLevel.ID] = mLowLevel

	detail, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:      18,
		Name:       "AdventureSquad",
		StageID:    "forest",
		Password:   "pass123",
		MaxMembers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	partyID := detail.Party.ID

	// 1. Password mismatch
	if _, err := svc.JoinParty(context.Background(), partyID, m1.ID, "wrongpass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}

	// 2. Joiners face no stage or job-level gate (quest.cgi add_member).
	stages := svc.stages.(*mockStageProvider)
	stages.reqJobLevel = map[string]int{"forest": 5}
	if err := stages.CanAccessStage(mLowLevel, "forest"); err == nil {
		t.Fatal("fixture must fail the creator stage gate")
	}
	if _, err := svc.JoinParty(context.Background(), partyID, mLowLevel.ID, "pass123"); err != nil {
		t.Fatalf("low-level joiner rejected: %v", err)
	}
	if err := svc.LeaveParty(context.Background(), partyID, mLowLevel.ID); err != nil {
		t.Fatal(err)
	}

	// 3. Successful join
	joinedDetail, err := svc.JoinParty(context.Background(), partyID, m1.ID, "pass123")
	if err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	if len(joinedDetail.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(joinedDetail.Members))
	}

	// 4. Party full rejection
	mExtra := corecharacter.Character{
		ID:    "char-extra",
		Name:  "Extra",
		Level: 10,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	charRepo.chars[mExtra.ID] = mExtra
	if _, err := svc.JoinParty(context.Background(), partyID, mExtra.ID, "pass123"); !errors.Is(err, ErrPartyFull) {
		t.Fatalf("expected ErrPartyFull, got %v", err)
	}

	// 5. Member leaves party
	if err := svc.LeaveParty(context.Background(), partyID, m1.ID); err != nil {
		t.Fatalf("LeaveParty failed: %v", err)
	}
	detailAfterLeave, _ := svc.GetParty(context.Background(), partyID)
	if len(detailAfterLeave.Members) != 1 {
		t.Fatalf("expected 1 member after leave, got %d", len(detailAfterLeave.Members))
	}
}

func TestPartyService_KickAndLeave(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	leader := corecharacter.Character{ID: "leader", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	m1 := corecharacter.Character{ID: "m1", Name: "M1", Level: 10, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	m2 := corecharacter.Character{ID: "m2", Name: "M2", Level: 10, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	for _, c := range []corecharacter.Character{leader, m1, m2} {
		charRepo.chars[c.ID] = c
	}

	detail, _ := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "PartyX",
		StageID: "forest",
	})
	partyID := detail.Party.ID
	for _, c := range []corecharacter.Character{m1, m2} {
		if _, err := svc.JoinParty(context.Background(), partyID, c.ID, ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond) // distinct join order
	}

	// 1. Non-leader cannot kick
	if _, err := svc.KickMember(context.Background(), partyID, m1.ID, leader.ID); !errors.Is(err, ErrNotPartyLeader) {
		t.Fatalf("expected ErrNotPartyLeader, got %v", err)
	}

	// 2. Leader cannot kick self
	if _, err := svc.KickMember(context.Background(), partyID, leader.ID, leader.ID); !errors.Is(err, ErrCannotKickSelf) {
		t.Fatalf("expected ErrCannotKickSelf, got %v", err)
	}

	// 3. The leader leaving hands leadership to the earliest remaining member.
	if err := svc.LeaveParty(context.Background(), partyID, leader.ID); err != nil {
		t.Fatalf("LeaveParty(leader) failed: %v", err)
	}
	after, err := svc.GetParty(context.Background(), partyID)
	if err != nil || after.Party.LeaderCharacterID != m1.ID || len(after.Members) != 2 || after.Members[0].CharacterID != m1.ID || !after.Members[0].IsLeader {
		t.Fatalf("leadership not handed over: %+v err=%v", after, err)
	}

	// 4. The new leader kicks a member.
	res, err := svc.KickMember(context.Background(), partyID, m1.ID, m2.ID)
	if err != nil {
		t.Fatalf("KickMember failed: %v", err)
	}
	if res.PartyName != "PartyX" || res.LeaderName != m1.Name {
		t.Fatalf("unexpected kick result: %+v", res)
	}

	// 5. The last member leaving deletes the lobby.
	if err := svc.LeaveParty(context.Background(), partyID, m1.ID); err != nil {
		t.Fatalf("LeaveParty(last) failed: %v", err)
	}
	if _, err := svc.GetParty(context.Background(), partyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after the last leave, got %v", err)
	}
	if err := svc.LeaveParty(context.Background(), partyID, m1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound leaving a deleted lobby, got %v", err)
	}
}

func TestPartyService_StartPartyAdventure(t *testing.T) {
	svc, partyRepo, charRepo, _ := setupTestService(t)

	leader := corecharacter.Character{
		ID:         "leader",
		Name:       "Hero1",
		Level:      5,
		Experience: 250,
		Money:      100,
		Stats:      corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 25, Defense: 10},
	}
	m1 := corecharacter.Character{
		ID:         "m1",
		Name:       "Hero2",
		Level:      5,
		Experience: 250,
		Money:      50,
		Stats:      corecharacter.Stats{HP: 45, MaxHP: 45, Attack: 20, Defense: 8},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1

	detail, _ := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "CoopParty",
		StageID: "forest",
	})
	partyID := detail.Party.ID
	_, _ = svc.JoinParty(context.Background(), partyID, m1.ID, "")

	// The leader starts without any readiness step (vs_monster.cgi:56–58).
	res, err := svc.StartPartyAdventure(context.Background(), partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected victory, got %s", res.Outcome)
	}
	if res.SynergyBonusPercent != 10 {
		t.Fatalf("expected 10%% synergy bonus for 2 players, got %d%%", res.SynergyBonusPercent)
	}
	if len(res.Rewards) != 2 {
		t.Fatalf("expected 2 member rewards, got %d", len(res.Rewards))
	}
	if len(partyRepo.logs) != 1 {
		t.Fatalf("expected adventure log saved, got %d", len(partyRepo.logs))
	}

	// Verify character rewards
	updatedLeader := charRepo.chars[leader.ID]
	if updatedLeader.Money <= leader.Money || updatedLeader.Experience <= leader.Experience {
		t.Fatalf("expected leader to gain gold and exp, got money=%d exp=%d", updatedLeader.Money, updatedLeader.Experience)
	}
}

func TestPartyService_StartPartyAdventure_CrystalRewards(t *testing.T) {
	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}

	stages := &mockStageProvider{
		stages: map[string]adventure.Stage{
			"forest": {
				ID:         "forest",
				Name:       "はじまりの森",
				MonsterIDs: []string{"slime"},
			},
		},
	}
	monsters := &mockMonsterProvider{
		monsters: map[string]adventure.Monster{
			"slime": {
				ID:               "slime",
				Name:             "スライム",
				HP:               20,
				Attack:           8,
				Defense:          2,
				ExperienceReward: 40,
				GoldReward:       20,
			},
		},
	}

	customBattleEngine := &mockCrystalPartyBattleEngine{crystalsPerFloor: 7}
	svc, err := NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		customBattleEngine,
	)
	if err != nil {
		t.Fatal(err)
	}

	leader := corecharacter.Character{
		ID:      "char-leader",
		Name:    "LeaderHero",
		JobID:   "warrior",
		Level:   10,
		Stats:   corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 50, Defense: 20},
		Crystal: 10,
	}
	charRepo.chars[leader.ID] = leader

	p, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Name:    "CrystalParty",
		StageID: "forest",
		Speed:   3,
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.StartPartyAdventure(context.Background(), p.Party.ID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected win, got %s", res.Outcome)
	}
	// 10 floors * 7 = 70 crystals
	if res.TotalCrystals != 70 {
		t.Errorf("res.TotalCrystals = %d, want 70", res.TotalCrystals)
	}
	if len(res.Rewards) != 1 || res.Rewards[0].GainedCrystals != 70 {
		t.Errorf("res.Rewards[0].GainedCrystals = %d, want 70", res.Rewards[0].GainedCrystals)
	}
	// Initial 10 + 70 = 80
	if charRepo.chars[leader.ID].Crystal != 80 {
		t.Errorf("charRepo.chars[leader.ID].Crystal = %d, want 80", charRepo.chars[leader.ID].Crystal)
	}
}

type mockCrystalPartyBattleEngine struct {
	crystalsPerFloor int
}

func (m *mockCrystalPartyBattleEngine) ResolvePartyBattle(req battle.PartyBattleRequest) (battle.PartyBattleResult, error) {
	remHP := make(map[string]int)
	for _, a := range req.Allies {
		remHP[a.ID] = a.HP
	}
	return battle.PartyBattleResult{
		Outcome:     battle.OutcomeWin,
		Turns:       1,
		RemainingHP: remHP,
		TotalReward: battle.Reward{
			Experience: 20,
			Currency:   10,
			Crystals:   m.crystalsPerFloor,
		},
	}, nil
}

func TestListParties(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	leader := corecharacter.Character{
		ID:    "char-leader",
		Name:  "LeaderHero",
		JobID: "warrior",
		Level: 10,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 20, Defense: 10},
	}
	charRepo.chars[leader.ID] = leader

	_, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Name:    "PartyOne",
		StageID: "forest",
		Speed:   3,
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	page, err := svc.ListParties(context.Background(), StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Limit != 10 || page.Offset != 0 {
		t.Fatalf("unexpected page result: %+v", page)
	}
	if page.Items[0].Name != "PartyOne" {
		t.Errorf("expected party name 'PartyOne', got %s", page.Items[0].Name)
	}
	if page.Items[0].StageName != "はじまりの森" {
		t.Errorf("expected stage name 'はじまりの森', got %q", page.Items[0].StageName)
	}
}

func TestPartyService_StartPartyAdventure_DuplicateCharacterNames(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	// Two characters with different IDs but exact same display name
	leader := corecharacter.Character{
		ID:         "char-same-1",
		Name:       "SameHero",
		Level:      5,
		Experience: 250,
		Money:      100,
		Stats:      corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 25, Defense: 10},
	}
	m1 := corecharacter.Character{
		ID:         "char-same-2",
		Name:       "SameHero",
		Level:      5,
		Experience: 250,
		Money:      50,
		Stats:      corecharacter.Stats{HP: 45, MaxHP: 45, Attack: 20, Defense: 8},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1

	detail, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "TwinParty",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := detail.Party.ID

	if _, err := svc.JoinParty(context.Background(), partyID, m1.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	res, err := svc.StartPartyAdventure(context.Background(), partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure with duplicate names failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected victory, got %s", res.Outcome)
	}
	if len(res.Rewards) != 2 {
		t.Fatalf("expected 2 distinct member rewards, got %d", len(res.Rewards))
	}
	if res.Rewards[0].CharacterID == res.Rewards[1].CharacterID {
		t.Fatalf("expected unique character IDs in rewards, got %s and %s", res.Rewards[0].CharacterID, res.Rewards[1].CharacterID)
	}
}

func TestPartyService_StartPartyAdventure_SurvivingDamagePersisted(t *testing.T) {
	engine := fixedBattleEngine{
		remainingHP: map[string]int{
			"char-dmg-leader": 80,
			"char-dmg-member": 0,
		},
		outcome: battle.OutcomeWin,
	}
	svc, _, charRepo, _ := setupTestServiceWithEngine(t, engine)

	leader := corecharacter.Character{
		ID:         "char-dmg-leader",
		Name:       "ToughHero",
		Level:      5,
		Experience: 250,
		Money:      100,
		Stats:      corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 15, Defense: 5},
	}
	m1 := corecharacter.Character{
		ID:         "char-dmg-member",
		Name:       "FragileHero",
		Level:      5,
		Experience: 250,
		Money:      50,
		Stats:      corecharacter.Stats{HP: 15, MaxHP: 50, Attack: 15, Defense: 2},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1

	detail, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "BattleDamageParty",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := detail.Party.ID

	if _, err := svc.JoinParty(context.Background(), partyID, m1.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	res, err := svc.StartPartyAdventure(context.Background(), partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	updatedLeader := charRepo.chars[leader.ID]
	updatedMember := charRepo.chars[m1.ID]

	// Leader took non-fatal damage and survived with HP < 100
	if updatedLeader.Stats.HP <= 0 {
		t.Errorf("expected leader to be alive, got HP=%d", updatedLeader.Stats.HP)
	}
	if updatedLeader.Stats.HP != 80 {
		t.Errorf("expected leader HP=80, got %d", updatedLeader.Stats.HP)
	}

	// Fallen member persists with HP = 0
	if updatedMember.Stats.HP != 0 {
		t.Errorf("expected member HP=0, got %d", updatedMember.Stats.HP)
	}
	t.Logf("Battle result outcome=%s, leader HP: %d/%d, member HP: %d/%d", res.Outcome, updatedLeader.Stats.HP, updatedLeader.Stats.MaxHP, updatedMember.Stats.HP, updatedMember.Stats.MaxHP)
}

func TestPartyService_StartPartyAdventure_OverLevelAndCap(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	// Leader with OverLevel at Lv 99
	leader := corecharacter.Character{
		ID:         "leader-overlevel",
		Name:       "OverHero",
		Level:      99,
		Experience: 99 * 99 * 10,
		Money:      1000,
		OverLevel:  true,
		Stats:      corecharacter.Stats{HP: 500, MaxHP: 500, Attack: 200, Defense: 150},
	}
	// Normal member without OverLevel at Lv 99
	normalMem := corecharacter.Character{
		ID:         "member-normal",
		Name:       "NormalHero",
		Level:      99,
		Experience: 99 * 99 * 10,
		Money:      1000,
		OverLevel:  false,
		Stats:      corecharacter.Stats{HP: 500, MaxHP: 500, Attack: 200, Defense: 150},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[normalMem.ID] = normalMem

	detail, err := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "HighLevelParty",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := detail.Party.ID

	if _, err := svc.JoinParty(context.Background(), partyID, normalMem.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	res, err := svc.StartPartyAdventure(context.Background(), partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected victory, got %s", res.Outcome)
	}

	updatedLeader := charRepo.chars[leader.ID]
	updatedNormal := charRepo.chars[normalMem.ID]

	// OverLevel character should advance to Lv 100+
	if updatedLeader.Level < 100 {
		t.Errorf("expected OverLevel character to reach level >= 100, got %d", updatedLeader.Level)
	}

	// Normal character should cap strictly at Lv 99
	if updatedNormal.Level != 99 {
		t.Errorf("expected normal character to remain capped at level 99, got %d", updatedNormal.Level)
	}
}

func TestPartyService_StartPartyAdventure_SaveAdventureLogError(t *testing.T) {
	ctx := context.Background()
	svc, partyRepo, charRepo, _ := setupTestService(t)

	leader := corecharacter.Character{
		ID:    "char-leader-err",
		Name:  "LeaderHero",
		JobID: "warrior",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 20, Defense: 10},
	}
	charRepo.chars[leader.ID] = leader

	detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "ログ失敗テスト",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	partyRepo.saveLogErr = errors.New("database connection broken")

	_, err = svc.StartPartyAdventure(ctx, detail.Party.ID, leader.ID)
	if err == nil {
		t.Fatal("expected StartPartyAdventure to fail when SaveAdventureLog returns error, got nil")
	}
}

func TestPartyService_NeedJoinCondition(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	leaderUnder := corecharacter.Character{
		ID:       "leader-under",
		Name:     "LeaderUnder",
		Level:    5,
		JobLevel: 1,
		Stats:    corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	leaderOver := corecharacter.Character{
		ID:       "leader-over",
		Name:     "LeaderOver",
		Level:    5,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	memberUnder := corecharacter.Character{
		ID:       "member-under",
		Name:     "MemberUnder",
		Level:    5,
		JobLevel: 1,
		Stats:    corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	memberOver := corecharacter.Character{
		ID:       "member-over",
		Name:     "MemberOver",
		Level:    5,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	charRepo.chars[leaderUnder.ID] = leaderUnder
	charRepo.chars[leaderOver.ID] = leaderOver
	charRepo.chars[memberUnder.ID] = memberUnder
	charRepo.chars[memberOver.ID] = memberOver

	ctx := context.Background()

	// Leader with JobLevel 1 tries to create party requiring JobLevel >= 3 -> fails
	_, err := svc.CreateParty(ctx, leaderUnder.ID, CreatePartyRequest{
		Speed:    18,
		Name:     "HighLevelOnly",
		StageID:  "forest",
		NeedJoin: "joblv_3_o",
	})
	if !errors.Is(err, ErrNeedJoinNotMet) {
		t.Fatalf("expected ErrNeedJoinNotMet when leader doesn't meet requirement, got %v", err)
	}

	// Leader with JobLevel 5 creates party requiring JobLevel >= 3 -> succeeds
	detail, err := svc.CreateParty(ctx, leaderOver.ID, CreatePartyRequest{
		Speed:    18,
		Name:     "HighLevelOnly",
		StageID:  "forest",
		NeedJoin: "joblv_3_o",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	if detail.Party.NeedJoin != "joblv_3_o" {
		t.Errorf("NeedJoin = %s, want joblv_3_o", detail.Party.NeedJoin)
	}

	// Member with JobLevel 1 tries to join -> fails
	_, err = svc.JoinParty(ctx, detail.Party.ID, memberUnder.ID, "")
	if !errors.Is(err, ErrNeedJoinNotMet) {
		t.Fatalf("expected ErrNeedJoinNotMet on join, got %v", err)
	}

	// Member with JobLevel 5 joins -> succeeds
	_, err = svc.JoinParty(ctx, detail.Party.ID, memberOver.ID, "")
	if err != nil {
		t.Fatalf("JoinParty failed for qualified member: %v", err)
	}
}

func TestPartyService_ExhaustedCharacter(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	exhaustedChar := corecharacter.Character{
		ID:    "tired-char",
		Name:  "Sleepy",
		Level: 5,
		Tired: 100,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	normalChar := corecharacter.Character{
		ID:    "normal-char",
		Name:  "Fresh",
		Level: 5,
		Tired: 0,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	charRepo.chars[exhaustedChar.ID] = exhaustedChar
	charRepo.chars[normalChar.ID] = normalChar

	ctx := context.Background()

	// Tired character cannot create party
	_, err := svc.CreateParty(ctx, exhaustedChar.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "TiredParty",
		StageID: "forest",
	})
	if !errors.Is(err, ErrCharacterExhausted) {
		t.Fatalf("expected ErrCharacterExhausted on CreateParty, got %v", err)
	}

	// Normal character creates party
	detail, err := svc.CreateParty(ctx, normalChar.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "FreshParty",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	// Tired character cannot join party
	_, err = svc.JoinParty(ctx, detail.Party.ID, exhaustedChar.ID, "")
	if !errors.Is(err, ErrCharacterExhausted) {
		t.Fatalf("expected ErrCharacterExhausted on JoinParty, got %v", err)
	}
}

func TestPartyService_SpeedSettings(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	c := corecharacter.Character{
		ID:    "speed-tester",
		Name:  "Speedy",
		Level: 5,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 50},
	}
	charRepo.chars[c.ID] = c

	ctx := context.Background()

	// Speed 3, 18, 25 allowed
	for _, speed := range []int{3, 18, 25} {
		det, err := svc.CreateParty(ctx, c.ID, CreatePartyRequest{
			Name:    fmt.Sprintf("Party-%d", speed),
			StageID: "forest",
			Speed:   speed,
		})
		if err != nil {
			t.Fatalf("CreateParty speed %d failed: %v", speed, err)
		}
		if det.Party.Speed != speed {
			t.Errorf("Speed = %d, want %d", det.Party.Speed, speed)
		}
		_ = svc.LeaveParty(ctx, det.Party.ID, c.ID)
	}

	// Unknown speeds are rejected (quest.cgi:551–552).
	for _, speed := range []int{0, 99} {
		if _, err := svc.CreateParty(ctx, c.ID, CreatePartyRequest{Name: "Party-InvalidSpeed", StageID: "forest", Speed: speed}); !errors.Is(err, ErrInvalidSpeed) {
			t.Fatalf("speed %d: %v", speed, err)
		}
	}
}

func TestPartyService_StartPartyAdventure_PostAdventureHook(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)
	ctx := context.Background()

	leader := corecharacter.Character{
		ID:         "leader",
		Name:       "Hero1",
		Level:      5,
		Experience: 250,
		Money:      100,
		Stats:      corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 25, Defense: 10},
	}
	m1 := corecharacter.Character{
		ID:         "m1",
		Name:       "Hero2",
		Level:      5,
		Experience: 250,
		Money:      50,
		Stats:      corecharacter.Stats{HP: 45, MaxHP: 45, Attack: 20, Defense: 8},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1

	var hookedCharIDs []string
	svc.SetPostAdventureHook(func(ctx context.Context, characterID string) (string, error) {
		hookedCharIDs = append(hookedCharIDs, characterID)
		return "", errors.New("tavern unavailable")
	})

	detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Speed:   18,
		Name:    "DeliveryParty",
		StageID: "forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := detail.Party.ID
	_, _ = svc.JoinParty(ctx, partyID, m1.ID, "")

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if len(res.Rewards) != 2 {
		t.Fatalf("expected 2 member rewards, got %d", len(res.Rewards))
	}

	if len(hookedCharIDs) != 2 {
		t.Fatalf("expected PostAdventureHook to be called 2 times, got %d", len(hookedCharIDs))
	}

	expectedIDs := map[string]bool{leader.ID: true, m1.ID: true}
	for _, id := range hookedCharIDs {
		if !expectedIDs[id] {
			t.Errorf("unexpected character ID hooked: %s", id)
		}
	}
}

type stubLifetimePartyBuilder struct {
	actors map[string]battle.LifetimeActor
	err    error
}

func (b *stubLifetimePartyBuilder) BuildParticipant(_ context.Context, characterID string) (battle.Participant, error) {
	if b.err != nil {
		return battle.Participant{}, b.err
	}
	return b.actors[characterID].Participant, nil
}

func (b *stubLifetimePartyBuilder) BuildLifetimeActor(_ context.Context, characterID, _ string) (battle.LifetimeActor, error) {
	if b.err != nil {
		return battle.LifetimeActor{}, b.err
	}
	return b.actors[characterID].Clone(), nil
}

func TestCreateAndJoinPartyCaptureBattleRowAndPublicJSON(t *testing.T) {
	ctx := context.Background()
	repo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()

	builder := &stubLifetimePartyBuilder{
		actors: map[string]battle.LifetimeActor{
			"leader": {
				Participant: battle.Participant{
					ID: "leader", Name: "Hero1", HP: 120, MaxHP: 220, MP: 20, MaxMP: 30,
					Attack: 50, Defense: 25, Agility: 15,
					Gear: &battle.BattleGearStats{WeaponID: "weapon-01"},
				},
				Tension: 1, Hit: 95,
			},
			"joiner": {
				Participant: battle.Participant{
					ID: "joiner", Name: "Hero2", HP: 90, MaxHP: 90, MP: 40, MaxMP: 140,
					Attack: 30, Defense: 40, Agility: 20,
					Gear: &battle.BattleGearStats{ArmorID: "armor-01"},
				},
				Tension: 1, Hit: 95,
			},
		},
	}

	svc, err := NewService(repo, charRepo, invRepo, stages, monsters, nil, WithParticipantBuilder(builder))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	charRepo.chars["leader"] = corecharacter.Character{
		ID: "leader", Name: "Hero1", Level: 5,
		Stats: corecharacter.Stats{HP: 120, MaxHP: 120, MP: 20, MaxMP: 30},
	}
	charRepo.chars["joiner"] = corecharacter.Character{
		ID: "joiner", Name: "Hero2", Level: 5,
		Stats: corecharacter.Stats{HP: 90, MaxHP: 90, MP: 40, MaxMP: 40},
	}

	created, err := svc.CreateParty(ctx, "leader", CreatePartyRequest{Speed: 18, Name: "BattleRowParty", StageID: adventure.StarterAdventure})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	if len(created.Members) != 1 || created.Members[0].MaxHP != 220 || created.Members[0].BattleActor == nil {
		t.Fatalf("leader battle row not captured on CreateParty: %+v", created.Members)
	}

	joined, err := svc.JoinParty(ctx, created.Party.ID, "joiner", "")
	if err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	if len(joined.Members) != 2 || joined.Members[1].BattleActor == nil || joined.Members[1].BattleActor.MaxMP != 140 {
		t.Fatalf("joiner battle row not captured on JoinParty: %+v", joined.Members)
	}

	// Public lobby response must not serialize internal battle row or unequipped persistent fields.
	raw, err := json.Marshal(joined)
	if err != nil {
		t.Fatalf("json.Marshal(PartyDetail) failed: %v", err)
	}
	for _, forbidden := range []string{"battle_actor", "weapon-01", "armor-01", "max_mp"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public PartyDetail JSON unexpectedly included %q: %s", forbidden, string(raw))
		}
	}

	// Builder error must abort CreateParty and JoinParty cleanly without leaving a partial lobby.
	builder.err = errors.New("builder unavailable")
	charRepo.chars["other"] = corecharacter.Character{ID: "other", Name: "Other", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	if _, err := svc.CreateParty(ctx, "other", CreatePartyRequest{Speed: 18, Name: "FailCreate", StageID: adventure.StarterAdventure}); err == nil {
		t.Fatal("expected CreateParty to fail when battle row capture fails")
	}
	if _, _, err := svc.GetActiveParty(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed CreateParty left active party for leader: %v", err)
	}
	if page, err := svc.ListParties(ctx, StatusRecruiting, 10, 0); err != nil || page.Total != 1 {
		t.Fatalf("failed CreateParty left partial lobby in listing: page=%+v err=%v", page, err)
	}
	if _, err := svc.JoinParty(ctx, created.Party.ID, "other", ""); err == nil {
		t.Fatal("expected JoinParty to fail when battle row capture fails")
	}
	if _, _, err := svc.GetActiveParty(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed JoinParty left active party for joiner: %v", err)
	}

	// Verify against ValkeyRepository (in-memory) that CreateParty failure leaves zero lobby or character keys.
	valkeyRepo := NewMemoryRepository()
	valkeySvc, err := NewService(valkeyRepo, charRepo, invRepo, stages, monsters, nil, WithParticipantBuilder(builder))
	if err != nil {
		t.Fatalf("NewService(ValkeyRepository) failed: %v", err)
	}
	if _, err := valkeySvc.CreateParty(ctx, "other", CreatePartyRequest{Speed: 18, Name: "FailValkeyCreate", StageID: adventure.StarterAdventure}); err == nil {
		t.Fatal("expected CreateParty to fail when battle row capture fails")
	}
	if page, err := valkeySvc.ListParties(ctx, StatusRecruiting, 10, 0); err != nil || page.Total != 0 {
		t.Fatalf("failed CreateParty left partial Valkey lobby: page=%+v err=%v", page, err)
	}
	if _, _, err := valkeySvc.GetActiveParty(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed CreateParty left character-party mapping: %v", err)
	}
}

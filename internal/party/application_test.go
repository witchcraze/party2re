package party

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type mockPartyRepository struct {
	parties     map[string]Party
	members     map[string]map[string]Member // partyID -> characterID -> Member
	memberParty map[string]string            // characterID -> partyID
	logs        []PartyAdventureLog
}

func newMockPartyRepository() *mockPartyRepository {
	return &mockPartyRepository{
		parties:     make(map[string]Party),
		members:     make(map[string]map[string]Member),
		memberParty: make(map[string]string),
	}
}

func (r *mockPartyRepository) SaveParty(_ context.Context, p Party) error {
	r.parties[p.ID] = p
	return nil
}

func (r *mockPartyRepository) GetParty(_ context.Context, id string) (Party, error) {
	p, ok := r.parties[id]
	if !ok || p.Status == StatusDisbanded {
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
			MinLevel:          p.MinLevel,
			MaxLevel:          p.MaxLevel,
			MinHP:             p.MinHP,
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
	r.parties[p.ID] = p
	return nil
}

func (r *mockPartyRepository) DeleteParty(_ context.Context, id string) error {
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
		list = append(list, m)
	}
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
	if !ok || p.Status == StatusDisbanded {
		return Party{}, Member{}, ErrNotFound
	}
	m := r.members[partyID][characterID]
	return p, m, nil
}

func (r *mockPartyRepository) RemoveMember(_ context.Context, partyID, characterID string) error {
	if mMap, ok := r.members[partyID]; ok {
		delete(mMap, characterID)
	}
	delete(r.memberParty, characterID)
	return nil
}

func (r *mockPartyRepository) UpdateMemberReady(_ context.Context, partyID, characterID string, ready bool) error {
	mMap, ok := r.members[partyID]
	if !ok {
		return ErrCharacterNotInParty
	}
	m, ok := mMap[characterID]
	if !ok {
		return ErrCharacterNotInParty
	}
	m.ReadyState = ready
	mMap[characterID] = m
	return nil
}

func (r *mockPartyRepository) CountMembers(_ context.Context, partyID string) (int, error) {
	return len(r.members[partyID]), nil
}

func (r *mockPartyRepository) SaveAdventureLog(_ context.Context, log PartyAdventureLog) error {
	r.logs = append(r.logs, log)
	return nil
}

type mockCharacterRepository struct {
	chars map[string]corecharacter.Character
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
	r.chars[value.ID] = value
	return nil
}

type mockInventoryRepository struct {
	inventories map[string]coreinventory.Inventory
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
	stages map[string]adventure.Stage
}

func (p *mockStageProvider) FindByID(id string) (adventure.Stage, error) {
	s, ok := p.stages[id]
	if !ok {
		return adventure.Stage{}, adventure.ErrStageNotFound
	}
	return s, nil
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

func setupTestService(t *testing.T) (*Service, *mockPartyRepository, *mockCharacterRepository, *mockNewsPublisher) {
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
				MinLevel:   1,
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

	svc, err := NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		battle.Engine{},
		WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatal(err)
	}

	return svc, partyRepo, charRepo, news
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
		MinLevel:   5,
		MaxLevel:   20,
		MinHP:      30,
	}
	detail, err := svc.CreateParty(context.Background(), leader.ID, req)
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	if detail.Party.Name != "勇者パーティ" || detail.Party.LeaderCharacterID != leader.ID {
		t.Fatalf("unexpected party details: %+v", detail.Party)
	}
	if len(detail.Members) != 1 || !detail.Members[0].IsLeader || !detail.Members[0].ReadyState {
		t.Fatalf("expected leader member ready, got: %+v", detail.Members)
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

	// 4. Invalid party name
	if _, err := svc.CreateParty(context.Background(), "char-other", CreatePartyRequest{Name: ""}); !errors.Is(err, ErrInvalidPartyName) {
		t.Fatalf("expected ErrInvalidPartyName, got %v", err)
	}

	// 5. Stage not found
	if _, err := svc.CreateParty(context.Background(), "char-other", CreatePartyRequest{Name: "Party", StageID: "invalid-stage"}); !errors.Is(err, ErrStageNotFound) {
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
		Name:       "AdventureSquad",
		StageID:    "forest",
		Password:   "pass123",
		MaxMembers: 2,
		MinLevel:   5,
		MaxLevel:   15,
		MinHP:      30,
	})
	if err != nil {
		t.Fatal(err)
	}
	partyID := detail.Party.ID

	// 1. Password mismatch
	if _, err := svc.JoinParty(context.Background(), partyID, m1.ID, "wrongpass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}

	// 2. Level requirement not met
	if _, err := svc.JoinParty(context.Background(), partyID, mLowLevel.ID, "pass123"); !errors.Is(err, ErrLevelRequirementNotMet) {
		t.Fatalf("expected ErrLevelRequirementNotMet, got %v", err)
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

	// 5. Ready state toggle
	readyDetail, err := svc.SetReady(context.Background(), partyID, m1.ID, true)
	if err != nil {
		t.Fatalf("SetReady failed: %v", err)
	}
	for _, m := range readyDetail.Members {
		if m.CharacterID == m1.ID && !m.ReadyState {
			t.Fatalf("expected m1 to be ready")
		}
	}

	// 6. Member leaves party
	if err := svc.LeaveParty(context.Background(), partyID, m1.ID); err != nil {
		t.Fatalf("LeaveParty failed: %v", err)
	}
	detailAfterLeave, _ := svc.GetParty(context.Background(), partyID)
	if len(detailAfterLeave.Members) != 1 {
		t.Fatalf("expected 1 member after leave, got %d", len(detailAfterLeave.Members))
	}
}

func TestPartyService_KickAndDisband(t *testing.T) {
	svc, _, charRepo, _ := setupTestService(t)

	leader := corecharacter.Character{ID: "leader", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	m1 := corecharacter.Character{ID: "m1", Name: "M1", Level: 10, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[m1.ID] = m1

	detail, _ := svc.CreateParty(context.Background(), leader.ID, CreatePartyRequest{
		Name:    "PartyX",
		StageID: "forest",
	})
	partyID := detail.Party.ID
	_, _ = svc.JoinParty(context.Background(), partyID, m1.ID, "")

	// 1. Non-leader cannot kick
	if err := svc.KickMember(context.Background(), partyID, m1.ID, leader.ID); !errors.Is(err, ErrNotPartyLeader) {
		t.Fatalf("expected ErrNotPartyLeader, got %v", err)
	}

	// 2. Leader cannot kick self
	if err := svc.KickMember(context.Background(), partyID, leader.ID, leader.ID); !errors.Is(err, ErrCannotKickSelf) {
		t.Fatalf("expected ErrCannotKickSelf, got %v", err)
	}

	// 3. Leader kicks member
	if err := svc.KickMember(context.Background(), partyID, leader.ID, m1.ID); err != nil {
		t.Fatalf("KickMember failed: %v", err)
	}

	// 4. Disband party
	if err := svc.DisbandParty(context.Background(), partyID, leader.ID); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}
	if _, err := svc.GetParty(context.Background(), partyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after disband, got %v", err)
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
		Name:    "CoopParty",
		StageID: "forest",
	})
	partyID := detail.Party.ID
	_, _ = svc.JoinParty(context.Background(), partyID, m1.ID, "")

	// 1. Cannot start if members not ready
	if _, err := svc.StartPartyAdventure(context.Background(), partyID, leader.ID); !errors.Is(err, ErrPartyNotReady) {
		t.Fatalf("expected ErrPartyNotReady, got %v", err)
	}

	// 2. Set ready and start
	_, _ = svc.SetReady(context.Background(), partyID, m1.ID, true)

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
		Name:    "Party One",
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
	if page.Items[0].Name != "Party One" {
		t.Errorf("expected party name 'Party One', got %s", page.Items[0].Name)
	}
}

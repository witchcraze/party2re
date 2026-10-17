package dungeon_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/party"
)

type mockInventoryRepo struct {
	invs map[string]coreinventory.Inventory
}

func (m *mockInventoryRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if inv, ok := m.invs[characterID]; ok {
		return inv, nil
	}
	inv, _ := coreinventory.New(characterID)
	return inv, nil
}

type mockPartyRepo struct {
	parties map[string]party.Party
	members map[string][]party.Member
}

func newMockPartyRepo() *mockPartyRepo {
	return &mockPartyRepo{
		parties: make(map[string]party.Party),
		members: make(map[string][]party.Member),
	}
}

func (m *mockPartyRepo) GetParty(ctx context.Context, id string) (party.Party, error) {
	p, ok := m.parties[id]
	if !ok {
		return party.Party{}, party.ErrNotFound
	}
	return p, nil
}

func (m *mockPartyRepo) GetMembers(ctx context.Context, partyID string) ([]party.Member, error) {
	mems, ok := m.members[partyID]
	if !ok {
		return nil, party.ErrNotFound
	}
	return mems, nil
}

func (m *mockPartyRepo) UpdateParty(ctx context.Context, p party.Party) error {
	m.parties[p.ID] = p
	return nil
}

func (m *mockPartyRepo) UpdateMemberReady(ctx context.Context, partyID string, characterID string, ready bool) error {
	mems, ok := m.members[partyID]
	if !ok {
		return party.ErrNotFound
	}
	for i := range mems {
		if mems[i].CharacterID == characterID {
			mems[i].ReadyState = ready
			return nil
		}
	}
	return nil
}

func (m *mockPartyRepo) addParty(p party.Party, members []party.Member) {
	m.parties[p.ID] = p
	m.members[p.ID] = members
}

func setupPartyTestService() (*dungeon.Service, *mockDungeonRepo, *mockCharRepo, *mockPartyRepo) {
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	partyRepo := newMockPartyRepo()
	engine := &corebattle.Engine{}

	c1 := createTestChar("char-1", 10, 150, 40, 30)
	c1.Name = "リーダー勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createTestChar("char-2", 10, 120, 35, 25)
	c2.Name = "盗賊シーフ"
	c2.JobID = "9" // Thief
	charRepo.chars[c2.ID] = c2

	c3 := createTestChar("char-3", 10, 110, 30, 20)
	c3.Name = "魔法使いメイジ"
	c3.JobID = "mage"
	charRepo.chars[c3.ID] = c3

	c4 := createTestChar("char-4", 10, 130, 45, 25)
	c4.Name = "トレハン冒険者"
	c4.JobID = "78" // Treasure Hunter
	charRepo.chars[c4.ID] = c4

	partyRepo.addParty(party.Party{
		ID:                "party-01",
		LeaderCharacterID: "char-1",
		Name:              "勇者パーティ",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-01", CharacterID: "char-1", CharacterName: "リーダー勇者", ReadyState: true, IsLeader: true},
		{PartyID: "party-01", CharacterID: "char-2", CharacterName: "盗賊シーフ", ReadyState: true},
		{PartyID: "party-01", CharacterID: "char-3", CharacterName: "魔法使いメイジ", ReadyState: true},
		{PartyID: "party-01", CharacterID: "char-4", CharacterName: "トレハン冒険者", ReadyState: true},
	})

	svc, err := dungeon.NewService(repo, charRepo, engine, dungeon.WithPartyRepository(partyRepo))
	if err != nil {
		panic(err)
	}

	return svc, repo, charRepo, partyRepo
}

func TestStartPartyExpedition(t *testing.T) {
	svc, _, charRepo, partyRepo := setupPartyTestService()
	ctx := context.Background()

	// 1. Success deriving roster from authorized party
	exp, err := svc.StartPartyExpedition(ctx, "char-1", "party-01", "dungeon-01")
	if err != nil {
		t.Fatalf("unexpected error starting party expedition: %v", err)
	}

	if exp == nil {
		t.Fatal("expected non-nil expedition")
	}
	if len(exp.Members) != 4 {
		t.Fatalf("expected 4 members, got %d", len(exp.Members))
	}
	if exp.CharacterID != "char-1" {
		t.Errorf("expected leader char-1, got %s", exp.CharacterID)
	}
	if exp.PartyID != "party-01" {
		t.Errorf("expected party ID party-01, got %s", exp.PartyID)
	}
	if exp.Members[0].Name != "リーダー勇者" {
		t.Errorf("expected member 0 name リーダー勇者, got %s", exp.Members[0].Name)
	}
	if exp.Members[1].JobID != "9" {
		t.Errorf("expected member 1 job 9, got %s", exp.Members[1].JobID)
	}

	// Verify party status transitioned to in_progress
	pUpdated, _ := partyRepo.GetParty(ctx, "party-01")
	if pUpdated.Status != party.StatusInProgress {
		t.Errorf("expected party status in_progress, got %s", pUpdated.Status)
	}

	// 2. Multi-member active indexing and privacy
	// Non-leader member can fetch the active expedition
	memExp, err := svc.GetActiveExpedition(ctx, "char-2")
	if err != nil || memExp == nil {
		t.Fatalf("expected member char-2 to find active expedition, err: %v", err)
	}
	if memExp.ID != exp.ID {
		t.Errorf("expected member expedition ID %s, got %s", exp.ID, memExp.ID)
	}

	// Outside character querying gets nil (privacy check)
	outsiderExp, err := svc.GetActiveExpedition(ctx, "outsider-char")
	if err != nil {
		t.Fatalf("unexpected error for outsider: %v", err)
	}
	if outsiderExp != nil {
		t.Errorf("expected nil expedition for outsider, got %v", outsiderExp)
	}

	// 3. Busy member rejection: starting again while active returns ErrActiveExpeditionExists
	partyRepo.addParty(party.Party{
		ID:                "party-busy",
		LeaderCharacterID: "char-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-busy", CharacterID: "char-1", ReadyState: true, IsLeader: true},
	})
	_, err = svc.StartPartyExpedition(ctx, "char-1", "party-busy", "dungeon-01")
	if !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Fatalf("expected ErrActiveExpeditionExists, got: %v", err)
	}

	// 4. Non-existent party
	_, err = svc.StartPartyExpedition(ctx, "char-1", "party-missing", "dungeon-01")
	if !errors.Is(err, dungeon.ErrPartyNotFound) {
		t.Fatalf("expected ErrPartyNotFound, got: %v", err)
	}

	// 5. Non-leader actor
	partyRepo.addParty(party.Party{
		ID:                "party-leader-test",
		LeaderCharacterID: "char-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-leader-test", CharacterID: "char-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-leader-test", CharacterID: "char-2", ReadyState: true},
	})
	_, err = svc.StartPartyExpedition(ctx, "char-2", "party-leader-test", "dungeon-01")
	if !errors.Is(err, dungeon.ErrNotPartyLeader) {
		t.Fatalf("expected ErrNotPartyLeader, got: %v", err)
	}

	// 6. Party not recruiting
	partyRepo.addParty(party.Party{
		ID:                "party-not-recruiting",
		LeaderCharacterID: "char-1",
		Status:            party.StatusInProgress,
	}, []party.Member{
		{PartyID: "party-not-recruiting", CharacterID: "char-1", ReadyState: true, IsLeader: true},
	})
	_, err = svc.StartPartyExpedition(ctx, "char-1", "party-not-recruiting", "dungeon-01")
	if !errors.Is(err, dungeon.ErrPartyNotRecruiting) {
		t.Fatalf("expected ErrPartyNotRecruiting, got: %v", err)
	}

	// 7. Member not ready
	partyRepo.addParty(party.Party{
		ID:                "party-unready",
		LeaderCharacterID: "char-notready-lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-unready", CharacterID: "char-notready-lead", ReadyState: true, IsLeader: true},
		{PartyID: "party-unready", CharacterID: "char-unready-2", ReadyState: false},
	})
	charRepo.chars["char-notready-lead"] = createTestChar("char-notready-lead", 10, 100, 30, 20)
	charRepo.chars["char-unready-2"] = createTestChar("char-unready-2", 10, 100, 30, 20)
	_, err = svc.StartPartyExpedition(ctx, "char-notready-lead", "party-unready", "dungeon-01")
	if !errors.Is(err, dungeon.ErrPartyNotReady) {
		t.Fatalf("expected ErrPartyNotReady, got: %v", err)
	}

	// 8. Duplicate party members
	partyRepo.addParty(party.Party{
		ID:                "party-dup",
		LeaderCharacterID: "char-dup-lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-dup", CharacterID: "char-dup-lead", ReadyState: true, IsLeader: true},
		{PartyID: "party-dup", CharacterID: "char-dup-lead", ReadyState: true},
	})
	charRepo.chars["char-dup-lead"] = createTestChar("char-dup-lead", 10, 100, 30, 20)
	_, err = svc.StartPartyExpedition(ctx, "char-dup-lead", "party-dup", "dungeon-01")
	if !errors.Is(err, dungeon.ErrDuplicatePartyMember) {
		t.Fatalf("expected ErrDuplicatePartyMember, got: %v", err)
	}

	// 9. Party exceeding 4 members
	partyRepo.addParty(party.Party{
		ID:                "party-5mem",
		LeaderCharacterID: "p5-lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-5mem", CharacterID: "p5-lead", ReadyState: true, IsLeader: true},
		{PartyID: "party-5mem", CharacterID: "p5-2", ReadyState: true},
		{PartyID: "party-5mem", CharacterID: "p5-3", ReadyState: true},
		{PartyID: "party-5mem", CharacterID: "p5-4", ReadyState: true},
		{PartyID: "party-5mem", CharacterID: "p5-5", ReadyState: true},
	})
	charRepo.chars["p5-lead"] = createTestChar("p5-lead", 10, 100, 30, 20)
	charRepo.chars["p5-2"] = createTestChar("p5-2", 10, 100, 30, 20)
	charRepo.chars["p5-3"] = createTestChar("p5-3", 10, 100, 30, 20)
	charRepo.chars["p5-4"] = createTestChar("p5-4", 10, 100, 30, 20)
	charRepo.chars["p5-5"] = createTestChar("p5-5", 10, 100, 30, 20)
	_, err = svc.StartPartyExpedition(ctx, "p5-lead", "party-5mem", "dungeon-01")
	if !errors.Is(err, dungeon.ErrTooManyPartyMembers) {
		t.Fatalf("expected ErrTooManyPartyMembers, got: %v", err)
	}

	// 10. Unconscious character
	partyRepo.addParty(party.Party{
		ID:                "party-dead",
		LeaderCharacterID: "p-dead-lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-dead", CharacterID: "p-dead-lead", ReadyState: true, IsLeader: true},
	})
	deadChar := createTestChar("p-dead-lead", 10, 100, 30, 20)
	deadChar.Stats.HP = 0
	charRepo.chars["p-dead-lead"] = deadChar
	_, err = svc.StartPartyExpedition(ctx, "p-dead-lead", "party-dead", "dungeon-01")
	if !errors.Is(err, dungeon.ErrCharacterUnconscious) {
		t.Fatalf("expected ErrCharacterUnconscious, got: %v", err)
	}
}

func TestPartyMapScouting(t *testing.T) {
	svc, _, charRepo, partyRepo := setupPartyTestService()
	ctx := context.Background()

	// 1. Solo warrior: base view radius = 1 (3x3 grid)
	_, err := svc.StartExpedition(ctx, "char-1", "dungeon-01")
	if err != nil {
		t.Fatalf("failed to start solo expedition: %v", err)
	}

	mapView1, err := svc.ViewMap(ctx, "char-1")
	if err != nil {
		t.Fatalf("ViewMap failed: %v", err)
	}
	if mapView1.Radius != 1 {
		t.Errorf("expected solo warrior map radius 1, got %d", mapView1.Radius)
	}
	if mapView1.ScoutingBonusActive {
		t.Errorf("expected no scouting bonus for solo warrior")
	}
	// Check formatted string contains "●"
	if !strings.Contains(mapView1.Formatted, "●") {
		t.Errorf("expected formatted map to contain ●, got:\n%s", mapView1.Formatted)
	}
	// Size of tiles should be (2*1+1) = 3
	if len(mapView1.Tiles) != 3 || len(mapView1.Tiles[0]) != 3 {
		t.Errorf("expected 3x3 tiles for radius 1, got %dx%d", len(mapView1.Tiles), len(mapView1.Tiles[0]))
	}

	// 2. Party with Thief (job 9): expanded view radius = 2 (5x5 grid)
	_, _ = svc.Escape(ctx, "char-1") // end solo expedition
	partyRepo.addParty(party.Party{
		ID:                "party-thief-pair",
		LeaderCharacterID: "char-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-thief-pair", CharacterID: "char-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-thief-pair", CharacterID: "char-2", ReadyState: true},
	})
	_, err = svc.StartPartyExpedition(ctx, "char-1", "party-thief-pair", "dungeon-01")
	if err != nil {
		t.Fatalf("failed to start party expedition: %v", err)
	}

	mapView2, err := svc.ViewMap(ctx, "char-1")
	if err != nil {
		t.Fatalf("ViewMap failed: %v", err)
	}
	if mapView2.Radius != 2 {
		t.Errorf("expected party with thief map radius 2, got %d", mapView2.Radius)
	}
	if !mapView2.ScoutingBonusActive {
		t.Errorf("expected scouting bonus active for thief")
	}
	// Size of tiles should be (2*2+1) = 5
	if len(mapView2.Tiles) != 5 || len(mapView2.Tiles[0]) != 5 {
		t.Errorf("expected 5x5 tiles for radius 2, got %dx%d", len(mapView2.Tiles), len(mapView2.Tiles[0]))
	}

	// 3. Ninja (26), Geomancer (27), Ranger (79)
	jobs := []string{"26", "27", "79", "ninja", "geomancer", "ranger"}
	for _, jobID := range jobs {
		cTest := createTestChar("scout-"+jobID, 10, 100, 30, 20)
		cTest.JobID = jobID
		charRepo.chars[cTest.ID] = cTest
		testSvcCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
			"lead":   c1Lead(10),
			cTest.ID: cTest,
		}}
		tPartyRepo := newMockPartyRepo()
		tPartyRepo.addParty(party.Party{
			ID:                "p-" + jobID,
			LeaderCharacterID: "lead",
			Status:            party.StatusRecruiting,
		}, []party.Member{
			{PartyID: "p-" + jobID, CharacterID: "lead", ReadyState: true, IsLeader: true},
			{PartyID: "p-" + jobID, CharacterID: cTest.ID, ReadyState: true},
		})
		tSvc, _ := dungeon.NewService(newMockDungeonRepo(), testSvcCharRepo, &corebattle.Engine{}, dungeon.WithPartyRepository(tPartyRepo))
		_, err := tSvc.StartPartyExpedition(ctx, "lead", "p-"+jobID, "dungeon-01")
		if err != nil {
			t.Fatalf("failed to start for job %s: %v", jobID, err)
		}
		mv, err := tSvc.ViewMap(ctx, "lead")
		if err != nil {
			t.Fatalf("ViewMap failed for job %s: %v", jobID, err)
		}
		if mv.Radius != 2 {
			t.Errorf("expected radius 2 for scouting job %s, got %d", jobID, mv.Radius)
		}
	}

	// 4. Item 197 / scope_goggles expands radius to 2 even without scouting job
	invRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory)}
	cItem := createTestChar("scope-hero", 10, 100, 30, 20)
	cItem.JobID = "warrior" // not a scouting job
	itemCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cItem.ID: cItem}}
	itemSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		itemCharRepo,
		&corebattle.Engine{},
		dungeon.WithInventoryProvider(invRepo),
	)
	// Give item 197 to scope-hero
	inv, _ := coreinventory.New(cItem.ID)
	_ = inv.Add(coreitem.Instance{ID: "scope-inst", DefinitionID: "197", Quantity: 1})
	invRepo.invs[cItem.ID] = inv

	_, err = itemSvc.StartExpedition(ctx, cItem.ID, "dungeon-01")
	if err != nil {
		t.Fatalf("failed to start item expedition: %v", err)
	}
	mvItem, err := itemSvc.ViewMap(ctx, cItem.ID)
	if err != nil {
		t.Fatalf("ViewMap failed: %v", err)
	}
	if mvItem.Radius != 2 {
		t.Errorf("expected radius 2 for item 197 holder, got %d", mvItem.Radius)
	}
	if len(mvItem.Tiles) != 5 || len(mvItem.Tiles[0]) != 5 {
		t.Errorf("expected 5x5 tiles for radius 2, got %dx%d", len(mvItem.Tiles), len(mvItem.Tiles[0]))
	}

	// 5. Stacking: Scouting Job (Thief 9) + Scope Goggles (item 197) expands radius to 3 (7x7 grid)
	cBoth := createTestChar("scout-scope-hero", 10, 100, 30, 20)
	cBoth.JobID = "9" // Thief
	stackCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cBoth.ID: cBoth}}
	stackInvRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory)}
	stackInv, _ := coreinventory.New(cBoth.ID)
	_ = stackInv.Add(coreitem.Instance{ID: "scope-inst-2", DefinitionID: "197", Quantity: 1})
	stackInvRepo.invs[cBoth.ID] = stackInv

	stackSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		stackCharRepo,
		&corebattle.Engine{},
		dungeon.WithInventoryProvider(stackInvRepo),
	)
	_, err = stackSvc.StartExpedition(ctx, cBoth.ID, "dungeon-01")
	if err != nil {
		t.Fatalf("failed to start stack expedition: %v", err)
	}
	mvBoth, err := stackSvc.ViewMap(ctx, cBoth.ID)
	if err != nil {
		t.Fatalf("ViewMap failed: %v", err)
	}
	if mvBoth.Radius != 3 {
		t.Errorf("expected radius 3 for scouting job + scope goggles, got %d", mvBoth.Radius)
	}
	if !mvBoth.ScoutingBonusActive {
		t.Errorf("expected scouting bonus active for stacked scouting")
	}
	if len(mvBoth.Tiles) != 7 || len(mvBoth.Tiles[0]) != 7 {
		t.Errorf("expected 7x7 tiles for radius 3, got %dx%d", len(mvBoth.Tiles), len(mvBoth.Tiles[0]))
	}

	// 6. Stacking in Multi-member Party: Member 1 is Thief, Member 2 holds Scope Goggles
	cScout := createTestChar("party-thief", 10, 100, 30, 20)
	cScout.JobID = "9"
	cGoggleUser := createTestChar("party-goggle-user", 10, 100, 30, 20)
	cGoggleUser.JobID = "warrior"
	multiCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		cScout.ID:      cScout,
		cGoggleUser.ID: cGoggleUser,
	}}
	multiInvRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory)}
	goggleInv, _ := coreinventory.New(cGoggleUser.ID)
	_ = goggleInv.Add(coreitem.Instance{ID: "goggle-inst", DefinitionID: "197", Quantity: 1})
	multiInvRepo.invs[cGoggleUser.ID] = goggleInv

	multiPartyRepo := newMockPartyRepo()
	multiPartyRepo.addParty(party.Party{
		ID:                "p-multi-stack",
		LeaderCharacterID: cScout.ID,
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-multi-stack", CharacterID: cScout.ID, ReadyState: true, IsLeader: true},
		{PartyID: "p-multi-stack", CharacterID: cGoggleUser.ID, ReadyState: true},
	})

	multiSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		multiCharRepo,
		&corebattle.Engine{},
		dungeon.WithInventoryProvider(multiInvRepo),
		dungeon.WithPartyRepository(multiPartyRepo),
	)
	_, err = multiSvc.StartPartyExpedition(ctx, cScout.ID, "p-multi-stack", "dungeon-01")
	if err != nil {
		t.Fatalf("failed to start multi-party expedition: %v", err)
	}
	mvMulti, err := multiSvc.ViewMap(ctx, cScout.ID)
	if err != nil {
		t.Fatalf("ViewMap failed: %v", err)
	}
	if mvMulti.Radius != 3 {
		t.Errorf("expected radius 3 for multi-party stacking, got %d", mvMulti.Radius)
	}
	if !mvMulti.ScoutingBonusActive {
		t.Errorf("expected scouting bonus active for multi-party stacking")
	}
	if len(mvMulti.Tiles) != 7 || len(mvMulti.Tiles[0]) != 7 {
		t.Errorf("expected 7x7 tiles for multi-party stacking radius 3, got %dx%d", len(mvMulti.Tiles), len(mvMulti.Tiles[0]))
	}
	if !strings.Contains(mvMulti.BonusReason, "Scouting Job") || !strings.Contains(mvMulti.BonusReason, "Scope Goggles") {
		t.Errorf("expected BonusReason to mention both bonuses, got: %s", mvMulti.BonusReason)
	}
}

func TestPartyTrapDamageDistribution(t *testing.T) {
	ctx := context.Background()

	// Create custom dungeon where trap 'X' is immediately to the south
	customCatalog := []dungeon.Dungeon{
		{
			ID:               "trap-dungeon",
			Tier:             1,
			Name:             "罠の洞窟",
			MinLevel:         5,
			MaxTurnsPerFloor: 20,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      3,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S00",
						"X00",
						"00D",
					},
				},
			},
		},
	}
	tPartyRepo := newMockPartyRepo()
	tPartyRepo.addParty(party.Party{
		ID:                "p-trap",
		LeaderCharacterID: "p1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-trap", CharacterID: "p1", ReadyState: true, IsLeader: true},
		{PartyID: "p-trap", CharacterID: "p2", ReadyState: true},
		{PartyID: "p-trap", CharacterID: "p3", ReadyState: true},
	})
	tSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"p1": createTestChar("p1", 10, 150, 40, 30),
			"p2": createTestChar("p2", 10, 100, 30, 20),
			"p3": createTestChar("p3", 10, 80, 25, 15),
		}},
		&corebattle.Engine{},
		dungeon.WithCustomDungeons(customCatalog),
		dungeon.WithPartyRepository(tPartyRepo),
	)

	_, err := tSvc.StartPartyExpedition(ctx, "p1", "p-trap", "trap-dungeon")
	if err != nil {
		t.Fatalf("failed to start expedition: %v", err)
	}

	// Move south onto 'X' trap
	res, err := tSvc.Move(ctx, "p1", dungeon.DirectionSouth)
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}

	if res.EventType != dungeon.EventTrap {
		t.Fatalf("expected EventTrap, got %s", res.EventType)
	}

	// Check that all 3 members received damage
	if len(res.MemberDamages) != 3 {
		t.Fatalf("expected 3 member damages, got %d (%v)", len(res.MemberDamages), res.MemberDamages)
	}
	for _, mID := range []string{"p1", "p2", "p3"} {
		dmg := res.MemberDamages[mID]
		if dmg <= 0 {
			t.Errorf("expected positive damage for member %s, got %d", mID, dmg)
		}
	}

	// Check member current HPs in active expedition
	active, err := tSvc.GetActiveExpedition(ctx, "p1")
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	for _, m := range active.Members {
		if m.CurrentHP >= m.MaxHP {
			t.Errorf("expected reduced HP for %s, got %d/%d", m.CharacterID, m.CurrentHP, m.MaxHP)
		}
	}
}

func TestTreasureHunterBonusChests(t *testing.T) {
	ctx := context.Background()

	// Catalog with chest 'T' to the east
	customCatalog := []dungeon.Dungeon{
		{
			ID:               "chest-dungeon",
			Tier:             1,
			Name:             "宝の洞窟",
			MinLevel:         5,
			MaxTurnsPerFloor: 20,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      3,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"ST0",
						"000",
						"00D",
					},
				},
			},
		},
	}

	// 1. Normal party without treasure hunter
	normalSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"p1": createTestChar("p1", 10, 150, 40, 30),
		}},
		&corebattle.Engine{},
		dungeon.WithCustomDungeons(customCatalog),
	)
	_, _ = normalSvc.StartExpedition(ctx, "p1", "chest-dungeon")
	resNormal, err := normalSvc.Move(ctx, "p1", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	if resNormal.EventType != dungeon.EventTreasure {
		t.Fatalf("expected EventTreasure, got %s", resNormal.EventType)
	}
	if resNormal.ChestsOpened != 1 {
		t.Errorf("expected 1 chest opened without treasure hunter, got %d", resNormal.ChestsOpened)
	}

	// 2. Party with Treasure Hunter (job 78)
	thChar := createTestChar("th-char", 10, 120, 30, 20)
	thChar.JobID = "78" // Treasure Hunter
	thPartyRepo := newMockPartyRepo()
	thPartyRepo.addParty(party.Party{
		ID:                "p-th",
		LeaderCharacterID: "p1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-th", CharacterID: "p1", ReadyState: true, IsLeader: true},
		{PartyID: "p-th", CharacterID: "th", ReadyState: true},
	})
	thSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"p1": createTestChar("p1", 10, 150, 40, 30),
			"th": thChar,
		}},
		&corebattle.Engine{},
		dungeon.WithCustomDungeons(customCatalog),
		dungeon.WithPartyRepository(thPartyRepo),
	)
	_, _ = thSvc.StartPartyExpedition(ctx, "p1", "p-th", "chest-dungeon")
	resTH, err := thSvc.Move(ctx, "p1", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	if resTH.EventType != dungeon.EventTreasure {
		t.Fatalf("expected EventTreasure, got %s", resTH.EventType)
	}
	// With Job 78, chestsOpened should be 1 + (1 or 2) = 2 or 3!
	if resTH.ChestsOpened < 2 || resTH.ChestsOpened > 3 {
		t.Errorf("expected 2 or 3 chests opened with treasure hunter, got %d", resTH.ChestsOpened)
	}
	if resTH.GoldFound < 200 {
		t.Errorf("expected at least 200 gold for bonus chests, got %d", resTH.GoldFound)
	}
}

func c1Lead(lv int) corecharacter.Character {
	c := createTestChar("lead", lv, 150, 40, 30)
	c.JobID = "hero"
	return c
}

func TestPartyExpedition_NonLeaderForbidden(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "party-auth",
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-auth", CharacterID: "leader-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-auth", CharacterID: "member-2", ReadyState: true},
	})

	svc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createTestChar("leader-1", 10, 100, 20, 20),
			"member-2": createTestChar("member-2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		dungeon.WithPartyRepository(partyRepo),
	)

	_, err := svc.StartPartyExpedition(ctx, "leader-1", "party-auth", "dungeon-01")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// Non-leader observation is permitted
	exp, err := svc.GetActiveExpedition(ctx, "member-2")
	if err != nil || exp == nil {
		t.Fatalf("GetActiveExpedition for member failed: %v", err)
	}
	mv, err := svc.ViewMap(ctx, "member-2")
	if err != nil || len(mv.Tiles) == 0 {
		t.Fatalf("ViewMap for member failed: %v", err)
	}

	// Non-leader mutation is forbidden
	_, err = svc.Move(ctx, "member-2", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-leader Move, got %v", err)
	}

	_, err = svc.Escape(ctx, "member-2")
	if !errors.Is(err, dungeon.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-leader Escape, got %v", err)
	}

	// Leader can Move and Escape
	_, err = svc.Move(ctx, "leader-1", dungeon.DirectionEast)
	if err != nil {
		t.Errorf("expected leader Move to succeed, got %v", err)
	}

	_, err = svc.Escape(ctx, "leader-1")
	if err != nil {
		t.Errorf("expected leader Escape to succeed, got %v", err)
	}

	// Party state and member ready states should be reset after escape
	p, err := partyRepo.GetParty(ctx, "party-auth")
	if err != nil || p.Status != party.StatusRecruiting {
		t.Errorf("expected party status recruiting after escape, got %s (err: %v)", p.Status, err)
	}
	members, err := partyRepo.GetMembers(ctx, "party-auth")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range members {
		if m.ReadyState {
			t.Errorf("expected member %s ready state to be false after escape", m.CharacterID)
		}
	}
}

func TestPartyExpedition_RollbackOnStartFailure(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "party-rollback",
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-rollback", CharacterID: "leader-1", ReadyState: true, IsLeader: true},
	})

	svc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createTestChar("leader-1", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		dungeon.WithPartyRepository(partyRepo),
	)

	// Attempt start with nonexistent dungeon -> startExpeditionRoster fails
	_, err := svc.StartPartyExpedition(ctx, "leader-1", "party-rollback", "nonexistent-dungeon")
	if err == nil {
		t.Fatal("expected start to fail for nonexistent dungeon")
	}

	// Party status should be rolled back to recruiting
	p, err := partyRepo.GetParty(ctx, "party-rollback")
	if err != nil || p.Status != party.StatusRecruiting {
		t.Errorf("expected party status to remain recruiting on start failure, got %s", p.Status)
	}
}

func TestPartyExpedition_ActivityChecker(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "party-act",
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-act", CharacterID: "leader-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-act", CharacterID: "member-2", ReadyState: true},
	})

	checker := dungeon.ActivityCheckerFunc(func(ctx context.Context, charID string) (bool, error) {
		if charID == "member-2" {
			return true, nil
		}
		return false, nil
	})

	svc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createTestChar("leader-1", 10, 100, 20, 20),
			"member-2": createTestChar("member-2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		dungeon.WithPartyRepository(partyRepo),
		dungeon.WithActivityChecker(checker),
	)

	// Party start fails because member-2 is active in another domain
	_, err := svc.StartPartyExpedition(ctx, "leader-1", "party-act", "dungeon-01")
	if !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Errorf("expected ErrActiveExpeditionExists when member has active activity, got %v", err)
	}

	// Solo start for member-2 fails as well
	_, err = svc.StartExpedition(ctx, "member-2", "dungeon-01")
	if !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Errorf("expected ErrActiveExpeditionExists when solo character has active activity, got %v", err)
	}
}

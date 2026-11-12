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

func setupPartyTestService() (*dungeon.Service, *mockDungeonRepo, *mockCharRepo) {
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
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

	svc, err := dungeon.NewService(repo, charRepo, engine, dungeon.WithAdmission(approvedRoster{}), dungeon.WithInventoryProvider(&mockInventoryRepo{}))
	if err != nil {
		panic(err)
	}

	return svc, repo, charRepo
}

func TestStartPartyExpedition(t *testing.T) {
	svc, _, _ := setupPartyTestService()
	ctx := context.Background()

	// 1. Success with up to 4 members
	memberIDs := []string{"char-1", "char-2", "char-3", "char-4"}
	exp, err := svc.StartPartyExpedition(ctx, "char-1", memberIDs, dungeon.NativeMap0ID, "party-01")
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

	// 2. Reject if more than 4 members
	c5 := createTestChar("char-5", 10, 100, 30, 20)
	svc5, _, charRepo := setupPartyTestService()
	charRepo.chars[c5.ID] = c5
	_, err = svc5.StartPartyExpedition(ctx, "char-1", []string{"char-1", "char-2", "char-3", "char-4", "char-5"}, dungeon.NativeMap0ID, "")
	if err == nil {
		t.Fatal("expected error for party exceeding 4 members")
	}

	// 3. Reject if non-existent member
	_, err = svc5.StartPartyExpedition(ctx, "char-1", []string{"char-1", "non-existent"}, dungeon.NativeMap0ID, "")
	if err == nil {
		t.Fatal("expected error for non-existent member")
	}
}

func TestPartyMapScouting(t *testing.T) {
	svc, _, charRepo := setupPartyTestService()
	ctx := context.Background()

	// 1. Solo warrior: base view radius = 1 (3x3 grid)
	_, err := svc.StartPartyExpedition(ctx, "char-1", []string{"char-1"}, dungeon.NativeMap0ID, "")
	if err != nil {
		t.Fatalf("failed to start solo expedition: %v", err)
	}

	mapView1, err := svc.ObserveExpedition(ctx, "char-1")
	if err != nil {
		t.Fatalf("ObserveExpedition failed: %v", err)
	}
	if mapView1.Map.Radius != 1 {
		t.Errorf("expected solo warrior map radius 1, got %d", mapView1.Map.Radius)
	}
	if mapView1.Map.ScoutingBonusActive {
		t.Errorf("expected no scouting bonus for solo warrior")
	}
	// Check formatted string contains "●"
	if !strings.Contains(mapView1.Map.Formatted, "●") {
		t.Errorf("expected formatted map to contain ●, got:\n%s", mapView1.Map.Formatted)
	}
	// Size of tiles should be (2*1+1) = 3
	if len(mapView1.Map.Tiles) != 3 || len(mapView1.Map.Tiles[0]) != 3 {
		t.Errorf("expected 3x3 tiles for radius 1, got %dx%d", len(mapView1.Map.Tiles), len(mapView1.Map.Tiles[0]))
	}

	// 2. Party with Thief (job 9): leader retains radius 1
	_, _ = svc.Escape(ctx, "char-1") // end solo expedition
	_, err = svc.StartPartyExpedition(ctx, "char-1", []string{"char-1", "char-2"}, dungeon.NativeMap0ID, "")
	if err != nil {
		t.Fatalf("failed to start party expedition: %v", err)
	}

	mapView2, err := svc.ObserveExpedition(ctx, "char-1")
	if err != nil {
		t.Fatalf("ObserveExpedition failed: %v", err)
	}
	if mapView2.Map.Radius != 1 {
		t.Errorf("expected non-scout leader map radius 1, got %d", mapView2.Map.Radius)
	}
	if mapView2.Map.ScoutingBonusActive {
		t.Errorf("another member must not supply scouting")
	}
	// Size of tiles should be (2*2+1) = 5
	if len(mapView2.Map.Tiles) != 3 || len(mapView2.Map.Tiles[0]) != 3 {
		t.Errorf("expected 3x3 tiles for radius 2, got %dx%d", len(mapView2.Map.Tiles), len(mapView2.Map.Tiles[0]))
	}

	// 3. Ninja (26), Geomancer (27), Ranger (79)
	jobs := []string{"job-09", "job-26", "job-27", "job-79", "9", "26", "27", "79", "thief", "ninja", "geomancer", "ranger", "盗賊", "忍者", "風水師", "レンジャー"}
	for _, jobID := range jobs {
		cTest := createTestChar("scout-"+jobID, 10, 100, 30, 20)
		cTest.JobID = jobID
		charRepo.chars[cTest.ID] = cTest
		testSvcCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
			"lead":   c1Lead(10),
			cTest.ID: cTest,
		}}
		tSvc, _ := dungeon.NewService(newMockDungeonRepo(), testSvcCharRepo, &corebattle.Engine{}, dungeon.WithAdmission(approvedRoster{}), dungeon.WithInventoryProvider(&mockInventoryRepo{}))
		_, err := tSvc.StartPartyExpedition(ctx, "lead", []string{"lead", cTest.ID}, dungeon.NativeMap0ID, "")
		if err != nil {
			t.Fatalf("failed to start for job %s: %v", jobID, err)
		}
		mv, err := tSvc.ObserveExpedition(ctx, cTest.ID)
		if err != nil {
			t.Fatalf("ObserveExpedition failed for job %s: %v", jobID, err)
		}
		if mv.Map.Radius != 2 {
			t.Errorf("expected radius 2 for scouting job %s, got %d", jobID, mv.Map.Radius)
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
		dungeon.WithAdmission(approvedRoster{}),
	)
	// Give item 197 to scope-hero
	inv, _ := coreinventory.New(cItem.ID)
	_ = inv.Add(coreitem.Instance{ID: "scope-inst", DefinitionID: "item-197", Quantity: 1})
	invRepo.invs[cItem.ID] = inv

	_, err = itemSvc.StartPartyExpedition(ctx, cItem.ID, []string{cItem.ID}, dungeon.NativeMap0ID, "")
	if err != nil {
		t.Fatalf("failed to start item expedition: %v", err)
	}
	mvItem, err := itemSvc.ObserveExpedition(ctx, cItem.ID)
	if err != nil {
		t.Fatalf("ObserveExpedition failed: %v", err)
	}
	if mvItem.Map.Radius != 2 {
		t.Errorf("expected radius 2 for item 197 holder, got %d", mvItem.Map.Radius)
	}
	if len(mvItem.Map.Tiles) != 5 || len(mvItem.Map.Tiles[0]) != 5 {
		t.Errorf("expected 5x5 tiles for radius 2, got %dx%d", len(mvItem.Map.Tiles), len(mvItem.Map.Tiles[0]))
	}

	// 5. Stacking: Scouting Job (Thief 9) + Scope Goggles (item 197) expands radius to 3 (7x7 grid)
	cBoth := createTestChar("scout-scope-hero", 10, 100, 30, 20)
	cBoth.JobID = "job-09" // Thief
	stackCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cBoth.ID: cBoth}}
	stackInvRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory)}
	stackInv, _ := coreinventory.New(cBoth.ID)
	_ = stackInv.Add(coreitem.Instance{ID: "scope-inst-2", DefinitionID: "item-197", Quantity: 1})
	stackInvRepo.invs[cBoth.ID] = stackInv

	stackSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		stackCharRepo,
		&corebattle.Engine{},
		dungeon.WithInventoryProvider(stackInvRepo),
		dungeon.WithAdmission(approvedRoster{}),
	)
	_, err = stackSvc.StartPartyExpedition(ctx, cBoth.ID, []string{cBoth.ID}, dungeon.NativeMap0ID, "")
	if err != nil {
		t.Fatalf("failed to start stack expedition: %v", err)
	}
	mvBoth, err := stackSvc.ObserveExpedition(ctx, cBoth.ID)
	if err != nil {
		t.Fatalf("ObserveExpedition failed: %v", err)
	}
	if mvBoth.Map.Radius != 3 {
		t.Errorf("expected radius 3 for scouting job + scope goggles, got %d", mvBoth.Map.Radius)
	}
	if !mvBoth.Map.ScoutingBonusActive {
		t.Errorf("expected scouting bonus active for stacked scouting")
	}
	if len(mvBoth.Map.Tiles) != 7 || len(mvBoth.Map.Tiles[0]) != 7 {
		t.Errorf("expected 7x7 tiles for radius 3, got %dx%d", len(mvBoth.Map.Tiles), len(mvBoth.Map.Tiles[0]))
	}

	// 6. Different members cannot stack their visibility bonuses.
	cScout := createTestChar("party-thief", 10, 100, 30, 20)
	cScout.JobID = "job-09"
	cGoggleUser := createTestChar("party-goggle-user", 10, 100, 30, 20)
	cGoggleUser.JobID = "warrior"
	multiCharRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		cScout.ID:      cScout,
		cGoggleUser.ID: cGoggleUser,
	}}
	multiInvRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory)}
	goggleInv, _ := coreinventory.New(cGoggleUser.ID)
	_ = goggleInv.Add(coreitem.Instance{ID: "goggle-inst", DefinitionID: "item-197", Quantity: 1})
	multiInvRepo.invs[cGoggleUser.ID] = goggleInv

	multiSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		multiCharRepo,
		&corebattle.Engine{},
		dungeon.WithInventoryProvider(multiInvRepo),
		dungeon.WithAdmission(approvedRoster{}),
	)
	_, err = multiSvc.StartPartyExpedition(ctx, cScout.ID, []string{cScout.ID, cGoggleUser.ID}, dungeon.NativeMap0ID, "")
	if err != nil {
		t.Fatalf("failed to start multi-party expedition: %v", err)
	}
	mvMulti, err := multiSvc.ObserveExpedition(ctx, cScout.ID)
	if err != nil {
		t.Fatalf("ObserveExpedition failed: %v", err)
	}
	if mvMulti.Map.Radius != 2 {
		t.Errorf("expected actor-only radius 2, got %d", mvMulti.Map.Radius)
	}
	if !mvMulti.Map.ScoutingBonusActive {
		t.Errorf("expected scouting bonus active for multi-party stacking")
	}
	if len(mvMulti.Map.Tiles) != 5 || len(mvMulti.Map.Tiles[0]) != 5 {
		t.Errorf("expected actor-only 5x5 tiles, got %dx%d", len(mvMulti.Map.Tiles), len(mvMulti.Map.Tiles[0]))
	}
	if !strings.Contains(mvMulti.Map.BonusReason, "Scouting Job") || strings.Contains(mvMulti.Map.BonusReason, "Scope Goggles") {
		t.Errorf("expected BonusReason to mention only actor scouting, got: %s", mvMulti.Map.BonusReason)
	}
}

func c1Lead(lv int) corecharacter.Character {
	c := createTestChar("lead", lv, 150, 40, 30)
	c.JobID = "hero"
	return c
}

// approvedRoster models a lobby that has already admitted these gameplay fixtures.
// Authorization regressions use the real Party service in composition tests.
type approvedRoster struct{}

func (approvedRoster) AdmitParty(ctx context.Context, actor, partyID string, ids []string, start func(context.Context, string, []string) error) error {
	if len(ids) == 0 {
		ids = []string{actor}
	}
	return start(ctx, partyID, ids)
}
func (approvedRoster) AdmitSolo(ctx context.Context, actor string, start func(context.Context) error) error {
	return start(ctx)
}

func TestPartyRequiresAdmission(t *testing.T) {
	svc, err := dungeon.NewService(newMockDungeonRepo(), &mockCharRepo{}, &corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartPartyExpedition(context.Background(), "leader", []string{"leader", "member"}, dungeon.NativeMap0ID, "p"); !errors.Is(err, dungeon.ErrPartyUnavailable) {
		t.Fatalf("party start without consent port: %v", err)
	}
}

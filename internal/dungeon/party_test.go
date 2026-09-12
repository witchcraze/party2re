package dungeon_test

import (
	"context"
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

	svc, err := dungeon.NewService(repo, charRepo, engine)
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
	exp, err := svc.StartPartyExpedition(ctx, "char-1", memberIDs, "dungeon-01", "party-01")
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
	_, err = svc5.StartPartyExpedition(ctx, "char-1", []string{"char-1", "char-2", "char-3", "char-4", "char-5"}, "dungeon-01", "")
	if err == nil {
		t.Fatal("expected error for party exceeding 4 members")
	}

	// 3. Reject if non-existent member
	_, err = svc5.StartPartyExpedition(ctx, "char-1", []string{"char-1", "non-existent"}, "dungeon-01", "")
	if err == nil {
		t.Fatal("expected error for non-existent member")
	}
}

func TestPartyMapScouting(t *testing.T) {
	svc, _, charRepo := setupPartyTestService()
	ctx := context.Background()

	// 1. Solo warrior: base view radius = 1 (3x3 grid)
	_, err := svc.StartPartyExpedition(ctx, "char-1", []string{"char-1"}, "dungeon-01", "")
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
	_, err = svc.StartPartyExpedition(ctx, "char-1", []string{"char-1", "char-2"}, "dungeon-01", "")
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
		tSvc, _ := dungeon.NewService(newMockDungeonRepo(), testSvcCharRepo, &corebattle.Engine{})
		_, err := tSvc.StartPartyExpedition(ctx, "lead", []string{"lead", cTest.ID}, "dungeon-01", "")
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

	_, err = itemSvc.StartPartyExpedition(ctx, cItem.ID, []string{cItem.ID}, "dungeon-01", "")
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
	tSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"p1": createTestChar("p1", 10, 150, 40, 30),
			"p2": createTestChar("p2", 10, 100, 30, 20),
			"p3": createTestChar("p3", 10, 80, 25, 15),
		}},
		&corebattle.Engine{},
		dungeon.WithCustomDungeons(customCatalog),
	)

	_, err := tSvc.StartPartyExpedition(ctx, "p1", []string{"p1", "p2", "p3"}, "trap-dungeon", "")
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
	_, _ = normalSvc.StartPartyExpedition(ctx, "p1", []string{"p1"}, "chest-dungeon", "")
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
	thSvc, _ := dungeon.NewService(
		newMockDungeonRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"p1": createTestChar("p1", 10, 150, 40, 30),
			"th": thChar,
		}},
		&corebattle.Engine{},
		dungeon.WithCustomDungeons(customCatalog),
	)
	_, _ = thSvc.StartPartyExpedition(ctx, "p1", []string{"p1", "th"}, "chest-dungeon", "")
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

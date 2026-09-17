package party

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (r *mockDepotRepo) FindByCharacterID(_ context.Context, charID string) (depot.Depot, error) {
	return r.FindByCharacterIDForUpdate(context.Background(), charID)
}

func (r *mockDepotRepo) FindByCharacterIDForUpdate(_ context.Context, charID string) (depot.Depot, error) {
	d, ok := r.depots[charID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (r *mockDepotRepo) Save(_ context.Context, d depot.Depot) error {
	r.depots[d.CharacterID] = d
	return nil
}

type fixedBattleEngine struct {
	remainingHP map[string]int
	remainingMP map[string]int
	outcome     corebattle.Outcome
}

func (e fixedBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	remHP := make(map[string]int)
	remMP := make(map[string]int)
	for _, p := range req.Allies {
		if e.remainingHP != nil {
			if hp, ok := e.remainingHP[p.ID]; ok {
				remHP[p.ID] = hp
			} else {
				remHP[p.ID] = p.HP
			}
		} else {
			remHP[p.ID] = p.HP
		}

		if e.remainingMP != nil {
			if mp, ok := e.remainingMP[p.ID]; ok {
				remMP[p.ID] = mp
			} else {
				remMP[p.ID] = p.MP
			}
		} else {
			remMP[p.ID] = p.MP
		}
	}

	outcome := e.outcome
	if outcome == "" {
		outcome = corebattle.OutcomeWin
	}

	return corebattle.PartyBattleResult{
		Outcome:     outcome,
		Turns:       1,
		RemainingHP: remHP,
		RemainingMP: remMP,
		TotalReward: corebattle.Reward{
			Experience: 100,
			Currency:   50,
			Crystals:   5,
		},
	}, nil
}

func setupPartyAdventureTestService(
	t *testing.T,
	engine BattleEngine,
	opts ...Option,
) (*Service, *mockPartyRepository, *mockCharacterRepository, *mockInventoryRepository, *mockDepotRepo) {
	t.Helper()

	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	depotRepo := newMockDepotRepo()

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	if engine == nil {
		engine = fixedBattleEngine{outcome: corebattle.OutcomeWin}
	}

	serviceOpts := []Option{
		WithDepotRepository(depotRepo),
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

	return svc, partyRepo, charRepo, invRepo, depotRepo
}

func createReadyParty(
	ctx context.Context,
	t *testing.T,
	svc *Service,
	partyRepo *mockPartyRepository,
	charRepo *mockCharacterRepository,
	leader corecharacter.Character,
	members ...corecharacter.Character,
) string {
	t.Helper()

	charRepo.chars[leader.ID] = leader
	for _, m := range members {
		charRepo.chars[m.ID] = m
	}

	pDetail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:       "TestParty",
		StageID:    "stage-01",
		MaxMembers: 1 + len(members),
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	partyID := pDetail.Party.ID
	for _, m := range members {
		_, err := svc.JoinParty(ctx, partyID, m.ID, "")
		if err != nil {
			t.Fatalf("JoinParty failed for %s: %v", m.ID, err)
		}
		_, err = svc.SetReady(ctx, partyID, m.ID, true)
		if err != nil {
			t.Fatalf("SetReady failed for %s: %v", m.ID, err)
		}
	}

	return partyID
}

func fillInventory(t *testing.T, invRepo *mockInventoryRepository, charID string) {
	t.Helper()
	inv, _ := coreinventory.New(charID)
	inst, err := coreitem.NewInstance("item-001", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(inst); err != nil {
		t.Fatal(err)
	}
	_ = invRepo.Save(context.Background(), inv)
}

func fillDepot(t *testing.T, depotRepo *mockDepotRepo, charID string, cap int) {
	t.Helper()
	dep, err := depot.NewDepot(charID)
	if err != nil {
		t.Fatal(err)
	}
	dep.Capacity = cap
	for i := 0; i < cap; i++ {
		inst, err := coreitem.NewInstance(fmt.Sprintf("fill-item-%d", i), 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := dep.AddItem(inst); err != nil {
			t.Fatalf("failed to fill depot slot %d: %v", i, err)
		}
	}
	_ = depotRepo.Save(context.Background(), dep)
}

func TestPartyAdventure_DepotFallbackOnFullInventory_FallbackPath(t *testing.T) {
	ctx := context.Background()
	engine := fixedBattleEngine{
		remainingHP: map[string]int{"leader": 80},
		remainingMP: map[string]int{"leader": 35},
		outcome:     corebattle.OutcomeWin,
	}
	svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(t, engine)

	leader := corecharacter.Character{
		ID:       "leader",
		Name:     "Hero",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    50,
			MaxMP: 50,
		},
	}

	// 1. Fill inventory to maximum (1 item)
	fillInventory(t, invRepo, leader.ID)

	// 2. Setup depot with space available
	dep, _ := depot.NewDepot(leader.ID)
	dep.Capacity = 10
	_ = depotRepo.Save(ctx, dep)

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	if res.Outcome != string(corebattle.OutcomeWin) {
		t.Fatalf("expected Win outcome, got %s", res.Outcome)
	}

	// Verify MP persistence
	updatedLeader, err := charRepo.FindByID(ctx, leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedLeader.Stats.MP != 35 {
		t.Errorf("expected remaining MP 35, got %d", updatedLeader.Stats.MP)
	}
	if updatedLeader.Stats.HP != 80 {
		t.Errorf("expected remaining HP 80, got %d", updatedLeader.Stats.HP)
	}

	// Verify treasure boxes were generated and delivered to depot
	if len(res.TreasureBoxes) == 0 {
		t.Fatal("expected at least 1 treasure box on Floor 11")
	}

	for _, box := range res.TreasureBoxes {
		if box.OpenedBy == leader.ID && box.ItemID != "" {
			if box.DeliveredTo != "depot" {
				t.Errorf("expected treasure box delivered to 'depot', got %q", box.DeliveredTo)
			}
		}
	}

	// Verify depot actually received the item
	savedDepot, err := depotRepo.FindByCharacterIDForUpdate(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find saved depot: %v", err)
	}
	if len(savedDepot.Items) == 0 {
		t.Error("expected depot to contain items from Floor 11 fallback")
	}

	// Verify drops in reward summary include the depot-delivered items
	if len(res.Rewards) == 0 || len(res.Rewards[0].Drops) == 0 {
		t.Error("expected reward summary to include acquired drops delivered to depot")
	}
}

func TestPartyAdventure_LostDropOnFullDepot_FallbackPath(t *testing.T) {
	ctx := context.Background()
	engine := fixedBattleEngine{
		remainingHP: map[string]int{"leader": 60},
		remainingMP: map[string]int{"leader": 20},
		outcome:     corebattle.OutcomeWin,
	}
	svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(t, engine)

	leader := corecharacter.Character{
		ID:       "leader",
		Name:     "Hero",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    40,
			MaxMP: 40,
		},
	}

	// 1. Fill inventory to maximum
	fillInventory(t, invRepo, leader.ID)

	// 2. Fill depot to maximum capacity
	fillDepot(t, depotRepo, leader.ID, depot.CalculateCapacity(leader.JobLevel, 0, 0))

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	// Verify treasure boxes were marked as lost
	for _, box := range res.TreasureBoxes {
		if box.OpenedBy == leader.ID && box.ItemID != "" {
			if box.DeliveredTo != "lost" {
				t.Errorf("expected treasure box delivered_to 'lost', got %q", box.DeliveredTo)
			}
		}
	}

	// Verify LostDrops map is populated
	if len(res.LostDrops[leader.ID]) == 0 {
		t.Error("expected LostDrops to contain lost item definition IDs")
	}

	// Verify drops in reward summary do NOT contain the lost items
	if len(res.Rewards) > 0 && len(res.Rewards[0].Drops) != 0 {
		t.Errorf("expected 0 acquired drops in reward summary when depot is full, got %d", len(res.Rewards[0].Drops))
	}

	// Verify MP was still persisted
	updatedLeader, _ := charRepo.FindByID(ctx, leader.ID)
	if updatedLeader.Stats.MP != 20 {
		t.Errorf("expected MP 20, got %d", updatedLeader.Stats.MP)
	}
}

func TestPartyAdventure_SurvivingMPAndFallenHP(t *testing.T) {
	ctx := context.Background()
	engine := fixedBattleEngine{
		remainingHP: map[string]int{
			"leader":  0,  // fallen ally survives with 1 HP
			"member2": 45, // normal survival
		},
		remainingMP: map[string]int{
			"leader":  15,
			"member2": 999, // exceeds max MP 50 -> must be clamped to 50
		},
		outcome: corebattle.OutcomeWin,
	}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, engine)

	leader := corecharacter.Character{
		ID:       "leader",
		Name:     "LeaderHero",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    50,
			MaxMP: 50,
		},
	}
	member2 := corecharacter.Character{
		ID:       "member2",
		Name:     "MageAlly",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    80,
			MaxHP: 80,
			MP:    50,
			MaxMP: 50,
		},
	}

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, member2)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	// Leader was fallen (HP=0) -> survives with HP=1, MP=15
	updatedLeader, _ := charRepo.FindByID(ctx, leader.ID)
	if updatedLeader.Stats.HP != 1 {
		t.Errorf("expected fallen leader HP=1, got %d", updatedLeader.Stats.HP)
	}
	if updatedLeader.Stats.MP != 15 {
		t.Errorf("expected leader MP=15, got %d", updatedLeader.Stats.MP)
	}

	// Member2 survived with HP=45, MP clamped to MaxMP=50
	updatedMember2, _ := charRepo.FindByID(ctx, member2.ID)
	if updatedMember2.Stats.HP != 45 {
		t.Errorf("expected member2 HP=45, got %d", updatedMember2.Stats.HP)
	}
	if updatedMember2.Stats.MP != 50 {
		t.Errorf("expected member2 MP clamped to 50, got %d", updatedMember2.Stats.MP)
	}

	if res.Outcome != string(corebattle.OutcomeWin) {
		t.Errorf("expected outcome Win, got %s", res.Outcome)
	}
}

func TestPartyAdventure_WithBattleSettler_FullIntegration(t *testing.T) {
	ctx := context.Background()

	engine := fixedBattleEngine{
		remainingHP: map[string]int{
			"lead-char": 77,
			"sub-char":  50,
		},
		remainingMP: map[string]int{
			"lead-char": 33,
			"sub-char":  12,
		},
		outcome: corebattle.OutcomeWin,
	}

	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	depotRepo := newMockDepotRepo()

	leader := corecharacter.Character{
		ID:       "lead-char",
		Name:     "LeadHero",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    50,
			MaxMP: 50,
		},
	}
	subMember := corecharacter.Character{
		ID:       "sub-char",
		Name:     "SubMage",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    80,
			MaxHP: 80,
			MP:    40,
			MaxMP: 40,
		},
	}

	charRepo.chars[leader.ID] = leader
	charRepo.chars[subMember.ID] = subMember

	// Fill leader inventory to trigger depot delivery
	fillInventory(t, invRepo, leader.ID)
	// Fill subMember inventory AND depot to trigger lost drop
	fillInventory(t, invRepo, subMember.ID)
	fillDepot(t, depotRepo, subMember.ID, depot.CalculateCapacity(subMember.JobLevel, 0, 0))

	// Create real battle adapter as PostBattleSettler
	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(corebattle.Engine{}),
	)

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		engine,
		WithPostBattleSettler(battleAdapter),
		WithDepotRepository(depotRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, subMember)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed with battle adapter: %v", err)
	}

	// 1. Verify HP & MP persistence via Battle Adapter
	updatedLeader, _ := charRepo.FindByID(ctx, leader.ID)
	if updatedLeader.Stats.HP != 77 {
		t.Errorf("expected leader HP=77, got %d", updatedLeader.Stats.HP)
	}
	if updatedLeader.Stats.MP != 33 {
		t.Errorf("expected leader MP=33, got %d", updatedLeader.Stats.MP)
	}

	updatedSub, _ := charRepo.FindByID(ctx, subMember.ID)
	if updatedSub.Stats.HP != 50 {
		t.Errorf("expected subMember HP=50, got %d", updatedSub.Stats.HP)
	}
	if updatedSub.Stats.MP != 12 {
		t.Errorf("expected subMember MP=12, got %d", updatedSub.Stats.MP)
	}

	// 2. Verify Floor 11 treasure deliveries
	for _, box := range res.TreasureBoxes {
		if box.OpenedBy == leader.ID && box.ItemID != "" {
			if box.DeliveredTo != "depot" {
				t.Errorf("expected leader box delivered to depot, got %q", box.DeliveredTo)
			}
		}
		if box.OpenedBy == subMember.ID && box.ItemID != "" {
			if box.DeliveredTo != "lost" {
				t.Errorf("expected subMember box delivered_to lost, got %q", box.DeliveredTo)
			}
		}
	}

	// 3. Verify subMember lost drops tracked
	if len(res.LostDrops[subMember.ID]) == 0 {
		t.Error("expected subMember to have lost drops recorded")
	}

	// 4. Verify leader has acquired drop from depot
	var leaderDrops []coreitem.Instance
	for _, r := range res.Rewards {
		if r.CharacterID == leader.ID {
			leaderDrops = r.Drops
		}
	}
	if len(leaderDrops) == 0 {
		t.Error("expected leader to have acquired drops in reward summary")
	}
}

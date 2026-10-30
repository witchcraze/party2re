package party

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

	bonusPercent := (len(req.Allies) - 1) * 10
	if bonusPercent < 0 {
		bonusPercent = 0
	} else if bonusPercent > 30 {
		bonusPercent = 30
	}

	baseReward := corebattle.Reward{
		Experience: 100,
		Currency:   50,
		Crystals:   5,
	}
	totalReward := corebattle.Reward{
		Experience: baseReward.Experience * (100 + bonusPercent) / 100,
		Currency:   baseReward.Currency * (100 + bonusPercent) / 100,
		Crystals:   baseReward.Crystals,
	}

	return corebattle.PartyBattleResult{
		Outcome:      outcome,
		Turns:        1,
		RemainingHP:  remHP,
		RemainingMP:  remMP,
		BaseReward:   baseReward,
		BonusPercent: bonusPercent,
		TotalReward:  totalReward,
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

func TestPartyAdventure_SaveAdventureLogErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-lead-err1", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	injectedErr := errors.New("db error saving adventure log")
	partyRepo.saveLogErr = injectedErr

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on save adventure log failure, got nil")
	}
	if !strings.Contains(err.Error(), "save party adventure log") {
		t.Errorf("expected error to contain 'save party adventure log', got: %v", err)
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected wrapped injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
	// Verify defer reverted party status back to recruiting
	p, _ := partyRepo.GetParty(ctx, partyID)
	if p.Status != StatusRecruiting {
		t.Errorf("expected party status reverted to recruiting, got %s", p.Status)
	}
}

func TestPartyAdventure_ResetPartyStatusErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-lead-err2", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	// Reset call is Call 2 of UpdateParty (Call 1 transitions to InProgress, Call 2 resets to Recruiting)
	injectedErr := errors.New("db error resetting party status")
	partyRepo.updatePartyErr = injectedErr
	partyRepo.updatePartyCallCount = 0
	partyRepo.failUpdatePartyOn = 2

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on reset party status failure, got nil")
	}
	if !strings.Contains(err.Error(), "reset party status") {
		t.Errorf("expected error to contain 'reset party status', got: %v", err)
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected wrapped injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
}

func TestPartyAdventure_ResetMemberReadyErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-lead-err3", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	injectedErr := errors.New("db error resetting member ready")
	partyRepo.updateMemberReadyErr = injectedErr

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on reset member ready failure, got nil")
	}
	if !strings.Contains(err.Error(), "reset member ready state") {
		t.Errorf("expected error to contain 'reset member ready state', got: %v", err)
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected wrapped injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
}

func TestPartyAdventure_SettlementErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-lead-err4", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	injectedErr := errors.New("db error updating character stats")
	charRepo.updateErr = injectedErr

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on settlement character update failure, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected wrapped injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
}

func TestLeaveParty_LeaderDisband_UpdatePartyErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-leave-lead", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	injectedErr := errors.New("db error disbanding party on leave")
	partyRepo.updatePartyErr = injectedErr

	err := svc.LeaveParty(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on LeaveParty update failure, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
}

func TestDisbandParty_UpdatePartyErrorRollback(t *testing.T) {
	ctx := context.Background()
	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		nil,
		WithTransactionProvider(txProvider),
	)

	leader := corecharacter.Character{ID: "c-disband-lead", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}}
	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader)

	injectedErr := errors.New("db error updating party on disband")
	partyRepo.updatePartyErr = injectedErr

	err := svc.DisbandParty(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected error on DisbandParty update failure, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}
}

func TestPartyService_StartPartyAdventure_SynergyMultiplierRewards(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name              string
		memberCount       int
		expectedSynergy   int
		expectedBaseEXP   int
		expectedBaseGold  int
		expectedFinalEXP  int
		expectedFinalGold int
	}{
		{
			name:              "1-player party (0% synergy)",
			memberCount:       1,
			expectedSynergy:   0,
			expectedBaseEXP:   1000,
			expectedBaseGold:  500,
			expectedFinalEXP:  1000,
			expectedFinalGold: 500,
		},
		{
			name:              "2-player party (+10% synergy)",
			memberCount:       2,
			expectedSynergy:   10,
			expectedBaseEXP:   1000,
			expectedBaseGold:  500,
			expectedFinalEXP:  1100,
			expectedFinalGold: 550,
		},
		{
			name:              "3-player party (+20% synergy)",
			memberCount:       3,
			expectedSynergy:   20,
			expectedBaseEXP:   1000,
			expectedBaseGold:  500,
			expectedFinalEXP:  1200,
			expectedFinalGold: 600,
		},
		{
			name:              "4-player party (+30% synergy)",
			memberCount:       4,
			expectedSynergy:   30,
			expectedBaseEXP:   1000,
			expectedBaseGold:  500,
			expectedFinalEXP:  1300,
			expectedFinalGold: 650,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var recordedGold int
			victoryHook := func(_ context.Context, _ []string, _ int, goldEarned int) error {
				recordedGold = goldEarned
				return nil
			}

			svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
				t,
				fixedBattleEngine{outcome: corebattle.OutcomeWin},
				WithVictoryHook(victoryHook),
			)

			leader := corecharacter.Character{
				ID:    fmt.Sprintf("lead-%d", tc.memberCount),
				Name:  "Leader",
				Level: 10,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			}

			var members []corecharacter.Character
			for i := 1; i < tc.memberCount; i++ {
				members = append(members, corecharacter.Character{
					ID:    fmt.Sprintf("mem-%d-%d", tc.memberCount, i),
					Name:  fmt.Sprintf("Member%d", i),
					Level: 10,
					Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
				})
			}

			partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, members...)

			res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatalf("StartPartyAdventure failed: %v", err)
			}

			if res.Outcome != "win" {
				t.Fatalf("expected win outcome, got %s", res.Outcome)
			}
			if res.SynergyBonusPercent != tc.expectedSynergy {
				t.Errorf("expected SynergyBonusPercent=%d, got %d", tc.expectedSynergy, res.SynergyBonusPercent)
			}
			if res.TotalEXP != tc.expectedFinalEXP {
				t.Errorf("expected TotalEXP=%d, got %d", tc.expectedFinalEXP, res.TotalEXP)
			}
			if res.TotalGold != tc.expectedFinalGold {
				t.Errorf("expected TotalGold=%d, got %d", tc.expectedFinalGold, res.TotalGold)
			}

			// Verify rewards per member
			if len(res.Rewards) != tc.memberCount {
				t.Fatalf("expected %d reward entries, got %d", tc.memberCount, len(res.Rewards))
			}
			for _, r := range res.Rewards {
				if r.GainedEXP != tc.expectedFinalEXP {
					t.Errorf("member %s expected GainedEXP=%d, got %d", r.CharacterID, tc.expectedFinalEXP, r.GainedEXP)
				}
				if r.GainedGold != tc.expectedFinalGold {
					t.Errorf("member %s expected GainedGold=%d, got %d", r.CharacterID, tc.expectedFinalGold, r.GainedGold)
				}
			}

			// Verify adventure log persistence
			if len(partyRepo.logs) != 1 {
				t.Fatalf("expected 1 adventure log saved, got %d", len(partyRepo.logs))
			}
			log := partyRepo.logs[0]
			if log.TotalEXP != tc.expectedFinalEXP {
				t.Errorf("log TotalEXP=%d, got %d", tc.expectedFinalEXP, log.TotalEXP)
			}
			if log.TotalGold != tc.expectedFinalGold {
				t.Errorf("log TotalGold=%d, got %d", tc.expectedFinalGold, log.TotalGold)
			}
			if log.SynergyBonusPercent != tc.expectedSynergy {
				t.Errorf("log SynergyBonusPercent=%d, got %d", tc.expectedSynergy, log.SynergyBonusPercent)
			}

			// Verify victory hook receives scaled gold
			if recordedGold != tc.expectedFinalGold {
				t.Errorf("victoryHook expected gold=%d, got %d", tc.expectedFinalGold, recordedGold)
			}
		})
	}
}

func TestPartyService_StartPartyAdventure_Defeat_UnboostedRewards(t *testing.T) {
	ctx := context.Background()

	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
		t,
		fixedBattleEngine{outcome: corebattle.OutcomeDefeat},
	)

	leader := corecharacter.Character{
		ID:    "defeat-lead",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem1 := corecharacter.Character{
		ID:    "defeat-mem1",
		Name:  "Member1",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem2 := corecharacter.Character{
		ID:    "defeat-mem2",
		Name:  "Member2",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem3 := corecharacter.Character{
		ID:    "defeat-mem3",
		Name:  "Member3",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, mem1, mem2, mem3)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	if res.Outcome != "defeat" {
		t.Fatalf("expected defeat outcome, got %s", res.Outcome)
	}
	if res.SynergyBonusPercent != 30 {
		t.Errorf("expected SynergyBonusPercent=30, got %d", res.SynergyBonusPercent)
	}
	if res.TotalGold != 0 {
		t.Errorf("expected TotalGold=0 on defeat, got %d", res.TotalGold)
	}
	// Defeated on floor 1 without clears, base EXP = 0
	if res.TotalEXP != 0 {
		t.Errorf("expected TotalEXP=0 on defeat, got %d", res.TotalEXP)
	}
}

type mockPartyBlessingProvider struct {
	blessings map[string]string
}

func (m mockPartyBlessingProvider) GetActiveBlessing(_ context.Context, characterID string) (string, error) {
	return m.blessings[characterID], nil
}

type fixedPartyBattleRand struct{ val int }

func (r fixedPartyBattleRand) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	return r.val % n, nil
}

func (r fixedPartyBattleRand) Float64() float64 { return 0 }

func TestPartyAdventure_TreasureBlessing_ChestGenerationAndNoSettlementDuplication(t *testing.T) {
	ctx := context.Background()

	leader := corecharacter.Character{
		ID:       "lead-bless",
		Name:     "Leader",
		Level:    10,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem2 := corecharacter.Character{
		ID:       "mem-bless-2",
		Name:     "Member2",
		Level:    10,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	blessingProv := mockPartyBlessingProvider{
		blessings: map[string]string{
			leader.ID: "宝箱がほしい",
		},
	}

	svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(
		t,
		fixedBattleEngine{outcome: corebattle.OutcomeWin},
		WithBlessingProvider(blessingProv),
	)
	svc.rng = func(n int) int { return 0 } // Lucky chest blessing roll (0 % 5 == 0 -> +1 chest)

	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBlessingProvider(blessingProv),
		battle.WithRandomSource(fixedPartyBattleRand{val: 0}), // Lucky roll in settlement too
		battle.WithMaxInventoryCapacity(10),
	)
	svc.battleSettler = battleSvc

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, mem2)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	// 2 alive party members + 1 from lucky treasure blessing = 3 treasure chests spawned.
	if len(res.TreasureBoxes) != 3 {
		t.Fatalf("expected 3 treasure boxes (2 base + 1 blessing), got %d", len(res.TreasureBoxes))
	}

	// Each member examines 1 chest (total 2 opened, 1 unopened).
	openedCount := 0
	for _, box := range res.TreasureBoxes {
		if box.OpenedBy != "" {
			openedCount++
		}
	}
	if openedCount != 2 {
		t.Fatalf("expected 2 boxes opened by 2 members without Master Key, got %d", openedCount)
	}

	// Each member must receive 1 item in settlement without duplication.
	for _, r := range res.Rewards {
		if len(r.Drops) != 1 {
			t.Errorf("member %s: expected 1 drop without settlement duplication, got %d", r.CharacterID, len(r.Drops))
		}
	}
}

func TestPartyAdventure_TreasureBlessing_ActorScope(t *testing.T) {
	tests := []struct {
		name        string
		leaderBless string
		memberBless string
		wantChests  int
	}{
		{
			name:        "none",
			leaderBless: "",
			memberBless: "",
			wantChests:  2,
		},
		{
			name:        "leader_only",
			leaderBless: "宝箱がほしい",
			memberBless: "",
			wantChests:  3,
		},
		{
			name:        "member_only",
			leaderBless: "",
			memberBless: "宝箱がほしい",
			wantChests:  2,
		},
		{
			name:        "both",
			leaderBless: "宝箱がほしい",
			memberBless: "宝箱がほしい",
			wantChests:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			leader := corecharacter.Character{
				ID:       "lead-" + tt.name,
				Name:     "Leader",
				Level:    10,
				JobLevel: 5,
				Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			}
			member := corecharacter.Character{
				ID:       "mem-" + tt.name,
				Name:     "Member",
				Level:    10,
				JobLevel: 5,
				Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			}

			blessings := make(map[string]string)
			if tt.leaderBless != "" {
				blessings[leader.ID] = tt.leaderBless
			}
			if tt.memberBless != "" {
				blessings[member.ID] = tt.memberBless
			}
			blessingProv := mockPartyBlessingProvider{blessings: blessings}

			svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(
				t,
				fixedBattleEngine{outcome: corebattle.OutcomeWin},
				WithBlessingProvider(blessingProv),
			)
			svc.rng = func(n int) int { return 0 } // Lucky chest blessing roll

			battleSvc := battle.NewService(
				battle.WithCharacterRepository(charRepo),
				battle.WithInventoryRepository(invRepo),
				battle.WithDepotRepository(depotRepo),
				battle.WithBlessingProvider(blessingProv),
				battle.WithRandomSource(fixedPartyBattleRand{val: 0}),
				battle.WithMaxInventoryCapacity(10),
			)
			svc.battleSettler = battleSvc

			partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, member)

			res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatalf("StartPartyAdventure failed: %v", err)
			}

			if len(res.TreasureBoxes) != tt.wantChests {
				t.Fatalf("expected %d treasure boxes, got %d", tt.wantChests, len(res.TreasureBoxes))
			}
		})
	}
}

func TestPartyAdventure_ExitFatigueAndMealReset(t *testing.T) {
	ctx := context.Background()

	t.Run("normal and calm mode victory with battleSettler and fallback", func(t *testing.T) {
		for _, useSettler := range []bool{true, false} {
			for _, mode := range []string{adventure.CrawlModeNormal, adventure.CrawlModeCalm} {
				wantFatigue := 34
				if mode == adventure.CrawlModeCalm {
					wantFatigue = 17
				}

				svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(
					t,
					fixedBattleEngine{outcome: corebattle.OutcomeWin},
				)
				if useSettler {
					svc.battleSettler = battle.NewService(
						battle.WithCharacterRepository(charRepo),
						battle.WithInventoryRepository(invRepo),
						battle.WithDepotRepository(depotRepo),
					)
				}

				leader := corecharacter.Character{
					ID:    "lead-fat",
					Name:  "Leader",
					Level: 10,
					Tired: 10,
					Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
				}
				ally := corecharacter.Character{
					ID:    "ally-fat",
					Name:  "Ally",
					Level: 10,
					Tired: 20,
					Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
				}
				charRepo.chars[leader.ID] = leader
				charRepo.chars[ally.ID] = ally

				isFull := map[string]bool{leader.ID: true, ally.ID: true}
				svc.SetPostAdventureHook(func(_ context.Context, characterID string) error {
					isFull[characterID] = false
					return nil
				})

				pDetail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
					Name:       "FatigueParty",
					StageID:    "stage-01",
					MaxMembers: 2,
				})
				if err != nil {
					t.Fatal(err)
				}
				if mode != adventure.CrawlModeNormal {
					p, _ := partyRepo.GetParty(ctx, pDetail.Party.ID)
					p.Mode = mode
					_ = partyRepo.UpdateParty(ctx, p)
				}
				if _, err := svc.JoinParty(ctx, pDetail.Party.ID, ally.ID, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.SetReady(ctx, pDetail.Party.ID, ally.ID, true); err != nil {
					t.Fatal(err)
				}

				res, err := svc.StartPartyAdventure(ctx, pDetail.Party.ID, leader.ID)
				if err != nil {
					t.Fatalf("StartPartyAdventure(settler=%v, mode=%s) failed: %v", useSettler, mode, err)
				}
				if res.Round != 11 || res.ExitFatigue != wantFatigue {
					t.Fatalf("Round=%d ExitFatigue=%d, want 11 and %d", res.Round, res.ExitFatigue, wantFatigue)
				}

				savedLead, _ := charRepo.FindByID(ctx, leader.ID)
				savedAlly, _ := charRepo.FindByID(ctx, ally.ID)
				if savedLead.Tired != 10+wantFatigue {
					t.Errorf("leader Tired = %d, want %d", savedLead.Tired, 10+wantFatigue)
				}
				if savedAlly.Tired != 20+wantFatigue {
					t.Errorf("ally Tired = %d, want %d", savedAlly.Tired, 20+wantFatigue)
				}
				if isFull[leader.ID] || isFull[ally.ID] {
					t.Errorf("expected meal flags reset for both party members, got %+v", isFull)
				}
			}
		}
	})

	t.Run("defeat on Floor 1 applies Round 1 exit fatigue and resets meal flag; rollback does not", func(t *testing.T) {
		txProvider := &mockTransactionProvider{}
		svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
			t,
			fixedBattleEngine{outcome: corebattle.OutcomeDefeat},
			WithTransactionProvider(txProvider),
		)

		leader := corecharacter.Character{
			ID:    "lead-def",
			Name:  "Leader",
			Level: 10,
			Tired: 12,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
		}
		ally := corecharacter.Character{
			ID:    "ally-def",
			Name:  "Ally",
			Level: 10,
			Tired: 15,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
		}
		partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, ally)

		isFull := map[string]bool{leader.ID: true, ally.ID: true}
		svc.SetPostAdventureHook(func(_ context.Context, characterID string) error {
			isFull[characterID] = false
			return nil
		})

		// 1. Persistence failure rolls back transaction and does not reset meal flag
		partyRepo.saveLogErr = errors.New("db log failure")
		if _, err := svc.StartPartyAdventure(ctx, partyID, leader.ID); err == nil {
			t.Fatal("expected error on log failure")
		}
		if !isFull[leader.ID] || !isFull[ally.ID] {
			t.Fatalf("expected meal flags untouched on rollback, got %+v", isFull)
		}

		// Reset character map and log error for successful defeat commit
		charRepo.chars[leader.ID] = leader
		charRepo.chars[ally.ID] = ally
		partyRepo.saveLogErr = nil

		res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
		if err != nil {
			t.Fatalf("StartPartyAdventure defeat failed: %v", err)
		}
		if res.Round != 1 || res.ExitFatigue != 4 {
			t.Fatalf("defeat Round=%d ExitFatigue=%d, want 1 and 4", res.Round, res.ExitFatigue)
		}
		savedLead, _ := charRepo.FindByID(ctx, leader.ID)
		savedAlly, _ := charRepo.FindByID(ctx, ally.ID)
		if savedLead.Tired != 16 || savedAlly.Tired != 19 {
			t.Errorf("Tired after Floor 1 defeat: lead=%d ally=%d, want 16 and 19", savedLead.Tired, savedAlly.Tired)
		}
		if isFull[leader.ID] || isFull[ally.ID] {
			t.Errorf("expected meal flags reset on committed defeat, got %+v", isFull)
		}
	})
}

func TestCreateParty_ModeValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, charRepo, _, _ := setupPartyAdventureTestService(t, nil)

	leader := corecharacter.Character{
		ID:    "mode-val-lead",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	charRepo.chars[leader.ID] = leader

	t.Run("empty mode defaults to normal", func(t *testing.T) {
		detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Name:    "PartyEmptyMode",
			StageID: "stage-01",
			Mode:    "",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if detail.Party.Mode != adventure.CrawlModeNormal {
			t.Errorf("expected Mode %q, got %q", adventure.CrawlModeNormal, detail.Party.Mode)
		}
		_ = svc.DisbandParty(ctx, detail.Party.ID, leader.ID)
	})

	t.Run("normal mode accepted case-insensitively", func(t *testing.T) {
		detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Name:    "PartyNormalMode",
			StageID: "stage-01",
			Mode:    "NORMAL",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if detail.Party.Mode != adventure.CrawlModeNormal {
			t.Errorf("expected Mode %q, got %q", adventure.CrawlModeNormal, detail.Party.Mode)
		}
		_ = svc.DisbandParty(ctx, detail.Party.ID, leader.ID)
	})

	t.Run("unimplemented modes return ErrUnsupportedMode", func(t *testing.T) {
		for _, unsupported := range []string{"calm", "try", "hard", "CALM"} {
			_, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
				Name:    "PartyUnsupported",
				StageID: "stage-01",
				Mode:    unsupported,
			})
			if !errors.Is(err, ErrUnsupportedMode) {
				t.Errorf("mode %q: expected ErrUnsupportedMode, got %v", unsupported, err)
			}
		}
	})

	t.Run("unrecognized mode returns ErrInvalidMode", func(t *testing.T) {
		_, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Name:    "PartyInvalid",
			StageID: "stage-01",
			Mode:    "not_a_valid_mode",
		})
		if !errors.Is(err, ErrInvalidMode) {
			t.Errorf("expected ErrInvalidMode, got %v", err)
		}
	})
}

type battleEngineFunc func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)

func (f battleEngineFunc) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return f(req)
}

func TestPartyAdventure_SynergyMultiplier_RealBattleEngine(t *testing.T) {
	ctx := context.Background()

	zeroRNG := func(n int) int { return 0 }

	type runResult struct {
		exp   int
		gold  int
		bonus int
	}
	results := make(map[int]runResult)

	for count := 1; count <= 4; count++ {
		svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(
			t,
			corebattle.Engine{},
			WithRNG(zeroRNG),
		)

		leader := corecharacter.Character{
			ID:    fmt.Sprintf("real-lead-%d", count),
			Name:  "RealLeader",
			Level: 99,
			Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999, MP: 500, MaxMP: 500},
		}

		var members []corecharacter.Character
		for i := 1; i < count; i++ {
			members = append(members, corecharacter.Character{
				ID:    fmt.Sprintf("real-mem-%d-%d", count, i),
				Name:  fmt.Sprintf("RealMember%d", i),
				Level: 99,
				Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999, MP: 500, MaxMP: 500},
			})
		}

		partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, members...)

		res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
		if err != nil {
			t.Fatalf("%d-player StartPartyAdventure failed: %v", count, err)
		}
		if res.Outcome != "win" {
			t.Fatalf("%d-player expected win, got %s", count, res.Outcome)
		}

		results[count] = runResult{
			exp:   res.TotalEXP,
			gold:  res.TotalGold,
			bonus: res.SynergyBonusPercent,
		}
	}

	if results[1].bonus != 0 {
		t.Errorf("1-player expected bonus 0, got %d", results[1].bonus)
	}
	if results[2].bonus != 10 {
		t.Errorf("2-player expected bonus 10, got %d", results[2].bonus)
	}
	if results[3].bonus != 20 {
		t.Errorf("3-player expected bonus 20, got %d", results[3].bonus)
	}
	if results[4].bonus != 30 {
		t.Errorf("4-player expected bonus 30, got %d", results[4].bonus)
	}

	// 4-player rewards must NOT be double-scaled (1.3 * 1.3 = 1.69)
	expected4ExpApprox := results[1].exp * 130 / 100
	buggy4ExpApprox := expected4ExpApprox * 130 / 100

	if results[4].exp >= buggy4ExpApprox {
		t.Errorf("4-player EXP (%d) was double-multiplied! Buggy expected ~%d, correct ~%d",
			results[4].exp, buggy4ExpApprox, expected4ExpApprox)
	}
}

func TestPartyAdventure_SynergyMultiplier_ExactBase100(t *testing.T) {
	ctx := context.Background()

	// 10 floors with base 10 EXP and 10 Gold per floor -> exactly 100 base EXP, 100 base Gold.
	// 1 player (0%):  10 * 10 = 100 EXP, 100 Gold
	// 2 player (10%): 10 * 11 = 110 EXP, 110 Gold
	// 3 player (20%): 10 * 12 = 120 EXP, 120 Gold
	// 4 player (30%): 10 * 13 = 130 EXP, 130 Gold (must NEVER be 169!)

	tests := []struct {
		members           int
		wantBonus         int
		expectedTotalEXP  int
		expectedTotalGold int
	}{
		{members: 1, wantBonus: 0, expectedTotalEXP: 100, expectedTotalGold: 100},
		{members: 2, wantBonus: 10, expectedTotalEXP: 110, expectedTotalGold: 110},
		{members: 3, wantBonus: 20, expectedTotalEXP: 120, expectedTotalGold: 120},
		{members: 4, wantBonus: 30, expectedTotalEXP: 130, expectedTotalGold: 130},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_members_base100", tc.members), func(t *testing.T) {
			engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				remHP := make(map[string]int)
				for _, p := range req.Allies {
					remHP[p.ID] = p.HP
				}
				bonusPercent := (len(req.Allies) - 1) * 10
				if bonusPercent < 0 {
					bonusPercent = 0
				} else if bonusPercent > 30 {
					bonusPercent = 30
				}
				return corebattle.PartyBattleResult{
					Outcome:      corebattle.OutcomeWin,
					Turns:        1,
					RemainingHP:  remHP,
					BonusPercent: bonusPercent,
					TotalReward: corebattle.Reward{
						Experience: 10 * (100 + bonusPercent) / 100,
						Currency:   10 * (100 + bonusPercent) / 100,
					},
				}, nil
			})

			svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, engine)

			leader := corecharacter.Character{
				ID:    fmt.Sprintf("b100-lead-%d", tc.members),
				Name:  "Leader",
				Level: 10,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			}
			var members []corecharacter.Character
			for i := 1; i < tc.members; i++ {
				members = append(members, corecharacter.Character{
					ID:    fmt.Sprintf("b100-mem-%d-%d", tc.members, i),
					Name:  fmt.Sprintf("Member%d", i),
					Level: 10,
					Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
				})
			}

			partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, members...)

			res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatalf("StartPartyAdventure failed: %v", err)
			}
			if res.SynergyBonusPercent != tc.wantBonus {
				t.Errorf("expected SynergyBonusPercent=%d, got %d", tc.wantBonus, res.SynergyBonusPercent)
			}
			if res.TotalEXP != tc.expectedTotalEXP {
				t.Errorf("expected TotalEXP=%d, got %d", tc.expectedTotalEXP, res.TotalEXP)
			}
			if res.TotalGold != tc.expectedTotalGold {
				t.Errorf("expected TotalGold=%d, got %d", tc.expectedTotalGold, res.TotalGold)
			}
		})
	}
}

func TestPartyAdventure_SynergyMultiplier_FractionalRounding(t *testing.T) {
	ctx := context.Background()

	// Base reward per floor: 105 EXP, 103 Gold.
	// Over 10 floors:
	// 1 player (0%):  floor 105 EXP, 103 Gold -> total 1050 EXP, 1030 Gold
	// 2 player (10%): floor int(105 * 1.1) = 115 EXP, int(103 * 1.1) = 113 Gold -> total 1150 EXP, 1130 Gold
	// 3 player (20%): floor int(105 * 1.2) = 126 EXP, int(103 * 1.2) = 123 Gold -> total 1260 EXP, 1230 Gold
	// 4 player (30%): floor int(105 * 1.3) = 136 EXP, int(103 * 1.3) = 133 Gold -> total 1360 EXP, 1330 Gold

	tests := []struct {
		members           int
		wantBonus         int
		expectedTotalEXP  int
		expectedTotalGold int
	}{
		{members: 1, wantBonus: 0, expectedTotalEXP: 1050, expectedTotalGold: 1030},
		{members: 2, wantBonus: 10, expectedTotalEXP: 1150, expectedTotalGold: 1130},
		{members: 3, wantBonus: 20, expectedTotalEXP: 1260, expectedTotalGold: 1230},
		{members: 4, wantBonus: 30, expectedTotalEXP: 1360, expectedTotalGold: 1330},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_members_fractional", tc.members), func(t *testing.T) {
			engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				remHP := make(map[string]int)
				for _, p := range req.Allies {
					remHP[p.ID] = p.HP
				}
				bonusPercent := (len(req.Allies) - 1) * 10
				if bonusPercent < 0 {
					bonusPercent = 0
				} else if bonusPercent > 30 {
					bonusPercent = 30
				}
				return corebattle.PartyBattleResult{
					Outcome:      corebattle.OutcomeWin,
					Turns:        1,
					RemainingHP:  remHP,
					BonusPercent: bonusPercent,
					TotalReward: corebattle.Reward{
						Experience: 105 * (100 + bonusPercent) / 100,
						Currency:   103 * (100 + bonusPercent) / 100,
					},
				}, nil
			})

			svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, engine)

			leader := corecharacter.Character{
				ID:    fmt.Sprintf("frac-lead-%d", tc.members),
				Name:  "Leader",
				Level: 10,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			}
			var members []corecharacter.Character
			for i := 1; i < tc.members; i++ {
				members = append(members, corecharacter.Character{
					ID:    fmt.Sprintf("frac-mem-%d-%d", tc.members, i),
					Name:  fmt.Sprintf("Member%d", i),
					Level: 10,
					Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
				})
			}

			partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, members...)

			res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatalf("StartPartyAdventure failed: %v", err)
			}
			if res.TotalEXP != tc.expectedTotalEXP {
				t.Errorf("expected TotalEXP=%d, got %d", tc.expectedTotalEXP, res.TotalEXP)
			}
			if res.TotalGold != tc.expectedTotalGold {
				t.Errorf("expected TotalGold=%d, got %d", tc.expectedTotalGold, res.TotalGold)
			}
		})
	}
}

func TestPartyAdventure_MidCrawlFallenAlly(t *testing.T) {
	ctx := context.Background()

	// 4 members start: leader, mem1, mem2, mem3.
	// Base reward per floor: 100 EXP, 100 Gold.
	// Floors 1-5: all 4 are alive -> 4 living allies earn +30% = 130 EXP, 130 Gold per floor (5 * 130 = 650).
	// On floor 5, mem3 falls (HP = 0).
	// Floors 6-10: 3 living allies -> earn +20% = 120 EXP, 120 Gold per floor (5 * 120 = 600).
	// Total expected across crawl = 650 + 600 = 1250 EXP, 1250 Gold.

	floorCounter := 0
	engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		floorCounter++
		remHP := make(map[string]int)
		var fallen []string
		for _, p := range req.Allies {
			if floorCounter >= 5 && p.ID == "mid-mem3" {
				remHP[p.ID] = 0
				fallen = append(fallen, p.ID)
			} else {
				remHP[p.ID] = p.HP
			}
		}

		bonusPercent := (len(req.Allies) - 1) * 10
		if bonusPercent < 0 {
			bonusPercent = 0
		} else if bonusPercent > 30 {
			bonusPercent = 30
		}

		return corebattle.PartyBattleResult{
			Outcome:      corebattle.OutcomeWin,
			Turns:        1,
			RemainingHP:  remHP,
			AlliesFallen: fallen,
			BonusPercent: bonusPercent,
			TotalReward: corebattle.Reward{
				Experience: 100 * (100 + bonusPercent) / 100,
				Currency:   100 * (100 + bonusPercent) / 100,
			},
		}, nil
	})

	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, engine)

	leader := corecharacter.Character{
		ID:    "mid-lead",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem1 := corecharacter.Character{
		ID:    "mid-mem1",
		Name:  "Mem1",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem2 := corecharacter.Character{
		ID:    "mid-mem2",
		Name:  "Mem2",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	mem3 := corecharacter.Character{
		ID:    "mid-mem3",
		Name:  "Mem3",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, mem1, mem2, mem3)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	expectedEXP := 650 + 600  // 1250
	expectedGold := 650 + 600 // 1250

	if res.TotalEXP != expectedEXP {
		t.Errorf("expected TotalEXP=%d, got %d", expectedEXP, res.TotalEXP)
	}
	if res.TotalGold != expectedGold {
		t.Errorf("expected TotalGold=%d, got %d", expectedGold, res.TotalGold)
	}

	// Verify fallen member survives with 1 HP
	updatedMem3, err := charRepo.FindByID(ctx, mem3.ID)
	if err != nil {
		t.Fatalf("FindByID for fallen member failed: %v", err)
	}
	if updatedMem3.Stats.HP != 1 {
		t.Errorf("expected fallen member HP=1, got %d", updatedMem3.Stats.HP)
	}
}

type countingPartyBattleEngine struct {
	fixedBattleEngine
	calls int
}

func (e *countingPartyBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls++
	return e.fixedBattleEngine.ResolvePartyBattle(req)
}

type mutablePartyBlessingProvider struct {
	blessings map[string]string
	err       error
}

func (m *mutablePartyBlessingProvider) GetActiveBlessing(_ context.Context, characterID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.blessings == nil {
		return "", nil
	}
	return m.blessings[characterID], nil
}

func TestPartyAdventure_TreasureBlessing_ReadFailure(t *testing.T) {
	ctx := context.Background()

	leader := corecharacter.Character{
		ID:       "lead-fail",
		Name:     "Leader",
		Level:    10,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 30, MaxMP: 30, Attack: 50, Defense: 30},
	}
	member := corecharacter.Character{
		ID:       "mem-fail",
		Name:     "Member",
		Level:    10,
		JobLevel: 5,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 30, MaxMP: 30, Attack: 50, Defense: 30},
	}

	blessingProv := &mutablePartyBlessingProvider{}
	battleEngine := &countingPartyBattleEngine{
		fixedBattleEngine: fixedBattleEngine{outcome: corebattle.OutcomeWin},
	}

	svc, partyRepo, charRepo, invRepo, depotRepo := setupPartyAdventureTestService(
		t,
		battleEngine,
		WithBlessingProvider(blessingProv),
	)
	svc.rng = func(n int) int { return 0 } // Lucky chest blessing roll

	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBlessingProvider(blessingProv),
		battle.WithRandomSource(fixedPartyBattleRand{val: 0}),
		battle.WithMaxInventoryCapacity(10),
	)
	svc.battleSettler = battleSvc

	partyID := createReadyParty(ctx, t, svc, partyRepo, charRepo, leader, member)

	// 1. Storage failure: provider returns error
	injectedErr := errors.New("blessing storage unavailable")
	blessingProv.err = injectedErr
	battleEngine.calls = 0

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected error wrapping %v, got %v", injectedErr, err)
	}
	if battleEngine.calls != 0 {
		t.Fatalf("expected 0 combat calls on blessing read failure, got %d", battleEngine.calls)
	}

	// Verify compensatory restoration: party is StatusRecruiting
	p, err := partyRepo.GetParty(ctx, partyID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusRecruiting {
		t.Fatalf("expected party status %s after error, got %s", StatusRecruiting, p.Status)
	}

	// Verify member ready states remain true
	members, err := partyRepo.GetMembers(ctx, partyID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if !m.ReadyState {
			t.Fatalf("expected member %s ready state to remain true, got false", m.CharacterID)
		}
	}

	// Verify no adventure logs written
	if len(partyRepo.logs) != 0 {
		t.Fatalf("expected 0 adventure logs on failure, got %d", len(partyRepo.logs))
	}

	// Verify characters unmutated
	savedLeader, _ := charRepo.FindByID(ctx, leader.ID)
	if savedLeader.Stats.HP != leader.Stats.HP || savedLeader.Money != leader.Money {
		t.Fatalf("leader character mutated: %+v vs %+v", savedLeader, leader)
	}

	// 2. Recovery: clearing the injected read failure permits one subsequent successful crawl (absent blessing: 2 chests)
	blessingProv.err = nil
	blessingProv.blessings = nil
	battleEngine.calls = 0

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("expected success after clearing blessing error, got: %v", err)
	}
	if battleEngine.calls == 0 {
		t.Fatal("expected combat execution on recovery")
	}
	if len(res.TreasureBoxes) != 2 {
		t.Fatalf("expected 2 treasure chests for absent blessing (2 members), got %d", len(res.TreasureBoxes))
	}
	if len(partyRepo.logs) != 1 {
		t.Fatalf("expected exactly 1 adventure log after recovery, got %d", len(partyRepo.logs))
	}

	// Verify member ready states are now reset to false on success
	membersAfter, err := partyRepo.GetMembers(ctx, partyID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range membersAfter {
		if m.ReadyState {
			t.Fatalf("expected member %s ready state reset to false after success, got true", m.CharacterID)
		}
	}

	// 3. Active blessing on leader: yields 3 chests (2 base + 1 blessing bonus)
	for _, m := range membersAfter {
		_, err = svc.SetReady(ctx, partyID, m.CharacterID, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	blessingProv.blessings = map[string]string{leader.ID: "4"}
	battleEngine.calls = 0

	resActive, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("expected success with active blessing, got: %v", err)
	}
	if len(resActive.TreasureBoxes) != 3 {
		t.Fatalf("expected 3 chests for active blessing (2 base + 1 bonus), got %d", len(resActive.TreasureBoxes))
	}
}

func TestPartyAdmission_MatchesStageCatalogAndAvailableStages(t *testing.T) {
	ctx := context.Background()
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	depotRepo := newMockDepotRepo()
	monsters, _ := adventure.InitialMonsterCatalog()

	svc, err := NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		fixedBattleEngine{outcome: corebattle.OutcomeWin},
		WithDepotRepository(depotRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	jst := time.FixedZone("JST", 9*60*60)

	testCases := []struct {
		name     string
		level    int
		jobLevel int
		time     time.Time
		stageID  string
	}{
		{"stage-02 with JobLevel 0", 20, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-02"},
		{"stage-03 with JobLevel 0 (rejected)", 20, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-03"},
		{"stage-03 with JobLevel 1 (accepted)", 20, 1, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-03"},
		{"stage-23 outside winter (rejected)", 99, 10, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-23"},
		{"stage-23 in winter (accepted)", 99, 10, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-23"},
		{"stage-25 in winter with JobLevel 9 (rejected)", 99, 9, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-25"},
		{"stage-25 in winter with JobLevel 10 (accepted)", 99, 10, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-25"},
		{"stage-27 outside halloween (rejected)", 99, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-27"},
		{"stage-27 in halloween (accepted)", 99, 0, time.Date(2026, 10, 15, 12, 0, 0, 0, jst), "stage-27"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			charID := fmt.Sprintf("char-%s", tc.stageID)
			char := corecharacter.Character{
				ID:       charID,
				Name:     "Leader",
				Level:    tc.level,
				JobLevel: tc.jobLevel,
				Stats: corecharacter.Stats{
					HP:    100,
					MaxHP: 100,
				},
			}
			charRepo.chars[char.ID] = char

			stages.SetNowFunc(func() time.Time { return tc.time })

			available := stages.AvailableStagesAt(char, tc.time)
			isAvailable := false
			for _, st := range available {
				if st.ID == tc.stageID {
					isAvailable = true
					break
				}
			}

			accessErr := stages.CanAccessStageAt(char, tc.stageID, tc.time)
			if (accessErr == nil) != isAvailable {
				t.Fatalf("AvailableStages and CanAccessStageAt mismatch: available=%v, accessErr=%v", isAvailable, accessErr)
			}

			_, createErr := svc.CreateParty(ctx, char.ID, CreatePartyRequest{
				Name:       fmt.Sprintf("Party-%s", tc.stageID),
				StageID:    tc.stageID,
				MaxMembers: 4,
			})

			if isAvailable {
				if createErr != nil {
					t.Fatalf("CreateParty unexpectedly failed for available stage %s: %v", tc.stageID, createErr)
				}
			} else {
				if !errors.Is(createErr, accessErr) {
					t.Fatalf("CreateParty error %v does not match expected access error %v", createErr, accessErr)
				}
			}
		})
	}
}

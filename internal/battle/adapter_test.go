package battle_test

import (
	"context"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// --- Mock Implementations ---

type mockCharRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
	lockOrder  []string
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(_ context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockOrder = append(m.lockOrder, "char:"+id)
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) Update(_ context.Context, character corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[character.ID] = character
	return nil
}

type mockInvRepo struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
	lockOrder   []string
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{inventories: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockOrder = append(m.lockOrder, "inv:"+characterID)
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInvRepo) Save(_ context.Context, inventory coreinventory.Inventory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inventories[inventory.CharacterID] = inventory
	return nil
}

type mockEquipRepo struct {
	mu         sync.Mutex
	equipments map[string]coreequipment.Equipment
}

func newMockEquipRepo() *mockEquipRepo {
	return &mockEquipRepo{equipments: make(map[string]coreequipment.Equipment)}
}

func (m *mockEquipRepo) FindByCharacterID(_ context.Context, characterID string) (coreequipment.Equipment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	eq, ok := m.equipments[characterID]
	if !ok {
		eq, _ = coreequipment.New(characterID)
	}
	return eq, nil
}

func (m *mockEquipRepo) Save(_ context.Context, value coreequipment.Equipment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.equipments[value.CharacterID] = value
	return nil
}

type mockDepotRepo struct {
	mu        sync.Mutex
	depots    map[string]depot.Depot
	lockOrder []string
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *mockDepotRepo) FindByCharacterID(_ context.Context, characterID string) (depot.Depot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (depot.Depot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockOrder = append(m.lockOrder, "depot:"+characterID)
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *mockDepotRepo) Save(_ context.Context, value depot.Depot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depots[value.CharacterID] = value
	return nil
}

type mockJobProvider map[string]job.Definition

func (m mockJobProvider) FindByID(id string) (job.Definition, error) {
	d, ok := m[id]
	if !ok {
		return job.Definition{}, job.ErrInvalidDefinition
	}
	return d, nil
}

type mockSkillProvider map[string][]skill.Definition

func (m mockSkillProvider) SkillsForJob(jobID string) []skill.Definition {
	return m[jobID]
}

type mockCustomSkillRepo map[string]*custom_skill.CustomSkill

func (m mockCustomSkillRepo) FindCustomSkill(_ context.Context, characterID string) (*custom_skill.CustomSkill, error) {
	return m[characterID], nil
}

type mockTxProvider struct {
	mu sync.Mutex
}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(ctx)
}

type fixedRNG struct {
	val int
}

func (f fixedRNG) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	return f.val % n, nil
}

// --- Unit Tests ---

func TestBuildParticipant(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	char := corecharacter.Character{
		ID:    "char-hero",
		Name:  "勇者アレル",
		JobID: "job-pharaoh",
		SP:    50,
		Stats: corecharacter.Stats{
			HP:      0, // Zero current HP -> should fallback to MaxHP
			MaxHP:   120,
			MP:      40,
			MaxMP:   40,
			Attack:  35,
			Defense: 25,
			Agility: 18,
		},
	}
	_ = charRepo.Update(ctx, char)

	// Inventory with items:
	// - 闘気の盾 (item-161, UsageCategory 3)
	// - ドクロのお守り (item-193, UsageCategory 3)
	// - 転生の呪魂符 (item-260, UsageCategory 3)
	// - 薬草 (item-001, UsageCategory 1)
	// - 祈りの指輪 (item-012, UsageCategory 1, equipped)
	inv, _ := coreinventory.New("char-hero")
	tShield, _ := coreitem.NewInstance(battle.ItemToukiShield, 1)
	dAmulet, _ := coreitem.NewInstance(battle.ItemDokuroAmulet, 1)
	cTalisman, _ := coreitem.NewInstance(battle.ItemCursedTalisman, 1)
	herb, _ := coreitem.NewInstance("item-001", 3)
	pRing, _ := coreitem.NewInstance(battle.ItemPrayerRing, 1)

	_ = inv.Add(tShield)
	_ = inv.Add(dAmulet)
	_ = inv.Add(cTalisman)
	_ = inv.Add(herb)
	_ = inv.Add(pRing)
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New("char-hero")
	equip.Slots[coreitem.SlotAccessory] = pRing.ID
	_ = equipRepo.Save(ctx, equip)

	skillDef, _ := skill.NewDefinition(
		"sk-01", "ギガデイン", []string{"job-pharaoh"}, 20, 15,
		corebattle.Effect{Kind: "attack", Power: 80},
	)
	skillProv := mockSkillProvider{"job-pharaoh": []skill.Definition{skillDef}}

	csRepo := mockCustomSkillRepo{
		"char-hero": &custom_skill.CustomSkill{
			CharacterID: "char-hero",
			Name:        "真・奥義天翔",
			Comment:     "燃え盛れ我が闘気！",
			CMP:         25,
		},
	}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithSkillProvider(skillProv),
		battle.WithCustomSkillRepository(csRepo),
	)

	part, err := svc.BuildParticipant(ctx, "char-hero")
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}

	// 1. HP fallback validation
	if part.HP != 120 {
		t.Errorf("part.HP = %d, want 120 (fallback to MaxHP)", part.HP)
	}
	if part.Attack != 35 || part.Defense != 25 || part.Agility != 18 {
		t.Errorf("part stats mismatch: atk=%d, def=%d, agi=%d", part.Attack, part.Defense, part.Agility)
	}

	// 2. Abilities validation
	hasTouki := false
	hasDokuro := false
	hasCursed := false
	hasPharaoh := false
	for _, ab := range part.Abilities {
		switch ab {
		case "touki_shield":
			hasTouki = true
		case "dokuro_amulet":
			hasDokuro = true
		case "cursed_revive":
			hasCursed = true
		case "pharaoh":
			hasPharaoh = true
		}
	}
	if !hasTouki || !hasDokuro || !hasCursed || !hasPharaoh {
		t.Errorf("abilities missing: touki=%v, dokuro=%v, cursed=%v, pharaoh=%v (got: %v)", hasTouki, hasDokuro, hasCursed, hasPharaoh, part.Abilities)
	}

	// 3. Action items validation (Category 1: 薬草 & 祈りの指輪)
	foundHerb := false
	foundRing := false
	for _, act := range part.ActionItems {
		if act.ID == "item-001" {
			foundHerb = true
		}
		if act.ID == battle.ItemPrayerRing {
			foundRing = true
			if act.InstanceID != pRing.ID {
				t.Errorf("prayer ring InstanceID = %s, want %s", act.InstanceID, pRing.ID)
			}
		}
	}
	if !foundHerb || !foundRing {
		t.Errorf("expected action items 薬草 and 祈りの指輪, got: %+v", part.ActionItems)
	}

	// 4. Skills & Custom Skills validation
	if len(part.Skills) != 1 || part.Skills[0].ID != "sk-01" {
		t.Errorf("skills mismatch: %+v", part.Skills)
	}
	if len(part.CustomSkills) != 1 || part.CustomSkills[0].Name != "真・奥義天翔" {
		t.Errorf("custom skills mismatch: %+v", part.CustomSkills)
	}
}

func TestExtractStatOrbOptions(t *testing.T) {
	inv, _ := coreinventory.New("char-1")
	lifeOrb, _ := coreitem.NewInstance(progression.ItemLifeStatOrb, 1)
	powerOrb, _ := coreitem.NewInstance(progression.ItemPowerStatOrb, 1)
	skillOrb, _ := coreitem.NewInstance(battle.ItemSkillOrb, 1)

	_ = inv.Add(lifeOrb)
	_ = inv.Add(powerOrb)
	_ = inv.Add(skillOrb)

	opts := battle.ExtractStatOrbOptions(inv)
	if !opts.HasSkillOrb {
		t.Errorf("expected HasSkillOrb = true")
	}
	if !opts.StatOrbItems[progression.ItemLifeStatOrb] {
		t.Errorf("expected ItemLifeStatOrb = true")
	}
	if !opts.StatOrbItems[progression.ItemPowerStatOrb] {
		t.Errorf("expected ItemPowerStatOrb = true")
	}
	if opts.StatOrbItems[progression.ItemMagicStatOrb] {
		t.Errorf("expected ItemMagicStatOrb = false")
	}
}

func TestApplyPostBattleResult_SingleCharacterWithRunner(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()

	char := corecharacter.Character{
		ID:         "char-1",
		Name:       "ソロ勇者",
		JobID:      "job-warrior",
		Money:      1000,
		Level:      1,
		Experience: 0,
		JobLevel:   1,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			MP:      50,
			MaxMP:   50,
			Attack:  20,
			Defense: 10,
		},
	}
	_ = charRepo.Update(ctx, char)

	// Inventory has 1 potion and 1 prayer ring equipped
	inv, _ := coreinventory.New("char-1")
	potion, _ := coreitem.NewInstance("item-001", 2)
	pRing, _ := coreitem.NewInstance(battle.ItemPrayerRing, 1)
	_ = inv.Add(potion)
	_ = inv.Add(pRing)
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New("char-1")
	equip.Slots[coreitem.SlotAccessory] = pRing.ID
	_ = equipRepo.Save(ctx, equip)

	jobDef, _ := job.NewDefinition("job-warrior", "戦士", 10, 5, 3, 3, 2, 1, "unspecified")
	jobProv := mockJobProvider{"job-warrior": jobDef}

	txProv := &mockTxProvider{}
	txRunner, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithJobDefinitionProvider(jobProv),
		battle.WithTransactionRunner(txRunner),
		battle.WithRandomSource(fixedRNG{val: 2}),
		battle.WithMaxInventoryCapacity(2), // limit to 2 slots -> 3rd item goes to depot
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-1": 65,
		},
		RemainingMP: map[string]int{
			"char-1": 40,
		},
		ConsumedItems: map[string][]corebattle.ConsumedItem{
			"char-1": {
				{ID: "item-001", InstanceID: potion.ID, Quantity: 1},
				{ID: battle.ItemPrayerRing, InstanceID: pRing.ID, Quantity: 1},
			},
		},
		TotalReward: corebattle.Reward{
			Experience:       50,
			Currency:         300,
			ItemDefinitionID: "item-drop-sword",
			ItemQuantity:     1,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-1"},
		BattleResult: battleRes,
		DropItems:    []string{"item-drop-shield"}, // second drop will exceed capacity (2) -> routes to depot
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// 1. Verify character resources & stats
	savedChar, _ := charRepo.FindByID(ctx, "char-1")
	if savedChar.Stats.HP != 65 {
		t.Errorf("savedChar HP = %d, want 65", savedChar.Stats.HP)
	}
	if savedChar.Stats.MP != 40 {
		t.Errorf("savedChar MP = %d, want 40", savedChar.Stats.MP)
	}
	if savedChar.Money != 1300 {
		t.Errorf("savedChar Money = %d, want 1300", savedChar.Money)
	}
	if savedChar.Experience != 50 {
		t.Errorf("savedChar Experience = %d, want 50", savedChar.Experience)
	}

	// 2. Verify consumed items from inventory
	savedInv, _ := invRepo.FindByCharacterID(ctx, "char-1")
	potionInst, found := savedInv.Find(potion.ID)
	if !found || potionInst.Quantity != 1 {
		t.Errorf("expected 1 remaining potion, got found=%v, qty=%d", found, potionInst.Quantity)
	}
	_, ringFound := savedInv.Find(pRing.ID)
	if ringFound {
		t.Errorf("expected prayer ring to be consumed and removed from inventory")
	}

	// 3. Verify prayer ring unequipped
	savedEquip, _ := equipRepo.FindByCharacterID(ctx, "char-1")
	if _, equipped := savedEquip.Equipped(coreitem.SlotAccessory); equipped {
		t.Errorf("expected prayer ring to be unequipped from accessory slot")
	}

	// 4. Verify drops routing: one to inventory, one to depot
	if len(resp.InventoryDrops["char-1"]) != 1 {
		t.Errorf("expected 1 inventory drop, got %d", len(resp.InventoryDrops["char-1"]))
	}
	if len(resp.DepotDeliveries["char-1"]) != 1 {
		t.Errorf("expected 1 depot delivery, got %d", len(resp.DepotDeliveries["char-1"]))
	}

	savedDepot, err := depotRepo.FindByCharacterID(ctx, "char-1")
	if err != nil {
		t.Fatalf("depot not found: %v", err)
	}
	if len(savedDepot.Items) != 1 || savedDepot.Items[0].DefinitionID != "item-drop-shield" {
		t.Errorf("depot items mismatch: %+v", savedDepot.Items)
	}
}

func TestApplyPostBattleResult_MultiCharacterPartyWithProvider(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	txProv := &mockTxProvider{}

	charIDs := []string{"char-gamma", "char-alpha", "char-beta"}
	for _, id := range charIDs {
		c := corecharacter.Character{
			ID:       id,
			Name:     id,
			Money:    500,
			Level:    1,
			JobLevel: 1,
			Stats: corecharacter.Stats{
				HP:    100,
				MaxHP: 100,
				MP:    30,
				MaxMP: 30,
			},
		}
		_ = charRepo.Update(ctx, c)

		inv, _ := coreinventory.New(id)
		herb, _ := coreitem.NewInstance("item-001", 1)
		_ = inv.Add(herb)
		_ = invRepo.Save(ctx, inv)
	}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithMaxInventoryCapacity(1), // inventory already has 1 item -> drops route to depot
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-alpha": 80,
			"char-beta":  0, // Fallen member -> survives with HP 1
			"char-gamma": 45,
		},
		RemainingMP: map[string]int{
			"char-alpha": 25,
			"char-beta":  10,
			"char-gamma": 20,
		},
		ConsumedItems: map[string][]corebattle.ConsumedItem{
			"char-alpha": {{ID: "item-001", Quantity: 1}},
		},
		TotalReward: corebattle.Reward{
			Experience:       100,
			Currency:         600,
			ItemDefinitionID: "item-drop-rare",
			ItemQuantity:     1,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: charIDs,
		BattleResult: battleRes,
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// 1. Verify lock order: must be lexicographically ascending for Rank 2: char-alpha, char-beta, char-gamma
	expectedCharLocks := []string{"char:char-alpha", "char:char-beta", "char:char-gamma"}
	if len(charRepo.lockOrder) != 3 {
		t.Fatalf("expected 3 char locks, got %d: %v", len(charRepo.lockOrder), charRepo.lockOrder)
	}
	for i := range expectedCharLocks {
		if charRepo.lockOrder[i] != expectedCharLocks[i] {
			t.Errorf("char lock order[%d] = %s, want %s", i, charRepo.lockOrder[i], expectedCharLocks[i])
		}
	}

	// 2. Verify fallen member (char-beta) survives with HP = 1
	cBeta, _ := charRepo.FindByID(ctx, "char-beta")
	if cBeta.Stats.HP != 1 {
		t.Errorf("cBeta HP = %d, want 1", cBeta.Stats.HP)
	}
	if cBeta.Money != 1100 { // 500 + 600
		t.Errorf("cBeta Money = %d, want 1100", cBeta.Money)
	}

	// 3. Verify surviving member (char-alpha) HP = 80, consumed herb and received item-drop-rare into inventory
	cAlpha, _ := charRepo.FindByID(ctx, "char-alpha")
	if cAlpha.Stats.HP != 80 {
		t.Errorf("cAlpha HP = %d, want 80", cAlpha.Stats.HP)
	}
	invAlpha, _ := invRepo.FindByCharacterID(ctx, "char-alpha")
	if len(invAlpha.Items) != 1 || invAlpha.Items[0].DefinitionID != "item-drop-rare" {
		t.Errorf("expected char-alpha inventory to have item-drop-rare, got: %+v", invAlpha.Items)
	}

	// 4. Verify depot deliveries occurred for characters whose inventory remained full (char-beta, char-gamma)
	for _, id := range []string{"char-beta", "char-gamma"} {
		dep, err := depotRepo.FindByCharacterID(ctx, id)
		if err != nil {
			t.Errorf("depot not created for %s: %v", id, err)
			continue
		}
		if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-drop-rare" {
			t.Errorf("expected item-drop-rare in %s depot, got: %+v", id, dep.Items)
		}
	}

	if len(resp.UpdatedCharacters) != 3 {
		t.Errorf("expected 3 updated characters in response, got %d", len(resp.UpdatedCharacters))
	}
}

func TestApplyPostBattleResult_DeadlockFreeConcurrency(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	txProv := &mockTxProvider{}

	for i := 1; i <= 6; i++ {
		id := "hero-" + string(rune('A'+i-1))
		c := corecharacter.Character{
			ID:       id,
			Name:     id,
			Money:    1000,
			Level:    1,
			JobLevel: 1,
			Stats: corecharacter.Stats{
				HP:    100,
				MaxHP: 100,
			},
		}
		_ = charRepo.Update(ctx, c)
		inv, _ := coreinventory.New(id)
		_ = invRepo.Save(ctx, inv)
	}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
	)

	// Run concurrent calls with reversed character ID orders to ensure strict sorting prevents deadlock
	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var ids []string
			if idx%2 == 0 {
				ids = []string{"hero-D", "hero-B", "hero-A"}
			} else {
				ids = []string{"hero-A", "hero-C", "hero-B"}
			}

			req := battle.ApplyPostBattleRequest{
				CharacterIDs: ids,
				BattleResult: corebattle.PartyBattleResult{
					Outcome: corebattle.OutcomeWin,
					TotalReward: corebattle.Reward{
						Currency: 50,
					},
				},
			}
			if _, err := svc.ApplyPostBattleResult(ctx, req); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent battle result application encountered error: %v", err)
	}
}

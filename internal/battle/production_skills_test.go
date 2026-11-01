package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func newTestProductionBattleService(charRepo *mockCharRepo) *battle.Service {
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	return battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(corebattle.Engine{}),
		battle.WithSkillProvider(skill.InitialCatalog()),
		battle.WithBattleDefinitions(skill.ProductionBattleDefinitions()),
	)
}

func TestProductionSkillWiringAndGates(t *testing.T) {
	ctx := context.Background()

	// 1. Current Job SP gates (before, equal to, after)
	t.Run("current job SP gates", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		// job-05 heal-30 (SP 6)
		for sp, wantCount := range map[int]int{5: 0, 6: 1, 7: 1} {
			charID := "hero-priest"
			charRepo.characters[charID] = corecharacter.Character{
				ID:    charID,
				Name:  "Priest",
				JobID: "job-05",
				SP:    sp,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
			}
			p, err := svc.BuildParticipant(ctx, charID)
			if err != nil {
				t.Fatalf("BuildParticipant(SP=%d) error = %v", sp, err)
			}
			if len(p.Skills) != wantCount {
				t.Fatalf("SP=%d: expected %d skills, got %d (%v)", sp, wantCount, len(p.Skills), p.Skills)
			}
			if wantCount == 1 {
				sk := p.Skills[0]
				if sk.ID != "job-5-entry-3" || sk.Name != "heal-30" || sk.Kind != corebattle.ActionKindHeal ||
					sk.Power != 30 || sk.MPCost != 3 || sk.TargetScope != corebattle.TargetScopeSingleAlly {
					t.Fatalf("unexpected skill payload: %+v", sk)
				}
			}
		}

		// job-06 magic-20 (SP 1) and magic-wave-25 (SP 8)
		for sp, wantIDs := range map[int][]string{
			0: nil,
			1: {"job-6-entry-1"},
			7: {"job-6-entry-1"},
			8: {"job-6-entry-3", "job-6-entry-1"},
			9: {"job-6-entry-3", "job-6-entry-1"},
		} {
			charID := "hero-mage"
			charRepo.characters[charID] = corecharacter.Character{
				ID:    charID,
				Name:  "Mage",
				JobID: "job-06",
				SP:    sp,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
			}
			p, err := svc.BuildParticipant(ctx, charID)
			if err != nil {
				t.Fatalf("BuildParticipant(SP=%d) error = %v", sp, err)
			}
			if len(p.Skills) != len(wantIDs) {
				t.Fatalf("SP=%d: expected %d skills, got %d (%v)", sp, len(wantIDs), len(p.Skills), p.Skills)
			}
			for i, wantID := range wantIDs {
				if p.Skills[i].ID != wantID {
					t.Fatalf("SP=%d: skill[%d] = %s, want %s", sp, i, p.Skills[i].ID, wantID)
				}
			}
		}
	})

	// 2. Previous Job SP gates: current SP must NEVER substitute for previous SP
	t.Run("previous job SP gates and isolation", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		// Character is currently job-01 (warrior) with high current SP (100)
		// Old job is job-05 (priest, heal-30 req SP 6)
		for oldSP, wantCount := range map[int]int{5: 0, 6: 1, 7: 1} {
			charID := "hero-ex-priest"
			charRepo.characters[charID] = corecharacter.Character{
				ID:       charID,
				Name:     "ExPriest",
				JobID:    "job-01",
				SP:       100, // current SP 100 must not unlock previous job-05 skill!
				OldJobID: "job-05",
				OldSP:    oldSP,
				Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
			}
			p, err := svc.BuildParticipant(ctx, charID)
			if err != nil {
				t.Fatalf("BuildParticipant(OldSP=%d) error = %v", oldSP, err)
			}
			if len(p.Skills) != wantCount {
				t.Fatalf("OldSP=%d: expected %d skills, got %d (%v)", oldSP, wantCount, len(p.Skills), p.Skills)
			}
			if wantCount == 1 && p.Skills[0].ID != "job-5-entry-3" {
				t.Fatalf("expected job-5-entry-3, got %s", p.Skills[0].ID)
			}
		}

		// Old job is job-06 (mage) with OldSP 7 (magic-20 unlocked, magic-wave-25 locked)
		charRepo.characters["ex-mage-7"] = corecharacter.Character{
			ID:       "ex-mage-7",
			JobID:    "job-01",
			SP:       50,
			OldJobID: "job-06",
			OldSP:    7,
			Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
		}
		p, err := svc.BuildParticipant(ctx, "ex-mage-7")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Skills) != 1 || p.Skills[0].ID != "job-6-entry-1" {
			t.Fatalf("expected [job-6-entry-1], got %v", p.Skills)
		}
	})

	// 3. Duplicate candidate deduplication (重複候補)
	t.Run("duplicate candidates deduplication", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		// Character changed job from job-05 to job-05, SP 10 and OldSP 10
		charRepo.characters["dup-priest"] = corecharacter.Character{
			ID:       "dup-priest",
			Name:     "DupPriest",
			JobID:    "job-05",
			SP:       10,
			OldJobID: "job-05",
			OldSP:    10,
			Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
		}
		p, err := svc.BuildParticipant(ctx, "dup-priest")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Skills) != 1 {
			t.Fatalf("expected exactly 1 skill due to deduplication, got %d (%v)", len(p.Skills), p.Skills)
		}
		if p.Skills[0].ID != "job-5-entry-3" {
			t.Fatalf("expected job-5-entry-3, got %s", p.Skills[0].ID)
		}
	})

	// 4. Combined current and previous job skills order
	t.Run("combined current and previous skills in order", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		// Current job-06 (SP 10 -> magic-20, magic-wave-25), Old job-05 (OldSP 10 -> heal-30)
		charRepo.characters["mage-priest"] = corecharacter.Character{
			ID:       "mage-priest",
			Name:     "MagePriest",
			JobID:    "job-06",
			SP:       10,
			OldJobID: "job-05",
			OldSP:    10,
			Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10},
		}
		p, err := svc.BuildParticipant(ctx, "mage-priest")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Skills) != 3 {
			t.Fatalf("expected 3 skills, got %d (%v)", len(p.Skills), p.Skills)
		}
		wantOrder := []string{"job-6-entry-3", "job-6-entry-1", "job-5-entry-3"}
		for i, id := range wantOrder {
			if p.Skills[i].ID != id {
				t.Errorf("skill[%d] = %s, want %s", i, p.Skills[i].ID, id)
			}
		}
	})
}

type deterministicBattleRNG struct {
	fVal float64
	iVal int
}

func (r deterministicBattleRNG) Intn(n int) int   { return r.iVal }
func (r deterministicBattleRNG) IntN(n int) int   { return r.iVal }
func (r deterministicBattleRNG) Float64() float64 { return r.fVal }

func newTestProductionBattleServiceWithInv(charRepo *mockCharRepo, invRepo *mockInvRepo) *battle.Service {
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	return battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(corebattle.Engine{}),
		battle.WithSkillProvider(skill.InitialCatalog()),
		battle.WithBattleDefinitions(skill.ProductionBattleDefinitions()),
	)
}

func TestProductionBattleExecution(t *testing.T) {
	ctx := context.Background()

	// 1. Single-ally heal (job-5-entry-3: heal-30, MPCost 3, TargetScopeSingleAlly)
	t.Run("heal-30 single-ally execution and legacy formulas", func(t *testing.T) {
		// 1a. Base healing with fixed variance (1.0)
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		charRepo.characters["priest"] = corecharacter.Character{
			ID:    "priest",
			Name:  "Priest",
			JobID: "job-05",
			SP:    10,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 10, Attack: 5, Defense: 50, Agility: 50},
		}
		priest, err := svc.BuildParticipant(ctx, "priest")
		if err != nil {
			t.Fatal(err)
		}

		ally := corebattle.Participant{
			ID:    "damaged-ally",
			Name:  "Wounded",
			HP:    50,
			MaxHP: 100,
		}
		enemy := corebattle.Participant{
			ID:        "target-enemy",
			Name:      "Dummy",
			HP:        1000,
			MaxHP:     1000,
			Attack:    0,
			Defending: true,
		}

		engine := corebattle.Engine{}
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priest, ally},
			Enemies: []corebattle.Participant{enemy},
			RNG:     deterministicBattleRNG{fVal: 1.0 / 3.0}, // variance 0.9 + 0.3 * (1/3) = 1.0
		})
		if err != nil {
			t.Fatal(err)
		}

		foundHeal := false
		for _, log := range res.Logs {
			if log.ActionName == "heal-30" && log.TargetID == ally.ID {
				foundHeal = true
				if log.HealingDone != 30 {
					t.Errorf("expected 30 healing, got %d", log.HealingDone)
				}
				if log.RemainingHP[ally.ID] != 80 {
					t.Errorf("expected ally HP 80, got %d", log.RemainingHP[ally.ID])
				}
				break
			}
		}
		if !foundHeal {
			t.Fatalf("heal-30 action was not executed in battle logs: %+v", res.Logs)
		}

		// 1b. Variance effect (fVal = 0.0 -> variance 0.9 -> 27 HP heal)
		resLow, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priest, ally},
			Enemies: []corebattle.Participant{enemy},
			RNG:     deterministicBattleRNG{fVal: 0.0},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, log := range resLow.Logs {
			if log.ActionName == "heal-30" && log.TargetID == ally.ID {
				if log.HealingDone != 27 {
					t.Errorf("expected 27 healing (0.9x), got %d", log.HealingDone)
				}
				break
			}
		}

		// 1c. Tension effect (initial tension 1.7 -> 30 * 1.7 = 51 HP heal)
		resTen, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{priest, ally},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{"priest": 1.7},
			RNG:            deterministicBattleRNG{fVal: 1.0 / 3.0},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, log := range resTen.Logs {
			if log.ActionName == "heal-30" && log.TargetID == ally.ID {
				if log.HealingDone != 51 {
					t.Errorf("expected 51 healing (1.7 tension), got %d", log.HealingDone)
				}
				break
			}
		}

		// 1d. Item-146 (ヒーリングストーン: 1.8x multiplier -> 30 * 1.8 = 54 HP heal)
		invRepo.inventories["priest"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-146"}},
		}
		priestWithStone, err := svc.BuildParticipant(ctx, "priest")
		if err != nil {
			t.Fatal(err)
		}
		resStone, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priestWithStone, ally},
			Enemies: []corebattle.Participant{enemy},
			RNG:     deterministicBattleRNG{fVal: 1.0 / 3.0},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, log := range resStone.Logs {
			if log.ActionName == "heal-30" && log.TargetID == ally.ID {
				if log.HealingDone != 54 {
					t.Errorf("expected 54 healing (1.8x stone), got %d", log.HealingDone)
				}
				break
			}
		}

		// 1e. Item-256 (1.2x multiplier -> 36 HP heal + 50% splash (18 HP) to other living ally)
		invRepo.inventories["priest"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-256"}},
		}
		priestWith256, err := svc.BuildParticipant(ctx, "priest")
		if err != nil {
			t.Fatal(err)
		}
		ally2 := corebattle.Participant{ID: "ally-2", Name: "Ally2", HP: 40, MaxHP: 100}
		res256, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priestWith256, ally, ally2},
			Enemies: []corebattle.Participant{enemy},
			RNG:     deterministicBattleRNG{fVal: 1.0 / 3.0},
		})
		if err != nil {
			t.Fatal(err)
		}
		target1Healed, target2Splashed := false, false
		for _, log := range res256.Logs {
			if log.ActionName == "heal-30" {
				if log.TargetID == ally.ID && log.HealingDone == 36 {
					target1Healed = true
				}
				if log.TargetID == ally2.ID && log.HealingDone == 18 {
					target2Splashed = true
				}
			}
		}
		if !target1Healed || !target2Splashed {
			t.Errorf("expected 36 primary heal and 18 splash heal: primary=%v, splash=%v", target1Healed, target2Splashed)
		}
	})

	// 2. Single-enemy magic attack (job-6-entry-1: magic-20, MPCost 2, TargetScopeSingleEnemy)
	t.Run("magic-20 legacy formula parity", func(t *testing.T) {
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		// 2a. Fixed damage 20 (variance 1.0): physical attack must NOT be added
		charRepo.characters["mage-weak"] = corecharacter.Character{
			ID:    "mage-weak",
			Name:  "MageWeak",
			JobID: "job-06",
			SP:    5, // unlocks magic-20 only
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 10, Attack: 0, Defense: 10, Agility: 50},
		}
		charRepo.characters["mage-strong"] = corecharacter.Character{
			ID:    "mage-strong",
			Name:  "MageStrong",
			JobID: "job-06",
			SP:    5,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 10, Attack: 999, Defense: 10, Agility: 50},
		}

		pWeak, _ := svc.BuildParticipant(ctx, "mage-weak")
		pStrong, _ := svc.BuildParticipant(ctx, "mage-strong")

		enemyLowDef := corebattle.Participant{ID: "e-low", HP: 200, MaxHP: 200, Defense: 0, Attack: 0}
		enemyHighDef := corebattle.Participant{ID: "e-high", HP: 200, MaxHP: 200, Defense: 500, Attack: 0}

		engine := corebattle.Engine{}
		detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}

		// Verify weak mage vs low def enemy: exactly 20 damage
		res1, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{pWeak}, Enemies: []corebattle.Participant{enemyLowDef}, RNG: detRNG,
		})
		for _, log := range res1.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 20 {
					t.Errorf("weak mage damage = %d, want 20", log.DamageDealt)
				}
				break
			}
		}

		// Verify strong mage (Atk 999) vs high def enemy (Def 500): STILL exactly 20 damage!
		res2, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{pStrong}, Enemies: []corebattle.Participant{enemyHighDef}, RNG: detRNG,
		})
		for _, log := range res2.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 20 {
					t.Errorf("strong mage vs high def damage = %d, want 20 (bypasses defense, no attack scaling)", log.DamageDealt)
				}
				break
			}
		}

		// 2b. Defending enemy: halves damage (20 / 2 = 10)
		enemyDefending := corebattle.Participant{ID: "e-def", HP: 200, MaxHP: 200, Defense: 10, Attack: 0, Defending: true}
		resDef, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{pWeak}, Enemies: []corebattle.Participant{enemyDefending}, RNG: detRNG,
		})
		for _, log := range resDef.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 10 {
					t.Errorf("defending enemy damage = %d, want 10", log.DamageDealt)
				}
				break
			}
		}

		// 2c. Magic immunity (魔無効 blocks, 攻無効 does not block)
		enemyMagicImmune := corebattle.Participant{ID: "e-m-imm", HP: 200, MaxHP: 200, Abilities: []string{"魔無効"}}
		resMImm, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{pWeak}, Enemies: []corebattle.Participant{enemyMagicImmune}, RNG: detRNG,
		})
		foundImmuneMsg := false
		for _, log := range resMImm.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 0 {
					t.Errorf("magic immune enemy damage = %d, want 0", log.DamageDealt)
				}
				foundImmuneMsg = true
				break
			}
		}
		if !foundImmuneMsg {
			t.Error("expected magic immune log message")
		}

		enemyPhysImmune := corebattle.Participant{ID: "e-p-imm", HP: 200, MaxHP: 200, Abilities: []string{"攻無効"}}
		resPImm, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{pWeak}, Enemies: []corebattle.Participant{enemyPhysImmune}, RNG: detRNG,
		})
		for _, log := range resPImm.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 20 {
					t.Errorf("phys immune enemy damage from magic = %d, want 20", log.DamageDealt)
				}
				break
			}
		}

		// 2d. Tension effect (initial tension 1.7 -> 20 * 1.7 = 34 damage)
		resTen, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{pWeak},
			Enemies:        []corebattle.Participant{enemyLowDef},
			InitialTension: map[string]float64{"mage-weak": 1.7},
			RNG:            detRNG,
		})
		for _, log := range resTen.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 34 {
					t.Errorf("tension 1.7 magic-20 damage = %d, want 34", log.DamageDealt)
				}
				break
			}
		}

		// 2e. Item-143 (魔性の首飾り: 2x multiplier for base < 300 -> 20 * 2 = 40 damage)
		invRepo.inventories["mage-weak"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-143"}},
		}
		pWith143, _ := svc.BuildParticipant(ctx, "mage-weak")
		res143, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{pWith143},
			Enemies: []corebattle.Participant{enemyLowDef},
			RNG:     detRNG,
		})
		for _, log := range res143.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 40 {
					t.Errorf("item-143 magic-20 damage = %d, want 40", log.DamageDealt)
				}
				break
			}
		}
	})

	// 3. All-enemies magic attack (job-6-entry-3: magic-wave-25, MPCost 5, TargetScopeAllEnemies)
	// and production catalog skill selection paths
	t.Run("magic-wave-25 production catalog selection and execution", func(t *testing.T) {
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		// Mage unlocks BOTH magic-20 (SP 1) and magic-wave-25 (SP 8)
		charRepo.characters["mage-full"] = corecharacter.Character{
			ID:    "mage-full",
			Name:  "MageFull",
			JobID: "job-06",
			SP:    10,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 20, Defense: 50, Agility: 50},
		}
		mageFull, err := svc.BuildParticipant(ctx, "mage-full")
		if err != nil {
			t.Fatal(err)
		}
		if len(mageFull.Skills) != 2 {
			t.Fatalf("expected 2 skills in production catalog for Mage SP 10, got %d", len(mageFull.Skills))
		}

		engine := corebattle.Engine{}
		detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}

		// 3a. Mage with SP 5 (< 8) only unlocks magic-20 -> executes single-target magic-20 from catalog
		enemy1 := corebattle.Participant{ID: "e1", HP: 200, MaxHP: 200, Defense: 10, Attack: 0}
		charRepo.characters["mage-sp5"] = corecharacter.Character{
			ID:    "mage-sp5",
			Name:  "MageSP5",
			JobID: "job-06",
			SP:    5,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 20, Defense: 50, Agility: 50},
		}
		mageSP5, err := svc.BuildParticipant(ctx, "mage-sp5")
		if err != nil {
			t.Fatal(err)
		}
		resSingle, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageSP5},
			Enemies: []corebattle.Participant{enemy1},
			RNG:     detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}
		foundAction := ""
		for _, log := range resSingle.Logs {
			if log.ActorID == mageSP5.ID {
				foundAction = log.ActionName
				break
			}
		}
		if foundAction != "magic-20" {
			t.Fatalf("Mage SP 5: expected action magic-20, got %s", foundAction)
		}

		// 3b. Facing 2 enemies with MP >= 5: should choose all-enemies magic-wave-25
		enemy2 := corebattle.Participant{ID: "e2", HP: 200, MaxHP: 200, Defense: 10, Attack: 0}
		resMulti, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageFull},
			Enemies: []corebattle.Participant{enemy1, enemy2},
			RNG:     detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}
		dmgE1, dmgE2 := -1, -1
		for _, log := range resMulti.Logs {
			if log.ActionName == "magic-wave-25" {
				if log.TargetID == "e1" {
					dmgE1 = log.DamageDealt
				}
				if log.TargetID == "e2" {
					dmgE2 = log.DamageDealt
				}
			}
		}
		if dmgE1 < 0 || dmgE2 < 0 {
			t.Fatalf("magic-wave-25 did not hit all enemies: dmgE1=%d, dmgE2=%d, logs=%+v", dmgE1, dmgE2, resMulti.Logs)
		}
		// First target gets full 25, second target gets 15% decay (25 * 0.85 = 21)
		if dmgE1 != 25 {
			t.Errorf("magic-wave-25 target 1 damage = %d, want 25", dmgE1)
		}
		if dmgE2 != 21 {
			t.Errorf("magic-wave-25 target 2 damage = %d, want 21 (0.85 decay)", dmgE2)
		}

		// 3c. Facing 2 enemies but having only 3 MP (< 5 for magic-wave-25, but >= 2 for magic-20)
		charRepo.characters["mage-low-mp"] = corecharacter.Character{
			ID:    "mage-low-mp",
			Name:  "MageLowMP",
			JobID: "job-06",
			SP:    10,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 3, Attack: 20, Defense: 50, Agility: 50},
		}
		mageLowMP, _ := svc.BuildParticipant(ctx, "mage-low-mp")
		resLowMP, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageLowMP},
			Enemies: []corebattle.Participant{enemy1, enemy2},
			RNG:     detRNG,
		})
		foundLowMPAction := ""
		for _, log := range resLowMP.Logs {
			if log.ActorID == mageLowMP.ID {
				foundLowMPAction = log.ActionName
				break
			}
		}
		if foundLowMPAction != "magic-20" {
			t.Fatalf("facing 2 enemies with 3 MP: expected fallback to magic-20, got %s", foundLowMPAction)
		}

		// 3d. Item-144 (ダイヤの指輪): disables 15% decay on multi-target magic
		invRepo.inventories["mage-ring"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-144"}},
		}
		charRepo.characters["mage-ring"] = corecharacter.Character{
			ID:    "mage-ring",
			Name:  "MageRing",
			JobID: "job-06",
			SP:    10,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 20, Defense: 50, Agility: 50},
		}
		mageRing, _ := svc.BuildParticipant(ctx, "mage-ring")
		resRing, _ := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageRing},
			Enemies: []corebattle.Participant{enemy1, enemy2},
			RNG:     detRNG,
		})
		dmgRingE1, dmgRingE2 := -1, -1
		for _, log := range resRing.Logs {
			if log.ActionName == "magic-wave-25" {
				if log.TargetID == "e1" {
					dmgRingE1 = log.DamageDealt
				}
				if log.TargetID == "e2" {
					dmgRingE2 = log.DamageDealt
				}
			}
		}
		if dmgRingE1 != 25 || dmgRingE2 != 25 {
			t.Errorf("item-144 should prevent decay: dmgE1=%d, dmgE2=%d, want 25 and 25", dmgRingE1, dmgRingE2)
		}
	})

	// 4. Insufficient MP skips skill and falls back to normal attack
	t.Run("insufficient MP skips skill", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		charRepo.characters["dry-priest"] = corecharacter.Character{
			ID:    "dry-priest",
			Name:  "DryPriest",
			JobID: "job-05",
			SP:    10,                                                                                    // unlocks heal-30 (MPCost 3)
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 0, Attack: 20, Defense: 50, Agility: 50}, // MP 0!
		}
		dryPriest, err := svc.BuildParticipant(ctx, "dry-priest")
		if err != nil {
			t.Fatal(err)
		}

		enemy := corebattle.Participant{
			ID:        "enemy",
			Name:      "Dummy",
			HP:        100,
			MaxHP:     100,
			Attack:    0,
			Defending: true,
		}

		engine := corebattle.Engine{}
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{dryPriest},
			Enemies: []corebattle.Participant{enemy},
			RNG:     random.NewDeterministic(42),
		})
		if err != nil {
			t.Fatal(err)
		}

		// First turn action should NOT be heal-30, it should be normal attack
		for _, log := range res.Logs {
			if log.ActorID == dryPriest.ID {
				if log.ActionName == "heal-30" {
					t.Fatalf("heal-30 executed with 0 MP!")
				}
				if log.ActionName == "こうげき" || log.ActionName == "attack" {
					// Expected normal attack fallback
					break
				}
			}
		}
	})
}

func TestMagicSealSkillExecution(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	svc := newTestProductionBattleService(charRepo)
	engine := corebattle.Engine{}

	charRepo.characters["mage-sealed"] = corecharacter.Character{
		ID:    "mage-sealed",
		Name:  "Mage",
		JobID: "job-06",
		SP:    5, // unlocks magic-20 only
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 10, Attack: 10, Defense: 10, Agility: 50},
	}
	mage, err := svc.BuildParticipant(ctx, "mage-sealed")
	if err != nil {
		t.Fatal(err)
	}

	enemy := corebattle.Participant{
		ID:        "enemy",
		Name:      "Target",
		HP:        100,
		MaxHP:     100,
		Defense:   10,
		Defending: true,
	}

	// 1. Seal persists (RNG float64 = 0.50 >= 0.25)
	t.Run("seal persists: MP deducted, effect aborted, tension preserved", func(t *testing.T) {
		mageSealed := mage
		mageSealed.Status = corebattle.StatusMagicSeal
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{mageSealed},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{mageSealed.ID: 1.5},
			RNG:            deterministicBattleRNG{fVal: 0.50}, // >= 0.25 -> fails cure
		})
		if err != nil {
			t.Fatal(err)
		}

		// Find turn 1 log for mage
		foundSealLog := false
		for _, log := range res.Logs {
			if log.ActorID == mageSealed.ID {
				if log.ActionName == "magic-20" && log.Message == "しかし、Mage は魔法が封じられていた" {
					foundSealLog = true
					if log.DamageDealt != 0 {
						t.Errorf("expected 0 damage when sealed, got %d", log.DamageDealt)
					}
					if log.RemainingHP[enemy.ID] != 100 {
						t.Errorf("enemy HP changed on turn 1 despite magic seal: %d", log.RemainingHP[enemy.ID])
					}
					break
				}
			}
		}
		if !foundSealLog {
			t.Fatalf("expected seal failure message in logs: %+v", res.Logs)
		}
	})

	// 2. Seal cured (RNG float64 = 0.10 < 0.25)
	t.Run("seal cured: status cleared, cure log emitted, magic executes", func(t *testing.T) {
		mageSealed := mage
		mageSealed.Status = corebattle.StatusMagicSeal
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{mageSealed},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{mageSealed.ID: 1.0},
			RNG:            deterministicBattleRNG{fVal: 0.10}, // < 0.25 -> cures seal
		})
		if err != nil {
			t.Fatal(err)
		}

		foundCureLog := false
		foundDamageLog := false
		for _, log := range res.Logs {
			if log.ActorID == mageSealed.ID {
				if log.ActionName == "magic-20" && log.Message == "Mage は魔法が使えるようになりました！" {
					foundCureLog = true
				}
				if log.ActionName == "magic-20" && log.DamageDealt > 0 {
					foundDamageLog = true
				}
			}
		}
		if !foundCureLog {
			t.Fatalf("expected seal cure log in logs: %+v", res.Logs)
		}
		if !foundDamageLog {
			t.Fatalf("expected damage log after seal cure in logs: %+v", res.Logs)
		}
	})

	// 3. Legacy Japanese status string "魔封" also triggers seal check
	t.Run("legacy 魔封 status recognized and cured", func(t *testing.T) {
		mageSealed := mage
		mageSealed.Status = "魔封"
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageSealed},
			Enemies: []corebattle.Participant{enemy},
			RNG:     deterministicBattleRNG{fVal: 0.10},
		})
		if err != nil {
			t.Fatal(err)
		}
		foundCureLog := false
		for _, log := range res.Logs {
			if log.ActorID == mage.ID && log.Message == "Mage は魔法が使えるようになりました！" {
				foundCureLog = true
				break
			}
		}
		if !foundCureLog {
			t.Fatalf("expected cure log for 魔封 status: %+v", res.Logs)
		}
	})
}

func TestItem149AndTensionHandling(t *testing.T) {
	ctx := context.Background()
	detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}
	engine := corebattle.Engine{}

	enemy := corebattle.Participant{
		ID:        "enemy",
		Name:      "Target",
		HP:        1000,
		MaxHP:     1000,
		Defense:   0,
		Defending: false,
	}

	// 1. Attack with item-149: ignores tension multiplier (base * 1.1) and preserves tension
	t.Run("attack with item-149: tension ignored and preserved", func(t *testing.T) {
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		invRepo.inventories["mage-149"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-149"}},
		}
		charRepo.characters["mage-149"] = corecharacter.Character{
			ID:    "mage-149",
			Name:  "Mage149",
			JobID: "job-06",
			SP:    5, // magic-20 (base 20)
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10, Agility: 50},
		}
		mage149, err := svc.BuildParticipant(ctx, "mage-149")
		if err != nil {
			t.Fatal(err)
		}

		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{mage149},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{mage149.ID: 2.0},
			RNG:            detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}

		// Base 20 * 1.1 (item-149) = 22 damage. Tension 2.0 is NOT multiplied!
		for _, log := range res.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 22 {
					t.Errorf("expected 22 damage with item-149 (tension 2.0 ignored), got %d", log.DamageDealt)
				}
				break
			}
		}
	})

	// 2. Attack without item-149: applies tension multiplier and consumes tension
	t.Run("attack without item-149: tension applied and consumed", func(t *testing.T) {
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		charRepo.characters["mage-plain"] = corecharacter.Character{
			ID:    "mage-plain",
			Name:  "MagePlain",
			JobID: "job-06",
			SP:    5,
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10, Agility: 50},
		}
		magePlain, err := svc.BuildParticipant(ctx, "mage-plain")
		if err != nil {
			t.Fatal(err)
		}

		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{magePlain},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{magePlain.ID: 2.0},
			RNG:            detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}

		// Base 20 * 2.0 (tension) = 40 damage.
		for _, log := range res.Logs {
			if log.ActionName == "magic-20" {
				if log.DamageDealt != 40 {
					t.Errorf("expected 40 damage without item-149 (tension applied), got %d", log.DamageDealt)
				}
				break
			}
		}
	})

	// 3. Heal with item-149: tension is ALWAYS multiplied and ALWAYS reset to 1.0 (_skill.cgi:9103,9105)
	t.Run("heal with item-149: tension applied and unconditionally reset to 1.0", func(t *testing.T) {
		charRepo := newMockCharRepo()
		invRepo := newMockInvRepo()
		svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

		invRepo.inventories["priest-149"] = coreinventory.Inventory{
			Items: []item.Instance{{DefinitionID: "item-149"}},
		}
		charRepo.characters["priest-149"] = corecharacter.Character{
			ID:    "priest-149",
			Name:  "Priest149",
			JobID: "job-05",
			SP:    10, // heal-30
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10, Agility: 50},
		}
		priest149, err := svc.BuildParticipant(ctx, "priest-149")
		if err != nil {
			t.Fatal(err)
		}

		ally := corebattle.Participant{ID: "ally", HP: 10, MaxHP: 100}
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{priest149, ally},
			Enemies:        []corebattle.Participant{enemy},
			InitialTension: map[string]float64{priest149.ID: 2.0},
			RNG:            detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}

		// Base 30 * 2.0 (tension) = 60 healing
		for _, log := range res.Logs {
			if log.ActionName == "heal-30" && log.TargetID == ally.ID {
				if log.HealingDone != 60 {
					t.Errorf("expected 60 heal with tension 2.0 and item-149, got %d", log.HealingDone)
				}
				break
			}
		}
	})
}

func TestItem256PrimaryTargetMaxHPCap(t *testing.T) {
	ctx := context.Background()
	detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}
	engine := corebattle.Engine{}

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

	invRepo.inventories["priest-256"] = coreinventory.Inventory{
		Items: []item.Instance{{DefinitionID: "item-256"}},
	}
	charRepo.characters["priest-256"] = corecharacter.Character{
		ID:    "priest-256",
		Name:  "Priest256",
		JobID: "job-05",
		SP:    10, // heal-30 (base 30 * 1.2 = 36 heal with item-256)
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10, Defense: 10, Agility: 50},
	}
	priest, err := svc.BuildParticipant(ctx, "priest-256")
	if err != nil {
		t.Fatal(err)
	}

	enemy := corebattle.Participant{ID: "e", HP: 1000, MaxHP: 1000, Defense: 0, Defending: true}

	// Case 1: Primary target has low MaxHP (50).
	// Primary heal = 36. Uncapped splash = 36 / 2 = 18.
	// Cap = int(primary.MaxHP * 0.3) = int(50 * 0.3) = 15!
	// Secondary ally has MaxHP = 500, but splash is capped at 15 (primary target's 30% MaxHP).
	t.Run("splash heal capped by primary target MaxHP * 0.3", func(t *testing.T) {
		primaryAlly := corebattle.Participant{ID: "primary", HP: 10, MaxHP: 50}
		secondaryAlly := corebattle.Participant{ID: "secondary", HP: 10, MaxHP: 500}

		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priest, primaryAlly, secondaryAlly},
			Enemies: []corebattle.Participant{enemy},
			RNG:     detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}

		var primaryHeal, secondaryHeal int
		for _, log := range res.Logs {
			if log.ActionName == "heal-30" {
				if log.TargetID == primaryAlly.ID {
					primaryHeal = log.HealingDone
				}
				if log.TargetID == secondaryAlly.ID {
					secondaryHeal = log.HealingDone
				}
			}
		}
		if primaryHeal != 36 {
			t.Errorf("primary heal = %d, want 36", primaryHeal)
		}
		if secondaryHeal != 15 {
			t.Errorf("secondary heal = %d, want 15 (capped at 50 * 0.3)", secondaryHeal)
		}
	})

	// Case 2: Primary target has high MaxHP (200).
	// Cap = int(200 * 0.3) = 60.
	// Secondary ally receives full 18 splash heal.
	t.Run("splash heal not capped when primary target MaxHP is high", func(t *testing.T) {
		primaryAlly := corebattle.Participant{ID: "primary-large", HP: 10, MaxHP: 200}
		secondaryAlly := corebattle.Participant{ID: "secondary-2", HP: 10, MaxHP: 500}

		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{priest, primaryAlly, secondaryAlly},
			Enemies: []corebattle.Participant{enemy},
			RNG:     detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}

		var secondaryHeal int
		for _, log := range res.Logs {
			if log.ActionName == "heal-30" && log.TargetID == secondaryAlly.ID {
				secondaryHeal = log.HealingDone
			}
		}
		if secondaryHeal != 18 {
			t.Errorf("secondary heal = %d, want 18 (uncapped)", secondaryHeal)
		}
	})
}

func TestBossSkillSelectionRegression(t *testing.T) {
	// Verify that a boss with single-target and multi-target skills in order
	// preserves its sequential evaluation against multiple opponents (does NOT jump to area attack).
	engine := corebattle.Engine{}
	detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}

	boss := corebattle.Participant{
		ID:      "boss-mage",
		Name:    "BossMage",
		HP:      1000,
		MaxHP:   1000,
		MP:      50,
		MaxMP:   50,
		Attack:  50,
		Defense: 50,
		Agility: 50,
		Skills: []corebattle.ActionSkill{
			{
				ID:     "dejon",
				Name:   "デジョン",
				Kind:   corebattle.ActionKindDejon,
				MPCost: 0,
			},
			{
				ID:          "mera",
				Name:        "メラ",
				Power:       15,
				MPCost:      2,
				Kind:        corebattle.ActionKindAttack,
				TargetScope: corebattle.TargetScopeSingleEnemy,
				Category:    string(corebattle.CategoryMagic),
				Direct:      true,
			},
			{
				ID:          "gira",
				Name:        "ギラ",
				Power:       20,
				MPCost:      5,
				Kind:        corebattle.ActionKindAttack,
				TargetScope: corebattle.TargetScopeAllEnemies,
				Category:    string(corebattle.CategoryMagic),
				Direct:      true,
			},
		},
	}

	ally1 := corebattle.Participant{ID: "ally1", Name: "Ally1", HP: 100, MaxHP: 100, Defense: 10}
	ally2 := corebattle.Participant{ID: "ally2", Name: "Ally2", HP: 100, MaxHP: 100, Defense: 10}

	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{boss},
		Enemies: []corebattle.Participant{ally1, ally2},
		RNG:     detRNG,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Boss must select Mera (first usable skill in its list), NOT jump to Gira
	bossAction := ""
	for _, log := range res.Logs {
		if log.ActorID == boss.ID {
			bossAction = log.ActionName
			break
		}
	}
	if bossAction != "メラ" {
		t.Fatalf("boss facing 2 enemies: expected Mera (sequential priority), got %s", bossAction)
	}
}

// Tension is consumed by the first effective hit of an all-enemies magic skill,
// so later targets must not inherit it (job-6-entry-3 magic-wave-25, no item-149).
func TestAllEnemiesMagicConsumesTensionAfterFirstHit(t *testing.T) {
	ctx := context.Background()
	detRNG := deterministicBattleRNG{fVal: 1.0 / 3.0}
	engine := corebattle.Engine{}

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc := newTestProductionBattleServiceWithInv(charRepo, invRepo)

	charRepo.characters["mage-wave"] = corecharacter.Character{
		ID:    "mage-wave",
		Name:  "MageWave",
		JobID: "job-06",
		SP:    10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 20, Defense: 50, Agility: 50},
	}
	mage, err := svc.BuildParticipant(ctx, "mage-wave")
	if err != nil {
		t.Fatal(err)
	}

	// firstHits resolves one battle and returns the first magic-wave-25 damage per target.
	firstHits := func(t *testing.T, actor corebattle.Participant, tension float64, rng deterministicBattleRNG, enemies ...corebattle.Participant) (map[string]int, corebattle.PartyBattleResult) {
		t.Helper()
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{actor},
			Enemies:        enemies,
			InitialTension: map[string]float64{actor.ID: tension},
			RNG:            rng,
		})
		if err != nil {
			t.Fatal(err)
		}
		hits := map[string]int{}
		for _, log := range res.Logs {
			if log.ActionName != "magic-wave-25" || log.ActorID != actor.ID {
				continue
			}
			if _, seen := hits[log.TargetID]; !seen {
				hits[log.TargetID] = log.DamageDealt
			}
		}
		return hits, res
	}

	e1 := corebattle.Participant{ID: "e1", Name: "E1", HP: 1000, MaxHP: 1000, Defense: 0}
	e2 := corebattle.Participant{ID: "e2", Name: "E2", HP: 1000, MaxHP: 1000, Defense: 0}

	// Power 25 * 2.0 (tension) = 50 on the first hit; tension is then reset,
	// so the second hit uses the decayed base int(25*0.85)=21 without tension.
	t.Run("tension 2 is consumed by the first hit", func(t *testing.T) {
		hits, _ := firstHits(t, mage, 2.0, detRNG, e1, e2)
		if hits["e1"] != 50 {
			t.Errorf("first target damage = %d, want 50", hits["e1"])
		}
		if hits["e2"] != 21 {
			t.Errorf("second target damage = %d, want 21 (tension consumed by first hit)", hits["e2"])
		}
	})

	t.Run("tension 1 control", func(t *testing.T) {
		hits, _ := firstHits(t, mage, 1.0, detRNG, e1, e2)
		if hits["e1"] != 25 || hits["e2"] != 21 {
			t.Errorf("damage = [%d %d], want [25 21]", hits["e1"], hits["e2"])
		}
	})

	// item-149 ignores tension entirely: 25*1.1 = 27, then 25*0.85*1.1 = 23.
	t.Run("item-149 keeps tension out of the damage formula", func(t *testing.T) {
		withItem := mage
		withItem.ItemDefinitionIDs = append([]string{"item-149"}, mage.ItemDefinitionIDs...)
		hits, _ := firstHits(t, withItem, 2.0, detRNG, e1, e2)
		if hits["e1"] != 27 || hits["e2"] != 23 {
			t.Errorf("damage = [%d %d], want [27 23]", hits["e1"], hits["e2"])
		}
	})

	// An immune hit is not effective, so it must not consume tension: e2 still gets the 2.0 multiplier.
	t.Run("immune first target does not consume tension", func(t *testing.T) {
		immune := corebattle.Participant{ID: "e-m-imm", Name: "Immune", HP: 1000, MaxHP: 1000, Abilities: []string{"魔無効"}}
		hits, _ := firstHits(t, mage, 2.0, detRNG, immune, e2)
		if hits["e-m-imm"] != 0 {
			t.Errorf("immune target damage = %d, want 0", hits["e-m-imm"])
		}
		// Decay still applies to the second target: int(25*0.85*2.0)=42.
		if hits["e2"] != 42 {
			t.Errorf("second target damage = %d, want 42 (immune hit must not consume tension)", hits["e2"])
		}
	})

	// A no-effect hit between effective hits is passed through without touching tension:
	// e1 consumes it (50), the immune hit deals 0, and e3 gets only the decayed base int(25*0.85*0.85)=18.
	t.Run("immune hit after consumption leaves later targets decayed", func(t *testing.T) {
		immune := corebattle.Participant{ID: "e-m-mid", Name: "Immune", HP: 1000, MaxHP: 1000, Abilities: []string{"魔無効"}}
		e3 := corebattle.Participant{ID: "e3", Name: "E3", HP: 1000, MaxHP: 1000, Defense: 0}
		hits, _ := firstHits(t, mage, 2.0, detRNG, e1, immune, e3)
		if hits["e1"] != 50 {
			t.Errorf("first target damage = %d, want 50", hits["e1"])
		}
		if hits["e-m-mid"] != 0 {
			t.Errorf("immune target damage = %d, want 0", hits["e-m-mid"])
		}
		if hits["e3"] != 18 {
			t.Errorf("third target damage = %d, want 18 (tension consumed by first hit, immune hit must not restore it)", hits["e3"])
		}
	})

	// A persistent seal aborts the skill before any hit, so no magic-wave-25 log deals damage.
	t.Run("seal persists: magic-wave-25 deals no damage", func(t *testing.T) {
		sealed := mage
		sealed.Status = corebattle.StatusMagicSeal
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:         []corebattle.Participant{sealed},
			Enemies:        []corebattle.Participant{e1, e2},
			InitialTension: map[string]float64{sealed.ID: 2.0},
			RNG:            deterministicBattleRNG{fVal: 0.50}, // >= 0.25 -> seal persists
		})
		if err != nil {
			t.Fatal(err)
		}
		attempts := 0
		for _, log := range res.Logs {
			if log.ActionName != "magic-wave-25" || log.ActorID != sealed.ID {
				continue
			}
			attempts++
			if log.DamageDealt != 0 {
				t.Errorf("sealed magic-wave-25 dealt %d damage to %s, want 0", log.DamageDealt, log.TargetID)
			}
		}
		if attempts == 0 {
			t.Fatal("expected sealed magic-wave-25 attempt in logs")
		}
	})

	t.Run("single target keeps tension 2 damage", func(t *testing.T) {
		hits, _ := firstHits(t, mage, 2.0, detRNG, e1)
		if hits["e1"] != 50 {
			t.Errorf("single target damage = %d, want 50", hits["e1"])
		}
	})
}

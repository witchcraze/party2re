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
			8: {"job-6-entry-1", "job-6-entry-3"},
			9: {"job-6-entry-1", "job-6-entry-3"},
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
		wantOrder := []string{"job-6-entry-1", "job-6-entry-3", "job-5-entry-3"}
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

		// 3a. Facing 1 enemy: should choose single-target magic-20
		enemy1 := corebattle.Participant{ID: "e1", HP: 200, MaxHP: 200, Defense: 10, Attack: 0}
		resSingle, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mageFull},
			Enemies: []corebattle.Participant{enemy1},
			RNG:     detRNG,
		})
		if err != nil {
			t.Fatal(err)
		}
		foundAction := ""
		for _, log := range resSingle.Logs {
			if log.ActorID == mageFull.ID {
				foundAction = log.ActionName
				break
			}
		}
		if foundAction != "magic-20" {
			t.Fatalf("facing 1 enemy: expected action magic-20, got %s", foundAction)
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

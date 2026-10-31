package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

func TestProductionBattleExecution(t *testing.T) {
	ctx := context.Background()

	// 1. Single-ally heal (job-5-entry-3: heal-30, MPCost 3, TargetScopeSingleAlly)
	t.Run("heal-30 single-ally execution", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

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

		// Damaged ally (HP 50 / MaxHP 100)
		ally := corebattle.Participant{
			ID:    "damaged-ally",
			Name:  "Wounded",
			HP:    50,
			MaxHP: 100,
		}
		// Passive enemy
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
			RNG:     random.NewDeterministic(42),
		})
		if err != nil {
			t.Fatal(err)
		}

		// Verify TurnLog has heal action on ally
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
	})

	// 2. Single-enemy magic attack (job-6-entry-1: magic-20, MPCost 2, TargetScopeSingleEnemy)
	t.Run("magic-20 single-enemy execution", func(t *testing.T) {
		charRepo := newMockCharRepo()
		svc := newTestProductionBattleService(charRepo)

		charRepo.characters["mage"] = corecharacter.Character{
			ID:    "mage",
			Name:  "Mage",
			JobID: "job-06",
			SP:    5, // unlocks magic-20 (SP 1), magic-wave-25 (SP 8) is locked
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 10, Attack: 20, Defense: 50, Agility: 50},
		}
		mage, err := svc.BuildParticipant(ctx, "mage")
		if err != nil {
			t.Fatal(err)
		}

		enemy := corebattle.Participant{
			ID:        "enemy-1",
			Name:      "Slime",
			HP:        200,
			MaxHP:     200,
			Defense:   10,
			Attack:    0,
			Defending: true,
		}

		engine := corebattle.Engine{}
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{mage},
			Enemies: []corebattle.Participant{enemy},
			RNG:     random.NewDeterministic(42),
		})
		if err != nil {
			t.Fatal(err)
		}

		foundMagic := false
		for _, log := range res.Logs {
			if log.ActionName == "magic-20" && log.TargetID == enemy.ID {
				foundMagic = true
				if log.DamageDealt <= 0 {
					t.Errorf("expected positive damage, got %d", log.DamageDealt)
				}
				break
			}
		}
		if !foundMagic {
			t.Fatalf("magic-20 action was not executed in battle logs: %+v", res.Logs)
		}
	})

	// 3. All-enemies magic attack (job-6-entry-3: magic-wave-25, MPCost 5, TargetScopeAllEnemies)
	t.Run("magic-wave-25 all-enemies execution", func(t *testing.T) {
		// Test directly with BuildParticipantFromData to verify direct participant building
		char := corecharacter.Character{
			ID:    "wave-mage",
			Name:  "WaveMage",
			JobID: "job-06",
			SP:    10, // unlocks both magic-20 and magic-wave-25
			Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 15, Attack: 20, Defense: 50, Agility: 50},
		}
		defs := skill.ProductionBattleDefinitions()
		p, err := battle.BuildParticipantFromData(
			char,
			coreinventory.Inventory{},
			coreequipment.Equipment{},
			// Only supply magic-wave-25 so it is the selected skill
			[]skill.Definition{
				{
					ID:             "job-6-entry-3",
					Name:           "magic-wave-25",
					RequiredJobIDs: []string{"job-06"},
					RequiredSP:     8,
					MPCost:         5,
					Effect:         corebattle.Effect{Kind: corebattle.ActionKindAttack, Power: 25},
				},
			},
			nil,
			defs,
		)
		if err != nil {
			t.Fatal(err)
		}

		enemy1 := corebattle.Participant{ID: "e1", HP: 200, MaxHP: 200, Attack: 0, Defending: true}
		enemy2 := corebattle.Participant{ID: "e2", HP: 200, MaxHP: 200, Attack: 0, Defending: true}

		engine := corebattle.Engine{}
		res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{p},
			Enemies: []corebattle.Participant{enemy1, enemy2},
			RNG:     random.NewDeterministic(42),
		})
		if err != nil {
			t.Fatal(err)
		}

		hitE1, hitE2 := false, false
		for _, log := range res.Logs {
			if log.ActionName == "magic-wave-25" {
				if log.TargetID == "e1" {
					hitE1 = true
				}
				if log.TargetID == "e2" {
					hitE2 = true
				}
			}
		}
		if !hitE1 || !hitE2 {
			t.Fatalf("magic-wave-25 did not hit all enemies: hitE1=%v, hitE2=%v, logs=%+v", hitE1, hitE2, res.Logs)
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

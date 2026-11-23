package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	battle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func runNPCFixture(t *testing.T) (battle.LifetimeState, battle.NPCOperation) {
	t.Helper()
	s, c, _, _ := npcSelectionFixture()
	for i := range s.Actors {
		s.Actors[i].Gear = &battle.BattleGearStats{}
	}
	s.Actors[2].NPC = true
	return s, battle.NPCOperation{
		Damage: c.DamageContext,
		Defeat: &battle.DefeatContext{},
	}
}

func registerNPCFixture(d *BattleDefinitions, _ *battle.LifetimeState, f *battle.CallbackContext) {
	d.RegisterNPCCallbacks(PlayerCallbackContexts{BaseGuard: &battle.BaseGuardContext{DamageContext: *f.Damage}}, f)
}

func TestNPCOperationCureOnlyReachesTails(t *testing.T) {
	s, op := runNPCFixture(t)
	a := &s.Actors[0]
	a.HP, a.MaxHP, a.Status = 50, 100, battle.StatusSleep
	a.ItemDefinitionIDs = []string{"item-202"}
	s.Fields = battle.LifetimeFields{Temporary: "burn", Turns: 3}
	s.SkillBans = map[string]int{"banned": 2}
	s.Actors[1].Extended, s.Actors[1].ExtendedTurns = battle.ExtendedIncense, 4
	a.CMP, a.CMPCharge = 5, 2
	seed := NPCDefaultSeed()
	rng := &npcJobRNG{t: t, draws: []float64{0, 0, 0}}
	got, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	a = &s.Actors[0]
	if err != nil || got.NormalAction || got.Choice.ID != "" || len(got.NextSeed) != 5 ||
		a.Status != "" || a.HP != 42 || a.CMP != 5 || a.CMPCharge != 2 ||
		s.Fields.Turns != 3 || s.SkillBans["banned"] != 2 || s.Actors[1].ExtendedTurns != 4 || len(rng.draws) != 0 {
		t.Fatalf("result %+v actor %+v state %+v remaining draws %v error %v", got, a, s, rng.draws, err)
	}
}

func TestNPCOperationWholePoolRejectionIsPure(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Actors[0].Status = battle.StatusConfusion
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "job-406", SP: 1000, OldJobID: "starter"}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcJobRNG{t: t})
	after, marshalErr := json.Marshal(s)
	if !errors.Is(err, battle.ErrUnsupportedEffect) || marshalErr != nil || string(after) != string(before) ||
		!reflect.DeepEqual(result, battle.NPCOperationResult{}) {
		t.Fatalf("result %+v state changed %t error %v marshal %v", result, string(after) != string(before), err, marshalErr)
	}
}

func TestNPCOperationPreflightsKnownTailsAndReachableDefeat(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Actors[0].ItemDefinitionIDs = []string{"item-164"}
	s.Actors[0].Tension = 5
	seed := []battle.CallbackBinding{responseBinding("job-0-entry-4")}
	_, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, &npcJobRNG{t: t})
	if !errors.Is(err, battle.ErrUnsupportedEffect) || s.Actors[0].Temporary != "" || s.Actors[0].Tension != 5 {
		t.Fatalf("known equipment tail state %+v error %v", s.Actors[0], err)
	}

	s, op = runNPCFixture(t)
	s.Actors[1].Gear.WeaponID = "weapon-1"
	s.Actors[1].HP = 1
	_, err = ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcJobRNG{t: t})
	if !errors.Is(err, battle.ErrUnsupportedEffect) || s.Actors[1].HP != 1 {
		t.Fatalf("reachable defeat HP %d error %v", s.Actors[1].HP, err)
	}
	s.Actors[1].HP = 1000
	_, err = ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcSelectionRNG{value: .95})
	if err != nil || s.Actors[1].HP >= 1000 {
		t.Fatalf("nonlethal attack HP %d error %v", s.Actors[1].HP, err)
	}
}

func TestNPCOperationNormalAttackPreflightsEquipmentTail(t *testing.T) {
	for _, mode := range []string{"one-hit defense", "zero power without defeat port"} {
		t.Run(mode, func(t *testing.T) {
			s, op := runNPCFixture(t)
			a := &s.Actors[0]
			a.ItemDefinitionIDs = []string{"item-164"}
			a.Temporary, a.Tension = battle.TemporaryCovering, 5
			if mode == "one-hit defense" {
				s.Actors[1].Temporary = battle.TemporaryOneHitDefense
			} else {
				a.Attack, op.Defeat = 0, nil
			}
			targetTemporary := s.Actors[1].Temporary
			result, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcJobRNG{t: t})
			if !errors.Is(err, battle.ErrUnsupportedEffect) || !reflect.DeepEqual(result, battle.NPCOperationResult{}) ||
				s.Actors[0].Temporary != battle.TemporaryCovering || s.Actors[0].Tension != 5 ||
				s.Actors[1].HP != 1000 || s.Actors[1].Temporary != targetTemporary {
				t.Fatalf("result %+v, actors %+v, error %v", result, s.Actors, err)
			}

			// Unblocked hits spend tension; a zero-power hit can still complete
			// with no need for defeat ports when the equipment tail is absent.
			if mode == "one-hit defense" {
				s.Actors[1].Temporary = ""
			} else {
				s.Actors[0].ItemDefinitionIDs = nil
			}
			result, err = ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcSelectionRNG{value: .95})
			if err != nil || result.Choice.ID != "npc-normal-attack" ||
				(mode == "one-hit defense" && (s.Actors[1].HP >= 1000 || s.Actors[0].Temporary != "")) ||
				(mode != "one-hit defense" && s.Actors[1].HP != 1000) {
				t.Fatalf("result %+v, actors %+v, error %v", result, s.Actors, err)
			}
		})
	}
}

func TestNPCOperationNormalAttackPreflightsEquipmentReachedAfterField(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Fields.Default = "burn"
	s.Actors[0].Status = battle.StatusMagicSeal
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 150
	s.Actors[1].Gear.WeaponID = "weapon-1"

	// The nonlethal normal attack leaves the target in reach of item206
	// after the burn field cures the actor. Its defeat needs a weapon reset.
	_, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcJobRNG{t: t})
	if !errors.Is(err, battle.ErrUnsupportedEffect) || s.Actors[0].Status != battle.StatusMagicSeal || s.Actors[1].HP != 150 {
		t.Fatalf("actors %+v, error %v", s.Actors, err)
	}
	s.Fields.AntiTurns = 1
	got, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcSelectionRNG{value: .95})
	if err != nil || got.Choice.ID != "npc-normal-attack" || s.Actors[0].Status != battle.StatusMagicSeal ||
		s.Actors[1].HP <= 0 || s.Actors[1].HP >= 150 {
		t.Fatalf("result %+v, actors %+v, error %v", got, s.Actors, err)
	}
}

func TestNPCOperationConfusionChoiceAndExpiryDrawOrder(t *testing.T) {
	for _, row := range []struct {
		draw   float64
		status string
	}{{.32, ""}, {1.0 / 3, battle.StatusConfusion}} {
		s, op := runNPCFixture(t)
		s.Actors[0].Status, s.Actors[0].Temporary = battle.StatusConfusion, battle.TemporaryRegeneration
		seeds := []battle.CallbackBinding{responseBinding("one"), responseBinding("two")}
		rng := &npcJobRNG{t: t, draws: []float64{row.draw, .75, .1}}
		got, err := ApplyNPCOperation(&s, op, seeds, func(d *BattleDefinitions, w *battle.LifetimeState, f *battle.CallbackContext) {
			registerNPCFixture(d, w, f)
			d.Invocations.Entries["one"] = responseCallback(func(*battle.LifetimeState, *battle.CallbackContext, random.Generator) error {
				t.Fatal("expiry roll was used to select the action")
				return nil
			})
			d.Invocations.Entries["two"] = responseCallback(func(s *battle.LifetimeState, _ *battle.CallbackContext, _ random.Generator) error {
				if a := s.Actors[0]; a.Status != row.status || a.Temporary != "" {
					t.Fatalf("callback saw status %q and temporary %q", a.Status, a.Temporary)
				}
				return nil
			})
		}, rng)
		if err != nil || got.Choice.ID != "two" || len(rng.draws) != 0 {
			t.Fatalf("draw %v result %+v remaining draws %v error %v", row.draw, got, rng.draws, err)
		}
	}
}

func TestNPCOperationPoisonAndRegeneration(t *testing.T) {
	for _, row := range []struct {
		status string
		hp     int
		draws  []float64
	}{{battle.StatusDeadlyPoison, 850, []float64{0, .9, 0}}, {battle.StatusPoison, 810, []float64{0, .9, 0, .9}}} {
		s, op := runNPCFixture(t)
		a := &s.Actors[0]
		a.HP, a.MaxHP, a.Status, a.Temporary = 850, 900, row.status, battle.TemporaryRegeneration
		rng := &npcJobRNG{t: t, draws: row.draws}
		got, err := ApplyNPCOperation(&s, op, []battle.CallbackBinding{responseBinding("pass")}, func(d *BattleDefinitions, w *battle.LifetimeState, f *battle.CallbackContext) {
			registerNPCFixture(d, w, f)
			d.Invocations.Entries["pass"] = responseCallback(func(*battle.LifetimeState, *battle.CallbackContext, random.Generator) error { return nil })
		}, rng)
		if err != nil || !got.NormalAction || s.Actors[0].HP != row.hp || len(rng.draws) != 0 {
			t.Fatalf("status %q result %+v HP %d remaining draws %v error %v", row.status, got, s.Actors[0].HP, rng.draws, err)
		}
	}
}

func TestNPCOperationRepeatSeedAndReload(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Actors[0].Jobs.OldJobID = "job-01"
	s.Actors[0].Status = battle.StatusDofuu
	mode := NPCDefaultSeed()
	first, err := ApplyNPCOperation(&s, op, mode, registerNPCFixture, &npcJobRNG{t: t})
	if err != nil || first.NormalAction || len(first.NextSeed) != 7 {
		t.Fatalf("first %+v error %v", first, err)
	}
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "starter", OldJobID: "starter", SP: 25}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	load := func() battle.LifetimeState {
		var out battle.LifetimeState
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	reloaded, newRequest := load(), load()
	second, err := ApplyNPCOperation(&s, op, first.NextSeed, registerNPCFixture, &npcSelectionRNG{value: .1})
	if err != nil || second.Choice.ID != "job-1-entry-1" || s.Actors[0].MP != 75 {
		t.Fatalf("repeat %+v MP %d error %v", second, s.Actors[0].MP, err)
	}
	got, err := ApplyNPCOperation(&reloaded, op, first.NextSeed, registerNPCFixture, &npcSelectionRNG{value: .1})
	if err != nil || !reflect.DeepEqual(got, second) || !reflect.DeepEqual(reloaded, s) {
		t.Fatalf("reloaded repeat %+v, want %+v error %v", got, second, err)
	}
	got, err = ApplyNPCOperation(&newRequest, op, mode, registerNPCFixture, &npcSelectionRNG{value: .1})
	if err != nil || got.Choice.ID != "job-0-entry-2" || newRequest.Actors[0].MP != 80 {
		t.Fatalf("new request %+v MP %d error %v", got, newRequest.Actors[0].MP, err)
	}
}

func TestNPCOperationRegisteredSummonCarriesRoster(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Actors[0].Jobs.JobID = "job-95"
	register := func(d *BattleDefinitions, w *battle.LifetimeState, f *battle.CallbackContext) {
		d.RegisterNPCCallbacks(PlayerCallbackContexts{Roster: &battle.RosterContext{
			EnemyTeamID: "party",
			EnemyPool: []battle.LifetimeActor{{Participant: battle.Participant{
				ID: "spawn", Name: "Spawn", HP: 50, Gear: &battle.BattleGearStats{},
			}}},
		}}, f)
	}
	rng := &npcJobRNG{t: t, draws: []float64{.5, .9, 0, 0}}
	got, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), register, rng)
	if err != nil || got.Choice.ID != "npc-job-95-position-1" || len(s.Actors) != 4 || len(got.Target.PartyIDs) != 3 ||
		got.Target.PartyIDs[2] != s.Actors[3].ID || s.Actors[0].MP != 30 || len(rng.draws) != 0 {
		t.Fatalf("summon %+v actor count %d MP %d remaining draws %v error %v", got, len(s.Actors), s.Actors[0].MP, rng.draws, err)
	}
}

func TestNPCOperationDefeatOrderConsumptionAndEndProcessing(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Fields.Default = "darkness"
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "job-01", SP: 50, OldJobID: "starter"}
	enemy := s.Actors[1].Clone()
	enemy.HP = 10
	s.Actors[1] = enemy
	enemy.ID = "saved-by-amulet"
	enemy.Gear.AccessoryID = "item-193"
	enemy.ItemDefinitionIDs = []string{"item-193"}
	enemy.ConsumedItems = []battle.ConsumedItem{{ID: "old", Quantity: 1}}
	s.Actors = append(s.Actors, enemy)
	enemy = s.Actors[1].Clone()
	enemy.ID = "last"
	s.Actors = append(s.Actors, enemy)
	op.Damage.EnemyIDs = []string{"last", "saved-by-amulet", "enemy"}
	got, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcSelectionRNG{value: .95})
	wantDefeats := []battle.NPCDefeat{{TargetID: "last", KillerID: "self"}, {TargetID: "enemy", KillerID: "self"}}
	wantItems := map[string][]battle.ConsumedItem{"saved-by-amulet": {{ID: "item-193", Quantity: 1}}}
	if err != nil || got.Choice.ID != "job-1-entry-4" || !reflect.DeepEqual(got.Defeats, wantDefeats) ||
		!reflect.DeepEqual(got.Consumed, wantItems) || s.Actors[3].HP != 500 || s.Actors[0].HP >= 100 || s.Actors[0].MP != 75 {
		t.Fatalf("result %+v NPC HP %d MP %d amulet HP %d error %v", got, s.Actors[0].HP, s.Actors[0].MP, s.Actors[3].HP, err)
	}
}

func TestNPCOperationFinalStrikeRecordsTheRetaliatingActor(t *testing.T) {
	s, op := runNPCFixture(t)
	s.Actors = s.Actors[:2]
	op.Damage.PartyIDs = []string{"self"}
	s.Actors[0].HP, s.Actors[1].HP = 10, 10
	s.Actors[1].BaseStats.Attack = 200
	s.Actors[1].Gear.AccessoryID = "item-212"
	s.Actors[1].ItemDefinitionIDs = []string{"item-212"}
	op.Defeat.Rewards = &battle.DefeatRewards{
		Validate: func(*battle.LifetimeState, battle.TargetContext, string) error {
			t.Fatal("NPC response reached player reward admission")
			return nil
		},
		Apply: func(*battle.LifetimeState, battle.TargetContext, string) error {
			t.Fatal("NPC response reached player reward application")
			return nil
		},
	}
	got, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerNPCFixture, &npcSelectionRNG{value: .95})
	want := []battle.NPCDefeat{{TargetID: "enemy", KillerID: "self"}, {TargetID: "self", KillerID: "enemy"}}
	if err != nil || !reflect.DeepEqual(got.Defeats, want) || s.Actors[0].HP != 0 || s.Actors[1].HP != 0 {
		t.Fatalf("defeats %v, want %v, HP %d/%d, error %v", got.Defeats, want, s.Actors[0].HP, s.Actors[1].HP, err)
	}
}

func TestNPCOperationLateRejectionRestoresStateAndPorts(t *testing.T) {
	s, op := runNPCFixture(t)
	loadout := &battle.NPCLoadout{CustomName: "custom", Gems: [3]string{"first", "later"}, CustomCost: 5}
	s.Actors[0].NPCLoadout = loadout
	s.Actors[0].Status = battle.StatusConfusion
	level := 20
	op.Damage.RequesterLevel, op.Defeat.RequesterLevel = &level, &level
	op.Assets = &battle.JobAssets{RequesterID: "enemy", Wallet: 100}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var executed bool
	rng := &npcJobRNG{t: t, draws: []float64{0, .9, .9}}
	res, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), func(d *BattleDefinitions, w *battle.LifetimeState, f *battle.CallbackContext) {
		registerNPCFixture(d, w, f)
		d.Invocations.Entries["first"] = responseCallback(func(s *battle.LifetimeState, c *battle.CallbackContext, _ random.Generator) error {
			executed = true
			if c.Assets == nil || c.Assets.Wallet != 100 {
				t.Fatalf("missing requester assets: %+v", c.Assets)
			}
			c.Assets.Wallet -= 25
			if op.Assets.Wallet != 100 {
				t.Fatal("in-flight NPC callback mutated requester assets")
			}
			s.Actors[0].Gear.AccessoryID = "changed"
			s.Actors[0].NPCLoadout.CustomCost = 30
			*c.Damage.RequesterLevel = 99
			*c.Defeat.RequesterLevel = 99
			c.PartyIDs[0] = "changed"
			return nil
		})
		d.Invocations.Entries["later"] = battle.CallbackDefinition{
			Validate: func(s *battle.LifetimeState, c *battle.CallbackContext) error {
				if s.Actors[0].Gear.AccessoryID != "" {
					if c.Assets.Wallet != 75 {
						t.Fatalf("later callback did not see the wallet debit: %+v", c.Assets)
					}
					return battle.ErrUnsupportedEffect
				}
				return nil
			},
			Execute: func(*battle.LifetimeState, *battle.CallbackContext, random.Generator) error {
				t.Fatal("unsupported later slot executed")
				return nil
			},
		}
	}, rng)
	after, marshalErr := json.Marshal(s)
	if !errors.Is(err, battle.ErrUnsupportedEffect) || !executed || marshalErr != nil || string(before) != string(after) ||
		level != 20 || loadout.CustomCost != 5 || op.Damage.PartyIDs[0] != "self" || len(rng.draws) != 0 ||
		op.Assets.Wallet != 100 || !reflect.DeepEqual(res, battle.NPCOperationResult{}) {
		t.Fatalf("result %+v executed %t level %d wallet %d state changed %t error %v", res, executed, level, op.Assets.Wallet, string(before) != string(after), err)
	}
}

func TestNPCOperationKinjuPreflightsTailsAndRetainsNonlethalContrast(t *testing.T) {
	seed := []battle.CallbackBinding{responseBinding("job-0-entry-4")}

	s, op := runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 100, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	rng := &npcJobRNG{t: t, draws: []float64{.1}}
	got, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	after, marshalErr := json.Marshal(s)
	if !errors.Is(err, battle.ErrUnsupportedEffect) || marshalErr != nil || string(after) != string(before) ||
		!reflect.DeepEqual(got, battle.NPCOperationResult{}) || len(rng.draws) != 1 ||
		s.Actors[0].HP != 100 || s.Actors[0].Status != battle.StatusKinju || s.Actors[1].HP != 1 {
		t.Fatalf("result %+v state changed %t draws %v actors %+v error %v marshal %v", got, string(after) != string(before), rng.draws, s.Actors, err, marshalErr)
	}

	// Removing item-206 leaves the curse-only idle continuation supported.
	s.Actors[0].ItemDefinitionIDs = nil
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	if err != nil || got.NormalAction || got.Choice.ID != "" || len(got.Defeats) != 0 ||
		s.Actors[0].HP != 90 || s.Actors[0].Status != "" || len(rng.draws) != 0 {
		t.Fatalf("supported curse result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}

	// Supported nonlethal curse with item-206 retains the debited HP and runs reached tails.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 50, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1000
	rng = &npcJobRNG{t: t, draws: []float64{.1, 0, .9}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	if err != nil || got.NormalAction || got.Choice.ID != "" || len(got.Defeats) != 0 ||
		s.Actors[0].HP != 67 || s.Actors[0].Status != "" || len(rng.draws) != 0 {
		t.Fatalf("supported item-206 curse result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}

	// Non-triggering curse proceeds to normal choice selection with StatusKinju intact.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 100, 100
	s.Actors[0].Status = battle.StatusKinju
	rng = &npcJobRNG{t: t, draws: []float64{.5, 0, .9}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	if err != nil || !got.NormalAction || got.Choice.ID != "job-0-entry-4" ||
		s.Actors[0].HP != 100 || s.Actors[0].Status != battle.StatusKinju ||
		s.Actors[0].Temporary != battle.TemporaryDefense || len(rng.draws) != 0 {
		t.Fatalf("non-triggering curse result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}

	// item-204 bypasses the curse roll (even when drawn < 0.25) without reaching item-206 under defend.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 100, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-204", "item-206"}
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1, 0, .9}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	if err != nil || !got.NormalAction || got.Choice.ID != "job-0-entry-4" ||
		s.Actors[0].HP != 100 || s.Actors[0].Status != battle.StatusKinju ||
		s.Actors[0].Temporary != battle.TemporaryDefense || len(rng.draws) != 0 {
		t.Fatalf("item-204 bypass result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}
}

func TestNPCOperationKinjuPreflightsFatalAndDeathPreventionContinuations(t *testing.T) {
	seed := []battle.CallbackBinding{responseBinding("job-0-entry-4")}

	// Fatal curse still reaches item-206 and rejects missing opponent reset before RNG.
	s, op := runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	rng := &npcJobRNG{t: t, draws: []float64{.1}}
	got, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	after, marshalErr := json.Marshal(s)
	if !errors.Is(err, battle.ErrUnsupportedEffect) || marshalErr != nil || string(after) != string(before) ||
		!reflect.DeepEqual(got, battle.NPCOperationResult{}) || len(rng.draws) != 1 {
		t.Fatalf("fatal item-206 result %+v state changed %t draws %v error %v", got, string(after) != string(before), rng.draws, err)
	}

	// Fatal curse without a defeat context rejects before RNG even without item-206.
	s, op = runNPCFixture(t)
	op.Defeat = nil
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	if _, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng); !errors.Is(err, battle.ErrUnsupportedEffect) || len(rng.draws) != 1 {
		t.Fatalf("fatal missing defeat context draws %v error %v", rng.draws, err)
	}

	// Supported fatal curse completes defeat and post-defeat tails.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1000
	rng = &npcJobRNG{t: t, draws: []float64{.1, .9}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	wantDefeats := []battle.NPCDefeat{{TargetID: "self", KillerID: "self"}}
	if err != nil || got.NormalAction || !reflect.DeepEqual(got.Defeats, wantDefeats) ||
		s.Actors[0].HP != 0 || s.Actors[0].Status != "" || len(rng.draws) != 0 {
		t.Fatalf("supported fatal result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}

	// Death prevention via TemporaryRevival keeps Status cleared and rejects item-206 before RNG.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].Temporary = battle.TemporaryRevival
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	if _, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng); !errors.Is(err, battle.ErrUnsupportedEffect) || len(rng.draws) != 1 {
		t.Fatalf("temporary revival item-206 draws %v error %v", rng.draws, err)
	}

	// Death prevention via item-260 raises Attack by +300 so item-206 can fell a 250 HP enemy.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Attack = 10
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206", "item-260"}
	s.Actors[1].HP = 250
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	if _, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng); !errors.Is(err, battle.ErrUnsupportedEffect) || len(rng.draws) != 1 {
		t.Fatalf("item-260 revival item-206 draws %v error %v", rng.draws, err)
	}

	// Death prevention via item-193 sets StatusDofuu so item-206 is skipped unless burn cures it.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-193", "item-206"}
	s.Actors[0].Gear.AccessoryID = "item-193"
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	got, err = ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng)
	wantConsumed := map[string][]battle.ConsumedItem{"self": {{ID: "item-193", Quantity: 1}}}
	if err != nil || got.NormalAction || !reflect.DeepEqual(got.Consumed, wantConsumed) ||
		len(got.Defeats) != 0 || s.Actors[0].HP != 50 || s.Actors[0].Status != battle.StatusDofuu || len(rng.draws) != 0 {
		t.Fatalf("item-193 revival result %+v actor %+v draws %v error %v", got, s.Actors[0], rng.draws, err)
	}

	s, op = runNPCFixture(t)
	s.Fields.Default = "burn"
	s.Actors[0].HP, s.Actors[0].MaxHP = 10, 100
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-193", "item-206"}
	s.Actors[0].Gear.AccessoryID = "item-193"
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	if _, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng); !errors.Is(err, battle.ErrUnsupportedEffect) || len(rng.draws) != 1 {
		t.Fatalf("item-193 under burn field draws %v error %v", rng.draws, err)
	}

	// High-MaxHP curse straddling [950, 1049] preflights both survivable and fatal continuations.
	s, op = runNPCFixture(t)
	s.Actors[0].HP, s.Actors[0].MaxHP = 1000, 20000
	s.Actors[0].Status = battle.StatusKinju
	s.Actors[0].ItemDefinitionIDs = []string{"item-206"}
	s.Actors[1].HP = 1
	s.Actors[1].Gear.WeaponID = "weapon-1"
	rng = &npcJobRNG{t: t, draws: []float64{.1}}
	if _, err := ApplyNPCOperation(&s, op, seed, registerNPCFixture, rng); !errors.Is(err, battle.ErrUnsupportedEffect) || len(rng.draws) != 1 {
		t.Fatalf("high-MaxHP item-206 draws %v error %v", rng.draws, err)
	}
}

func responseBinding(id string) battle.CallbackBinding {
	return battle.CallbackBinding{CallbackEntry: battle.CallbackEntry{ID: id, Name: id}, Invocation: battle.CallbackInvocation{Kind: "entry", EntryID: id}}
}

func responseCallback(execute func(*battle.LifetimeState, *battle.CallbackContext, random.Generator) error) battle.CallbackDefinition {
	return battle.CallbackDefinition{
		Validate: func(*battle.LifetimeState, *battle.CallbackContext) error { return nil },
		Execute:  execute,
	}
}

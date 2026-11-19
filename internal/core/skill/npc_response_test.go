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

func registerNPCFixture(d *BattleDefinitions, f *battle.CallbackContext) {
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
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "job-405", SP: 1000, OldJobID: "starter"}
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

func TestNPCOperationConfusionChoiceAndExpiryDrawOrder(t *testing.T) {
	for _, row := range []struct {
		draw   float64
		status string
	}{{.32, ""}, {1.0 / 3, battle.StatusConfusion}} {
		s, op := runNPCFixture(t)
		s.Actors[0].Status, s.Actors[0].Temporary = battle.StatusConfusion, battle.TemporaryRegeneration
		seeds := []battle.CallbackBinding{responseBinding("one"), responseBinding("two")}
		rng := &npcJobRNG{t: t, draws: []float64{row.draw, .75, .1}}
		got, err := ApplyNPCOperation(&s, op, seeds, func(d *BattleDefinitions, f *battle.CallbackContext) {
			registerNPCFixture(d, f)
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
		got, err := ApplyNPCOperation(&s, op, []battle.CallbackBinding{responseBinding("pass")}, func(d *BattleDefinitions, f *battle.CallbackContext) {
			registerNPCFixture(d, f)
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
	register := func(d *BattleDefinitions, f *battle.CallbackContext) {
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

func TestNPCOperationLateRejectionRestoresStateAndPorts(t *testing.T) {
	s, op := runNPCFixture(t)
	loadout := &battle.NPCLoadout{CustomName: "custom", Gems: [3]string{"first", "later"}, CustomCost: 5}
	s.Actors[0].NPCLoadout = loadout
	s.Actors[0].Status = battle.StatusConfusion
	level := 20
	op.Damage.RequesterLevel, op.Defeat.RequesterLevel = &level, &level
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var executed bool
	rng := &npcJobRNG{t: t, draws: []float64{0, .9, .9}}
	res, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), func(d *BattleDefinitions, f *battle.CallbackContext) {
		registerNPCFixture(d, f)
		d.Invocations.Entries["first"] = responseCallback(func(s *battle.LifetimeState, c *battle.CallbackContext, _ random.Generator) error {
			executed = true
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
		!reflect.DeepEqual(res, battle.NPCOperationResult{}) {
		t.Fatalf("result %+v executed %t level %d state changed %t error %v", res, executed, level, string(before) != string(after), err)
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

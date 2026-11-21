package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func invocationFixture() (LifetimeState, CallbackContext, CallbackCatalog) {
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "self", TeamID: "party", HP: 100, MP: 20, Attack: 1}, Jobs: BattleJobs{JobID: "job-1"}},
		{Participant: Participant{ID: "ally", TeamID: "party", HP: 100, MP: 20}},
		{Participant: Participant{ID: "enemy", TeamID: "enemy", HP: 100}},
	}}
	c := CallbackContext{TargetContext: TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally"}, EnemyIDs: []string{"enemy"}}, Arguments: []string{"ally", "tail"}}
	d := CallbackCatalog{JobTables: map[int]CallbackTable{}, Entries: map[string]CallbackDefinition{}}
	d.Entries["noop"] = CallbackDefinition{
		Validate: func(*LifetimeState, *CallbackContext) error { return nil },
		Execute:  func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
	}
	for i := 1; i <= 87; i++ {
		if i != 73 {
			d.JobTables[i] = invocationTable(CallbackEntry{ID: "noop", Name: "ordinary"})
		}
	}
	d.JobTables[300] = d.JobTables[1]
	return s, c, d
}

func invocationTable(entries ...CallbackEntry) CallbackTable {
	return CallbackTable{Variants: [][]CallbackEntry{entries}}
}

func TestInvocationTableDrawsRedrawAndDuplicateNames(t *testing.T) {
	s, c, d := invocationFixture()
	var selected []string
	for _, id := range []string{"first", "second"} {
		d.Entries[id] = CallbackDefinition{Validate: d.Entries["noop"].Validate,
			Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
				selected = append(selected, id+":"+c.Arguments[0])
				c.Arguments = c.Arguments[1:]
				return nil
			}}
	}
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "first", Name: "duplicate", RequiredSP: 999, MPCost: 999}, CallbackEntry{ID: "second", Name: "duplicate"}, CallbackEntry{ID: "missing", Extended: true})
	d.JobTables[2] = invocationTable(CallbackEntry{ID: "second", Name: "duplicate"})
	// Generated job82 can reach extended entries, so its full closure needs this too.
	d.Entries["missing"] = d.Entries["noop"]
	rng := &targetRNG{t: t, draws: []float64{0, .99, .5, 0, 0, 1.1 / 87, 0}}
	for range 3 {
		c.Arguments = []string{"ally", "tail"}
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, rng); err != nil {
			t.Fatal(err)
		}
	}
	rng.done()
	if !reflect.DeepEqual(selected, []string{"second:ally", "first:ally", "second:ally"}) || s.actor("self").MP != 20 {
		t.Fatal(selected, s)
	}
}

func TestInvocationClosureRejectsBeforeAnyMutationOrDraw(t *testing.T) {
	for _, edge := range []string{"random", "tool-84", "entry"} {
		s, c, d := invocationFixture()
		d.Entries["outer"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
			Children: []CallbackInvocation{{Kind: edge, EntryID: "missing"}}}
		d.JobTables[1] = invocationTable(CallbackEntry{ID: "outer"})
		// Even a hidden generated job82 child or temporary-job tool child blocks all slots.
		if edge == "random" {
			d.JobTables[2] = invocationTable(CallbackEntry{ID: "noop"}, CallbackEntry{ID: "missing"})
		} else if edge == "tool-84" {
			d.JobTables[300] = invocationTable(CallbackEntry{ID: "missing"})
		}
		d.Entries["mutate"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, _ *CallbackContext, _ random.Generator) error {
			s.actor("self").CMP--
			s.actor("self").ConsumedItems = append(s.actor("self").ConsumedItems, ConsumedItem{})
			return nil
		}}
		before := reloadLifetime(t, s)
		ops := []CallbackInvocation{{Kind: "entry", EntryID: "mutate"}, {Kind: "entry", EntryID: "outer"}}
		if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(edge, s, err)
		}
	}
}

func TestInvocationRandomMPOnlyRestoresActorDecrease(t *testing.T) {
	for _, after := range []int{0, 35} {
		s, c, d := invocationFixture()
		d.Entries["effect"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			s.actor(c.ActorID).MP, s.actor(c.ActorID).HP = after, 0
			s.actor("ally").MP = 3
			s.DroppedCrystals++
			s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "helper", TeamID: "party", HP: 1}})
			c.PartyIDs = append(c.PartyIDs, "helper")
			return nil
		}}
		d.JobTables[1] = invocationTable(CallbackEntry{ID: "effect", MPCost: 999, RequiredSP: 999})
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}, {Kind: "entry", EntryID: "effect"}}, &targetRNG{t: t, draws: []float64{0, 0}}); err != nil {
			t.Fatal(err)
		}
		if s.actor("self").MP != max(20, after) || s.actor("self").HP != 0 || s.actor("ally").MP != 3 || s.DroppedCrystals != 1 || len(s.Actors) != 4 {
			t.Fatal(s)
		}
	}
}

func TestInvocationEmptyAndAllExtendedTables(t *testing.T) {
	s, c, d := invocationFixture()
	d.JobTables[1] = invocationTable()
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, &targetRNG{t: t, draws: []float64{0}}); err != nil {
		t.Fatal(err)
	}
	// Tool84 has no source empty-table success; neither it nor all-ex random is repaired.
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "noop", Extended: true})
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestApplyInvocationsRestoresRetainedReferentsAndHealthyContrast(t *testing.T) {
	setup := func() (LifetimeState, CallbackContext, CallbackCatalog, *string, *JobAssets, *string, *DamageContext, *StatusContext, *DefeatContext, *int) {
		s, c, d := invocationFixture()
		actorHeld, durableHeld := "item-169", "item-169"
		weaponAtk := 12
		s.Actors[0].HeldToolID = &actorHeld
		s.Actors[0].Gear = &BattleGearStats{AccessoryID: "item-169", WeaponAttack: &weaponAtk}
		s.SkillBans = map[string]int{"existing": 1}
		s.SkillHistory = map[string]map[string]bool{"self": {"prior": true}}
		s.StoredToolHistory = map[string]map[string]bool{"party": {"item-001": true}}
		c.Assets = &JobAssets{
			RequesterID:    "self",
			Wallet:         100,
			HeldToolID:     &durableHeld,
			HelperToolIDs:  []int{1},
			StoredTools:    []StoredTool{{InstanceID: "book", DefinitionID: "item-170"}},
			StoredOutcomes: make([]StoredToolOutcome, 0, 2),
		}
		sharedDefeat := &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}}
		c.Defeat = sharedDefeat
		c.Damage = &DamageContext{EffectContext: EffectContext{TargetContext: c.TargetContext}, responseDefeat: sharedDefeat}
		c.Status = &StatusContext{EffectContext: EffectContext{TargetContext: c.TargetContext}, Defeat: sharedDefeat}
		c.Inspection = map[string]InspectionContent{"a": {Empty: true}}
		d.Entries["debit-and-swap"] = CallbackDefinition{
			Validate: func(*LifetimeState, *CallbackContext) error { return nil },
			Execute: func(state *LifetimeState, frame *CallbackContext, _ random.Generator) error {
				a := state.actor(frame.ActorID)
				a.setHeldTool("item-170")
				*a.Gear.WeaponAttack = 99
				state.SkillBans["job-86-entry-3"] = 2
				state.SkillHistory["self"]["item-170"] = true
				state.StoredToolHistory["party"]["item-170"] = true
				frame.Assets.Wallet -= 30
				*frame.Assets.HeldToolID = "item-170"
				frame.Assets.StoredTools = nil
				frame.Assets.StoredOutcomes = append(frame.Assets.StoredOutcomes, StoredToolOutcome{InstanceID: "book", Result: StoredToolExchanged})
				frame.Damage.DamageCalls++
				frame.Status.HealCalls++
				frame.Defeat.Cursor = TargetCursor{ActorID: "enemy"}
				replacementDefeat := *frame.Defeat
				replacementDefeat.Cursor = TargetCursor{ActorID: "other"}
				frame.Status.Defeat = &replacementDefeat
				frame.Damage.responseDefeat = &replacementDefeat
				frame.Inspection["b"] = InspectionContent{Empty: true}
				return nil
			},
		}
		d.Entries["reject-if-debited"] = CallbackDefinition{
			Validate: func(state *LifetimeState, frame *CallbackContext) error {
				if frame.Assets.Wallet == 70 && *state.Actors[0].HeldToolID == "item-170" {
					return ErrUnsupportedEffect
				}
				return nil
			},
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
		}
		return s, c, d, s.Actors[0].HeldToolID, c.Assets, c.Assets.HeldToolID, c.Damage, c.Status, c.Defeat, s.Actors[0].Gear.WeaponAttack
	}

	s, c, d, actorHeld, origAssets, assetsHeld, origDamage, origStatus, origDefeat, weaponAtk := setup()
	origGear := s.Actors[0].Gear
	origActors := s.Actors
	origBans := s.SkillBans
	origSkillInner := s.SkillHistory["self"]
	origStoredHist := s.StoredToolHistory
	origStoredInner := s.StoredToolHistory["party"]
	origInspection := c.Inspection
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "debit-and-swap"}, {Kind: "entry", EntryID: "reject-if-debited"}}
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatalf("err = %v, want ErrUnsupportedEffect", err)
	}
	if *actorHeld != "item-169" || *s.Actors[0].HeldToolID != "item-169" || *origActors[0].HeldToolID != "item-169" {
		t.Fatalf("actor hand leaked: %q %q", *actorHeld, *s.Actors[0].HeldToolID)
	}
	if origGear.AccessoryID != "item-169" || *weaponAtk != 12 {
		t.Fatalf("gear referent leaked: %+v %d", origGear, *weaponAtk)
	}
	if origAssets.Wallet != 100 || *assetsHeld != "item-169" || *origAssets.HeldToolID != "item-169" || len(origAssets.StoredTools) != 1 || len(origAssets.StoredOutcomes) != 0 {
		t.Fatalf("assets referent leaked: %+v %q", origAssets, *assetsHeld)
	}
	if origDamage.DamageCalls != 0 || origStatus.HealCalls != 0 || origDefeat.Cursor.ActorID != "" {
		t.Fatalf("context pointers leaked: %+v %+v %+v", origDamage, origStatus, origDefeat)
	}
	if origDamage.responseDefeat != origDefeat || origStatus.Defeat != origDefeat || c.Damage != origDamage || c.Status != origStatus || c.Defeat != origDefeat {
		t.Fatalf("Defeat sharing broke on restore: dmg=%p status=%p defeat=%p", origDamage.responseDefeat, origStatus.Defeat, origDefeat)
	}
	if len(origBans) != 1 || origBans["existing"] != 1 || origStoredHist["party"]["item-170"] || origStoredInner["item-170"] || !origStoredInner["item-001"] || origSkillInner["item-170"] || !origSkillInner["prior"] || len(origInspection) != 1 {
		t.Fatalf("maps leaked: bans=%v stored=%v innerStored=%v innerSkill=%v insp=%v", origBans, origStoredHist, origStoredInner, origSkillInner, origInspection)
	}

	// Independent non-shared Status.Defeat / Damage.responseDefeat referents also restore.
	s, c, d, _, _, _, origDamage, origStatus, origDefeat, _ = setup()
	statusDefeat := &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext, Cursor: TargetCursor{ActorID: "status-orig"}}}
	respDefeat := &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext, Cursor: TargetCursor{ActorID: "resp-orig"}}}
	c.Status.Defeat = statusDefeat
	c.Damage.responseDefeat = respDefeat
	d.Entries["mutate-independent-defeats"] = CallbackDefinition{
		Validate: func(*LifetimeState, *CallbackContext) error { return nil },
		Execute: func(state *LifetimeState, frame *CallbackContext, _ random.Generator) error {
			state.actor(frame.ActorID).setHeldTool("item-170")
			frame.Assets.Wallet -= 30
			frame.Defeat.Cursor = TargetCursor{ActorID: "mutated-defeat"}
			frame.Status.Defeat.Cursor = TargetCursor{ActorID: "mutated-status-defeat"}
			frame.Damage.responseDefeat.Cursor = TargetCursor{ActorID: "mutated-resp-defeat"}
			return nil
		},
	}
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "entry", EntryID: "mutate-independent-defeats"}, {Kind: "entry", EntryID: "reject-if-debited"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatalf("err = %v, want ErrUnsupportedEffect", err)
	}
	if origDefeat.Cursor.ActorID != "" || statusDefeat.Cursor.ActorID != "status-orig" || respDefeat.Cursor.ActorID != "resp-orig" || origStatus.Defeat != statusDefeat || origDamage.responseDefeat != respDefeat {
		t.Fatalf("independent defeat referents leaked: defeat=%+v status=%+v resp=%+v", origDefeat.Cursor, statusDefeat.Cursor, respDefeat.Cursor)
	}

	// Healthy contrast: caller observes updates through original references.
	s, c, d, actorHeld, origAssets, assetsHeld, origDamage, origStatus, origDefeat, _ = setup()
	if err := s.ApplyInvocations(&c, d, ops[:1], &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if *actorHeld != "item-170" || origAssets.Wallet != 70 || *assetsHeld != "item-170" || len(origAssets.StoredOutcomes) != 1 || origDamage.DamageCalls != 1 || origStatus.HealCalls != 1 || origDefeat.Cursor.ActorID != "enemy" {
		t.Fatalf("healthy contrast missing updates: %q %+v %+v %+v %+v", *actorHeld, origAssets, origDamage, origStatus, origDefeat)
	}
}

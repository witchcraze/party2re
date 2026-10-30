package battle

import (
	"slices"
	"testing"
)

func TestNamedFieldBurnTailSnapshotsAndRole(t *testing.T) {
	for _, row := range []struct {
		before, after                  string
		anti                           int
		npc, enabled, suppressed, burn bool
	}{
		{"burn", "", 0, false, true, false, true}, {"", "burn", 0, false, true, false, true}, {"burn", "burn", 1, false, true, false, false},
		{"burn", "burn", 0, false, true, true, false}, {"burn", "burn", 0, false, false, false, false}, {"burn", "burn", 0, true, false, true, true},
	} {
		s, c := namedFieldFixture()
		a := s.actor("self")
		a.HP, a.MaxHP = 3, 3000
		a.Temporary, a.Status, a.Extended, a.ExtendedTurns = TemporaryDefense, StatusSleep, ExtendedAwakening, 8
		s.Fields = LifetimeFields{Default: row.after, AntiTurns: row.anti}
		if err := s.ApplyNamedFieldTail(&c, NamedFieldTail{Before: row.before, NPC: row.npc, ResponseEnabled: row.enabled, Suppressed: row.suppressed}, &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		if row.burn && (a.HP != 1 || a.Status != "" || a.Temporary != "" || a.Extended != "" || a.ExtendedTurns != 0) || !row.burn && a.HP != 3 {
			t.Fatal(row, a)
		}
	}
}

func TestNamedFieldGraveSnapshotRestoreAndRecreate(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, roster := rosterFixture()
		helper := addRosterHelper(t, &s, &roster, 64)
		h := s.actor(helper)
		h.Agility = 17
		g := s.SnapshotGrave()
		h.HP, h.MP, h.TeamID = 0, 2, "enemy"
		h.RosterDetached = true
		c := NamedFieldContext{DamageContext: roster.DamageContext, Roster: &roster}
		s.Fields.Default = "grave"
		if err := s.ApplyNamedFieldTail(&c, NamedFieldTail{ResponseEnabled: true, NPC: npc, Grave: g}, &targetRNG{t: t}); err != nil || h.HP != int(g.Stats[0]) || h.TeamID != "party" || (!npc && h.MP != int(g.Stats[1])) || (npc && h.MP != 2) {
			t.Fatal(err, h, g)
		}
		s.Actors = slices.DeleteFunc(s.Actors, func(a LifetimeActor) bool { return a.ID == helper })
		c.PartyIDs = slices.DeleteFunc(slices.Clone(c.PartyIDs), func(id string) bool { return id == helper })
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0}}
		if err := s.ApplyNamedFieldTail(&c, NamedFieldTail{ResponseEnabled: true, NPC: npc, Grave: g}, rng); err != nil {
			t.Fatal(err)
		}
		rng.done()
		newHelper := s.rosterIdentity(callingIdentity(64))
		if newHelper.ID == helper || newHelper.Agility != 15 || newHelper.HP != int(g.Stats[0]*.9) || newHelper.MP != int(g.Stats[1]*.9) || newHelper.Icon != g.Icon {
			t.Fatal(newHelper, g)
		}
	}
}

func TestNamedFieldLibraryFallenAndNPCHaveNoRecoveryDraw(t *testing.T) {
	for _, row := range []struct {
		npc   bool
		pick  float64
		mp    int
		draws []float64
	}{
		{false, .21, 0, []float64{.21}}, {false, 0, 30, []float64{0, 0}}, {true, 0, 3, nil},
	} {
		s, c := namedFieldFixture()
		s.actor("self").MaxMP = 1000
		s.Fields.Default = "library"
		rng := &targetRNG{t: t, draws: row.draws}
		if err := s.ApplyNamedFieldTail(&c, NamedFieldTail{ResponseEnabled: true, NPC: row.npc}, rng); err != nil {
			t.Fatal(err)
		}
		rng.done()
		if row.pick == 0 && s.actor("self").MP != row.mp {
			t.Fatal(row, s)
		}
	}
}

func TestNamedFieldIncenseUsesFirstHolderAndRestoresTension(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("self").Extended, s.actor("self").ExtendedTurns, s.actor("self").Tension = ExtendedIncense, 1, 5
	s.actor("ally").Extended, s.actor("ally").ExtendedTurns = ExtendedIncense, 2
	s.actor("self").ItemDefinitionIDs = []string{"item-146"}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
	if err := s.ApplyIncenseHealing(&c.EffectContext, rng); err != nil || s.actor("self").HP != 424 || s.actor("ally").HP != 262 || s.actor("self").Tension != 5 || c.HealCalls != 3 {
		t.Fatal(err, s, c)
	}
	rng.done()
	if first := s.TickIncense(c.PartyIDs); first != "self" || s.actor("self").Extended != "" || s.actor("ally").ExtendedTurns != 1 {
		t.Fatal(s, first)
	}
}

func TestNamedFieldFortressPureProjectionAndToolCap(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("self").HP, s.actor("self").MaxHP = 5000, 100
	s.actor("ally").HP, s.actor("ally").MaxHP = 1, 100
	s.actor("fallen").HP, s.actor("fallen").MaxHP = 0, 100
	field := FortressDefeat(&targetRNG{t: t, draws: []float64{0, 0}})
	copy := s.callbackCopy()
	if err := field.Project(&copy, c.TargetContext, "fallen"); err != nil || copy.actor("self").HP != 100 || copy.actor("ally").HP != 1 || s.actor("self").HP != 5000 {
		t.Fatal(err, s, copy)
	}
	if err := field.Apply(&s, c.TargetContext, "fallen"); err != nil || s.actor("self").HP != 100 || s.actor("ally").HP != 19 || s.actor("fallen").HP != 0 {
		t.Fatal(err, s)
	}
}

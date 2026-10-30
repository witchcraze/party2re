package battle

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestPlayerTailValidationLeavesNestedFactsAndExternalHooksUntouched(t *testing.T) {
	for _, id := range []string{"item-194-combat-tail", "frenzy-tail"} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.ItemDefinitionIDs, a.Attack, a.Extended = []string{"item-194", "weapon-13"}, 0, ExtendedFrenzy
		a.Gear.AccessoryID = "item-194"
		raw := 5
		a.Gear.WeaponID, a.Gear.WeaponAttack = "weapon-13", &raw
		c.DefeatContext.Seals = &WeaponSealTable{DragonAttack: 30, PhoenixWeight: -20}
		c.Cursor = TargetCursor{Source: "item150", ActorID: "ally"}
		applies := 0
		c.DefeatContext.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { applies++; return nil }
		before := s.callbackCopy()
		level, seals, cursor, target := *c.RequesterLevel, *c.DefeatContext.Seals, c.DefeatContext.Cursor, c.DefeatContext.TargetContext.ActorID
		rng := &targetRNG{t: t}
		if err := s.ValidatePlayerTail(&c, id, rng); err != nil || !reflect.DeepEqual(before, s) || *c.RequesterLevel != level || *c.DefeatContext.Seals != seals || c.DefeatContext.Cursor != cursor || c.DefeatContext.TargetContext.ActorID != target || raw != 5 || applies != 0 {
			t.Fatal(id, err, s, c)
		}
		rng.done()
	}
}

func TestPlayerTailNPCNeverUsesPlayerProducers(t *testing.T) {
	for _, provenance := range []string{"completion", "actor", "effect"} {
		s, c := playerTailFixture()
		s.actor("self").ItemDefinitionIDs = []string{"item-189", "item-194", "item-202"}
		c.NormalAction, c.Item210Fallen = true, []string{"ally"}
		switch provenance {
		case "completion":
			c.Completion.NPC = true
		case "actor":
			s.actor("self").NPC = true
		case "effect":
			c.ActorNPC = true
		}
		before := s.callbackCopy()
		for _, id := range PlayerTailEntries() {
			if err := s.ApplyPlayerTail(&c, id, &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
				t.Fatal(id, err, s)
			}
		}
	}
}

func TestPlayerTailCMPOriginalLevelAndOddBoostRounding(t *testing.T) {
	for _, level := range []int{0, 1, 99, 150} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.ItemDefinitionIDs, a.CMPCharge = []string{"item-238"}, a.CMPNeedCharge
		c.RequesterLevel = &level
		rng := &targetRNG{t: t, draws: []float64{math.Nextafter(1, 0), math.Nextafter(1, 0)}}
		want := 1
		if level >= 99 {
			want = 60
		}
		if err := s.ApplyPlayerTail(&c, "cmp-recharge-tail", rng); err != nil || c.Completion.CMPAmount != want || c.Completion.NextCMPThreshold != 9 {
			t.Fatal(level, err, c)
		}
		rng.done()
	}
}

func TestPlayerTailSnapshotSuppressionAndInvalidMemberGate(t *testing.T) {
	s, c := playerTailFixture()
	c.Item210Fallen, c.Completion.Suppressed = []string{"ally"}, true
	before := s.callbackCopy()
	if err := s.ApplyPlayerTail(&c, "item-210-revival-tail", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
	c.Item210Fallen, c.Completion.Suppressed = []string{"ally", "missing"}, false
	if err := s.ApplyPlayerTail(&c, "item-210-revival-tail", &targetRNG{t: t}); !errors.Is(err, ErrInvalidParticipant) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

func TestPlayerTailWeaponChanceBoundaryAndWrongArmor(t *testing.T) {
	for _, armor := range []string{"armor-54", "armor-55"} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.ItemDefinitionIDs = []string{"weapon-69", armor}
		draws := []float64{}
		if armor == "armor-54" {
			draws = []float64{1.0 / 6}
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyPlayerTail(&c, "weapon-69-armor-54-tail", rng); err != nil || a.Tension != 1 || c.TensionCalls != 0 {
			t.Fatal(err, a)
		}
		rng.done()
	}
}

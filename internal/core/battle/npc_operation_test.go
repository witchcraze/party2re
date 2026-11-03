package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestNPCResponseCureSkipsActionButReachesFieldAndItemTails(t *testing.T) {
	s, c := equipmentTailFixture(true, 1)
	a := s.actor("self")
	a.Status, a.ItemDefinitionIDs = StatusSleep, []string{"item-202"}
	s.Fields.Default = "burn"
	var err error
	c.Snapshot, err = s.CaptureEquipmentTail(c.TargetContext)
	if err != nil {
		t.Fatal(err)
	}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
	decision, err := s.BeginNPCAction(&c, rng)
	if err != nil || decision.NormalAction || a.Status != "" {
		t.Fatal(decision, a, err)
	}
	// Reload after the cure-only branch; no normal callback, poison or regen ran.
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	field := &NamedFieldContext{DamageContext: c.DamageContext}
	if err := loaded.FinishNPCTails(&c, field, NamedFieldTail{Before: "burn"}, rng); err != nil {
		t.Fatal(err)
	}
	if loaded.actor("self").HP != 28 || loaded.Fields.Current() != "burn" {
		t.Fatal(loaded)
	}
	rng.done()
}

func TestNPCNormalBlockPoisonAndRegenerationOrder(t *testing.T) {
	for _, row := range []struct {
		status string
		want   int
		draws  []float64
	}{{StatusPoison, 810, []float64{0, .9}}, {StatusDeadlyPoison, 850, []float64{0}}, {StatusVirulentPoison, 850, []float64{0}}} {
		s, c := equipmentTailFixture(true, 1)
		a := s.actor("self")
		a.MaxHP, a.HP = 900, 850
		a.Status, a.Temporary = row.status, TemporaryRegeneration
		rng := &targetRNG{t: t, draws: row.draws}
		if err := s.FinishNPCNormalAction(&c, rng); err != nil || a.HP != row.want {
			t.Fatal(row, a, err)
		}
		rng.done()
	}
	// Ordinary NPCs never run the player's other-ally virulent-poison pass.
	s, c := equipmentTailFixture(true, 2)
	s.actor("ally-1").Status = StatusVirulentPoison
	if err := s.FinishNPCNormalAction(&c, &targetRNG{t: t}); err != nil || s.actor("ally-1").HP != 100 {
		t.Fatal(s, err)
	}
}

func TestNPCActualConfusionAndPurificationBoundaries(t *testing.T) {
	s, c := equipmentTailFixture(true, 1)
	a := s.actor("self")
	a.Status = StatusConfusion
	rng := &targetRNG{t: t, draws: []float64{.25, .9}}
	d, err := s.BeginNPCAction(&c, rng)
	if err != nil || !d.NormalAction || a.Status != "" {
		t.Fatal(d, a, err)
	}
	rng.done()
	player := *a
	player.Status = StatusConfusion
	if _, err := player.CheckIncapacitation(IncapacitationContext{}, &targetRNG{t: t, draws: []float64{.25}}); err != nil || player.Status == "" {
		t.Fatal(player, err)
	}
	a.Status, a.Extended, a.ExtendedTurns = StatusSleep, ExtendedPurification, 1
	rng = &targetRNG{t: t, draws: []float64{.9}}
	d, err = s.BeginNPCAction(&c, rng)
	if err != nil || !d.NormalAction || a.Status != "" {
		t.Fatal(d, a, err)
	}
	field := &NamedFieldContext{DamageContext: c.DamageContext}
	if err := s.FinishNPCTails(&c, field, NamedFieldTail{}, rng); err != nil || a.Extended != "" || a.ExtendedTurns != 0 {
		t.Fatal(a, err)
	}
	rng.done()
}

func TestOrdinarySpecialBossFieldCapsAndResetExcludesDefault(t *testing.T) {
	s, c := equipmentTailFixture(false, 2)
	for i := 0; i < 2; i++ {
		a := &s.Actors[i]
		a.MaxHP, a.MaxMP = 3000, 4000
		a.HP, a.MP = 2100, 2200
		a.BaseStats = BattleBaseStats{600, 700, 800}
		a.OverLevel = i == 1
		a.Status, a.Temporary, a.Extended = StatusPoison, TemporaryRegeneration, ExtendedBow
		a.Tension = 8
	}
	reset := &EquipmentResetContext{TargetContext: c.TargetContext}
	if err := s.ApplySpecialBossField(reset, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		a := s.Actors[i]
		rate := i + 1
		if a.MaxHP != 999*rate || a.MaxMP != 999*rate || a.Attack != 255*rate || a.Defense != 255*rate || a.Agility != 255*rate || a.Status != "" || a.Extended != "" || a.Temporary != "" || a.Tension != 1 || a.HP != 2100 || a.MP != 2200 {
			t.Fatal(a)
		}
	}
	if !s.SpecialBossField || s.Fields.Default != "" || s.actor("enemy").MaxHP != 1000 {
		t.Fatal("ordinary marker incorrectly became an EX default field or capped NPCs")
	}
	// An unsupported reset must reject before applying any cap or marker.
	s, c = equipmentTailFixture(false, 1)
	s.actor("self").Gear.AccessoryID = "item-110"
	before := s.callbackCopy()
	if err := s.ApplySpecialBossField(&EquipmentResetContext{TargetContext: c.TargetContext}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, err)
	}
}

func TestNPCPoisonAndProducerGatesPrecedeMutation(t *testing.T) {
	s, c := equipmentTailFixture(true, 1)
	a := s.actor("self")
	a.Status, a.HP = StatusPoison, 10
	before := s.callbackCopy()
	rng := &targetRNG{t: t}
	if err := s.FinishNPCNormalAction(&c, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, err)
	}
	a.Status = StatusKinju
	before = s.callbackCopy()
	if _, err := s.BeginNPCAction(&c, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, err)
	}
	a.Status = ""
	a.Abilities = []string{"specter"}
	before = s.callbackCopy()
	if _, err := s.BeginNPCAction(&c, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, err)
	}
	rng.done()
}

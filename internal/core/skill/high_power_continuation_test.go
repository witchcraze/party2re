package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHighPowerReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := highPowerFixture()
	s.Actors[1].Tension = 3
	executeHighPower(t, &s, c, d, []string{"job-48-entry-5"}, nil)
	type checkpoint struct {
		State          corebattle.LifetimeState
		Target         corebattle.TargetContext
		DamageCalls    int
		RequesterLevel int
		End            corebattle.EndProcessing
		Reset          corebattle.EquipmentResetContext
		Arguments      []string
		EntryID        string
	}
	level := 99
	c.RequesterLevel = &level
	c.Command = "frozen-command"
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, level, c.End, *c.EquipmentReset, []string{"enemy", "tail"}, "job-47-entry-2"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.HighPowerContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, RequesterLevel: &loaded.RequesterLevel}, End: loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil }}, EquipmentReset: &loaded.Reset}
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterHighPower(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.HighPowerContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeHighPower(t, run.s, run.c, run.d, []string{loaded.EntryID}, loaded.Arguments, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 819 || s.Actors[0].Attack != 90 || s.Actors[1].Tension != 3 || c.DamageCalls != 1 || resumed.DamageCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || resumed.Command != "frozen-command" || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestHighPowerReturnedTargetAliasing(t *testing.T) {
	for _, row := range []struct {
		id              string
		hp, at, df, hit int
		magic           bool
		draws           []float64
	}{
		{"job-47-entry-2", 441, 90, 80, 95, false, []float64{0}},
		{"job-47-entry-3", 140, 100, 80, 95, false, []float64{0, 0}},
		{"job-48-entry-2", 230, 100, 80, 88, true, []float64{0, 0}},
		{"job-52-entry-2", 500, 100, 45, 95, false, []float64{0}},
	} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := highPowerFixture()
			s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
			if row.magic {
				s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
			}
			executeHighPower(t, &s, c, d, []string{row.id}, []string{"enemy", "tail"}, row.draws...)
			a := s.Actors[0]
			if a.HP != row.hp || a.Attack != row.at || a.Defense != row.df || a.Hit != row.hit || s.Actors[1].HP != 900 {
				t.Fatal(a, s.Actors[1])
			}
		})
	}
}

func TestHighPowerAreaStatusContinuesAfterCasterDeath(t *testing.T) {
	s, c, d := highPowerFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP = 1
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeHighPower(t, &s, c, d, []string{"job-48-entry-0"}, nil, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[3].HP != 900 || s.Actors[3].Status != corebattle.StatusDeadlyPoison || c.DamageCalls != 1 || c.AreaStatusCalls != 1 || c.StatusCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestHighPowerResetPreservesCursorAndAccessoryOrder(t *testing.T) {
	s, c, d := highPowerFixture()
	s.Actors[2].Gear.AccessoryID = "item-159"
	s.Actors[2].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 50, Agility: 20}
	s.Actors[2].Tension = 5
	executeHighPower(t, &s, c, d, []string{"job-48-entry-5"}, nil, 0, 0, 0)
	if s.Actors[2].Attack != 110 || s.Actors[2].Tension != 5 || s.Actors[2].HP != 1 || c.Cursor.Source != "item159" || c.Cursor.ActorID != "fallen" || c.EquipmentReset.Cursor != c.Cursor {
		t.Fatal(s, c)
	}
}

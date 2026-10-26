package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBasicMagicReloadFrozenInputWithoutReplay(t *testing.T) {
	s, c, d := basicMagicFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-basic-magic", true
	c.Cursor = corebattle.TargetCursor{Source: "item159", ActorID: "self"}
	s.SpecialBossField = true
	s.Actors[0].Status = corebattle.StatusPoison
	executeBasicMagic(t, &s, c, d, "job-5-entry-2", []string{"self"})
	executeBasicMagic(t, &s, c, d, "job-6-entry-5", []string{"enemy"}, 0)
	type checkpoint struct {
		State                  corebattle.LifetimeState
		Target                 corebattle.TargetContext
		DamageCalls, CureCalls int
		RequesterLevel         int
		Cursor                 corebattle.TargetCursor
		Entry                  string
		Arguments              []string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.CureCalls, level, c.Cursor, "job-5-entry-5", []string{"self", "tail"}}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.BasicMagicContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, RequesterLevel: &loaded.RequesterLevel},
		Cursor:        loaded.Cursor, Defeat: func(string) error { t.Fatal("unexpected defeat"); return nil },
	}, CureCalls: loaded.CureCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterBasicMagic(resumed)
	executeBasicMagic(t, &s, c, d, loaded.Entry, loaded.Arguments, 0)
	executeBasicMagic(t, &loaded.State, resumed, rebuilt, loaded.Entry, loaded.Arguments, 0)
	// Damage1 reduces the later base90 heal to45, then the .9 roll truncates40.
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 540 || s.Actors[0].Status != "" || s.Actors[1].HP != 837 || resumed.DamageCalls != 1 || resumed.CureCalls != 1 || resumed.HealCalls != 1 || resumed.Command != "frozen-basic-magic" || !resumed.Custom || *resumed.RequesterLevel != 99 || resumed.Cursor != c.Cursor || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || loaded.Arguments[0] != "self" {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestBasicMagicDelegatedExtendedThresholdsAndCosts(t *testing.T) {
	for _, row := range []struct {
		jobID     string
		sp, mp    int
		remaining int
		damage    int
		draws     []float64
	}{
		{"job-05", 99, 23, 23, 0, nil},
		{"job-05", 100, 22, 22, 0, nil},
		{"job-05", 100, 23, 0, 72, []float64{0, 0}},
		{"job-06", 89, 40, 40, 0, nil},
		{"job-06", 90, 39, 39, 0, nil},
		{"job-06", 90, 40, 0, 540, []float64{0}},
	} {
		for _, previous := range []bool{false, true} {
			s, c, d := basicMagicFixture()
			s.Actors[0].MP = row.mp
			if previous {
				s.Actors[0].Jobs.OldJobID, s.Actors[0].Jobs.OldSP = row.jobID, row.sp
			} else {
				s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = row.jobID, row.sp
			}
			d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
			executeBasicMagic(t, &s, c, d, "extended", []string{"enemy", "tail"}, row.draws...)
			if s.Actors[0].MP != row.remaining || s.Actors[1].HP != 900-row.damage {
				t.Fatal(row, previous, s, c)
			}
		}
	}
	// Direct/indirect bodies do not charge listed costs. Ordinary owners do so;
	// delegated extended invokes the same body and then charges its own cost.
	s, c, d := basicMagicFixture()
	s.Actors[0].MP = 0
	executeBasicMagic(t, &s, c, d, "job-6-entry-9", []string{"enemy", "tail"}, 0)
	if s.Actors[0].MP != 0 || s.Actors[1].HP != 360 || len(c.Arguments) != 1 {
		t.Fatal(s, c)
	}
}

func TestBasicMagicEarlierAreaHitChangesLaterTargetEligibility(t *testing.T) {
	s, c, d := basicMagicFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, other.ID)
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP = 100
	// The first hit activates item195; the second helper confuses the caster
	// and its two roster draws finally select the ally instead of the enemy.
	executeBasicMagic(t, &s, c, d, "job-5-entry-4", nil, 0, 0, 0, .6, .6, 0)
	if s.Actors[0].Status != corebattle.StatusConfusion || s.Actors[1].HP != 891 || s.Actors[2].HP != 93 || s.Actors[3].HP != 900 || !c.Item195Activated || c.DamageCalls != 2 {
		t.Fatal(s, c)
	}
}

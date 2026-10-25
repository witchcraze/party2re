package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestProtectiveMagicEveryPositionGuard(t *testing.T) {
	_, _, catalog := protectiveMagicFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := protectiveMagicFixture()
			a := &s.Actors[0]
			a.Status = corebattle.StatusMagicSeal
			args, draws := []string{"self", "tail"}, []float64{.9}
			switch id {
			case "job-32-entry-0":
				args[0], draws = "enemy", []float64{.25, .9}
			case "job-34-entry-0":
				a.Status = corebattle.StatusPhysicalSeal
			case "job-33-entry-7":
				// An empty cure tuple still grants the five-submission effect.
				a.Status, draws = "", nil
			case "job-34-entry-2":
				// Untyped meditation bypasses magic seal, including at full HP.
				a.HP, draws = 1000, []float64{0}
			}
			executeProtectiveMagic(t, &s, c, d, id, args, draws...)
			if a.MP != 80 || s.Actors[2].MP != 15 || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.StatUpCalls != 0 || c.PartyHealCalls != 0 || c.CureCalls != 0 || a.Temporary != "" || s.Actors[1].HP != 900 {
				t.Fatal(id, s, c)
			}
			if id == "job-33-entry-7" && (a.Extended != corebattle.ExtendedAdditional || a.ExtendedTurns != 5) {
				t.Fatal(a)
			}
			if id == "job-34-entry-2" && (a.HP != 1000 || c.HealCalls != 1 || a.Status != corebattle.StatusMagicSeal) {
				t.Fatal(a, c)
			}
		})
	}
}

func TestProtectiveMagicBranchAndBarrierBoundaries(t *testing.T) {
	for _, row := range []struct {
		draw   float64
		damage int
	}{{.249999, 90}, {.25, 27}, {.99999, 27}} {
		s, c, d := protectiveMagicFixture()
		executeProtectiveMagic(t, &s, c, d, "job-32-entry-0", []string{"enemy", "tail"}, row.draw, 0)
		if s.Actors[1].HP != 900-row.damage || len(c.Arguments) != 1 || c.Arguments[0] != "tail" {
			t.Fatal(s, c)
		}
	}
	for id, tmp := range map[string]corebattle.TemporaryEffect{
		"job-32-entry-2": corebattle.TemporaryReduceAttack,
		"job-32-entry-3": corebattle.TemporaryReflectMagic,
		"job-33-entry-0": corebattle.TemporaryReduceMagic,
		"job-33-entry-2": corebattle.TemporaryReduceBreath,
		"job-34-entry-4": corebattle.TemporaryNullMagic,
	} {
		for _, hp := range []int{0, 1} {
			s, c, d := protectiveMagicFixture()
			s.Actors[2].HP, s.Actors[2].Temporary = hp, corebattle.TemporaryDefense
			s.Actors[0].Status, s.Actors[0].Temporary = corebattle.StatusMagicSeal, corebattle.TemporaryRage
			executeProtectiveMagic(t, &s, c, d, id, nil, .249999)
			want := corebattle.TemporaryDefense
			if hp > 0 && id != "job-34-entry-4" {
				want = tmp
			}
			if s.Actors[0].Status != "" || s.Actors[0].Temporary != tmp || s.Actors[2].Temporary != want {
				t.Fatal(id, hp, s)
			}
		}
	}
}

func TestProtectiveMagicGroupMPBoundaries(t *testing.T) {
	for _, hp := range []int{0, 100} {
		for _, mp := range []int{14, 15, 16} {
			t.Run(fmt.Sprintf("hp%d-mp%d", hp, mp), func(t *testing.T) {
				s, c, d := protectiveMagicFixture()
				s.Actors[2].HP, s.Actors[2].MP = hp, mp
				executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy", "tail"}, 0)
				wantMP, wantHP := mp, 810
				if mp >= 15 {
					wantMP, wantHP = mp-15, 734
				}
				if s.Actors[2].MP != wantMP || s.Actors[1].HP != wantHP || s.Actors[0].MP != 80 || c.DamageCalls != 1 || len(c.Arguments) != 1 {
					t.Fatal(s, c)
				}
			})
		}
	}
	for _, absent := range []bool{false, true} {
		s, c, d := protectiveMagicFixture()
		c.Defeat = nil // A skipped hit needs no damage descendant.
		if absent {
			s.Actors = append(s.Actors[:1:1], s.Actors[2])
			c.EnemyIDs = nil
		} else {
			s.Actors[1].HP = 0
		}
		executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy", "tail"})
		if s.Actors[len(s.Actors)-1].MP != 15 || c.DamageCalls != 0 {
			t.Fatal(s, c)
		}
	}
}

func TestProtectiveMagicCoverAndStatGuards(t *testing.T) {
	for _, target := range []string{"self", "fallen", "ally"} {
		s, c, d := protectiveMagicFixture()
		if target == "ally" {
			s.Actors[2].HP = 100
			target = "fallen"
		}
		executeProtectiveMagic(t, &s, c, d, "job-34-entry-0", []string{target, "tail"})
		wantSelf, wantAlly := corebattle.TemporaryEffect(""), corebattle.TemporaryEffect("")
		if s.Actors[2].HP > 0 {
			wantSelf, wantAlly = corebattle.TemporaryCovering, corebattle.TemporaryCover
		}
		if s.Actors[0].Temporary != wantSelf || s.Actors[2].Temporary != wantAlly || len(c.Arguments) != 1 {
			t.Fatal(s, c)
		}
	}
	// Base AT400 gives a1000 cap; current999 can rise,1000 returns no effect.
	for _, attack := range []int{999, 1000} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].BaseStats.Attack, s.Actors[0].Attack, s.Actors[0].Tension = 400, attack, 2
		executeProtectiveMagic(t, &s, c, d, "job-33-entry-4", []string{"self"})
		wantTension := 1.0
		if attack == 1000 {
			wantTension = 2
		}
		if s.Actors[0].Attack != 1000 || s.Actors[0].Tension != wantTension || c.StatUpCalls != 1 {
			t.Fatal(s, c)
		}
	}
}

func TestProtectiveMagicCureAndFiveSubmissions(t *testing.T) {
	s, c, d := protectiveMagicFixture()
	// Captured poison is compared against the redirected recipient's status.
	s.Actors[0].Status, s.Actors[0].Temporary = corebattle.StatusPoison, corebattle.TemporaryCover
	s.Actors[2].HP, s.Actors[2].Status, s.Actors[2].Temporary = 100, corebattle.StatusPoison, corebattle.TemporaryCovering
	executeProtectiveMagic(t, &s, c, d, "job-33-entry-7", nil)
	if s.Actors[0].Status != corebattle.StatusPoison || s.Actors[2].Status != "" || c.CureCalls != 1 {
		t.Fatal(s, c)
	}
	for i := 1; i <= 5; i++ {
		expired := s.Actors[0].FinishExtended(corebattle.LifetimeTail{SecondSubmitted: true})
		if s.Actors[0].ExtendedTurns != 5-i || expired != (i == 5) || (s.Actors[0].Extended == "") != (i == 5) {
			t.Fatal(i, s.Actors[0])
		}
	}
}

func TestProtectiveMagicPartyHealingOrderAndCap(t *testing.T) {
	for _, row := range []struct {
		id             string
		selfHP, allyHP int
	}{
		{"job-32-entry-1", 576, 145},
		{"job-33-entry-3", 683, 208},
		{"job-34-entry-5", 1000, 999},
	} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].Tension = 1.7
		s.Actors[2].HP, s.Actors[2].MaxHP = 100, 1000
		executeProtectiveMagic(t, &s, c, d, row.id, []string{"unused"}, 0, 0)
		if s.Actors[0].HP != row.selfHP || s.Actors[2].HP != row.allyHP || s.Actors[0].Tension != 1 || c.HealCalls != 2 || c.PartyHealCalls != 1 || len(c.Arguments) != 1 {
			t.Fatal(row.id, s, c)
		}
	}
	s, c, d := protectiveMagicFixture()
	s.Actors[2].HP, s.Actors[2].MaxHP = 101, 1000
	executeProtectiveMagic(t, &s, c, d, "job-34-entry-5", nil, 0, 0)
	if s.Actors[2].HP != 1000 {
		t.Fatal(s)
	}
}

func TestProtectiveMagicCureCapturesStatusBeforeConfusion(t *testing.T) {
	s, c, d := protectiveMagicFixture()
	c.Item195Activated = true
	executeProtectiveMagic(t, &s, c, d, "job-33-entry-7", []string{"unused", "tail"}, 0, .4)
	if s.Actors[0].Status != corebattle.StatusConfusion || s.Actors[0].ExtendedTurns != 5 || c.CureCalls != 0 || len(c.Arguments) != 2 {
		t.Fatal(s, c)
	}
}

func TestProtectiveMagicAreaDamageStopsAfterCasterDeath(t *testing.T) {
	for _, id := range []string{"job-32-entry-5", "job-33-entry-1", "job-33-entry-5", "job-34-entry-3"} {
		s, c, d := protectiveMagicFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
		executeProtectiveMagic(t, &s, c, d, id, []string{"unused"}, 0)
		if s.Actors[0].HP != 0 || s.Actors[1].HP != 900 || s.Actors[3].HP != 900 || c.DamageCalls != 1 || c.AreaDamageCalls != 1 {
			t.Fatal(id, s, c)
		}
	}
}

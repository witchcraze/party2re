package skill

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestWeaponWeatherStanceEveryPositionGuardBoundary(t *testing.T) {
	_, _, catalog := weaponWeatherStanceFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := weaponWeatherStanceFixture()
			args := []string{"enemy", "tail"}
			c.Defeat, c.StatusDefeat = nil, nil
			s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
			draws := []float64(nil)
			switch id {
			case "job-24-entry-2":
				args[0], s.Actors[0].Attack = "self", 250
			case "job-24-entry-5":
				s.Actors[0].HP, s.Actors[0].Status = 0, corebattle.StatusMagicSeal
			case "job-27-entry-0":
				s.Actors[1].Hit = 50
			case "job-27-entry-2":
				s.Actors[1].Agility = 4
			case "job-27-entry-4":
				s.Actors[0].HP = 0
			case "job-27-entry-5":
				s.Actors[0].Status = corebattle.StatusDofuu
				s.Actors[2].Temporary = corebattle.TemporaryRage
			case "job-27-entry-7":
				s.Actors[1].HP = 0
			case "job-27-entry-8":
				s.Actors[0].HP = 0
			case "job-28-entry-0", "job-28-entry-2", "job-28-entry-4":
				s.Actors[0].HP, s.Actors[0].Status = 0, corebattle.StatusPhysicalSeal
			case "job-28-entry-1", "job-28-entry-5", "job-28-entry-6":
				s.Actors[1].HP = 1000 // HP999/1000 is the shared immunity boundary.
			}
			e := s.Actors[1]
			executeWeaponWeatherStance(t, &s, c, d, id, args, draws...)
			if s.Actors[1].HP != e.HP || s.Actors[1].Attack != e.Attack || s.Actors[1].Hit != e.Hit || s.Actors[1].Agility != e.Agility || s.Actors[1].Status != e.Status {
				t.Fatal(s, c)
			}
			switch id {
			case "job-24-entry-2":
				if s.Actors[0].Attack != 250 || c.StatUpCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-24-entry-5":
				if s.Actors[0].ExtendedTurns != 7 || s.Actors[0].Extended != corebattle.ExtendedMagicSword {
					t.Fatal(s)
				}
			case "job-27-entry-4":
				if c.CureCalls != 0 || c.PartyCureCalls != 1 {
					t.Fatal(c)
				}
			case "job-27-entry-5":
				if s.Actors[0].Temporary != corebattle.TemporaryReflectBreath || s.Actors[2].Temporary != corebattle.TemporaryRage || s.Actors[0].Status != corebattle.StatusDofuu {
					t.Fatal(s)
				}
			case "job-27-entry-8":
				if c.PartyHealCalls != 1 || c.HealCalls != 0 || c.CureCalls != 0 {
					t.Fatal(c)
				}
			case "job-28-entry-0":
				if s.Actors[0].Hit != 95 {
					t.Fatal(s)
				}
			case "job-28-entry-2", "job-28-entry-4":
				want := corebattle.TemporaryDeflect
				if id == "job-28-entry-4" {
					want = corebattle.TemporaryNullAttack
				}
				if s.Actors[0].Temporary != want || s.Actors[0].Status != corebattle.StatusPhysicalSeal {
					t.Fatal(s)
				}
			}
		})
	}
}

func TestWeaponWeatherStanceWeaponBreakBaseAndImmunity(t *testing.T) {
	for _, row := range []struct {
		name           string
		hp, base, want int
		tension        float64
		item           string
	}{
		{"ordinary", 999, 50, 40, 1, ""},
		{"actor-tension", 999, 50, 20, 3, ""},
		{"hp-immune", 1000, 50, 50, 1, ""},
		{"item-immune", 999, 50, 50, 1, "item-159"},
		{"rounding", 999, 1, 1, 1, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := weaponWeatherStanceFixture()
			s.Actors[0].Tension = row.tension
			s.Actors[1].HP, s.Actors[1].BaseStats.Attack = row.hp, row.base
			s.Actors[1].Attack, s.Actors[1].Tension, s.Actors[1].Temporary = 999, 5, corebattle.TemporaryReflectMagic
			s.Actors[1].ItemDefinitionIDs = []string{row.item}
			executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-7", []string{"enemy"})
			if e := s.Actors[1]; e.Attack != row.want || e.Tension != 1 || e.Temporary != "" || e.Defense != 80 {
				t.Fatal(e)
			}
		})
	}
}

func TestWeaponWeatherStanceAttackBuffCapAndAlias(t *testing.T) {
	for _, row := range []struct {
		target                string
		attack, tension, want int
	}{
		{"self", 249, 1, 250}, {"self", 100, 3, 250}, {"fallen", 100, 1, 200},
	} {
		s, c, d := weaponWeatherStanceFixture()
		s.Actors[0].Attack, s.Actors[0].Tension = row.attack, float64(row.tension)
		executeWeaponWeatherStance(t, &s, c, d, "job-24-entry-2", []string{row.target})
		if s.Actors[0].Attack != row.want || s.Actors[0].Tension != 1 || s.Actors[2].Attack != 0 {
			t.Fatal(row, s)
		}
	}
}

func TestWeaponWeatherStanceStatusAndDeathChanceBoundaries(t *testing.T) {
	for _, row := range []struct {
		id     string
		chance float64
		death  bool
	}{
		{"job-28-entry-1", .8, false}, {"job-28-entry-5", .65, false}, {"job-28-entry-6", .25, true},
	} {
		for _, delta := range []float64{-.001, 0, .001} {
			s, c, d := weaponWeatherStanceFixture()
			s.Actors[1].HP, s.Actors[1].Tension = 999, 3
			executeWeaponWeatherStance(t, &s, c, d, row.id, []string{"enemy"}, row.chance+delta)
			success := delta < 0 || row.death && delta == 0
			if row.death {
				if (s.Actors[1].HP == 0) != success {
					t.Fatal(row, delta, s)
				}
			} else if (s.Actors[1].Status == corebattle.StatusPhysicalSeal) != success || success && s.Actors[1].Tension != 1 {
				t.Fatal(row, delta, s)
			}
		}
	}
}

func TestWeaponWeatherStanceHealingOrderAndFullFallenAllies(t *testing.T) {
	// item256 would splash if the producer did not record the party count
	// before the first child. The fallen ally redirects to self, then cures
	// using its own captured status, rather than the resolved target's status.
	s, c, d := weaponWeatherStanceFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-256"}
	s.Actors[0].Status, s.Actors[2].Status = corebattle.StatusSleep, corebattle.StatusPoison
	ally := s.Actors[0]
	ally.ID, ally.HP, ally.Status, ally.ItemDefinitionIDs = "ally", 1000, corebattle.StatusParalyze, nil
	s.Actors = append(s.Actors, ally)
	c.PartyIDs = append(c.PartyIDs, "ally")
	executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-8", nil, 0, 0, 0)
	if c.PartyHealCalls != 1 || c.HealCalls != 3 || c.CureCalls != 3 || c.PartyCureCalls != 0 || s.Actors[0].HP != 1000 || s.Actors[0].Status != "" || s.Actors[2].HP != 0 || s.Actors[2].Status != corebattle.StatusPoison || s.Actors[3].HP != 1000 || s.Actors[3].Status != "" {
		t.Fatal(s, c)
	}
}

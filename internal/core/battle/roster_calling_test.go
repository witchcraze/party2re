package battle

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func addRosterHelper(t *testing.T, s *LifetimeState, c *RosterContext, job int) string {
	t.Helper()
	r, err := s.AddHelper(c, HelperPayload{Identity: callingIdentity(job), Name: "援護者", Stats: [5]float64{100, 100, 100, 100, 100}}, &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	return r.HelperID
}

func admitRosterDamage(s *LifetimeState, c *RosterContext) {
	c.GuaranteedHit = true
	c.Defeat = func(id string) error { s.actor(id).HP = 0; return nil }
}

func TestCallingLookupPresenceAppearanceAndFallenOrder(t *testing.T) {
	s, c := rosterFixture()
	s.actor("player").Icon = nativeFormIcon("job-39", "m")
	for _, a := range []LifetimeActor{
		{Participant: Participant{ID: "mixed", TeamID: "party", Icon: nativeFormIcon("job-39-mix", "m")}},
		{Participant: Participant{ID: "first", TeamID: "party", Icon: nativeFormIcon("job-390", "f")}, Jobs: BattleJobs{JobID: "job-1"}},
		{Participant: Participant{ID: "second", TeamID: "party", HP: 100, Icon: nativeFormIcon("job-39", "m")}},
		{Participant: Participant{ID: "job-only", TeamID: "party", HP: 100}, Jobs: BattleJobs{JobID: "job-39"}},
	} {
		s.Actors = append(s.Actors, a)
		c.PartyIDs = append(c.PartyIDs, a.ID)
	}
	if got := s.FindCallingHelper(c.TargetContext, 59); got != "first" {
		t.Fatal(got)
	}
	r, err := s.ApplyRoster(&c, "job-59-attack", "enemy", &targetRNG{t: t})
	if err != nil || r.Applied {
		t.Fatal("first fallen appearance must not fall through", r, err)
	}
	id := addRosterHelper(t, &s, &c, 59)
	if got := s.FindCallingHelper(c.TargetContext, 59); got != id {
		t.Fatal(got)
	}
	s.actor(id).HP = 0
	if got := s.FindCallingHelper(c.TargetContext, 59); got != id {
		t.Fatal("fallen named helper must remain selected", got)
	}
	if _, err := s.ApplyRoster(&c, "job-59-special", "enemy", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	s.PruneRoster(&c.TargetContext)
	if got := s.FindCallingHelper(c.TargetContext, 59); got != "first" {
		t.Fatal(got)
	}
}

func TestCallingSummonLevelBoundariesAndBonus(t *testing.T) {
	for job, tiers := range callingStats {
		if job == 87 {
			continue
		}
		for _, level := range []int{39, 40, 69, 70} {
			for _, bonus := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%d/%v", job, level, bonus), func(t *testing.T) {
					s, c := rosterFixture()
					c.RequesterLevel = &level
					if bonus {
						c.RequesterItemID = "item-165"
					}
					rng := &targetRNG{t: t, draws: []float64{0, .999, .1, .5, .8}}
					r, err := s.ApplyRoster(&c, fmt.Sprintf("job-%d-summon", job), "", rng)
					if err != nil {
						t.Fatal(err)
					}
					rng.done()
					form := 0
					if level >= 70 {
						form = 2
					} else if level >= 40 {
						form = 1
					}
					base := tiers[form]
					want := [5]int{}
					for i, draw := range []float64{0, .999, .1, .5, .8} {
						want[i] = int(base[i] * (.9 + draw*.3))
						if bonus {
							want[i] = int(float64(want[i]) * 1.2)
						}
					}
					a := s.actor(r.HelperID)
					if [5]int{a.HP, a.MP, a.Attack, a.Defense, a.Agility} != want || a.MaxHP != a.HP || a.MaxMP != a.MP || a.BaseStats != (BattleBaseStats{want[2], want[3], want[4]}) || a.SummonForm != form {
						t.Fatal(a, want)
					}
				})
			}
		}
	}
	// Bonus reads the requester, never the temporarily commanded actor's items.
	s, c := rosterFixture()
	s.actor("player").ItemDefinitionIDs = []string{"item-165"}
	r, err := s.ApplyRoster(&c, "tool-216", "", &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0}})
	if err != nil || s.actor(r.HelperID).HP != 315 {
		t.Fatal(r, err)
	}
}

func TestCallingRandomSummonBranches(t *testing.T) {
	for _, job := range []int{68, 69, 80, 87} {
		forms := 3
		if job == 80 {
			forms = 4
		}
		for form := 0; form < forms; form++ {
			t.Run(fmt.Sprintf("%d/%d", job, form), func(t *testing.T) {
				s, c := rosterFixture()
				var prefix []float64
				switch job {
				case 68, 69:
					prefix = [][]float64{{0}, {.9, 0}, {.9, .9}}[form]
				case 87:
					prefix = [][]float64{{.9, .9}, {.9, 0}, {0}}[form]
				case 80:
					prefix = [][]float64{{0}, {.9, 0, 0, 0, 0, 0, 0}, {.9, .9, 0}, {.9, .9, .9, 0, 0, 0, 0, 0, 0, 0, 0}}[form]
				}
				rng := &targetRNG{t: t, draws: append(prefix, 0, 0, 0, 0, 0)}
				r, err := s.ApplyRoster(&c, fmt.Sprintf("job-%d-summon", job), "", rng)
				if err != nil || s.actor(r.HelperID).SummonForm != form {
					t.Fatal(r, err)
				}
				rng.done()
			})
		}
	}
}

func TestCallingEveryFirstNonCommandAndMissingHelper(t *testing.T) {
	for _, job := range []int{59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 71, 80, 87} {
		t.Run(fmt.Sprint(job), func(t *testing.T) {
			s, c := rosterFixture()
			entry := RosterEntries(job)[5]
			if entry.Extended || !callingMatch(entry, "") {
				t.Fatal(entry)
			}
			if r, err := s.ApplyRoster(&c, entry.ID, "enemy", &targetRNG{t: t}); err != nil || r.Applied {
				t.Fatal("missing helper", r, err)
			}
			id := addRosterHelper(t, &s, &c, job)
			admitRosterDamage(&s, &c)
			s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "fallen", TeamID: "party", MaxHP: 200, MP: 10, Gear: &BattleGearStats{}}})
			c.PartyIDs = append(c.PartyIDs, "fallen")
			s.actor("player").HP = 50
			if job == 87 {
				s.actor("enemy").MaxHP = 999
			}
			before, _ := json.Marshal(s)
			if job == 87 {
				if _, err := s.ApplyRoster(&c, entry.ID, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				after, _ := json.Marshal(s)
				if string(after) != string(before) {
					t.Fatal("unsupported callback changed state")
				}
				return
			}
			r, err := s.ApplyRoster(&c, entry.ID, "enemy", random.NewDeterministic(1339))
			if err != nil {
				t.Fatal(entry, r, err)
			}
			if c.ActorID != "player" {
				t.Fatal("changed actor", c.ActorID)
			}
			switch job {
			case 59, 60, 63, 64, 71:
				if s.actor(id).HP != 0 || s.actor(id).TeamID != "npc" || s.actor("player").HP != 140 || s.actor("player").Attack != 190 {
					t.Fatal(s.Actors)
				}
			case 61:
				if s.actor("fallen").HP != 84 || s.actor("fallen").Attack != 300 || s.actor("fallen").Agility != 300 {
					t.Fatal(s.actor("fallen"))
				}
			case 62:
				if len(r.Effects) != 4 || s.actor("enemy").HP >= 1000 || s.actor("player").HP <= 50 {
					t.Fatal(r, s.Actors)
				}
			case 65:
				if len(r.Effects) < 3 || len(r.Effects) > 4 {
					t.Fatal(r)
				}
			case 66:
				if s.actor("enemy").HP >= 600 {
					t.Fatal(s.actor("enemy"))
				}
			case 67:
				if s.actor("enemy").HP >= 1000 || c.AreaDamageCalls != 0 || !c.AdditionalEffect {
					t.Fatal(r, c)
				}
			case 68:
				if s.actor("player").HP != 100 {
					t.Fatal(s.actor("player"))
				}
			case 69:
				if s.actor("fallen").MP != 5 || s.actor(id).MP != 85 || s.actor("player").MP != 100 || s.actor("enemy").HP >= 800 {
					t.Fatal(s.Actors)
				}
			case 80:
				if s.actor("player").Attack < 100 || s.actor(id).Attack < 100 {
					t.Fatal(s.Actors)
				}
			}
		})
	}
}

func TestCommandRestorationThroughReloadAndError(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 59)
	admitRosterDamage(&s, &c)
	s.actor(id).Tension = 2
	s.actor(id).Extended = ExtendedAwakening
	s.actor(id).ExtendedTurns = 8
	s.actor("player").Tension = 3
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	c2 := c
	c2.Defeat = func(target string) error { loaded.actor(target).HP = 0; return nil }
	r1, err := s.ApplyRoster(&c, "job-59-attack", "enemy", random.NewDeterministic(9))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := loaded.ApplyRoster(&c2, "job-59-attack", "enemy", random.NewDeterministic(9))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, loaded) || !reflect.DeepEqual(r1, r2) || c.ActorID != "player" || c2.ActorID != "player" || c.DamageCalls != 1 || s.actor(id).ExtendedTurns != 8 || s.actor("player").Tension != 3 {
		t.Fatal(s, loaded, c, r1, r2)
	}
	failure := errors.New("defeat failed")
	s.actor("enemy").HP = 1
	c.Defeat = func(string) error { return failure }
	if _, err := s.ApplyRoster(&c, "job-59-attack", "enemy", random.NewDeterministic(9)); !errors.Is(err, failure) || c.ActorID != "player" {
		t.Fatal(err, c.ActorID)
	}
}

func TestSpecialUsesHelperStatsAndSummonerModifiers(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 59)
	admitRosterDamage(&s, &c)
	s.actor("player").Attack = 10
	s.actor("player").Tension = 2
	s.actor(id).Tension = 4
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	r, err := s.ApplyRoster(&c, "job-59-special", "enemy", rng)
	if err != nil || r.Effects[0].Amount != 121 || c.ActorID != "player" || s.actor(id).Tension != 4 || s.actor("player").Tension != 1 {
		t.Fatal(r, err, s.Actors)
	}
	rng.done()
}

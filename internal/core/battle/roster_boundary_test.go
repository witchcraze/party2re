package battle

import (
	"errors"
	"testing"
)

func TestSharedMPKeepsInternalFiveMPThreshold(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 69)
	admitRosterDamage(&s, &c)
	s.actor("player").MP = 0
	s.actor(id).MP = 4
	s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "fallen", TeamID: "party", MP: 5, MaxHP: 100}})
	c.PartyIDs = append(c.PartyIDs, "fallen")
	r, err := s.ApplyRoster(&c, "job-69-share-mp", "enemy", &targetRNG{t: t, draws: []float64{0}})
	if err != nil || s.actor(id).MP != 4 || s.actor("fallen").MP != 0 || s.actor("player").MP != 0 || r.Effects[1].Amount != 166 {
		t.Fatal(r, err, s.Actors)
	}
}

func TestSnakeLevelShortCircuitsAndCorruptGrowthForm(t *testing.T) {
	for _, level := range []int{39, 40, 69, 70} {
		s, c := rosterFixture()
		c.RequesterLevel = &level
		draws := []float64{}
		if level >= 70 {
			draws = append(draws, .9)
		}
		if level >= 40 {
			draws = append(draws, .9)
		}
		rng := &targetRNG{t: t, draws: append(draws, 0, 0, 0, 0, 0)}
		r, err := s.ApplyRoster(&c, "job-87-summon", "", rng)
		if err != nil || s.actor(r.HelperID).SummonForm != 0 {
			t.Fatal(level, r, err)
		}
		rng.done()
		s.actor(r.HelperID).SummonForm = -1
		if _, err := s.ApplyRoster(&c, "job-87-defend", "", &targetRNG{t: t}); !errors.Is(err, ErrInvalidParticipant) {
			t.Fatal(err)
		}
	}
}

func TestMissingDistributionHelperDoesNotRequireSkippedCursor(t *testing.T) {
	s, c := rosterFixture()
	c.Cursor = TargetCursor{Source: "unrepresented", ActorID: "player"}
	if _, err := s.ApplyRoster(&c, "job-80-distribution", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	addRosterHelper(t, &s, &c, 80)
	if _, err := s.ApplyRoster(&c, "job-80-distribution", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNamedHelperSelfMergeKeepsOrderedAliasedWrites(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 59)
	c.ActorID = id
	if _, err := s.ApplyRoster(&c, "job-59-merge", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if s.actor(id).Attack != 270 || s.actor(id).MaxHP != 90 || s.actor(id).HP != 0 {
		t.Fatal(s.actor(id))
	}
}

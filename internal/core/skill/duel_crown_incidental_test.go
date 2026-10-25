package skill

import (
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDuelCrownAllyFallbackAndReplyActivation(t *testing.T) {
	s, c, d := duelFixture()
	ally, enemy := s.Actors[2], s.Actors[1]
	ally.HP, ally.Attack = 1000, 20
	enemy.HP = 0
	other := enemy
	other.ID = "other"
	s.Actors = []corebattle.LifetimeActor{s.Actors[0], ally, enemy, other}
	c.EnemyIDs = []string{"enemy", "other"}
	original := c.TargetContext
	executeDuel(t, &s, c, d, "job-74-entry-2", []string{"enemy"}, .75, 0, .5, 0, .5, 0, 0, 0, .5, 0, .5, 0, 0)
	if s.Actors[0].HP != 982 || s.Actors[1].HP != 1000 || c.DamageCalls != 2 || !reflect.DeepEqual(c.TargetContext, original) {
		t.Fatal(s, c)
	}

	s, c, d = duelFixture()
	s.Actors[1].Extended = corebattle.ExtendedPurification
	var replyActors []string
	c.Reset = func(string) error {
		replyActors = append(replyActors, c.ActorID)
		if c.ActorID != "enemy" || !reflect.DeepEqual(c.PartyIDs, []string{"enemy"}) || !reflect.DeepEqual(c.EnemyIDs, []string{"self", "fallen"}) {
			t.Fatal(c)
		}
		c.Item195Activated = true
		return nil
	}
	executeDuel(t, &s, c, d, "job-74-entry-2", []string{"enemy"}, .5, 0, 0, .5, 0, .5, .99, 0, 0, .5, .99, 0)
	if !c.Item195Activated || c.ActorID != "self" || !reflect.DeepEqual(replyActors, []string{"enemy", "enemy"}) {
		t.Fatal(s, c, replyActors)
	}
}

func TestDuelCrownFlurryLosesCrownAfterDefeat(t *testing.T) {
	s, c, d := duelFixture()
	s.Actors[0].Extended = corebattle.ExtendedCrown
	s.Actors[1].HP = 1
	defeats := 0
	c.Defeat = func(string) error {
		defeats++
		s.Actors[0].Extended = ""
		s.Actors[1].HP = 1000
		return nil
	}
	var draws []float64
	for range 9 {
		draws = append(draws, 0, 0, 0, .5)
	}
	executeDuel(t, &s, c, d, "job-74-entry-5", []string{"enemy", "tail"}, draws...)
	if c.DamageCalls != 9 || defeats != 1 || s.Actors[1].HP != 680 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(s, c, defeats)
	}
}

func TestDuelCrownFlurryStopsOnDeathAndSkipsMissingTupleTails(t *testing.T) {
	for _, defense := range []corebattle.TemporaryEffect{corebattle.TemporaryOneHitDefense, corebattle.TemporaryNullAttack, corebattle.TemporaryReflectAttack} {
		s, c, d := duelFixture()
		s.Actors[0].Extended = corebattle.ExtendedCrown
		s.Actors[1].Temporary = defense
		switch defense {
		case corebattle.TemporaryOneHitDefense:
			executeDuel(t, &s, c, d, "job-74-entry-5", []string{"enemy"}, 0, 0, .5, .5, 0, 0, .5, .5, 0, 0, .5, .5, 0, 0, .5, .5, 0, 0, .5, .5)
			if c.DamageCalls != 6 || s.Actors[1].HP != 800 || s.Actors[1].Temporary != "" || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(s, c)
			}
		case corebattle.TemporaryNullAttack:
			c.Defeat = nil
			executeDuel(t, &s, c, d, "job-74-entry-5", []string{"enemy"})
			if c.DamageCalls != 0 || s.Actors[1].HP != 1000 || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(s, c)
			}
		case corebattle.TemporaryReflectAttack:
			s.Actors[0].HP = 1
			executeDuel(t, &s, c, d, "job-74-entry-5", []string{"enemy"}, 0, 0, .5, 0)
			if c.DamageCalls != 1 || s.Actors[0].HP != 0 || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(s, c)
			}
		}
	}
}

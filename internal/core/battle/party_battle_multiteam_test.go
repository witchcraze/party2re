package battle_test

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestResolvePartyBattle_MultiTeam_NonLeaderTeamsTargetEachOther(t *testing.T) {
	engine := corebattle.Engine{}

	t.Run("Green attacks Blue when Blue is lowest HP enemy", func(t *testing.T) {
		redHero := corebattle.MustNewParticipant("red-1", 100, 25, 20)
		redHero.TeamID = "red"

		blueHero := corebattle.MustNewParticipant("blue-1", 80, 25, 10)
		blueHero.TeamID = "blue"

		greenHero := corebattle.MustNewParticipant("green-1", 100, 25, 10)
		greenHero.TeamID = "green"

		req := corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{redHero},
			Enemies: []corebattle.Participant{blueHero, greenHero},
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		greenAttackedBlue := false
		for _, log := range res.Logs {
			if log.ActorID == "green-1" && log.TargetID == "blue-1" {
				greenAttackedBlue = true
				break
			}
		}
		if !greenAttackedBlue {
			t.Errorf("expected Green to attack Blue in multi-team combat")
		}
	})

	t.Run("Blue attacks Green when Green is lowest HP enemy", func(t *testing.T) {
		redHero := corebattle.MustNewParticipant("red-1", 100, 25, 20)
		redHero.TeamID = "red"

		blueHero := corebattle.MustNewParticipant("blue-1", 100, 25, 10)
		blueHero.TeamID = "blue"

		greenHero := corebattle.MustNewParticipant("green-1", 80, 25, 10)
		greenHero.TeamID = "green"

		req := corebattle.PartyBattleRequest{
			Allies:  []corebattle.Participant{redHero},
			Enemies: []corebattle.Participant{blueHero, greenHero},
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		blueAttackedGreen := false
		for _, log := range res.Logs {
			if log.ActorID == "blue-1" && log.TargetID == "green-1" {
				blueAttackedGreen = true
				break
			}
		}
		if !blueAttackedGreen {
			t.Errorf("expected Blue to attack Green in multi-team combat")
		}
	})
}

func TestResolvePartyBattle_MultiTeam_LeaderFallsEarlyCombatContinues(t *testing.T) {
	engine := corebattle.Engine{}

	// Red leader has only 1 HP and no defense, falls on first hit.
	// Blue and Green have high HP (100 HP each) and high attack.
	redHero := corebattle.MustNewParticipant("red-1", 1, 5, 0)
	redHero.Agility = 1 // Acts last
	redHero.TeamID = "red"

	blueHero := corebattle.MustNewParticipant("blue-1", 100, 30, 10)
	blueHero.Agility = 20
	blueHero.TeamID = "blue"

	greenHero := corebattle.MustNewParticipant("green-1", 80, 25, 10)
	greenHero.Agility = 15
	greenHero.TeamID = "green"

	req := corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{redHero},
		Enemies: []corebattle.Participant{blueHero, greenHero},
		DefeatReward: corebattle.Reward{
			Experience: 10,
		},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Red leader must be fallen
	if res.RemainingHP["red-1"] > 0 {
		t.Fatalf("expected Red leader to fall, but HP is %d", res.RemainingHP["red-1"])
	}

	// The combat must have continued between Blue and Green!
	// Exactly one of Blue or Green must be the sole survivor, or both dead
	blueHP := res.RemainingHP["blue-1"]
	greenHP := res.RemainingHP["green-1"]

	if blueHP > 0 && greenHP > 0 {
		t.Errorf("combat ended prematurely while both Blue (%d HP) and Green (%d HP) were still alive", blueHP, greenHP)
	}

	if res.Outcome != corebattle.OutcomeDefeat {
		t.Errorf("expected outcome defeat (since allies died), got %s", res.Outcome)
	}

	if blueHP > 0 {
		if res.WinnerSide != "blue" || res.WinnerTeam != "blue" {
			t.Errorf("expected winner blue, got side=%s team=%s", res.WinnerSide, res.WinnerTeam)
		}
	} else if greenHP > 0 {
		if res.WinnerSide != "green" || res.WinnerTeam != "green" {
			t.Errorf("expected winner green, got side=%s team=%s", res.WinnerSide, res.WinnerTeam)
		}
	}
}

func TestResolvePartyBattle_MultiTeam_TeamsMapSpecification(t *testing.T) {
	engine := corebattle.Engine{}

	redHero := corebattle.MustNewParticipant("red-1", 100, 50, 20)
	blueHero := corebattle.MustNewParticipant("blue-1", 100, 20, 10)
	greenHero := corebattle.MustNewParticipant("green-1", 100, 20, 10)

	req := corebattle.PartyBattleRequest{
		Teams: map[string][]corebattle.Participant{
			"team-red":   {redHero},
			"team-blue":  {blueHero},
			"team-green": {greenHero},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle with Teams map failed: %v", err)
	}

	if len(res.RemainingHP) != 3 {
		t.Errorf("expected 3 participants in RemainingHP, got %d", len(res.RemainingHP))
	}
}

func TestResolvePartyBattle_MultiTeam_Validation(t *testing.T) {
	engine := corebattle.Engine{}

	p1 := corebattle.MustNewParticipant("p-1", 100, 10, 10)
	p1.TeamID = "same-team"
	p2 := corebattle.MustNewParticipant("p-2", 100, 10, 10)
	p2.TeamID = "same-team"

	// All combatants on the same team must error
	_, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{p1},
		Enemies: []corebattle.Participant{p2},
	})
	if !errors.Is(err, corebattle.ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest when all combatants share the same team, got %v", err)
	}

	// Less than 2 teams in Teams map must error
	_, err = engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Teams: map[string][]corebattle.Participant{
			"only-one": {p1, p2},
		},
	})
	if !errors.Is(err, corebattle.ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest when Teams has fewer than 2 teams, got %v", err)
	}
}

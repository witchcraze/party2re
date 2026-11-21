package battle_test

import (
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/battle"
)

type zeroRNG struct{}

func (zeroRNG) Intn(int) int     { return 0 }
func (zeroRNG) IntN(int) int     { return 0 }
func (zeroRNG) Float64() float64 { return 0 }

func TestResolvePartyBattle_RecordsDefeatsWithoutRevival(t *testing.T) {
	eng := battle.Engine{}

	t.Run("defeating_enemy_with_pharaoh_revive_records_only_final_defeat", func(t *testing.T) {
		hero := battle.MustNewParticipant("hero", 500, 120, 20)
		hero.TeamID = "red"
		hero.Agility = 100

		foe := battle.MustNewParticipant("foe", 40, 10, 0)
		foe.TeamID = "blue"
		foe.MaxHP = 40
		foe.MP = 30
		foe.MaxMP = 30
		foe.Agility = 10
		foe.Abilities = []string{"pharaoh"}

		res, err := eng.ResolvePartyBattle(battle.PartyBattleRequest{
			Teams: map[string][]battle.Participant{
				"red":  {hero},
				"blue": {foe},
			},
			RNG: zeroRNG{},
		})
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}
		want := []battle.PartyBattleDefeat{{ActorID: "hero", TargetID: "foe"}}
		if !reflect.DeepEqual(res.Defeats, want) {
			t.Fatalf("Defeats = %+v, want %+v", res.Defeats, want)
		}
	})

	t.Run("three_teams_records_winning_third_team_killer", func(t *testing.T) {
		red := battle.MustNewParticipant("red-1", 10, 5, 0)
		red.TeamID = "red"
		red.Agility = 10

		blue := battle.MustNewParticipant("blue-1", 10, 5, 0)
		blue.TeamID = "blue"
		blue.Agility = 5

		green := battle.MustNewParticipant("green-1", 500, 200, 50)
		green.TeamID = "green"
		green.Agility = 200

		res, err := eng.ResolvePartyBattle(battle.PartyBattleRequest{
			Teams: map[string][]battle.Participant{
				"red":   {red},
				"blue":  {blue},
				"green": {green},
			},
			RNG: zeroRNG{},
		})
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}
		if len(res.Defeats) != 2 {
			t.Fatalf("expected 2 defeats, got %+v", res.Defeats)
		}
		for _, d := range res.Defeats {
			if d.ActorID != "green-1" {
				t.Fatalf("expected actor green-1, got %+v", res.Defeats)
			}
		}
	})
}

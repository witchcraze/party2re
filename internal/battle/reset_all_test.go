package battle_test

import (
	"encoding/json"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestResetAllRevivalAndUnsupportedAdmission(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		s := corebattle.LifetimeState{Fields: corebattle.LifetimeFields{Temporary: "burn", Turns: 4, AntiTurns: 2}, Actors: []corebattle.LifetimeActor{
			{Participant: corebattle.Participant{ID: "a", TeamID: "party", HP: 10, MaxHP: 100, MP: 3, MaxMP: 10, ItemDefinitionIDs: []string{"item-188"}, Gear: &corebattle.BattleGearStats{AccessoryID: "item-188"}}},
			{Participant: corebattle.Participant{ID: "b", TeamID: "party", HP: 0, MaxHP: 100, Gear: &corebattle.BattleGearStats{}, Status: corebattle.StatusPoison}, Extended: corebattle.ExtendedAwakening},
		}}
		if unsupported {
			s.Actors[1].Gear.AccessoryID = "item-110"
		}
		c := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}}}
		rng := &fixedSequenceRNG{}
		before, _ := json.Marshal(s)
		err := battle.ValidateResetAll(&s, &c, rng)
		after, _ := json.Marshal(s)
		if string(before) != string(after) || rng.calls != 0 {
			t.Fatal("validation mutated state or RNG")
		}
		err = battle.ResetAll(&s, &c, rng)
		if unsupported {
			after, _ = json.Marshal(s)
			if err == nil || string(before) != string(after) || rng.calls != 0 {
				t.Fatal("unsupported reset changed state")
			}
			continue
		}
		if err != nil || s.Actors[1].HP != 50 || s.Actors[1].Status != "" || s.Actors[1].Extended != "" || s.Actors[0].Temporary != corebattle.TemporaryRevival || s.Actors[0].HP != 10 || s.Actors[0].MP != 3 || s.Fields.Turns != 4 || s.Fields.AntiTurns != 2 {
			t.Fatal(s, err)
		}
	}
}

type fixedSequenceRNG struct{ calls int }

func (r *fixedSequenceRNG) Float64() float64 { r.calls++; return 0 }
func (r *fixedSequenceRNG) Intn(int) int     { r.calls++; return 0 }
func (r *fixedSequenceRNG) IntN(n int) int   { return r.Intn(n) }

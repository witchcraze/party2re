package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

func TestFinalizedRecipientRewardsDoNotRepeatModifiers(t *testing.T) {
	for _, ids := range [][]string{{"a"}, {"a", "b"}} {
		svc, chars, invs := setupRewardTestService("GOLD", 0)
		for _, id := range ids {
			if err := chars.Update(context.Background(), corecharacter.Character{ID: id, Level: 1, Money: 100, Stats: corecharacter.Stats{HP: 10, MaxHP: 10}}); err != nil {
				t.Fatal(err)
			}
			inv, err := coreinventory.New(id)
			if err != nil {
				t.Fatal(err)
			}
			if err := invs.Save(context.Background(), inv); err != nil {
				t.Fatal(err)
			}
		}
		resp, err := svc.ApplyPostBattleResult(context.Background(), battle.ApplyPostBattleRequest{CharacterIDs: ids, PreserveEarnedRewards: true, RecipientRewards: map[string]corebattle.Reward{"a": {Currency: 20}, "b": {Currency: 30}}, BattleResult: corebattle.PartyBattleResult{TotalReward: corebattle.Reward{Currency: 1000}}})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			want := 20
			if id == "b" {
				want = 30
			}
			if resp.GainedGold[id] != want || resp.UpdatedCharacters[id].Money != 100+want {
				t.Fatal("reward modifiers or shared reward repeated", resp)
			}
		}
	}
}

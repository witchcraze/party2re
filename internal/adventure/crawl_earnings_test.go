package adventure

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestCrawlEarnings_RevivalAndReload(t *testing.T) {
	s := CrawlSession{CharacterIDs: []string{"a", "b"}}
	for i, ids := range [][]string{{"a", "b"}, {"a"}, {"a", "b"}} {
		if err := s.recordDefeatEarnings(corebattle.PartyBattleResult{BonusPercent: 10, DefeatEarnings: []corebattle.DefeatEarnings{{Reward: corebattle.Reward{Experience: 10, Currency: 20, Crystals: 1}, RecipientIDs: ids}}}); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded CrawlSession
		if err := json.Unmarshal(data, &reloaded); err != nil {
			t.Fatal(err)
		}
		s = reloaded
		if i == 1 && s.Result().RewardForCharacter("b") != (corebattle.Reward{Experience: 11, Currency: 22, Crystals: 1}) {
			t.Fatal("earnings from before falling were lost", s.Earnings)
		}
	}
	r := s.Result()
	if r.RewardForCharacter("a") != (corebattle.Reward{Experience: 33, Currency: 66, Crystals: 3}) || r.RewardForCharacter("b") != (corebattle.Reward{Experience: 22, Currency: 44, Crystals: 2}) {
		t.Fatal("revived member did not resume earning", r.RecipientRewards)
	}
	r.RecipientRewards["b"] = corebattle.Reward{}
	if s.Result().RewardForCharacter("b").Currency != 44 {
		t.Fatal("result aliases session earnings")
	}
}

func TestCrawlEarnings_OverflowKeepsPriorEarnings(t *testing.T) {
	for _, gold := range []int{-1, math.MaxInt} {
		s := CrawlSession{Earnings: map[string]RunEarnings{"a": {Gold: 5}}}
		if err := s.recordDefeatEarnings(corebattle.PartyBattleResult{BonusPercent: 10, DefeatEarnings: []corebattle.DefeatEarnings{{Reward: corebattle.Reward{Currency: gold}, RecipientIDs: []string{"a"}}}}); err == nil {
			t.Fatal("accepted invalid or overflowing earnings", gold)
		}
		if !reflect.DeepEqual(s.Earnings, map[string]RunEarnings{"a": {Gold: 5}}) {
			t.Fatal("failed accumulation changed prior earnings", s.Earnings)
		}
	}
}

package adventure

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
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
		s := CrawlSession{Earnings: map[string]runhost.Earnings{"a": {Gold: 5}}}
		if err := s.recordDefeatEarnings(corebattle.PartyBattleResult{BonusPercent: 10, DefeatEarnings: []corebattle.DefeatEarnings{{Reward: corebattle.Reward{Currency: gold}, RecipientIDs: []string{"a"}}}}); err == nil {
			t.Fatal("accepted invalid or overflowing earnings", gold)
		}
		if !reflect.DeepEqual(s.Earnings, map[string]runhost.Earnings{"a": {Gold: 5}}) {
			t.Fatal("failed accumulation changed prior earnings", s.Earnings)
		}
	}
}

func TestCrawlEarnings_ChapelBlessings(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rngVal    int
		blessings map[string]string
		defeats   []corebattle.DefeatEarnings
		wantA     corebattle.Reward
		wantB     corebattle.Reward
	}{
		{
			name:      "acting player GOLD lucky shared by living recipients only",
			rngVal:    0,
			blessings: map[string]string{"a": "GOLD", "b": "EXP"},
			defeats: []corebattle.DefeatEarnings{
				{ActorID: "a", Reward: corebattle.Reward{Experience: 100, Currency: 50, Crystals: 1}, RecipientIDs: []string{"a", "b"}},
				{ActorID: "a", Reward: corebattle.Reward{Experience: 100, Currency: 50}, RecipientIDs: []string{"a"}},
			},
			// Floor 1: defeat 1 -> EXP 100, Gold 75; defeat 2 -> EXP 100, Gold 75.
			// a: (200, 150) * 1.1 = (220, 165); b: (100, 75) * 1.1 = (110, 82)
			wantA: corebattle.Reward{Experience: 220, Currency: 165, Crystals: 1},
			wantB: corebattle.Reward{Experience: 110, Currency: 82, Crystals: 1},
		},
		{
			name:      "non-leader actor EXP lucky",
			rngVal:    0,
			blessings: map[string]string{"a": "GOLD", "b": "EXP"},
			defeats: []corebattle.DefeatEarnings{
				{ActorID: "b", Reward: corebattle.Reward{Experience: 100, Currency: 50}, RecipientIDs: []string{"a", "b"}},
			},
			wantA: corebattle.Reward{Experience: 165, Currency: 55},
			wantB: corebattle.Reward{Experience: 165, Currency: 55},
		},
		{
			name:      "unlucky roll grants base cooperative reward",
			rngVal:    1,
			blessings: map[string]string{"a": "GOLD"},
			defeats: []corebattle.DefeatEarnings{
				{ActorID: "a", Reward: corebattle.Reward{Experience: 100, Currency: 50}, RecipientIDs: []string{"a", "b"}},
			},
			wantA: corebattle.Reward{Experience: 110, Currency: 55},
			wantB: corebattle.Reward{Experience: 110, Currency: 55},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := CrawlSession{
				CharacterIDs: []string{"a", "b"},
				Blessings:    tc.blessings,
				Rng:          func(int) int { return tc.rngVal },
			}
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded CrawlSession
			if err := json.Unmarshal(data, &reloaded); err != nil {
				t.Fatal(err)
			}
			reloaded.Rng = s.Rng
			if err := reloaded.recordDefeatEarnings(corebattle.PartyBattleResult{
				BonusPercent:   10,
				DefeatEarnings: tc.defeats,
			}); err != nil {
				t.Fatal(err)
			}
			res := reloaded.Result()
			if res.RewardForCharacter("a") != tc.wantA || res.RewardForCharacter("b") != tc.wantB {
				t.Fatalf("got a=%+v b=%+v, want a=%+v b=%+v", res.RewardForCharacter("a"), res.RewardForCharacter("b"), tc.wantA, tc.wantB)
			}
		})
	}
}

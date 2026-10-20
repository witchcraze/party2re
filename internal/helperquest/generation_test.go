package helperquest

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/testutil"
)

type questDraw struct{ max, value int }

type questRandom struct {
	t     *testing.T
	draws []questDraw
}

func (r *questRandom) Intn(max int) (int, error) {
	r.t.Helper()
	if len(r.draws) == 0 {
		r.t.Fatalf("unexpected draw Intn(%d)", max)
	}
	draw := r.draws[0]
	r.draws = r.draws[1:]
	if max != draw.max || draw.value < 0 || draw.value >= max {
		r.t.Fatalf("draw Intn(%d) = %d, want Intn(%d)", max, draw.value, draw.max)
	}
	return draw.value, nil
}

func TestGenerateQuestTargetPools(t *testing.T) {
	// Independently verified ranges: lib/helper.cgi:22–44.
	// Monster holes must retain their slots and draw a fallback.
	cases := []struct {
		kind QuestKind
		rare bool
		ids  []int
	}{
		{KindWeapon, false, testNumbers(1, 27)},
		{KindWeapon, true, testNumbers(28, 40)},
		{KindArmor, false, testNumbers(1, 27)},
		{KindArmor, true, testNumbers(28, 40)},
		{KindItem, false, append(append(append(testNumbers(1, 27), testNumbers(30, 43)...), testNumbers(60, 65)...), append(testNumbers(72, 103), 108)...)},
		{KindItem, true, append([]int{28, 29, 40, 58, 59}, append(testNumbers(66, 71), 104, 105, 106, 107, 109)...)},
		{KindMonster, false, append(testNumbers(4, 120), testNumbers(198, 260)...)},
		{KindMonster, true, testNumbers(500, 579)},
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/rare=%t", tc.kind, tc.rare), func(t *testing.T) {
			var ids []string
			for _, n := range tc.ids {
				prefix, width := "item", 3
				switch tc.kind {
				case KindWeapon:
					prefix, width = "weapon", 2
				case KindArmor:
					prefix, width = "armor", 2
				case KindMonster:
					prefix = "monster"
				}
				ids = append(ids, fmt.Sprintf("%s-%0*d", prefix, width, n))
			}
			for slot, want := range ids {
				rareRoll, rewardSlots, countSlots := 1, 10, 3
				if tc.rare {
					rareRoll, rewardSlots = 0, 6
				}
				if tc.kind == KindMonster {
					countSlots = 2
				}
				rng := &questRandom{t: t, draws: []questDraw{{4, int(tc.kind) - 1}, {14, rareRoll}, {15, 1}, {countSlots, 0}, {len(ids), slot}, {8, 0}, {999, 0}, {rewardSlots, 0}}}
				q, err := GenerateQuest(rng, time.Time{}, func(id string) (string, bool) { return id, true })
				if err != nil || q.TargetID != want || q.TargetName == "" {
					t.Fatalf("slot %d: target=%+v, err=%v, want %s", slot, q, err, want)
				}
				if tc.kind != KindMonster {
					def, err := catalog.FindByID(want)
					if err != nil || q.TargetName != def.Name {
						t.Fatalf("%s: name=%s, want native name %s, err=%v", want, q.TargetName, def.Name, err)
					}
				}
				if len(rng.draws) != 0 {
					t.Fatalf("unused draws: %v", rng.draws)
				}
			}
		})
	}
}

func TestGenerateQuestMonsterFallback(t *testing.T) {
	native := testutil.MonsterNameLookup(t)
	for _, rare := range []bool{false, true} {
		primarySlots, fallbackSlots, first, rareRoll, rewardSlots := 180, 3, 1, 1, 10
		if rare {
			primarySlots, fallbackSlots, first, rareRoll, rewardSlots = 80, 6, 160, 0, 6
		}
		for primary := 0; primary < primarySlots; primary++ {
			for fallback := 0; fallback < fallbackSlots; fallback++ {
				var lookups []string
				lookup := func(id string) (string, bool) {
					lookups = append(lookups, id)
					if !rare && len(lookups) == 1 {
						return "", false // A hole in the normal catalog, without removing its draw slot.
					}
					return native(id)
				}
				rng := &questRandom{t: t, draws: []questDraw{{4, 3}, {14, rareRoll}, {15, 1}, {2, 0}, {primarySlots, primary}, {fallbackSlots, fallback}, {8, 0}, {999, 0}, {rewardSlots, 0}}}
				q, err := GenerateQuest(rng, time.Time{}, lookup)
				want := fmt.Sprintf("monster-%03d", first+fallback)
				name, ok := native(want)
				if err != nil || !ok || q.TargetID != want || q.TargetName != name || len(lookups) != 2 || len(rng.draws) != 0 {
					t.Fatalf("rare=%t primary=%d fallback=%d: quest=%+v lookups=%v err=%v, want %s", rare, primary, fallback, q, lookups, err, want)
				}
			}
		}
	}
}

func TestGenerateQuestRarityGuildQuantityAndDeadline(t *testing.T) {
	now := time.Date(2026, 10, 20, 23, 59, 59, 0, time.FixedZone("JST", 9*60*60))
	for kind := KindWeapon; kind <= KindMonster; kind++ {
		countSlots, base := 3, 2
		if kind == KindMonster {
			countSlots, base = 2, 1
		}
		for rareRoll := 0; rareRoll < 14; rareRoll++ {
			for guildRoll := 0; guildRoll < 15; guildRoll++ {
				for count := 0; count < countSlots; count++ {
					rng := &mockRandomSource{values: []int{int(kind) - 1, rareRoll, guildRoll, count, 0, 0, 0, 0}}
					q, err := GenerateQuest(rng, now, func(id string) (string, bool) { return id, true })
					want := count + base
					if guildRoll == 0 {
						want *= 2
					}
					if err != nil || q.Kind != kind || q.IsRare != (rareRoll == 0) || q.IsGuild != (guildRoll == 0) || q.RequiredCount != want || !q.CreatedAt.Equal(now) || !q.ExpiresAt.Equal(now.Add(144*time.Hour)) {
						t.Fatalf("kind=%d rare=%d guild=%d count=%d: %+v, err=%v", kind, rareRoll, guildRoll, count, q, err)
					}
					if q.IsGuild && q.RewardItemID != "item-126" {
						t.Fatalf("guild reward=%s, want item-126", q.RewardItemID)
					}
				}
			}
		}
	}
}

type failingQuestRandom struct {
	RandomSource
	remaining int
	err       error
}

func (r *failingQuestRandom) Intn(max int) (int, error) {
	if r.remaining == 0 {
		return 0, r.err
	}
	r.remaining--
	return r.RandomSource.Intn(max)
}

func TestGenerateQuestFailures(t *testing.T) {
	failure := errors.New("random source failed")
	for draw := 0; draw < 9; draw++ {
		rng := &failingQuestRandom{RandomSource: &mockRandomSource{values: []int{3, 0, 1, 0, 0, 0, 0, 0, 0}}, remaining: draw, err: failure}
		q, err := GenerateQuest(rng, time.Time{}, testutil.MonsterNameLookup(t))
		if !errors.Is(err, failure) || q != (Quest{}) {
			t.Fatalf("draw %d: quest=%+v err=%v, want zero quest and RNG failure", draw, q, err)
		}
	}
	for _, lookup := range []func(string) (string, bool){nil, func(string) (string, bool) { return "", false }} {
		q, err := GenerateQuest(&mockRandomSource{values: []int{3, 0, 1, 0, 0, 0, 0, 0, 0}}, time.Time{}, lookup)
		if !errors.Is(err, ErrInvalidParameters) || q != (Quest{}) {
			t.Fatalf("missing catalog/fallback: quest=%+v err=%v", q, err)
		}
	}
}

func testNumbers(first, last int) []int {
	var values []int
	for n := first; n <= last; n++ {
		values = append(values, n)
	}
	return values
}

func TestGenerateQuestRewardSlots(t *testing.T) {
	for _, tc := range []struct {
		rareRoll int
		want     []string
	}{
		{1, []string{"item-128", "item-128", "item-128", "item-130", "item-131", "item-132", "item-133", "item-134", "item-134", "item-184"}},
		{0, []string{"item-129", "item-129", "item-129", "item-135", "item-136", "item-137"}},
	} {
		for slot, want := range tc.want {
			targetSlots := 27
			if tc.rareRoll == 0 {
				targetSlots = 13
			}
			rng := &questRandom{t: t, draws: []questDraw{{4, 0}, {14, tc.rareRoll}, {15, 1}, {3, 0}, {targetSlots, 0}, {8, 0}, {999, 0}, {len(tc.want), slot}}}
			q, err := GenerateQuest(rng, time.Time{}, func(id string) (string, bool) { return id, true })
			if err != nil || q.RewardItemID != want {
				t.Fatalf("rare=%d slot=%d: reward=%s, err=%v, want %s", tc.rareRoll, slot, q.RewardItemID, err, want)
			}
		}
	}
}

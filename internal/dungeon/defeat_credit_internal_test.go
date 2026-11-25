package dungeon

import (
	"context"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// strictRNG replays exactly the scripted draws of the defeat routine.
type strictRNG struct {
	t      *testing.T
	ints   []int
	floats []float64
}

func (r *strictRNG) Intn(n int) int {
	r.t.Helper()
	if len(r.ints) == 0 {
		r.t.Fatalf("unexpected Intn(%d)", n)
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	return v
}

func (r *strictRNG) Float64() float64 {
	r.t.Helper()
	if len(r.floats) == 0 {
		r.t.Fatal("unexpected Float64")
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func (r *strictRNG) done() {
	r.t.Helper()
	if len(r.ints)+len(r.floats) != 0 {
		r.t.Fatalf("unused draws: ints=%v floats=%v", r.ints, r.floats)
	}
}

type defeatChars map[string]corecharacter.Character

func (c defeatChars) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if ch, ok := c[id]; ok {
		return ch, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

type defeatWishes map[string]string

func (w defeatWishes) GetActiveBlessing(_ context.Context, id string) (string, error) {
	return w[id], nil
}

type defeatHands map[string]string

func (h defeatHands) FindByCharacterID(_ context.Context, id string) (coreinventory.Inventory, error) {
	inv := coreinventory.Inventory{CharacterID: id}
	if def := h[id]; def != "" {
		inv.Items = []coreitem.Instance{{ID: "held-" + id, DefinitionID: def, Quantity: 1}}
	}
	return inv, nil
}

// Killer strength 100+50+20/2+30 = 190: an enemy is strong above 95.
var defeatKiller = corecharacter.Character{ID: "b", JobID: "job-01", Stats: corecharacter.Stats{MaxHP: 100, Attack: 50, Defense: 20, Agility: 30}}

func defeatEnemy(id string, strong bool) corebattle.Participant {
	hp := 50 // strength 60
	if strong {
		hp = 100 // strength 110
	}
	p := corebattle.MustNewParticipant(id, hp, 10, 0)
	p.MaxHP, p.TeamID = hp, "enemies"
	return p
}

type defeatSetup struct {
	wish   string
	hand   string
	gear   *corebattle.BattleGearStats
	jobID  string
	strong bool
	rng    *strictRNG
}

// shareOne resolves one defeat of e1 (31 EXP, 21 G) by killer b with a, b and c alive; a requests.
func shareOne(t *testing.T, d defeatSetup) defeatShares {
	t.Helper()
	killer := defeatKiller
	if d.jobID != "" {
		killer.JobID = d.jobID
	}
	s := &Service{
		characterRepo: defeatChars{"b": killer},
		rng:           d.rng,
		blessings:     defeatWishes{"b": d.wish},
		invRepo:       defeatHands{"b": d.hand},
		dungeonMap:    map[string]Dungeon{"d": {ID: "d", Name: "Cave"}},
	}
	exp := &ActiveExpedition{Revision: 4, CharacterID: "a", DungeonID: "d", Members: []ExpeditionMember{{CharacterID: "a"}, {CharacterID: "b"}, {CharacterID: "c"}}}
	out := encounterOutcome{
		allies:  []corebattle.Participant{{ID: "b", Gear: d.gear}},
		defeats: []corebattle.DefeatEarnings{{ActorID: "b", TargetID: "e1", RecipientIDs: []string{"a", "b", "c"}}},
	}
	monsters := []DungeonMonster{{ID: "slime", Name: "スライムA", ExpReward: 31, GoldReward: 21}}
	res, err := s.shareDefeats(context.Background(), exp, "a", out, []corebattle.Participant{defeatEnemy("e1", d.strong)}, monsters)
	if err != nil {
		t.Fatal(err)
	}
	d.rng.done()
	return res
}

func TestShareDefeats_CrystalChance(t *testing.T) {
	// rand(50) < 1, or < 2 against a stronger enemy (_battle.cgi:145–150).
	for _, tc := range []struct {
		name   string
		strong bool
		draw   int
		want   int
	}{
		{"weak hit", false, 0, 1},
		{"weak miss", false, 1, 0},
		{"strong hit", true, 1, 1},
		{"strong miss", true, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := shareOne(t, defeatSetup{strong: tc.strong, rng: &strictRNG{t: t, ints: []int{tc.draw}, floats: []float64{0.99}}})
			if got := res.stock["c"].Crystals; got != tc.want {
				t.Fatalf("crystals = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("enemy crystal reward skips the roll", func(t *testing.T) {
		rng := &strictRNG{t: t, floats: []float64{0.99}}
		s := &Service{characterRepo: defeatChars{"b": defeatKiller}, rng: rng, dungeonMap: map[string]Dungeon{}}
		exp := &ActiveExpedition{CharacterID: "a", Members: []ExpeditionMember{{CharacterID: "a"}, {CharacterID: "b"}}}
		out := encounterOutcome{defeats: []corebattle.DefeatEarnings{{ActorID: "b", TargetID: "e1", RecipientIDs: []string{"a", "b"}}}}
		monsters := []DungeonMonster{{ID: "gem", combat: &corebattle.LifetimeActor{CrystalReward: 7}}}
		res, err := s.shareDefeats(context.Background(), exp, "", out, []corebattle.Participant{defeatEnemy("e1", false)}, monsters)
		if err != nil {
			t.Fatal(err)
		}
		rng.done()
		if res.stock["a"].Crystals != 7 || res.credits[0].Reward.Crystals != 7 {
			t.Fatalf("crystal reward not shared: %+v", res)
		}
	})
}

func TestShareDefeats_WishAndGearModifiers(t *testing.T) {
	gear11 := &corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01"}
	// Crystal miss first, then the killer's modifiers at rand(4) < 1 (_battle.cgi:186–197).
	for _, tc := range []struct {
		name string
		wish string
		gear *corebattle.BattleGearStats
		draw int
		want corebattle.Reward
	}{
		{"gold wish hit", "1", nil, 0, corebattle.Reward{Experience: 31, Currency: 31}},
		{"gold wish miss", "1", nil, 1, corebattle.Reward{Experience: 31, Currency: 21}},
		{"exp wish hit", "2", nil, 0, corebattle.Reward{Experience: 46, Currency: 21}},
		{"exp wish miss", "2", nil, 1, corebattle.Reward{Experience: 31, Currency: 21}},
		{"equipment 1/1 hit", "", gear11, 0, corebattle.Reward{Experience: 36, Currency: 21}},
		{"equipment 1/1 miss", "", gear11, 1, corebattle.Reward{Experience: 31, Currency: 21}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := shareOne(t, defeatSetup{wish: tc.wish, gear: tc.gear, rng: &strictRNG{t: t, ints: []int{1, tc.draw}, floats: []float64{0.99}}})
			// Every living member receives the killer-modified earnings.
			for _, r := range []corebattle.Reward{res.stock["c"], res.credits[0].Reward, res.credits[1].Reward} {
				if r != tc.want {
					t.Fatalf("reward = %+v, want %+v (shares %+v)", r, tc.want, res)
				}
			}
		})
	}
	t.Run("no wish or gear draws nothing more", func(t *testing.T) {
		shareOne(t, defeatSetup{gear: &corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-02"}, rng: &strictRNG{t: t, ints: []int{1}, floats: []float64{0.99}}})
	})
}

func TestShareDefeats_RecruitmentChance(t *testing.T) {
	// rand(200) < par (_battle.cgi:229–241): 2, 1 against a stronger enemy
	// unless job 12, +2 item 77, +0.5 wish 3, +0.5 equipment 36/39.
	dragon := &corebattle.BattleGearStats{WeaponID: "weapon-36", ArmorID: "armor-39"}
	for _, tc := range []struct {
		name string
		d    defeatSetup
		par  float64
	}{
		{"weaker enemy", defeatSetup{}, 2},
		{"stronger enemy", defeatSetup{strong: true}, 1},
		{"stronger enemy, tamer", defeatSetup{strong: true, jobID: "job-12"}, 2},
		{"monster food", defeatSetup{hand: "item-077"}, 4},
		{"monster wish", defeatSetup{wish: "3"}, 2.5},
		{"dragon set", defeatSetup{gear: dragon}, 2.5},
	} {
		for _, hit := range []bool{true, false} {
			roll := tc.par / 200
			if hit {
				roll -= 1e-9
			}
			d := tc.d
			d.rng = &strictRNG{t: t, ints: []int{1}, floats: []float64{roll}}
			res := shareOne(t, d)
			fact := res.credits[1].Defeats[0]
			if fact.Capture != hit || fact.StrongKill != tc.d.strong || fact.PvP || fact.Monster.MonsterID != "slime" || fact.Monster.MonsterName != "スライム" || fact.Monster.Habitat != "Cave" {
				t.Fatalf("%s hit=%v: fact %+v", tc.name, hit, fact)
			}
		}
	}
}

func TestShareDefeats_RecipientsAtEachDefeat(t *testing.T) {
	rng := &strictRNG{t: t, ints: []int{1, 1, 1, 1}, floats: []float64{0.99, 0.99, 0.99}}
	s := &Service{characterRepo: defeatChars{"a": defeatKiller, "b": defeatKiller, "d": defeatKiller}, rng: rng, dungeonMap: map[string]Dungeon{}}
	exp := &ActiveExpedition{Revision: 9, CharacterID: "a", Members: []ExpeditionMember{{CharacterID: "a"}, {CharacterID: "b"}, {CharacterID: "c"}, {CharacterID: "d"}}}
	out := encounterOutcome{defeats: []corebattle.DefeatEarnings{
		{ActorID: "b", TargetID: "e1", RecipientIDs: []string{"a", "b", "c", "d"}}, // d falls after this defeat
		{ActorID: "b", TargetID: "e2", RecipientIDs: []string{"a", "b"}},           // c has fallen
		// c is revived before the next defeats.
		{ActorID: "a", TargetID: "e3", RecipientIDs: []string{"a", "b", "c"}},
		{ActorID: "d", TargetID: "e4", RecipientIDs: []string{"a", "b", "c"}}, // a fallen killer earns nothing itself
	}}
	enemies := []corebattle.Participant{defeatEnemy("e1", false), defeatEnemy("e2", false), defeatEnemy("e3", false), defeatEnemy("e4", false)}
	monsters := []DungeonMonster{{ID: "m1", ExpReward: 1}, {ID: "m2", ExpReward: 10}, {ID: "m3", ExpReward: 100}, {ID: "m4", ExpReward: 1000}}
	res, err := s.shareDefeats(context.Background(), exp, "c", out, enemies, monsters)
	if err != nil {
		t.Fatal(err)
	}
	rng.done()
	exps := func(r corebattle.Reward) int { return r.Experience }
	credits := map[string]int{}
	facts := map[string]int{}
	for _, c := range res.credits {
		if c.ID != "r9:"+c.CharacterID {
			t.Fatalf("credit id %q", c.ID)
		}
		credits[c.CharacterID] = exps(c.Reward)
		facts[c.CharacterID] = len(c.Defeats)
	}
	// Each killer's share of its own kills and every share of the requester c
	// commit at once; the rest is stock. d earns only the defeat it was alive for.
	if want := map[string]int{"a": 100, "b": 11, "c": 1101}; !reflect.DeepEqual(credits, want) {
		t.Fatalf("credits %v, want %v", credits, want)
	}
	if want := map[string]int{"a": 1, "b": 2, "c": 0}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("defeat facts %v, want %v", facts, want)
	}
	stock := map[string]int{}
	for id, r := range res.stock {
		stock[id] = exps(r)
	}
	if want := map[string]int{"a": 1011, "b": 1100, "d": 1}; !reflect.DeepEqual(stock, want) || exps(res.leader) != 1111 {
		t.Fatalf("stock %v leader %+v, want %v", stock, res.leader, want)
	}
}

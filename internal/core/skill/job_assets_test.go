package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

func jobAssetFixture() (corebattle.LifetimeState, *corebattle.JobAssetContext, corebattle.CallbackContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP = 500
	s.Actors[0].BaseStats.Agility = 40
	held, durable := "", ""
	s.Actors[0].HeldToolID = &held
	c := &corebattle.JobAssetContext{DamageContext: base.DamageContext}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}, Assets: &corebattle.JobAssets{RequesterID: "self", Wallet: 100, HeldToolID: &durable, HelperToolIDs: []int{}, AddWallet: economy.SafeAdd}}
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{JobAssets: c}, 2)
	return s, c, f, d
}

func executeJobAsset(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, id string, draws ...float64) {
	t.Helper()
	ops, err := d.InvocationAction(s, f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) {
		t.Fatal(id, rng.used, len(draws))
	}
}

// Independent vectors: ff97fc5:_skill.cgi:1283–1300,1363–1368,
// 2470–2476,6179–6196,6503–6511. Listed MP is paid by the outer consumer.
func TestJobAssetManifestAndWalletBoundaries(t *testing.T) {
	for _, tc := range []struct {
		id                     string
		wallet, damage, credit int
		draws                  []float64
	}{
		{"job-7-entry-0", 0, 1, 0, []float64{0, 0}},
		{"job-8-entry-0", 99, 1, 0, []float64{0}},
		{"job-7-entry-0", 100, 1, 0, []float64{0}},
		{"job-8-entry-0", 16600, 224, 0, []float64{0}},
		{"job-7-entry-0", 20000, 225, 0, []float64{0}},
		{"job-7-entry-2", 100, 36, 8, []float64{0}},
		{"job-28-entry-3", 0, 45, 0, []float64{0}},
		{"job-28-entry-3", 99, 45, -99, []float64{0}},
		{"job-28-entry-3", 100, 162, -100, []float64{0}},
		{"job-81-entry-2", 100, 54, 21, []float64{0, 0}},
	} {
		t.Run(tc.id+"/"+strconv.Itoa(tc.wallet), func(t *testing.T) {
			s, c, f, d := jobAssetFixture()
			f.Assets.Wallet = tc.wallet
			executeJobAsset(t, &s, &f, d, tc.id, tc.draws...)
			if 1000-s.Actors[1].HP != tc.damage || f.Assets.Wallet != tc.wallet+tc.credit || c.DamageCalls != 1 || s.Actors[0].MP != 80 {
				t.Fatal(s, c, f.Assets)
			}
			if tc.id == "job-81-entry-2" && (s.Actors[0].HP != 536 || c.HealCalls != 1) {
				t.Fatal(s, c)
			}
			if tc.id == "job-28-entry-3" && (c.AreaDamageCalls != 1 || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"})) {
				t.Fatal(c, f)
			}
		})
	}
	s, _, f, d := jobAssetFixture()
	s.Actors[0].Attack = 3000
	s.Actors[1].HP = 10000
	executeJobAsset(t, &s, &f, d, "job-81-entry-2", 0, 0)
	if f.Assets.Wallet != 500 {
		t.Fatal("credit cap", f.Assets.Wallet)
	}
}

func TestJobAssetScavengingWeightedDrawAndSuppression(t *testing.T) {
	pool := []string{"001", "001", "001", "001", "001", "002", "002", "003", "004", "011", "011", "012", "015", "015", "041", "042", "080", "080", "081", "081", "084", "084"}
	for position, want := range pool {
		s, c, f, d := jobAssetFixture()
		executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0, 0, (float64(position)+.1)/22)
		if *s.Actors[0].HeldToolID != "item-"+want || *f.Assets.HeldToolID != "item-"+want || s.Actors[0].Agility != 50 || 1000-s.Actors[1].HP != 25 {
			t.Fatal(position, s, f.Assets, c)
		}
	}
	for _, tc := range []struct {
		helper []int
		held   string
		chance float64
		draws  []float64
		want   string
	}{
		{[]int{11}, "", 0, []float64{0, 0, 0}, ""}, // 1 matches helper11; no redraw.
		{[]int{2}, "", 0, []float64{0, 0, 0}, "item-001"},
		{[]int{}, "item-015", 0, nil, "item-015"},
		{[]int{}, "", .2, []float64{0, .2}, ""}, // Exact 1/5 boundary fails.
	} {
		s, _, f, d := jobAssetFixture()
		*s.Actors[0].HeldToolID = tc.held
		*f.Assets.HeldToolID = tc.held
		f.Assets.HelperToolIDs = tc.helper
		draws := tc.draws
		if tc.held != "" {
			draws = []float64{0}
		}
		executeJobAsset(t, &s, &f, d, "job-78-entry-5", draws...)
		if *s.Actors[0].HeldToolID != tc.want || *f.Assets.HeldToolID != tc.want {
			t.Fatal(s, f.Assets)
		}
	}
}

func TestJobAssetRequesterAndActorRemainDistinct(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, id := range []string{"job-7-entry-0", "job-7-entry-2", "job-28-entry-3", "job-81-entry-2", "job-78-entry-5"} {
			s, _, f, d := jobAssetFixture()
			s.Actors[2] = s.Actors[0].Clone()
			s.Actors[2].ID, s.Actors[2].NPC = "helper", npc
			f.ActorID = "helper"
			f.PartyIDs = []string{"self", "helper"}
			f.Assets.Wallet = 1000
			draws := []float64{0}
			if id == "job-81-entry-2" {
				draws = []float64{0, 0}
			}
			if id == "job-78-entry-5" {
				draws = []float64{0, 0, 0}
			}
			executeJobAsset(t, &s, &f, d, id, draws...)
			want := map[string]int{"job-7-entry-0": 1000, "job-7-entry-2": 1008, "job-28-entry-3": 900, "job-81-entry-2": 1021, "job-78-entry-5": 1000}[id]
			if f.Assets.Wallet != want || s.Actors[0].HP != 500 || *s.Actors[0].HeldToolID != "" {
				t.Fatal(npc, id, s, f.Assets)
			}
			if id == "job-78-entry-5" && (*s.Actors[2].HeldToolID != "item-001" || (*f.Assets.HeldToolID != "") == npc) {
				t.Fatal(npc, s, f.Assets)
			}
		}
	}
}

func TestJobAssetAdmissionAndNoTargetGuards(t *testing.T) {
	for _, bad := range []string{"requester", "helper-facts", "held-facts", "held-mismatch", "damage", "lethal-tail", "stat-tail", "late-child"} {
		s, c, f, d := jobAssetFixture()
		switch bad {
		case "requester":
			f.Assets.RequesterID = ""
		case "helper-facts":
			f.Assets.HelperToolIDs = nil
		case "held-facts":
			s.Actors[0].HeldToolID = nil
		case "held-mismatch":
			*f.Assets.HeldToolID = "item-002"
		case "damage":
			s.Actors[1].Gear = nil
		case "lethal-tail":
			s.Actors[1].HP = 1
		case "stat-tail":
			s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
		}
		ids := []string{"job-78-entry-5"}
		if bad == "late-child" {
			ids = append(ids, "missing")
		}
		before, _ := json.Marshal(s)
		assets, _ := json.Marshal(f.Assets)
		ops := []corebattle.CallbackInvocation{}
		for _, id := range ids {
			ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
		}
		if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(bad, err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(bad, err)
		}
		after, _ := json.Marshal(s)
		saved, _ := json.Marshal(f.Assets)
		if string(before) != string(after) || string(assets) != string(saved) || c.DamageCalls != 0 {
			t.Fatal(bad, s, c, f.Assets)
		}
	}
	for _, guard := range []string{"one-hit", "sealed", "missing", "zero"} {
		s, _, f, d := jobAssetFixture()
		draws := []float64{}
		switch guard {
		case "one-hit":
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		case "sealed":
			s.Actors[0].Status = corebattle.StatusPhysicalSeal
			draws = []float64{.99}
		case "missing":
			f.EnemyIDs = nil
			s.Actors = append(s.Actors[:1], s.Actors[2])
			f.Arguments = []string{""}
		case "zero":
			s.Actors[0].Attack = 0
		}
		executeJobAsset(t, &s, &f, d, "job-7-entry-2", draws...)
		if f.Assets.Wallet != 100 {
			t.Fatal(guard, f.Assets.Wallet)
		}
	}
}

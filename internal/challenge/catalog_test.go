package challenge_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// TestCatalogMatchesLegacyStageFiles checks each stage against
// party2/challenge/<n>.cgi: %k, $tresure_round and one @monsters entry.
func TestCatalogMatchesLegacyStageFiles(t *testing.T) {
	svc, err := challenge.NewService(newMockChallengeRepo(), &mockCharRepo{chars: map[string]corecharacter.Character{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	type monster struct {
		index                         int
		name                          string
		hp, mp, at, df, ag, exp, gold int
		job, oldJob                   string
		sp, oldSP                     int
	}
	for _, want := range []struct {
		id             string
		cap, low, high int
		needJoin       string
		overLv         bool
		count          int
		m              monster
	}{
		{"0", 1, 10, 20, "hp_400_u", false, 1, monster{0, "スライム", 150, 50, 100, 50, 200, 20, 5, "starter", "starter", 999, 20}},
		{"1", 2, 10, 20, "hp_400_u", false, 1, monster{0, "ラースシープ", 200, 40, 140, 100, 140, 40, 21, "job-10", "starter", 999, 20}},
		{"2", 2, 10, 20, "", false, 7, monster{5, "血塗られた手", 380, 62, 280, 150, 200, 35, 25, "job-61", "starter", 10, 20}},
		{"3", 3, 10, 20, "", false, 3, monster{2, "メイジマッシュ", 400, 99, 300, 150, 300, 50, 40, "job-90", "job-19", 999, 999}},
		{"4", 4, 10, 20, "", false, 6, monster{2, "ブルーストーン", 5, 100, 200, 2000, 500, 50, 100, "job-31", "job-31", 44, 44}},
		{"5", 4, 20, 30, "", false, 1, monster{0, "爆弾岩", 400, 42, 250, 150, 150, 30, 100, "job-94", "job-94", 20, 20}},
		{"6", 3, 20, 30, "", false, 9, monster{8, "流銀スライム", 10, 180, 210, 2000, 600, 500, 20, "job-40", "job-40", 999, 999}},
		{"7", 1, 30, 50, "", false, 14, monster{10, "竜王", 650, 200, 300, 200, 100, 200, 50, "job-41", "job-25", 999, 999}},
		{"8", 1, 30, 50, "", true, 14, monster{0, "トレント人面樹", 800, 999, 525, 210, 300, 180, 150, "job-07", "starter", 999, 999}},
	} {
		tier, err := svc.GetTier(want.id)
		if err != nil {
			t.Fatal(want.id, err)
		}
		if tier.MaxParticipants != want.cap || tier.TreasureRound != want.low || tier.TreasureRoundMax != want.high || tier.NeedJoin != want.needJoin || tier.NeedOverLv != want.overLv || len(tier.Monsters) != want.count {
			t.Errorf("stage %s settings = %d %d-%d %q %v %d", want.id, tier.MaxParticipants, tier.TreasureRound, tier.TreasureRoundMax, tier.NeedJoin, tier.NeedOverLv, len(tier.Monsters))
			continue
		}
		m := tier.Monsters[want.m.index]
		got := monster{want.m.index, m.Name, m.HP, m.MP, m.Attack, m.Defense, m.Agility, m.ExperienceReward, m.GoldReward, m.Combat.Jobs.JobID, m.Combat.Jobs.OldJobID, m.Combat.Jobs.SP, m.Combat.Jobs.OldSP}
		if got != want.m {
			t.Errorf("stage %s monster = %+v, want %+v", want.id, got, want.m)
		}
	}

	// Findings: stage 0 draws its job per load; the ボマー keeps its unlisted
	// state and tmp; stage 8's tree revives and wears item247.
	if zero, _ := svc.GetTier("0"); zero.Monsters[0].RandomJob != 59 {
		t.Error("stage 0 job is not drawn from 59 jobs")
	}
	bomber, _ := svc.GetTier("7")
	if b := bomber.Monsters[13].Combat; b.Status != "大爆発" || b.Temporary != "するぞ" || b.Tension != 3 {
		t.Errorf("bomber payload = %q %q %v", b.Status, b.Temporary, b.Tension)
	}
	tree, _ := svc.GetTier("8")
	if c := tree.Monsters[0].Combat; c.Temporary != corebattle.TemporaryRevival || c.Gear == nil || c.Gear.AccessoryID != "item-247" {
		t.Errorf("tree payload = %q %+v", c.Temporary, c.Gear)
	}
}

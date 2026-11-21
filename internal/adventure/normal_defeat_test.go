package adventure

import (
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

type rewardDraws struct {
	exDraws
	floats []float64
}

func (d *rewardDraws) Float64() float64 {
	d.calls = append(d.calls, 200)
	v := d.floats[0]
	d.floats = d.floats[1:]
	return v
}

func runRewardFixture() (NormalRun, corebattle.TargetContext, corecharacter.Character) {
	actors := []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "actor", TeamID: "party", HP: 10, Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "ally", TeamID: "party", HP: 10, Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", Name: "Bee", HP: 30, MaxHP: 30, Gear: &corebattle.BattleGearStats{}, ExperienceReward: 25, GoldReward: 21}, NPC: true, EncounterIdentity: royalBeeIdentity + "A"},
	}
	r := NormalRun{Stage: Stage{ID: "stage-22", StageTraits: StageTraits{EX: &EXStage{Marker: markerRoyalBeeDefeat}}}, Roster: []string{"actor", "ally", "fallen"}, Battle: corebattle.LifetimeState{Actors: actors}}
	return r, corebattle.TargetContext{ActorID: "actor", PartyIDs: r.Roster, EnemyIDs: []string{"enemy"}}, corecharacter.Character{ID: "actor", Level: 2, Stats: corecharacter.Stats{MaxHP: 50}}
}

func TestRunDefeatRewardModifiers(t *testing.T) {
	for _, tc := range []struct {
		name, mode, blessing string
		gear                 corebattle.BattleGearStats
		rolls, bounds        []int
		want                 RunEarnings
	}{
		{name: "normal", rolls: []int{1, 2}, bounds: []int{50, 200, 3}, want: RunEarnings{30, 25, 1}},
		{name: "calm gold", mode: CrawlModeCalm, blessing: "GOLD", rolls: []int{1, 0, 2}, bounds: []int{50, 4, 200, 3}, want: RunEarnings{14, 18, 0}},
		{name: "exp and equipment", blessing: "EXP", gear: corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01", AccessoryID: "item-167"}, rolls: []int{2, 0, 0, 2}, bounds: []int{50, 4, 4, 200, 3}, want: RunEarnings{60, 25, 0}},
		{name: "Try misses", mode: CrawlModeTry, blessing: "EXP", gear: corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01"}, rolls: []int{2, 3, 1, 0}, bounds: []int{50, 4, 4, 200, 3}, want: RunEarnings{30, 25, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, role, requester := runRewardFixture()
			r.Mode = tc.mode
			r.actor("actor").Gear = &tc.gear
			rng := &rewardDraws{exDraws: exDraws{values: tc.rolls}, floats: []float64{.9}}
			var effects RunActionEffects
			port := NewRunDefeatRewards(&r, requester, tc.blessing, false, rng, &effects)
			ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: port}
			before := r.actor("enemy").Clone()
			if err := r.Battle.ValidateDefeat(ctx, "enemy", rng); err != nil {
				t.Fatal(err)
			}
			if _, err := port.Project(&r.Battle, role, "enemy"); err != nil {
				t.Fatal(err)
			}
			if len(rng.calls) != 0 || !reflect.DeepEqual(before, *r.actor("enemy")) || !reflect.DeepEqual(effects, RunActionEffects{}) {
				t.Fatal("admission changed caller state", rng.calls, effects)
			}
			if got, err := r.Battle.ApplyDefeat(ctx, "enemy", rng); err != nil || !got.RewardsReached {
				t.Fatal(got, err)
			}
			if !slices.Equal(rng.calls, tc.bounds) || !reflect.DeepEqual(effects.Earnings, map[string]RunEarnings{"actor": tc.want, "ally": tc.want}) || !slices.Equal(effects.CreditedDefeats, []string{royalBeeIdentity + "A"}) || effects.EXUnlocked != (tc.mode == CrawlModeTry) {
				t.Fatal(effects, rng.calls)
			}
			r.Recipients = make(map[string]RunRecipient)
			if err := r.recordEarnings(effects.Earnings); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"actor", "ally"} {
				p := r.Recipients[id]
				if r.Mode == CrawlModeTry && (p.Conditional != tc.want || p.Pending != (RunEarnings{})) || r.Mode != CrawlModeTry && p.Pending != tc.want {
					t.Fatal(id, p)
				}
			}
		})
	}
}

func TestRunDefeatCaptureAndCrystalIdentity(t *testing.T) {
	for _, pvp := range []bool{false, true} {
		r, role, requester := runRewardFixture()
		r.Stage = Stage{ID: "stage-01"}
		target := r.actor("enemy")
		target.Status = "capture"
		rng := &rewardDraws{exDraws: exDraws{values: []int{1}}, floats: []float64{.02, .02}}
		var effects RunActionEffects
		port := NewRunDefeatRewards(&r, requester, "", pvp, rng, &effects)
		ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: port}
		for range 2 {
			if _, err := r.Battle.ApplyDefeat(ctx, "enemy", rng); err != nil {
				t.Fatal(err)
			}
			target.HP = 10
		}
		if !target.CrystalDropped || effects.Earnings["actor"].Crystals != 1 || len(effects.Defeats) != 2 || effects.Defeats[0].Capture == pvp || effects.Defeats[1].Capture || !effects.Defeats[0].StrongKill {
			t.Fatal(pvp, target, effects)
		}
		want := []int{50, 200, 200}
		if pvp {
			want = []int{50}
		}
		if !slices.Equal(rng.calls, want) {
			t.Fatal(rng.calls)
		}
	}
}

func TestRunDefeatSelfAndNPCGrantNothing(t *testing.T) {
	for _, npc := range []bool{false, true} {
		r, role, requester := runRewardFixture()
		r.actor("actor").NPC = npc
		id := "actor"
		if npc {
			id = "ally"
		}
		var effects RunActionEffects
		rng := &rewardDraws{}
		ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: NewRunDefeatRewards(&r, requester, "gold", false, rng, &effects)}
		got, err := r.Battle.ApplyDefeat(ctx, id, rng)
		if err != nil || !got.Defeated || got.RewardsReached || !reflect.DeepEqual(effects, RunActionEffects{}) || r.actor(id).CrystalDropRolled || len(rng.calls) != 0 {
			t.Fatal(npc, got, err, effects, rng.calls)
		}
	}
}

func TestRunDefeatLevelUpBranch(t *testing.T) {
	for _, tc := range []struct {
		level, saved, direct, released int
		mode                           string
		over, ex, want                 bool
	}{
		{level: 2, saved: 30, direct: 6, released: 4, want: true},
		{level: 2, saved: 30, direct: 5, released: 4},
		{level: 2, saved: 30, direct: 6, released: 4, mode: CrawlModeTry},
		{level: 2, saved: 40, mode: CrawlModeTry, want: true},
		{level: 2, saved: 30, direct: 10, ex: true},
		{level: 99, saved: 98010},
		{level: 99, saved: 98010, over: true, want: true},
		{level: 150, saved: 225000, over: true},
	} {
		requester := corecharacter.Character{ID: "actor", Level: tc.level, Experience: tc.saved, OverLevel: tc.over}
		fx := RunActionEffects{Earnings: map[string]RunEarnings{"actor": {EXP: tc.direct}, "ally": {EXP: 100}}}
		got, err := fx.LevelUpEligible(&corebattle.LifetimeState{SpecialBossField: tc.ex}, requester, tc.mode, tc.released)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}

func TestRunDefeatEarnedEXPReachesPlayerBranch(t *testing.T) {
	for _, mode := range []string{"normal", CrawlModeTry} {
		r, targets, requester := runRewardFixture()
		r.Mode, r.Stage = mode, Stage{ID: "stage-01"}
		requester.Experience = 10
		rng := &ordinaryRNG{value: .9}
		var fx RunActionEffects
		defeat := &corebattle.DefeatContext{
			EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: targets},
			Rewards:               NewRunDefeatRewards(&r, requester, "", false, rng, &fx),
		}
		op := corebattle.PlayerOperation{
			Tail: corebattle.PlayerTailContext{
				DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: targets, RequesterLevel: &requester.Level}},
				DefeatContext: defeat,
			},
			First: corebattle.PlayerAction{
				EntryID: "test-defeat",
				Validate: func(s *corebattle.LifetimeState) error {
					return s.ValidateDefeat(defeat, "enemy", rng)
				},
				Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
					_, err := s.ApplyDefeat(defeat, "enemy", rng)
					return corebattle.PlayerActionFacts{}, err
				},
			},
			ResponseEnabled: true,
			LevelUp: func(s *corebattle.LifetimeState, _ random.Generator) (bool, error) {
				return fx.LevelUpEligible(s, requester, mode, 0)
			},
		}
		result, err := r.Battle.ApplyPlayerOperation(&op, rng)
		if err != nil || !result.Accepted || result.ResponseReached != (mode == CrawlModeTry) || fx.Earnings[requester.ID].EXP != 30 {
			t.Fatal(mode, result, fx.Earnings, err)
		}
	}
}

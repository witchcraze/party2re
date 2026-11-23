package runhost

import (
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

// rewardDraws records each draw's bound (200 for a float) and returns the
// queued values.
type rewardDraws struct {
	values, calls []int
	floats        []float64
}

func (d *rewardDraws) Intn(n int) int {
	d.calls = append(d.calls, n)
	v := d.values[0]
	d.values = d.values[1:]
	return v
}
func (d *rewardDraws) IntN(n int) int { return d.Intn(n) }
func (d *rewardDraws) Float64() float64 {
	d.calls = append(d.calls, 200)
	v := d.floats[0]
	d.floats = d.floats[1:]
	return v
}

// rewardRoom is a room library halving rewards in calm mode and drawing a
// marker on each credited defeat, as Adventure's Calm and Royal Bee seams do.
func rewardRoom(calm bool, credited *[]string, marked *bool) RoomCombat {
	return RoomCombat{
		Habitat: func() string { return "Hive" },
		Scale: func(e Earnings) Earnings {
			if calm {
				e.EXP, e.Gold, e.Crystals = e.EXP/2, e.Gold/2, e.Crystals/2
			}
			return e
		},
		Credited: func(identity string, rng random.Generator) {
			*credited = append(*credited, identity)
			*marked = rng.Intn(3) == 0 || *marked
		},
	}
}

func runRewardFixture() (State, corebattle.TargetContext, corecharacter.Character) {
	actors := []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "actor", TeamID: "party", HP: 10, Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "ally", TeamID: "party", HP: 10, Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", Gear: &corebattle.BattleGearStats{}}},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", Name: "Bee", HP: 30, MaxHP: 30, Gear: &corebattle.BattleGearStats{}, ExperienceReward: 25, GoldReward: 21}, NPC: true, EncounterIdentity: "bee-A"},
	}
	r := State{Roster: []string{"actor", "ally", "fallen"}, Battle: corebattle.LifetimeState{Actors: actors}}
	return r, corebattle.TargetContext{ActorID: "actor", PartyIDs: r.Roster, EnemyIDs: []string{"enemy"}}, corecharacter.Character{ID: "actor", Level: 2, Stats: corecharacter.Stats{MaxHP: 50}}
}

func TestRunDefeatRewardModifiers(t *testing.T) {
	for _, tc := range []struct {
		name, blessing string
		calm, try      bool
		gear           corebattle.BattleGearStats
		rolls, bounds  []int
		want           Earnings
	}{
		{name: "normal", rolls: []int{1, 2}, bounds: []int{50, 200, 3}, want: Earnings{EXP: 30, Gold: 25, Crystals: 1}},
		{name: "calm gold", calm: true, blessing: "GOLD", rolls: []int{1, 0, 2}, bounds: []int{50, 4, 200, 3}, want: Earnings{EXP: 14, Gold: 18, Crystals: 0}},
		{name: "exp and equipment", blessing: "EXP", gear: corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01", AccessoryID: "item-167"}, rolls: []int{2, 0, 0, 2}, bounds: []int{50, 4, 4, 200, 3}, want: Earnings{EXP: 60, Gold: 25, Crystals: 0}},
		{name: "conditional misses", try: true, blessing: "EXP", gear: corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01"}, rolls: []int{2, 3, 1, 0}, bounds: []int{50, 4, 4, 200, 3}, want: Earnings{EXP: 30, Gold: 25, Crystals: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, role, requester := runRewardFixture()
			r.Actor("actor").Gear = &tc.gear
			rng := &rewardDraws{values: tc.rolls, floats: []float64{.9}}
			var effects Effects
			var credited []string
			marked := false
			port := defeatRewards(&r, rewardRoom(tc.calm, &credited, &marked), requester, tc.blessing, rng, &effects)
			ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: port}
			before := r.Actor("enemy").Clone()
			if err := r.Battle.ValidateDefeat(ctx, "enemy", rng); err != nil {
				t.Fatal(err)
			}
			if _, err := port.Project(&r.Battle, role, "enemy"); err != nil {
				t.Fatal(err)
			}
			if len(rng.calls) != 0 || !reflect.DeepEqual(before, *r.Actor("enemy")) || !reflect.DeepEqual(effects, Effects{}) || credited != nil {
				t.Fatal("admission changed caller state", rng.calls, effects)
			}
			if got, err := r.Battle.ApplyDefeat(ctx, "enemy", rng); err != nil || !got.RewardsReached {
				t.Fatal(got, err)
			}
			if !slices.Equal(rng.calls, tc.bounds) || !reflect.DeepEqual(effects.Earnings, map[string]Earnings{"actor": tc.want, "ally": tc.want}) || !slices.Equal(credited, []string{"bee-A"}) || marked != tc.try ||
				len(effects.Defeats) != 1 || effects.Defeats[0].Monster.Habitat != "Hive" {
				t.Fatal(effects, rng.calls)
			}
			r.Recipients = make(map[string]Recipient)
			if err := r.RecordEarnings(effects.Earnings, tc.try); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"actor", "ally"} {
				p := r.Recipients[id]
				if tc.try && (p.Conditional != tc.want || p.Pending != (Earnings{})) || !tc.try && p.Pending != tc.want {
					t.Fatal(id, p)
				}
			}
		})
	}
}

func TestRunDefeatCaptureAndCrystalIdentity(t *testing.T) {
	r, role, requester := runRewardFixture()
	target := r.Actor("enemy")
	target.Status = "capture"
	rng := &rewardDraws{values: []int{1}, floats: []float64{.02, .02}}
	var effects Effects
	port := defeatRewards(&r, RoomCombat{}, requester, "", rng, &effects)
	ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: port}
	for range 2 {
		if _, err := r.Battle.ApplyDefeat(ctx, "enemy", rng); err != nil {
			t.Fatal(err)
		}
		target.HP = 10
	}
	if !target.CrystalDropped || effects.Earnings["actor"].Crystals != 1 || len(effects.Defeats) != 2 || !effects.Defeats[0].Capture || effects.Defeats[1].Capture || !effects.Defeats[0].StrongKill || effects.Defeats[0].Monster.Habitat != "" {
		t.Fatal(target, effects)
	}
	if !slices.Equal(rng.calls, []int{50, 200, 200}) {
		t.Fatal(rng.calls)
	}
}

func TestRunDefeatSelfAndNPCGrantNothing(t *testing.T) {
	for _, npc := range []bool{false, true} {
		r, role, requester := runRewardFixture()
		r.Actor("actor").NPC = npc
		id := "actor"
		if npc {
			id = "ally"
		}
		var effects Effects
		rng := &rewardDraws{}
		ctx := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: role}, Rewards: defeatRewards(&r, RoomCombat{}, requester, "gold", rng, &effects)}
		got, err := r.Battle.ApplyDefeat(ctx, id, rng)
		if err != nil || !got.Defeated || got.RewardsReached || !reflect.DeepEqual(effects, Effects{}) || r.Actor(id).CrystalDropRolled || len(rng.calls) != 0 {
			t.Fatal(npc, got, err, effects, rng.calls)
		}
	}
}

func TestRunDefeatLevelUpBranch(t *testing.T) {
	for _, tc := range []struct {
		level, saved, direct, released int
		conditional, over, ex, want    bool
	}{
		{level: 2, saved: 30, direct: 6, released: 4, want: true},
		{level: 2, saved: 30, direct: 5, released: 4},
		{level: 2, saved: 30, direct: 6, released: 4, conditional: true},
		{level: 2, saved: 40, conditional: true, want: true},
		{level: 2, saved: 30, direct: 10, ex: true},
		{level: 99, saved: 98010},
		{level: 99, saved: 98010, over: true, want: true},
		{level: 150, saved: 225000, over: true},
	} {
		requester := corecharacter.Character{ID: "actor", Level: tc.level, Experience: tc.saved, OverLevel: tc.over}
		got, err := levelUpDue(&corebattle.LifetimeState{SpecialBossField: tc.ex}, requester, tc.conditional, tc.released, tc.direct)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}

func TestRunDefeatEarnedEXPReachesPlayerBranch(t *testing.T) {
	for _, conditional := range []bool{false, true} {
		r, targets, requester := runRewardFixture()
		requester.Experience = 10
		rng := &fixedRNG{value: .9}
		var fx Effects
		defeat := &corebattle.DefeatContext{
			EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: targets},
			Rewards:               defeatRewards(&r, RoomCombat{}, requester, "", rng, &fx),
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
				return levelUpDue(s, requester, conditional, 0, fx.Earnings[requester.ID].EXP)
			},
		}
		result, err := r.Battle.ApplyPlayerOperation(&op, rng)
		if err != nil || !result.Accepted || result.ResponseReached != conditional || fx.Earnings[requester.ID].EXP != 30 {
			t.Fatal(conditional, result, fx.Earnings, err)
		}
	}
}

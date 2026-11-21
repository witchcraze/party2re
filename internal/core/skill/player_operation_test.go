package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type zeroDraws struct{}

func (zeroDraws) Float64() float64 { return 0 }
func (zeroDraws) Intn(int) int     { return 0 }
func (zeroDraws) IntN(int) int     { return 0 }

// _battle.cgi:761–789: a custom entry (listed MP 0) is charged CMP once by its
// bound callback; an ordinary entry is charged its listed MP and no CMP.
func TestPlayerOperationChargesCustomCMPAndOrdinaryMP(t *testing.T) {
	for _, custom := range []bool{false, true} {
		s, c, d := playerCustomFixture([3]string{"gem_dance_combo", "", ""}, 30)
		a := &s.Actors[0]
		a.CMPCharge, a.CMPNeedCharge = 0, 9
		cmp, mp := a.CMP, a.MP
		level := 50
		action := corebattle.PlayerAction{EntryID: "job-0-entry-0", Cost: 5, Validate: func(*corebattle.LifetimeState) error { return nil },
			Execute: func(*corebattle.LifetimeState, random.Generator) (corebattle.PlayerActionFacts, error) {
				return corebattle.PlayerActionFacts{}, nil
			}}
		if custom {
			action = corebattle.PlayerAction{EntryID: "custom", Validate: func(*corebattle.LifetimeState) error { return nil },
				Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
					f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
					return corebattle.PlayerActionFacts{}, d.ApplyPlayerCustomAction(s, &f, []string{"attack"}, rng)
				}}
		}
		op := &corebattle.PlayerOperation{First: action, Tail: corebattle.PlayerTailContext{
			DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext, RequesterLevel: &level}},
			DefeatContext: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext, RequesterLevel: &level}},
		}}
		result, err := s.ApplyPlayerOperation(op, zeroDraws{})
		wantCMP, wantMP := cmp, mp-5
		if custom {
			wantCMP, wantMP = cmp-30, mp
		}
		if err != nil || !result.Accepted || s.Actors[0].CMP != wantCMP || s.Actors[0].MP != wantMP {
			t.Fatal(custom, result, err, s.Actors[0].CMP, s.Actors[0].MP)
		}
	}
}

func TestPlayerOperationCertifiedConcreteSecondPair(t *testing.T) {
	newState := func(mp int) (corebattle.LifetimeState, corebattle.TargetContext, BattleDefinitions, corebattle.CallbackContext) {
		level := 50
		c := corebattle.TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}, EnemyIDs: []string{"e"}}
		s := corebattle.LifetimeState{
			Actors: []corebattle.LifetimeActor{
				{Participant: corebattle.Participant{ID: "a", TeamID: "party", HP: 100, MaxHP: 100, MP: mp, MaxMP: 50, CMP: 1, MaxCMP: 10}, CMPNeedCharge: 3, Tension: 1, Hit: 95, Extended: corebattle.ExtendedAdditional, ExtendedTurns: 2},
				{Participant: corebattle.Participant{ID: "b", TeamID: "party", HP: 100, MaxHP: 100}, Tension: 1, Hit: 95},
				{Participant: corebattle.Participant{ID: "e", TeamID: "enemy", HP: 100, MaxHP: 100}, NPC: true, Tension: 1, Hit: 95},
			},
		}
		for i := range s.Actors {
			s.Actors[i].Gear = &corebattle.BattleGearStats{}
		}
		damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: c, RequesterLevel: &level}}
		var d BattleDefinitions
		d.RegisterPlayerCallbacks(PlayerCallbackContexts{
			ProtectiveMagic: &corebattle.ProtectiveMagicContext{DamageContext: damage},
		}, len(c.PartyIDs))
		frame := corebattle.CallbackContext{TargetContext: c, Damage: &damage}
		return s, c, d, frame
	}
	bind := func(d BattleDefinitions, frame corebattle.CallbackContext, id string, cost int) corebattle.PlayerAction {
		inv := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}
		return corebattle.PlayerAction{
			EntryID: id, Cost: cost, Whitelisted: true,
			Certify: func(s *corebattle.LifetimeState) error {
				return s.CertifyInvocations(&frame, d.Invocations, inv, zeroDraws{})
			},
			Validate: func(s *corebattle.LifetimeState) error {
				return s.ValidateInvocations(&frame, d.Invocations, inv, zeroDraws{})
			},
			Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
				work := frame
				return corebattle.PlayerActionFacts{}, s.ApplyInvocations(&work, d.Invocations, inv, rng)
			},
		}
	}
	level := 50
	for _, tc := range []struct {
		mp, wantMP int
		wantTemp   corebattle.TemporaryEffect
	}{
		{13, 7, corebattle.TemporaryReduceMagic},  // 13-6=7 == 7: strict boundary skips second
		{14, 1, corebattle.TemporaryReduceBreath}, // 14-6=8 > 7: second executes after first
	} {
		s, c, d, frame := newState(tc.mp)
		second := bind(d, frame, "job-33-entry-2", 7)
		op := &corebattle.PlayerOperation{
			First:  bind(d, frame, "job-33-entry-0", 6),
			Second: &second,
			Tail: corebattle.PlayerTailContext{
				DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: c, RequesterLevel: &level}},
				DefeatContext: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c, RequesterLevel: &level}},
			},
		}
		res, err := s.ApplyPlayerOperation(op, zeroDraws{})
		if err != nil || !res.Accepted || s.Actors[0].MP != tc.wantMP || s.Actors[0].Temporary != tc.wantTemp || s.Actors[0].ExtendedTurns != 1 {
			t.Fatalf("mp=%d res=%+v err=%v actor=%+v", tc.mp, res, err, s.Actors[0])
		}
	}

	// Unregistered reachable descendant on the concrete pair rejects at admission even at MP=13.
	s, c, d, frame := newState(13)
	def := d.Invocations.Entries["job-33-entry-2"]
	def.Children = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "unregistered-child"}}
	d.Invocations.Entries["job-33-entry-2"] = def
	second := bind(d, frame, "job-33-entry-2", 7)
	op := &corebattle.PlayerOperation{
		First:  bind(d, frame, "job-33-entry-0", 6),
		Second: &second,
		Tail: corebattle.PlayerTailContext{
			DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: c, RequesterLevel: &level}},
			DefeatContext: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c, RequesterLevel: &level}},
		},
	}
	before := s
	before.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
	if _, err := s.ApplyPlayerOperation(op, zeroDraws{}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

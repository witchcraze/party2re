package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func recoveryGemFixture(npc bool) (corebattle.LifetimeState, corebattle.EffectContext) {
	s := corebattle.LifetimeState{}
	for _, id := range []string{"self", "fallen", "ally", "enemy"} {
		a := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: id, TeamID: "party", HP: 100, MaxHP: 1000, MP: 100, CMP: 200, Attack: 150, Defense: 150, Agility: 150, BaseStats: corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}}, Tension: 1, Hit: 50}
		if id == "enemy" {
			a.TeamID, a.NPC = "enemy", true
		}
		if id == "fallen" {
			a.HP = 0
		}
		if id == "self" {
			a.NPC = npc
		}
		s.Actors = append(s.Actors, a)
	}
	level := 99
	c := corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self", "fallen", "ally"}, EnemyIDs: []string{"enemy"}, Custom: true}, ActorNPC: npc, RequesterLevel: &level}
	return s, c
}

type recoveryGemRNG struct{}

func (recoveryGemRNG) Float64() float64 { return 0 }
func (recoveryGemRNG) Intn(int) int     { panic("unexpected integer draw") }
func (recoveryGemRNG) IntN(int) int     { panic("unexpected integer draw") }

func TestRecoveryGemCertifiedRegistrationAndBodies(t *testing.T) {
	wantCosts := map[int]int{19: 3, 20: 10, 21: 16, 22: 12, 23: 22, 28: 16, 29: 28, 30: 12, 31: 18, 32: 8, 33: 13, 54: 1, 67: 57, 68: 20, 69: 37, 74: 11, 93: 12, 94: 9, 95: 7, 96: 12}
	if len(corebattle.RecoveryStatGemEntries()) != len(wantCosts) {
		t.Fatal("incomplete subset")
	}
	for _, npc := range []bool{false, true} {
		for _, entry := range corebattle.RecoveryStatGemEntries() {
			t.Run(entry.ID+map[bool]string{false: "/player", true: "/npc"}[npc], func(t *testing.T) {
				s, c := recoveryGemFixture(npc)
				var d BattleDefinitions
				if _, err := d.RecoveryStatAction(&s, &c, []string{entry.ID}, "ally", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				d.RegisterRecoveryStatGems(&c)
				before := s.Actors[0]
				entries, err := d.RecoveryStatAction(&s, &c, []string{entry.ID}, "ally", noArithmeticDraws{t})
				if err != nil || len(entries) != 1 || entries[0] != entry || entry.CMPCost != wantCosts[entry.SourceID] {
					t.Fatal(entries, err, entry)
				}
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
				ops, err := d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, recoveryGemRNG{}); err != nil {
					t.Fatal(err)
				}
				if s.Actors[0].CMP != before.CMP || s.Actors[0].MP != before.MP {
					t.Fatal("callback charged outer cost", s)
				}
				self, fallen, ally, enemy := s.Actors[0], s.Actors[1], s.Actors[2], s.Actors[3]
				ok := false
				switch entry.SourceID {
				case 19:
					ok = ally.HP == 190 && self.HP == 100
				case 20:
					ok = self.HP == 370 && ally.HP == 100
				case 21:
					ok = ally.HP == 370 && self.HP == 100
				case 22:
					ok = self.HP == 280 && ally.HP == 190 && fallen.HP == 0
				case 23:
					ok = self.HP == 640 && ally.HP == 370 && fallen.HP == 0
				case 28:
					ok = self.Attack == 250 && ally.Attack == 150
				case 29:
					ok = ally.Attack == 250 && self.Attack == 150
				case 30:
					ok = self.Defense == 250 && ally.Defense == 150
				case 31:
					ok = ally.Defense == 250 && self.Defense == 150
				case 32:
					ok = self.Agility == 250 && ally.Agility == 150
				case 33:
					ok = ally.Agility == 250 && self.Agility == 150
				case 54:
					ok = self.Hit == 95 && fallen.Hit == 95 && ally.Hit == 95 && enemy.Hit == 50
				case 67:
					ok = self.Tension == 3 && ally.Tension == 1 && c.TensionCalls == 2
				case 68:
					ok = enemy.Tension == 1
				case 69:
					ok = self.Tension == 1 && fallen.Tension == 1 && ally.Tension == 1 && enemy.Tension == 1
				case 74:
					ok = ally.Tension == 1.7 && self.Tension == 1
				case 93:
					ok = enemy.Attack == 113
				case 94:
					ok = enemy.Defense == 113
				case 95:
					ok = enemy.Agility == 113
				case 96:
					ok = enemy.Hit == 50 && c.StatDownCalls == 1
				}
				if !ok {
					t.Fatal(entry, s, c)
				}
			})
		}
	}
}

func TestRecoveryGemCompositeAdmissionAndReload(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		ids := []string{"gem_divine_tension_sky", "gem_heal_1", "gem_heal_1"}
		if unsupported {
			ids[2] = "gem-101"
		}
		d.Invocations.Entries["custom"] = d.RecoveryStatInvocation(&c, ids)
		ids[0] = "mutated input"
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		ops, err := d.InvocationAction(&s, &frame, []string{"custom"}, noArithmeticDraws{t})
		if unsupported {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			err = s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "custom"}}, noArithmeticDraws{t})
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || c.TensionCalls != 0 {
				t.Fatal(err, c)
			}
			after, err := json.Marshal(s)
			if err != nil || string(before) != string(after) {
				t.Fatal(err, s)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var reloaded corebattle.LifetimeState
		if err := json.Unmarshal(before, &reloaded); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, recoveryGemRNG{}); err != nil {
			t.Fatal(err)
		}
		_, op := recoveryGemFixture(false)
		var loadedDefinitions BattleDefinitions
		loadedDefinitions.RegisterRecoveryStatGems(&op)
		loadedDefinitions.Invocations.Entries["custom"] = loadedDefinitions.RecoveryStatInvocation(&op, []string{"gem_divine_tension_sky", "gem_heal_1", "gem_heal_1"})
		loadedFrame := corebattle.CallbackContext{TargetContext: op.TargetContext, Arguments: []string{"ally"}}
		if err := reloaded.ApplyInvocations(&loadedFrame, loadedDefinitions.Invocations, ops, recoveryGemRNG{}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, reloaded) || s.Actors[2].HP != 460 || c.TensionCalls != 2 || c.HealCalls != 2 {
			t.Fatal(s, reloaded, c)
		}
		// Loading committed state requires no execution or callback reconstruction.
		committed, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(committed, &reloaded); err != nil || !reflect.DeepEqual(s, reloaded) {
			t.Fatal(err, reloaded)
		}
	}
}

func TestRecoveryGemArgumentRoles(t *testing.T) {
	for _, tc := range []struct {
		id              string
		custom, consume bool
	}{
		{"gem_heal_sky_1", false, false},
		{"gem_heal_1", false, true},
		{"gem_heal_sky_1", true, true},
	} {
		s, c := recoveryGemFixture(false)
		c.Custom = tc.custom
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "trailing"}}
		err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: tc.id}}, recoveryGemRNG{})
		// ApplyInvocations freezes each slot's arguments; naked invocation shares
		// its child frame internally, while the submitted argument slice is intact.
		if err != nil || !reflect.DeepEqual(frame.Arguments, []string{"ally", "trailing"}) {
			t.Fatal(tc, err, frame)
		}
		child := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "trailing"}}
		def := d.Invocations.Entries[tc.id]
		if err := def.Execute(&s, &child, recoveryGemRNG{}); err != nil {
			t.Fatal(err)
		}
		want := []string{"ally", "trailing"}
		if tc.consume {
			want = []string{"trailing"}
		}
		if !reflect.DeepEqual(child.Arguments, want) {
			t.Fatal(tc, child)
		}
	}
}

func TestRecoveryGemSlotLimitSharedByDiscoveryAndInvocation(t *testing.T) {
	for _, ids := range [][]string{
		{"gem_buff_atk_1", "gem_divine_tension_sky", "gem_buff_atk_1", "gem_divine_tension_sky"},
		{"gem_heal_1", "gem_buff_atk_1", "gem_divine_tension_sky", "gem_buff_atk_1"},
		{"gem_buff_atk_1", "", "gem_divine_tension_sky", "gem_buff_atk_1"},
	} {
		s, c := recoveryGemFixture(false)
		c.RequesterLevel = nil
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		d.Invocations.Entries["oversized"] = d.RecoveryStatInvocation(&c, ids)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		contextBefore := c
		if _, err := d.RecoveryStatAction(&s, &c, ids, "self", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		if _, err := d.InvocationAction(&s, &frame, []string{"oversized"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "oversized"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		after, err := json.Marshal(s)
		if err != nil || string(before) != string(after) || !reflect.DeepEqual(c, contextBefore) || !reflect.DeepEqual(frame.Arguments, []string{"self"}) {
			t.Fatal("oversized invocation mutated input/state/counters", ids, err, s, c, frame)
		}
	}
}

type queuedRecoveryGemRNG struct {
	t     *testing.T
	draws []float64
}

func (r *queuedRecoveryGemRNG) Float64() float64 {
	if len(r.draws) == 0 {
		r.t.Fatal("unexpected float draw")
	}
	v := r.draws[0]
	r.draws = r.draws[1:]
	return v
}

func (r *queuedRecoveryGemRNG) Intn(int) int { panic("unexpected integer draw") }
func (r *queuedRecoveryGemRNG) IntN(int) int { panic("unexpected integer draw") }

func TestRecoveryGemTargetAlternativesSharedByDiscoveryAndInvocation(t *testing.T) {
	// Single registered gem_inspire_sky (74): cover/fallen alternatives all
	// converge on ally (1.7->3) without requiring requester level.
	s, c := recoveryGemFixture(false)
	c.RequesterLevel = nil
	s.Actors[0].Temporary = corebattle.TemporaryCover
	s.Actors[2].Temporary, s.Actors[2].Tension = corebattle.TemporaryCovering, 1.7
	var d BattleDefinitions
	d.RegisterRecoveryStatGems(&c)
	entries, err := d.RecoveryStatAction(&s, &c, []string{"gem_inspire_sky"}, "enemy", noArithmeticDraws{t})
	if err != nil || len(entries) != 1 || entries[0].SourceID != 74 {
		t.Fatal(entries, err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&s, &frame, []string{"gem_inspire_sky"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &queuedRecoveryGemRNG{t: t, draws: []float64{.4}}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if len(rng.draws) != 0 || s.Actors[2].Tension != 3 || s.Actors[0].Tension != 1 || c.TensionCalls != 1 {
		t.Fatal(s, c, rng.draws)
	}

	// 3-slot composite [94, 28, 74]: discovery and invocation agree across cure
	// failure/success, confusion presence, and requester presence.
	ids := []string{"gem_debuff_def", "gem_buff_atk_1", "gem_inspire_sky"}
	for _, tc := range []struct {
		name       string
		confused   bool
		enemyStart float64
		requester  bool
		draws      []float64
		wantErr    error
	}{
		{"confused_enemy_8_seal_cure_fails", true, 8, false, []float64{.9, .9}, nil},
		{"confused_enemy_8_seal_cure_succeeds", true, 8, false, []float64{0, 0, 0, 0}, nil},
		{"confused_enemy_5_rejects_without_requester", true, 5, false, nil, corebattle.ErrUnsupportedEffect},
		{"confused_enemy_5_admitted_with_requester", true, 5, true, []float64{.9, .9}, nil},
		{"no_confusion_enemy_5_seal_cure_fails", false, 5, false, []float64{.9, .9}, nil},
		{"no_confusion_enemy_5_seal_cure_succeeds", false, 5, false, []float64{0, .9}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryGemFixture(false)
			c.Item195Activated = tc.confused
			if !tc.requester {
				c.RequesterLevel = nil
			}
			s.Actors[0].Status, s.Actors[0].Temporary, s.Actors[0].Tension = corebattle.StatusPhysicalSeal, corebattle.TemporaryCover, 8
			s.Actors[2].Temporary, s.Actors[2].Tension = corebattle.TemporaryCovering, 1.7
			s.Actors[3].Tension = tc.enemyStart
			var d BattleDefinitions
			d.RegisterRecoveryStatGems(&c)
			d.Invocations.Entries["custom"] = d.RecoveryStatInvocation(&c, ids)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			contextBefore := c
			_, err = d.RecoveryStatAction(&s, &c, ids, "enemy", noArithmeticDraws{t})
			if !errors.Is(err, tc.wantErr) {
				t.Fatal(err)
			}
			ops, err := d.InvocationAction(&s, &frame, []string{"custom"}, noArithmeticDraws{t})
			if !errors.Is(err, tc.wantErr) {
				t.Fatal(err)
			}
			if tc.wantErr != nil {
				err = s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "custom"}}, noArithmeticDraws{t})
				if !errors.Is(err, tc.wantErr) {
					t.Fatal(err)
				}
				after, err := json.Marshal(s)
				if err != nil || string(before) != string(after) || !reflect.DeepEqual(c, contextBefore) {
					t.Fatal("rejected composite mutated state or counters", s, c)
				}
				return
			}
			rng := &queuedRecoveryGemRNG{t: t, draws: tc.draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			if len(rng.draws) != 0 || s.Actors[3].Defense != 20 || s.Actors[0].Tension != 1 || s.Actors[2].Tension != 3 || c.PartyStatDownCalls != 1 || c.TensionCalls != 1 {
				t.Fatal(s, c, rng.draws)
			}
		})
	}
}

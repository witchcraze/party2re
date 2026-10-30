package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestNamedFieldDarknessUsesAdapterBaseAttack(t *testing.T) {
	ctx := context.Background()
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "player", Level: 70, JobID: "job-22", Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 100, MaxMP: 100, Attack: 100, Defense: 80, Agility: 60}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(newMockInvRepo()), battle.WithEquipmentRepository(newMockEquipRepo()), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Attack = "party", 300
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a}}
	c := corebattle.NamedFieldContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: a.ID, PartyIDs: []string{a.ID}}}}}
	var d skill.BattleDefinitions
	d.RegisterNamedFields(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	rng := &resetTableRNG{t: t}
	ops, err := d.InvocationAction(&s, &f, []string{"job-22-entry-5"}, rng)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || s.Actors[0].Attack != 350 || s.Fields.Temporary != "darkness" || rng.draws != 1 || chars.characters[char.ID].Stats != char.Stats {
		t.Fatal(err, s, rng.draws)
	}
}

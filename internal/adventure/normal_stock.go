package adventure

import (
	"context"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/random"
)

const stockLevelAction = "stock-level"

func (s *NormalRunService) stockLevel(ctx context.Context, r *NormalRun, cmd RunCommand, requester corecharacter.Character, now time.Time) error {
	ops, err := r.normalOperations(cmd.ActorID, NormalOperationContext{Requester: &requester})
	if err != nil {
		return err
	}
	parsed, err := ops.parse(cmd)
	if err != nil {
		return err
	}
	if now.Before(r.LastActions[cmd.ActorID].Add(time.Duration(r.Admission.Speed) * time.Second)) {
		return ErrRunCooldown
	}
	if s.cfg.Jobs == nil {
		return ErrRunUnavailable
	}
	def, err := s.cfg.Jobs.FindByID(requester.JobID)
	if err != nil {
		return err
	}
	seals, err := battle.BuildWeaponSealTable(ops.actor.Participant, s.cfg.RNG)
	if err != nil {
		return err
	}
	targets := ops.ctx.Callback.TargetContext
	damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: targets, RequesterLevel: &requester.Level}}
	defeat := &corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: targets, RequesterLevel: &requester.Level, Seals: &seals},
		FieldAtLoad:           r.Battle.Fields.Current(),
	}
	level := requester.Level
	grow := func(b *corebattle.LifetimeState, rng random.Generator, single bool) (bool, error) {
		i := slices.IndexFunc(b.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == cmd.ActorID })
		if i < 0 {
			return false, ErrRunConflict
		}
		a := &b.Actors[i]
		held := ""
		if a.HeldToolID != nil {
			held = *a.HeldToolID
		} else if a.Gear != nil {
			held = a.Gear.AccessoryID
		}
		startingLevel, startingSP := requester.Level, requester.SP
		growth, err := progression.ApplyStockLevels(&requester, def, rng, held, single)
		if err != nil {
			return false, err
		}
		cappedLevels := 0
		if requester.OverLevel {
			cappedLevels = max(0, progression.MaxLevel-startingLevel)
		}
		a.ApplyLevelGrowth(growth, requester.SP-startingSP, cappedLevels)
		return len(growth) > 0, nil
	}
	first := parsed.First.PlayerAction
	first.Validate = func(*corebattle.LifetimeState) error { return nil } // Parsed and admitted above.
	first.Execute = func(b *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		_, err := grow(b, rng, false)
		return corebattle.PlayerActionFacts{}, err
	}
	op := corebattle.PlayerOperation{
		First: first,
		Tail:  corebattle.PlayerTailContext{DamageContext: damage, DefeatContext: defeat},
		Field: corebattle.NamedFieldContext{DamageContext: damage, Death: defeat},
		// Treasure rooms disable NPC-dependent field exercises even after all boxes open.
		ResponseEnabled: r.Floor != TreasureRoomFloor && r.Floor != EXBossFloor+1,
		LevelUp: func(b *corebattle.LifetimeState, rng random.Generator) (bool, error) {
			if b.SpecialBossField || requester.Level != level {
				return false, nil
			}
			return grow(b, rng, true)
		},
	}
	result, err := r.applyPlayerOperation(&op, s.cfg.RNG)
	if err != nil {
		return err
	}
	if !result.Accepted {
		return ErrRunUnavailable
	}
	if requester.Level == level {
		return nil
	}
	return s.cfg.Characters.Update(ctx, requester)
}

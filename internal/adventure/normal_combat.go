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
	"github.com/witchcraze/party2re/internal/core/skill"
)

// runCombat is the production RunCombat. It runs inside Command's transaction
// after the run and character locks; every port is rebuilt per operation.
type runCombat struct{ s *NormalRunService }

// NewRunCombat composes the explicit player step and one NPC response over s.
func NewRunCombat(s *NormalRunService) RunCombat { return runCombat{s: s} }

func (s *NormalRunService) SetCombat(c RunCombat) { s.cfg.Combat = c }

func (c runCombat) resetContext(ctx context.Context, r *NormalRun, actorID string) (corebattle.EquipmentResetContext, error) {
	actor := r.actor(actorID)
	if actor == nil {
		return corebattle.EquipmentResetContext{}, ErrNotParticipant
	}
	requester, err := c.s.cfg.Characters.FindByID(ctx, actorID)
	if err != nil {
		return corebattle.EquipmentResetContext{}, err
	}
	frame := corebattle.TargetContext{ActorID: actorID}
	for _, a := range r.Battle.Actors {
		if a.TeamID == actor.TeamID {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		} else if slices.Contains(r.Enemies, a.ID) && a.HP > 0 {
			frame.EnemyIDs = append(frame.EnemyIDs, a.ID)
		}
	}
	return corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: &requester.Level}, nil
}

// PrepareAdvance is explicit advance's all-member reset with item188's revival
// pass, then the floor field reset that keeps the default field.
func (c runCombat) PrepareAdvance(ctx context.Context, r *NormalRun, actorID string) error {
	reset, err := c.resetContext(ctx, r, actorID)
	if err != nil {
		return err
	}
	if err := battle.ResetAll(&r.Battle, &reset, c.s.cfg.RNG); err != nil {
		return err
	}
	r.Battle.Fields.ResetFloor()
	return nil
}

// PrepareBossField caps the party through the shared reset owner once the
// stage24 floor10 enemies exist.
func (c runCombat) PrepareBossField(ctx context.Context, r *NormalRun, actorID string) error {
	reset, err := c.resetContext(ctx, r, actorID)
	if err != nil {
		return err
	}
	return r.Battle.ApplySpecialBossField(&reset, c.s.cfg.RNG)
}

// ExecuteRunAction runs one parsed submission through the source-ordered
// player step, then at most one interval-ready NPC response. Any rejection or
// execution error is returned so Command commits nothing.
// Transaction: delegated to Command. Lock Order: none of its own; a level-up
// writes the requester Command already locked (Rank 2) before settlement.
func (c runCombat) ExecuteRunAction(ctx context.Context, r *NormalRun, cmd RunCommand) (RunActionEffects, error) {
	s := c.s
	fx := RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	if s.cfg.Jobs == nil {
		return fx, ErrRunUnavailable
	}
	requester, err := s.cfg.Characters.FindByID(ctx, cmd.ActorID)
	if err != nil {
		return fx, err
	}
	blessing := ""
	if s.cfg.Blessings != nil {
		if blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, cmd.ActorID); err != nil {
			return fx, err
		}
	}
	def, err := s.cfg.Jobs.FindByID(requester.JobID)
	if err != nil {
		return fx, err
	}
	state, frame, inspection, err := r.ProjectCombat(cmd.ActorID)
	if err != nil {
		return fx, err
	}
	seals, err := battle.BuildWeaponSealTable(findActor(state.Actors, cmd.ActorID).Participant, s.cfg.RNG)
	if err != nil {
		return fx, err
	}
	level := requester.Level
	rewards := NewRunDefeatRewards(r, requester, blessing, false, s.cfg.RNG, &fx)
	ports := newRunPorts(frame, &level, &seals, state.Fields.Current(), rewards, false)
	ports.damage = state.NativeDamageContext(ports.damage, ports.defeat, s.cfg.RNG)
	contexts := ports.callbacks(r, &state, &requester, inspection)
	var defs skill.BattleDefinitions
	defs.RegisterPlayerCallbacks(contexts, len(frame.PartyIDs))
	callback := corebattle.CallbackContext{TargetContext: frame, Damage: &ports.damage, Status: ports.status, Defeat: ports.defeat, Inspection: inspection}
	parsed, err := r.ParseNormalOperation(cmd, NormalOperationContext{Definitions: defs, Callback: callback, Requester: &requester})
	if err != nil {
		return fx, err
	}
	// Room, comment, whisper and screenshot dispatch belong to other owners.
	if parsed.Kind != RunOperationBattle || parsed.First.EntryID == stockLevelAction {
		return fx, ErrRunUnavailable
	}
	now := s.cfg.Clock.Now()
	if now.Before(r.LastActions[cmd.ActorID].Add(time.Duration(r.Admission.Speed) * time.Second)) {
		return fx, ErrRunCooldown
	}
	bindRunAction(&parsed.First, callback, defs)
	reportFieldSuppression(&parsed.First.PlayerAction, contexts.Fields)
	op := corebattle.PlayerOperation{
		First: parsed.First.PlayerAction,
		Tail:  corebattle.PlayerTailContext{DamageContext: ports.damage, DefeatContext: ports.defeat},
		Field: corebattle.NamedFieldContext{
			DamageContext: ports.damage, Death: ports.defeat, Roster: contexts.Roster, Catalog: defs.Invocations,
			SubmittedCommand: parsed.First.EntryID, SubmittedTarget: parsed.First.TargetID,
			RequesterGender: contexts.Fields.RequesterGender, RequesterIcon: contexts.Fields.RequesterIcon,
		},
		// Treasure rooms disable the NPC response (vs_monster.cgi:38–41).
		ResponseEnabled: !isTreasureFloor(r.Floor),
	}
	if second := parsed.Second; second != nil {
		if second.EntryID != "" {
			bindRunAction(second, callback, defs)
			reportFieldSuppression(&second.PlayerAction, contexts.Fields)
		}
		op.Second = &second.PlayerAction
	}
	released := 0
	if r.Mode != CrawlModeTry {
		_, due, err := s.retainedDue(ctx, cmd.ActorID, r.Recipients[cmd.ActorID].Pending)
		if err != nil {
			return fx, err
		}
		released = due.EXP
	}
	op.LevelUp = func(b *corebattle.LifetimeState, rng random.Generator) (bool, error) {
		due, err := fx.LevelUpEligible(b, requester, r.Mode, released)
		if err != nil || !due || requester.Level != level {
			return false, err
		}
		pending := 0
		if r.Mode != CrawlModeTry {
			pending = released + fx.Earnings[requester.ID].EXP
		}
		return growRequester(b, cmd.ActorID, &requester, func(c *corecharacter.Character, held string) ([]corecharacter.Stats, error) {
			return progression.ApplyTailLevel(c, def, rng, held, pending)
		})
	}
	hadEnemies := r.livingEnemies()
	consumed := consumedCounts(state)
	result, err := state.ApplyPlayerOperation(&op, s.cfg.RNG)
	if err != nil {
		return fx, err
	}
	if !result.Accepted {
		return fx, ErrRunUnavailable
	}
	fx.Consumed = addConsumed(fx.Consumed, state, consumed)
	if err := r.ReconcileCombat(state, result.Target); err != nil {
		return fx, err
	}
	if result.NormalAction && (op.First.EntryID == "front" || op.First.EntryID == "back") {
		r.syncFormation()
	}
	for _, id := range result.Target.MemberOnlyIDs {
		if a := r.actor(id); a != nil && a.HP > 0 && a.TeamID != r.actor(cmd.ActorID).TeamID && !slices.Contains(r.Enemies, id) && !r.hasBox(id) {
			fx.MemberOnlyIDs = append(fx.MemberOnlyIDs, id)
		}
	}
	if result.ResponseReached && !result.Suppressed {
		if npc := r.NextNPC(now); npc != "" {
			if err := c.respond(r, npc, &level, &seals, rewards, &fx); err != nil {
				return fx, err
			}
			if err := r.RecordNPCResponse(npc, now); err != nil {
				return fx, err
			}
		}
	}
	if requester.Level != level {
		if err := s.cfg.Characters.Update(ctx, requester); err != nil {
			return fx, err
		}
	}
	fx.EncounterDefeated = hadEnemies && !r.livingEnemies() && len(fx.MemberOnlyIDs) == 0 &&
		r.EncounterFloor >= 1 && r.EncounterFloor <= EXBossFloor && !isTreasureFloor(r.EncounterFloor)
	return fx, nil
}

// respond runs the selected NPC's response on the saved state. Its opponents
// are the living members of other teams in member order.
func (c runCombat) respond(r *NormalRun, npc string, level *int, seals *corebattle.WeaponSealTable, rewards *corebattle.DefeatRewards, fx *RunActionEffects) error {
	team := r.actor(npc).TeamID
	frame := corebattle.TargetContext{ActorID: npc}
	before := make(map[string]bool, len(r.Battle.Actors))
	for _, a := range r.Battle.Actors {
		before[a.ID] = true
		switch {
		case a.TeamID == team:
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		case a.HP > 0:
			frame.EnemyIDs = append(frame.EnemyIDs, a.ID)
		default:
			frame.MemberOnlyIDs = append(frame.MemberOnlyIDs, a.ID)
		}
	}
	ports := newRunPorts(frame, level, seals, r.Battle.Fields.Current(), rewards, true)
	op := corebattle.NPCOperation{Damage: ports.damage, Defeat: ports.defeat, Field: corebattle.NamedFieldContext{DamageContext: ports.damage, Death: ports.defeat}}
	saved := cloneLifetimeState(r.Battle)
	register := func(d *skill.BattleDefinitions, f *corebattle.CallbackContext) {
		p := runPorts{damage: *f.Damage, defeat: f.Defeat, status: &corebattle.StatusContext{EffectContext: f.Damage.EffectContext, Defeat: f.Defeat}}
		d.RegisterNPCCallbacks(p.callbacks(r, &saved, nil, nil), f)
	}
	res, err := skill.ApplyNPCOperation(&r.Battle, op, skill.NPCDefaultSeed(), register, c.s.cfg.RNG)
	if err != nil {
		return err
	}
	for id, items := range res.Consumed {
		if fx.Consumed == nil {
			fx.Consumed = make(map[string][]corebattle.ConsumedItem)
		}
		fx.Consumed[id] = append(fx.Consumed[id], items...)
	}
	// Members the response generated join the opposing order last.
	for _, id := range res.Target.PartyIDs {
		if a := r.actor(id); a != nil && !before[id] && a.HP > 0 {
			r.Enemies = append(r.Enemies, id)
		}
	}
	return nil
}

func consumedCounts(s corebattle.LifetimeState) map[string]int {
	counts := make(map[string]int, len(s.Actors))
	for _, a := range s.Actors {
		counts[a.ID] = len(a.ConsumedItems)
	}
	return counts
}

func addConsumed(dst map[string][]corebattle.ConsumedItem, s corebattle.LifetimeState, before map[string]int) map[string][]corebattle.ConsumedItem {
	for _, a := range s.Actors {
		if n := before[a.ID]; len(a.ConsumedItems) > n {
			if dst == nil {
				dst = make(map[string][]corebattle.ConsumedItem)
			}
			dst[a.ID] = append(dst[a.ID], a.ConsumedItems[n:]...)
		}
	}
	return dst
}

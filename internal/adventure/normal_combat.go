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
	"github.com/witchcraze/party2re/internal/economy"
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
// player step, then at most one interval-ready NPC response, inside the
// requester's asset session when one is configured. Any rejection or
// execution error is returned so Command commits nothing.
// Transaction: delegated to Command. Lock Order: all party inventories(3)
// before WithJobAssets's requester depot(5), under Command's run and characters.
func (c runCombat) ExecuteRunAction(ctx context.Context, r *NormalRun, cmd RunCommand) (RunActionEffects, error) {
	if c.s.cfg.JobAssetRunner == nil {
		requester, err := c.s.cfg.Characters.FindByID(ctx, cmd.ActorID)
		if err != nil {
			return RunActionEffects{}, err
		}
		level := requester.Level
		fx, err := c.execute(ctx, r, cmd, &requester, nil)
		if err == nil && requester.Level != level {
			err = c.s.cfg.Characters.Update(ctx, requester)
		}
		return fx, err
	}
	if len(r.Roster) > 1 {
		if err := economy.LockInOrder(ctx, r.Roster, func(ctx context.Context, id string) error {
			_, err := c.s.cfg.Inventories.FindByCharacterIDForUpdate(ctx, id)
			return err
		}); err != nil {
			return RunActionEffects{}, err
		}
	}
	var fx RunActionEffects
	err := c.s.WithJobAssets(ctx, r, cmd, func(assets *corebattle.JobAssets, requester *corecharacter.Character) error {
		var err error
		fx, err = c.execute(ctx, r, cmd, requester, assets)
		return err
	})
	return fx, err
}

func (c runCombat) execute(ctx context.Context, r *NormalRun, cmd RunCommand, requester *corecharacter.Character, assets *corebattle.JobAssets) (RunActionEffects, error) {
	s := c.s
	fx := RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	if s.cfg.Jobs == nil {
		return fx, ErrRunUnavailable
	}
	blessing := ""
	if s.cfg.Blessings != nil {
		var err error
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
	rewards := NewRunDefeatRewards(r, *requester, blessing, false, s.cfg.RNG, &fx)
	ports := newRunPorts(frame, &level, &seals, state.Fields.Current(), rewards, false)
	ports.damage = state.NativeDamageContext(ports.damage, ports.defeat, s.cfg.RNG)
	contexts := ports.callbacks(r, &state, requester, inspection)
	contexts.Tools.RequesterMaxHP = &requester.Stats.MaxHP
	rooms, reset, err := c.bindRoomPorts(ctx, r, cmd.ActorID, &state, frame, &contexts, ports, assets)
	if err != nil {
		return fx, err
	}
	var base skill.BattleDefinitions
	base.RegisterPlayerCallbacks(contexts, len(frame.PartyIDs))
	defs := c.withHeldTravel(ctx, r, cmd.ActorID, &state, base, &reset, rooms)
	callback := corebattle.CallbackContext{TargetContext: frame, Damage: &ports.damage, Status: ports.status, Defeat: ports.defeat, Inspection: inspection, Assets: assets}
	parsed, err := r.ParseNormalOperation(cmd, NormalOperationContext{Definitions: defs, Callback: callback, Requester: requester})
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
	if assets != nil && parsed.First.EntryID == "job-50-entry-7" {
		assets.StoredItem, assets.StoredTarget = parsed.First.TargetID, parsed.First.Input
	}
	bindRunAction(&parsed.First, callback, defs)
	if parsed.First.Action == "tool" {
		inv, err := s.cfg.Inventories.FindByCharacterID(ctx, cmd.ActorID)
		if err != nil {
			return fx, err
		}
		held := inv.HeldToolID()
		if held == nil || toolEntry(*held) != parsed.First.EntryID {
			return fx, ErrRunConflict
		}
		consumeHeld(&parsed.First.PlayerAction, cmd.ActorID, *held, inv.Items[0].ID)
	}
	rooms.reportSelection(&parsed.First.PlayerAction, assets)
	reportFieldSuppression(&parsed.First.PlayerAction, contexts.Fields, rooms)
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
			reportFieldSuppression(&second.PlayerAction, contexts.Fields, rooms)
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
		due, err := fx.LevelUpEligible(b, *requester, r.Mode, released)
		if err != nil || !due || requester.Level != level {
			return false, err
		}
		pending := 0
		if r.Mode != CrawlModeTry {
			pending = released + fx.Earnings[requester.ID].EXP
		}
		return growRequester(b, cmd.ActorID, requester, func(c *corecharacter.Character, held string) ([]corecharacter.Stats, error) {
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
		if rooms.selection {
			// Only the stored-tool history survives a selection message.
			r.Battle.StoredToolHistory = rooms.history
			return RunActionEffects{}, nil
		}
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
	travel := rooms.transition()
	fx.UnsealDemonKing = travel.UnsealDemonKing
	if result.ResponseReached && !result.Suppressed {
		// A replaced encounter answers with its old responders, never new members.
		if npc := travel.BeginNPCResponse(r, now); npc != "" {
			if err := c.respond(r, npc, &level, &seals, rewards, assets, &fx); err != nil {
				return fx, err
			}
			if err := r.RecordNPCResponse(npc, now); err != nil {
				return fx, err
			}
		}
	}
	fx.EncounterDefeated = hadEnemies && !r.livingEnemies() && len(fx.MemberOnlyIDs) == 0 &&
		r.EncounterFloor >= 1 && r.EncounterFloor <= EXBossFloor && !isTreasureFloor(r.EncounterFloor)
	return fx, nil
}

// respond runs the selected NPC's response on the saved state. Its opponents
// are the living members of other teams in member order.
func (c runCombat) respond(r *NormalRun, npc string, level *int, seals *corebattle.WeaponSealTable, rewards *corebattle.DefeatRewards, assets *corebattle.JobAssets, fx *RunActionEffects) error {
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
	op := corebattle.NPCOperation{Damage: ports.damage, Defeat: ports.defeat, Field: corebattle.NamedFieldContext{DamageContext: ports.damage, Death: ports.defeat}, Assets: assets}
	saved := cloneLifetimeState(r.Battle)
	actor := findActor(saved.Actors, npc)
	if assets != nil && actor.HeldToolID == nil && actor.Gear != nil {
		// Encounter payloads store the NPC's hand as its accessory, including an empty slot.
		actor.HeldToolID = new(actor.Gear.AccessoryID)
	}
	register := func(d *skill.BattleDefinitions, f *corebattle.CallbackContext) {
		p := runPorts{damage: *f.Damage, defeat: f.Defeat, status: &corebattle.StatusContext{EffectContext: f.Damage.EffectContext, Defeat: f.Defeat}}
		callbacks := p.callbacks(r, &saved, nil, nil)
		if f.Assets != nil {
			callbacks.JobAssets = &corebattle.JobAssetContext{DamageContext: p.damage, DefeatContext: p.defeat, Roster: callbacks.Roster}
		}
		d.RegisterNPCCallbacks(callbacks, f)
	}
	res, err := skill.ApplyNPCOperation(&saved, op, skill.NPCDefaultSeed(), register, c.s.cfg.RNG)
	if err != nil {
		return err
	}
	r.Battle = saved
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

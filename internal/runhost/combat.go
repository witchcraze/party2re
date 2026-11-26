package runhost

import (
	"context"
	"maps"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/economy"
)

// ResetContext is actor's all-member reset frame on the saved run: actor's
// side, the living members of the current encounter and everyone else as
// member-only.
func (r *State) ResetContext(actorID string, requesterLevel *int) (corebattle.EquipmentResetContext, error) {
	actor := r.Actor(actorID)
	if actor == nil {
		return corebattle.EquipmentResetContext{}, ErrNotParticipant
	}
	frame := corebattle.TargetContext{ActorID: actorID}
	for _, a := range r.Battle.Actors {
		if a.TeamID == actor.TeamID {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		} else if slices.Contains(r.frameEnemies(actor.TeamID), a.ID) && a.HP > 0 {
			frame.EnemyIDs = append(frame.EnemyIDs, a.ID)
		} else {
			frame.MemberOnlyIDs = append(frame.MemberOnlyIDs, a.ID)
		}
	}
	return corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: requesterLevel}, nil
}

func (s *Service[R, P]) responds(run *R) bool {
	return s.cfg.CombatRoom == nil || s.cfg.CombatRoom.Responds(run)
}

// executeCombat runs one parsed submission through the source-ordered player
// step, then at most one interval-ready NPC response, inside the requester's
// asset session when one is configured. Any rejection or execution error is
// returned so Command commits nothing.
// Transaction: delegated to Command. Lock Order: all party inventories(3)
// before withJobAssets's requester depot(5), under Command's run and characters.
func (s *Service[R, P]) executeCombat(ctx context.Context, run *R, cmd Command) (Effects, error) {
	r := P(run).Host()
	if s.cfg.JobAssetRunner == nil {
		requester, err := s.cfg.Characters.FindByID(ctx, cmd.ActorID)
		if err != nil {
			return Effects{}, err
		}
		level := requester.Level
		fx, err := s.execute(ctx, run, cmd, &requester, nil)
		if err == nil && requester.Level != level {
			err = s.cfg.Characters.Update(ctx, requester)
		}
		return fx, err
	}
	if len(r.Roster) > 1 {
		if err := economy.LockInOrder(ctx, r.Roster, func(ctx context.Context, id string) error {
			_, err := s.cfg.Inventories.FindByCharacterIDForUpdate(ctx, id)
			return err
		}); err != nil {
			return Effects{}, err
		}
	}
	var fx Effects
	err := s.withJobAssets(ctx, r, cmd, func(assets *corebattle.JobAssets, requester *corecharacter.Character) error {
		var err error
		fx, err = s.execute(ctx, run, cmd, requester, assets)
		return err
	})
	return fx, err
}

func (s *Service[R, P]) execute(ctx context.Context, run *R, cmd Command, requester *corecharacter.Character, assets *corebattle.JobAssets) (Effects, error) {
	r := P(run).Host()
	fx := Effects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	su, err := s.setupCombat(ctx, run, cmd.ActorID, requester, assets, &fx)
	if err != nil {
		return fx, err
	}
	def, state, room, contexts, rooms, defs, callback := su.def, &su.state, su.room, &su.contexts, su.rooms, su.defs, su.callback
	conditional := s.cfg.Room.Conditional(run)
	parsed, err := r.parseOperation(cmd, su.operation(requester), su.choices)
	if err != nil {
		return fx, err
	}
	// Room, comment, whisper and screenshot dispatch belong to other owners.
	if parsed.Kind != OperationBattle || parsed.First.EntryID == StockLevelAction {
		return fx, ErrUnavailable
	}
	now := s.cfg.Clock.Now()
	if err := r.Ready(cmd.ActorID, now); err != nil {
		return fx, err
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
		held, ok := inv.HeldTool()
		if !ok || held.ID == "" || toolEntry(held.DefinitionID) != parsed.First.EntryID {
			return fx, ErrConflict
		}
		consumeHeld(&parsed.First.PlayerAction, cmd.ActorID, held.DefinitionID, held.ID)
	}
	rooms.reportSelection(&parsed.First.PlayerAction, assets)
	reportFieldSuppression(&parsed.First.PlayerAction, contexts.Fields, rooms)
	op := corebattle.PlayerOperation{
		First: parsed.First.PlayerAction,
		Tail:  corebattle.PlayerTailContext{DamageContext: su.ports.damage, DefeatContext: su.ports.defeat},
		Field: corebattle.NamedFieldContext{
			DamageContext: su.ports.damage, Death: su.ports.defeat, Roster: contexts.Roster, Catalog: defs.Invocations,
			RequesterGender: contexts.Fields.RequesterGender, RequesterIcon: contexts.Fields.RequesterIcon,
		},
		ResponseEnabled: s.responds(run),
	}
	if second := parsed.Second; second != nil {
		if second.EntryID != "" {
			bindRunAction(second, callback, defs)
			reportFieldSuppression(&second.PlayerAction, contexts.Fields, rooms)
		}
		op.Second = &second.PlayerAction
	}
	released := 0
	if !conditional {
		_, due, err := s.retainedDue(ctx, cmd.ActorID, r.Recipients[cmd.ActorID].Pending)
		if err != nil {
			return fx, err
		}
		released = due.EXP
	}
	op.LevelUp = func(b *corebattle.LifetimeState, rng random.Generator) (bool, error) {
		due, err := levelUpDue(b, *requester, conditional, released, fx.Earnings[requester.ID].EXP)
		if err != nil || !due || requester.Level != su.level {
			return false, err
		}
		pending := 0
		if !conditional {
			pending = released + fx.Earnings[requester.ID].EXP
		}
		return growRequester(b, cmd.ActorID, requester, func(c *corecharacter.Character, held string) ([]corecharacter.Stats, error) {
			return progression.ApplyTailLevel(c, def, rng, held, pending)
		})
	}
	hadEnemies := r.LivingEnemies()
	consumed := consumedCounts(*state)
	result, err := state.ApplyPlayerOperation(&op, s.cfg.RNG)
	if err != nil {
		return fx, err
	}
	if !result.Accepted {
		if rooms.selection {
			// Only the stored-tool history survives a selection message.
			r.Battle.StoredToolHistory = rooms.history
			return Effects{}, nil
		}
		return fx, ErrUnavailable
	}
	fx.Consumed = addConsumed(fx.Consumed, *state, consumed)
	fx.Inspections = appendInspections(nil, cmd.ActorID, contexts.Tools.Results)
	if err := r.ReconcileCombat(*state, result.Target); err != nil {
		return fx, err
	}
	if result.NormalAction && (op.First.EntryID == "front" || op.First.EntryID == "back") {
		r.syncFormation()
	}
	// Enemies a callback moved out of the frame sit out this operation's
	// response and return last for the next request.
	var memberOnly []string
	for _, id := range result.Target.MemberOnlyIDs {
		if a := r.Actor(id); a != nil && a.HP > 0 && a.TeamID != r.Actor(cmd.ActorID).TeamID && !slices.Contains(r.Enemies, id) && !r.HasBox(id) {
			memberOnly = append(memberOnly, id)
		}
	}
	if result.ResponseReached && !result.Suppressed {
		respond := r.NextNPC
		if room.Respond != nil {
			// A replaced encounter answers with its old responders, never new members.
			respond = room.Respond
		}
		if npc := respond(now); npc != "" {
			if err := s.respond(run, npc, cmd.ActorID, requester, &su.level, &su.seals, su.rewards, assets, result.ArithmeticDivisor, result.GuaranteedHit, room, &fx); err != nil {
				return fx, err
			}
			if err := r.RecordNPCResponse(npc, now); err != nil {
				return fx, err
			}
		}
	}
	defeated := hadEnemies && len(memberOnly) == 0 && !r.LivingEnemies()
	for _, id := range memberOnly {
		if a := r.Actor(id); a != nil && a.HP > 0 && !slices.Contains(r.Enemies, id) {
			r.Enemies = append(r.Enemies, id)
		}
	}
	if room.Settle != nil {
		if fx.UnsealDemonKing, err = room.Settle(defeated); err != nil {
			return fx, err
		}
	}
	return fx, nil
}

// respond runs the selected NPC's response on the saved state. Its opponents
// are the living members of other teams in member order.
func (s *Service[R, P]) respond(run *R, npc, requesterID string, requester *corecharacter.Character, level *int, seals *corebattle.WeaponSealTable, rewards *corebattle.DefeatRewards, assets *corebattle.JobAssets, divisor int, guaranteedHit bool, room RoomCombat, fx *Effects) error {
	r := P(run).Host()
	team := r.Actor(npc).TeamID
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
	ports.damage.GuaranteedHit = guaranteedHit
	op := corebattle.NPCOperation{Damage: ports.damage, Defeat: ports.defeat, Field: corebattle.NamedFieldContext{DamageContext: ports.damage, Death: ports.defeat}, Assets: assets, ArithmeticDivisor: divisor}
	saved := cloneBattle(r.Battle)
	actor := findActor(saved.Actors, npc)
	if assets != nil && actor.HeldToolID == nil && actor.Gear != nil {
		// Encounter payloads store the NPC's hand as its accessory, including an empty slot.
		actor.HeldToolID = new(actor.Gear.AccessoryID)
	}
	var tools *corebattle.OrdinaryToolContext
	register := func(d *skill.BattleDefinitions, work *corebattle.LifetimeState, f *corebattle.CallbackContext) {
		p := runPorts{damage: *f.Damage, defeat: f.Defeat, status: &corebattle.StatusContext{EffectContext: f.Damage.EffectContext, Defeat: f.Defeat}, requesterID: requesterID}
		// Boxes are not projected into a response, so every member is empty.
		inspection := make(map[string]corebattle.InspectionContent, len(work.Actors))
		for _, a := range work.Actors {
			inspection[a.ID] = corebattle.InspectionContent{Empty: true}
		}
		callbacks := p.callbacks(room, work, requester, inspection)
		tools = callbacks.Tools
		callbacks.JobEncounters = s.npcEncounters(r, work, f, room.NormalStage)
		if f.Assets != nil {
			callbacks.JobAssets = &corebattle.JobAssetContext{DamageContext: p.damage, DefeatContext: p.defeat, Roster: callbacks.Roster}
		}
		d.RegisterNPCCallbacks(callbacks, f)
		maps.Copy(d.Invocations.Entries, room.NPCEntries)
	}
	seed := skill.NPCDefaultSeed()
	if room.NPCSkills != nil {
		seed = room.NPCSkills
	}
	res, err := skill.ApplyNPCOperation(&saved, op, seed, register, s.cfg.RNG)
	if err != nil {
		return err
	}
	r.Battle = saved
	if tools != nil {
		fx.Inspections = appendInspections(fx.Inspections, npc, tools.Results)
	}
	for id, items := range res.Consumed {
		if fx.Consumed == nil {
			fx.Consumed = make(map[string][]corebattle.ConsumedItem)
		}
		fx.Consumed[id] = append(fx.Consumed[id], items...)
	}
	// Members the response removed leave the order before the head rotates.
	r.Enemies = slices.DeleteFunc(r.Enemies, func(id string) bool {
		a := r.Actor(id)
		return slices.Contains(frame.PartyIDs, id) && !slices.Contains(res.Target.PartyIDs, id) && (a == nil || a.HP <= 0)
	})
	// Members the response generated join the opposing order last.
	for _, id := range res.Target.PartyIDs {
		if a := r.Actor(id); a != nil && !before[id] && a.HP > 0 {
			r.Enemies = append(r.Enemies, id)
		}
	}
	return nil
}

// npcEncounters binds job78's room bodies for a response. npc_turn restores
// the members after the body (_npc_action.cgi:141–151), discarding set_monster's
// list, so an NPC's bypass jump keeps the encounter: only the all-member reset
// (its revival pass on the NPC's side) and the floor counter remain
// (_skill.cgi:6116–6133). The NPC's enemies are never boxes, so treasure retry
// finds none. f is the working frame the response executes on.
func (s *Service[R, P]) npcEncounters(r *State, work *corebattle.LifetimeState, f *corebattle.CallbackContext, normalStage bool) *corebattle.JobEncounterContext {
	if f.Defeat == nil {
		return nil
	}
	room := &corebattle.JobRoomContext{ActorID: f.ActorID, NormalStage: normalStage, Floor: r.Round}
	reset := func() corebattle.EquipmentResetContext {
		reset := f.Defeat.EquipmentResetContext
		reset.TargetContext = f.TargetContext
		return reset
	}
	room.Validate = func(id string) error {
		if id != "job-78-entry-2" {
			return corebattle.ErrUnsupportedEffect
		}
		current := reset()
		return battle.ValidateResetAll(work, &current, random.Default())
	}
	room.Apply = func(id string) (corebattle.TargetContext, error) {
		if id != "job-78-entry-2" {
			return f.TargetContext, corebattle.ErrUnsupportedEffect
		}
		current := reset()
		if err := battle.ResetAll(work, &current, s.cfg.RNG); err != nil {
			return f.TargetContext, err
		}
		r.Round++
		room.Floor, room.Cursor = r.Round, current.Cursor
		return f.TargetContext, nil
	}
	return &corebattle.JobEncounterContext{DamageContext: *f.Damage, DefeatContext: f.Defeat, Room: room}
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

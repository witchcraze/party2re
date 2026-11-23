package runhost

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// Choice is one add_battle_action entry: Action is its command, Source its
// legacy name, and Check rejects a target the room forbids.
type Choice struct {
	Action, Source string
	Check          func(target string) error
}

// CombatRoom supplies a room library's branches of the shared battle routines
// (legacy `$m{lib}`). Without one the host keeps legacy's non-vs_monster
// branch: NPCs respond, job78 finds no room, nothing travels or summons from a
// pool, defeat rewards are unmodified and the room records nothing.
type CombatRoom[R any] interface {
	// Choices are add_battle_action's entries for actor, after the shared ones.
	Choices(r *R, actor string) []Choice
	// Responds reports whether NPCs respond in the current room.
	Responds(r *R) bool
	// Combat binds one player operation's seams to the locked run. reset is the
	// operation's all-member reset frame; stored reports a requester asset session.
	Combat(ctx context.Context, r *R, actor string, reset *corebattle.EquipmentResetContext, stored bool) (RoomCombat, error)
}

// RoomCombat is one operation's room seams. Callbacks mutate only the locked
// run; the host adopts their result into the working combat state.
type RoomCombat struct {
	// NormalStage enables job78's room bodies (_skill.cgi:6120,6205): the
	// requester's through JobRoom, an NPC's bypass jump in the response.
	NormalStage bool
	JobRoom     JobRoom
	// Tools bind held travel tools by definition ID; StoredTravel is job50's
	// stored invocation of them (_data.cgi:1060–1130,1249,1623–1632).
	Tools        map[string]corebattle.CallbackDefinition
	StoredTravel *corebattle.StoredToolTravel
	// Summons reads the roster callbacks' room inputs when ports are built.
	Summons func() Summons
	// Habitat names defeat facts, Scale is the room's reward modifier before
	// shared bonuses, and Credited observes each credited defeat in order
	// (_battle.cgi:277). Each reads the run when a defeat is credited.
	Habitat  func() string
	Scale    func(Earnings) Earnings
	Credited func(identity string, rng random.Generator)
	// Respond selects the responding NPC after the player step (default NextNPC).
	Respond func(now time.Time) string
	// Settle records the room's facts once the operation succeeded; defeated
	// reports that this operation felled the whole encounter.
	Settle func(defeated bool) (unseal bool, err error)
}

// Summons are the encounter pool, appearance positions and Hard flag that
// summoning callbacks draw from.
type Summons struct {
	EnemyPool           []corebattle.LifetimeActor
	AppearancePositions []int
	Hard                bool
	// EXPool/EXXPool, ReloadDraws, RebuildPool and StageLoaded feed
	// request-scoped stage-file reloads in enemy summon and job407.
	EXPool, EXXPool []corebattle.LifetimeActor
	ReloadDraws     int
	RebuildPool     func(summoner corebattle.LifetimeActor, rng random.Generator) []corebattle.LifetimeActor
	StageLoaded     *bool
}

// JobRoom is job78's room mutation; Apply runs on the locked run.
type JobRoom struct {
	Validate func(id string) error
	Apply    func(id string) error
}

// roomPorts binds the requester's assets and the room's travel/encounter
// mutations to one operation.
type roomPorts struct {
	assets     *corebattle.JobAssetContext
	encounters *corebattle.JobEncounterContext
	// selection is the first callback's source selection message; history is
	// the stored-tool history it keeps (_battle.cgi:761 `return if $mes`).
	selection bool
	history   map[string]map[string]bool
}

// adoptRoom runs a room mutation inside the player step: r first receives the
// working combat, then the working state adopts r's result.
func (r *State) adoptRoom(work *corebattle.LifetimeState, frame corebattle.TargetContext, mutate func() error) (corebattle.TargetContext, error) {
	if err := r.ReconcileCombat(*work, frame); err != nil {
		return frame, err
	}
	if err := mutate(); err != nil {
		return frame, err
	}
	next, projected, _, err := r.ProjectCombat(frame.ActorID)
	if err != nil {
		return frame, err
	}
	// Field-wise copy keeps core's unexported operation bookkeeping.
	work.Actors, work.Fields, work.SkillBans, work.SkillHistory = next.Actors, next.Fields, next.SkillBans, next.SkillHistory
	work.DroppedCrystals, work.SpecialBossField, work.RosterSequence, work.StoredToolHistory = next.DroppedCrystals, next.SpecialBossField, next.RosterSequence, next.StoredToolHistory
	return projected, nil
}

// bindRoomPorts supplies the asset, stored-travel and encounter contexts. work
// is the state the player step executes on.
func bindRoomPorts(r *State, room RoomCombat, work *corebattle.LifetimeState, frame corebattle.TargetContext, reset *corebattle.EquipmentResetContext, contexts *skill.PlayerCallbackContexts, p runPorts, assets *corebattle.JobAssets) *roomPorts {
	rooms := &roomPorts{}
	if assets != nil {
		var travel *corebattle.StoredToolTravel
		if room.StoredTravel != nil {
			stored := *room.StoredTravel
			apply := stored.Apply
			stored.Apply = func(tool, target string, at corebattle.TargetContext) (corebattle.TargetContext, error) {
				return r.adoptRoom(work, at, func() error {
					_, err := apply(tool, target, at)
					return err
				})
			}
			travel = &stored
		}
		rooms.assets = &corebattle.JobAssetContext{DamageContext: p.damage, DefeatContext: p.defeat, Roster: contexts.Roster, Travel: travel}
		contexts.JobAssets = rooms.assets
	}
	jobRoom := &corebattle.JobRoomContext{ActorID: frame.ActorID, NormalStage: room.NormalStage, Floor: r.Round, Cursor: reset.Cursor}
	for _, box := range r.Boxes {
		if box.IsUnopened() {
			jobRoom.TreasureIDs = append(jobRoom.TreasureIDs, box.ID)
		}
	}
	if room.JobRoom.Validate != nil && room.JobRoom.Apply != nil {
		jobRoom.Validate = room.JobRoom.Validate
		jobRoom.Apply = func(id string) (corebattle.TargetContext, error) {
			projected, err := r.adoptRoom(work, frame, func() error { return room.JobRoom.Apply(id) })
			if err == nil {
				jobRoom.Floor, jobRoom.Cursor = r.Round, reset.Cursor
			}
			return projected, err
		}
	}
	rooms.encounters = &corebattle.JobEncounterContext{DamageContext: p.damage, DefeatContext: p.defeat, Room: jobRoom}
	contexts.JobEncounters = rooms.encounters
	return rooms
}

// withHeldTravel returns a catalog that also binds the room's held travel
// tools. Stored job50 travel keeps the base catalog so it reaches its own port.
func withHeldTravel(r *State, defs skill.BattleDefinitions, tools map[string]corebattle.CallbackDefinition) skill.BattleDefinitions {
	if len(tools) == 0 {
		return defs
	}
	held := defs
	held.Invocations.Entries = maps.Clone(defs.Invocations.Entries)
	for id, tool := range tools {
		apply := tool.Execute
		tool.Execute = func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			projected, err := r.adoptRoom(s, f.TargetContext, func() error { return apply(s, f, rng) })
			f.TargetContext = projected
			return err
		}
		held.Invocations.Entries[toolEntry(id)] = tool
	}
	return held
}

func toolEntry(definitionID string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(definitionID, "item-"))
	if err != nil {
		return ""
	}
	return "tool-" + strconv.Itoa(n)
}

// suppressed reports the ports' `$npc_turn_skip` outputs.
func (rooms *roomPorts) suppressed() bool {
	return rooms.encounters.ResponseSuppressed || rooms.assets != nil && rooms.assets.ResponseSuppressed
}

// reportSelection makes the first callback's selection error inert, keeping
// only the stored-tool history it wrote.
func (rooms *roomPorts) reportSelection(action *corebattle.PlayerAction, assets *corebattle.JobAssets) {
	execute := action.Execute
	if execute == nil || assets == nil {
		return
	}
	action.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		facts, err := execute(s, rng)
		if err == nil && assets.SelectionError {
			rooms.selection, facts.Inert = true, true
			rooms.history = make(map[string]map[string]bool, len(s.StoredToolHistory))
			for color, used := range s.StoredToolHistory {
				rooms.history[color] = maps.Clone(used)
			}
		}
		return facts, err
	}
}

// consumeHeld is dougu's tail: a normally returning held tool clears the hand
// and consumes its instance (_skill.cgi:758–764).
func consumeHeld(action *corebattle.PlayerAction, actorID, definitionID, instanceID string) {
	execute := action.Execute
	action.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		facts, err := execute(s, rng)
		if err != nil || facts.Inert {
			return facts, err
		}
		if a := findActor(s.Actors, actorID); a != nil {
			a.ClearConsumedHeldTool(definitionID)
			a.ConsumedItems = append(a.ConsumedItems, corebattle.ConsumedItem{ID: definitionID, InstanceID: instanceID, Quantity: 1})
		}
		return facts, nil
	}
}

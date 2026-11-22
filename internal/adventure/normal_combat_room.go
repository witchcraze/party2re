package adventure

import (
	"context"
	"maps"
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// roomPorts binds the requester's assets and Adventure's travel/encounter
// mutations to one operation. Each transition stays operation-local.
type roomPorts struct {
	assets     *corebattle.JobAssetContext
	encounters *corebattle.JobEncounterContext
	held       *CombatTravelTransition
	stored     *CombatTravelTransition
	encounter  *CombatTravelTransition
	// selection is the first callback's source selection message; history is
	// the stored-tool history it keeps (_battle.cgi:761 `return if $mes`).
	selection bool
	history   map[string]map[string]bool
}

// adoptRoom runs an Adventure room mutation inside the player step: r first
// receives the working combat, then the working state adopts r's result.
func (r *NormalRun) adoptRoom(work *corebattle.LifetimeState, frame corebattle.TargetContext, mutate func() error) (corebattle.TargetContext, error) {
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
func (c runCombat) bindRoomPorts(ctx context.Context, r *NormalRun, actor string, work *corebattle.LifetimeState, frame corebattle.TargetContext, contexts *skill.PlayerCallbackContexts, p runPorts, assets *corebattle.JobAssets) (*roomPorts, corebattle.EquipmentResetContext, error) {
	reset, err := c.resetContext(ctx, r, actor)
	if err != nil {
		return nil, reset, err
	}
	rooms := &roomPorts{held: &CombatTravelTransition{}}
	if assets != nil {
		travel, transition := c.s.PrepareStoredToolTravel(ctx, r, actor, &reset)
		apply := travel.Apply
		travel.Apply = func(tool, target string, at corebattle.TargetContext) (corebattle.TargetContext, error) {
			return r.adoptRoom(work, at, func() error {
				_, err := apply(tool, target, at)
				return err
			})
		}
		rooms.stored = transition
		rooms.assets = &corebattle.JobAssetContext{DamageContext: p.damage, DefeatContext: p.defeat, Roster: contexts.Roster, Travel: travel}
		contexts.JobAssets = rooms.assets
	}
	encounters, transition, err := c.s.PrepareJobEncounters(ctx, r, actor, &reset)
	if err != nil {
		return nil, reset, err
	}
	apply := encounters.Room.Apply
	encounters.Room.Apply = func(id string) (corebattle.TargetContext, error) {
		return r.adoptRoom(work, frame, func() error {
			_, err := apply(id)
			return err
		})
	}
	encounters.DamageContext, encounters.DefeatContext = p.damage, p.defeat
	rooms.encounters, rooms.encounter = encounters, transition
	contexts.JobEncounters = encounters
	return rooms, reset, nil
}

// withHeldTravel returns a catalog that also binds the held travel tools.
// Stored job50 travel keeps the base catalog so it reaches its own port.
func (c runCombat) withHeldTravel(ctx context.Context, r *NormalRun, actor string, work *corebattle.LifetimeState, defs skill.BattleDefinitions, reset *corebattle.EquipmentResetContext, rooms *roomPorts) skill.BattleDefinitions {
	held := defs
	held.Invocations.Entries = maps.Clone(defs.Invocations.Entries)
	for _, tool := range travelTools {
		input := func(f *corebattle.CallbackContext) (CombatTravelInput, error) {
			target := ""
			if len(f.Arguments) > 0 {
				target = f.Arguments[0]
			}
			return travelInput(tool, target)
		}
		held.Invocations.Entries[toolEntry(tool)] = corebattle.CallbackDefinition{
			Validate: func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				value, err := input(f)
				if err != nil {
					return err
				}
				return c.s.ValidateCombatTravel(r, actor, tool, value, reset)
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, _ random.Generator) error {
				value, err := input(f)
				if err != nil {
					return err
				}
				projected, err := r.adoptRoom(s, f.TargetContext, func() error {
					t, err := c.s.ApplyCombatTravel(ctx, r, actor, tool, value, reset)
					*rooms.held = t
					return err
				})
				f.TargetContext = projected
				return err
			},
		}
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

// transition is the operation's room change: a replacement carries the old
// responders, and any genuine stage19 treasure creation unseals once.
func (rooms *roomPorts) transition() CombatTravelTransition {
	var t CombatTravelTransition
	unseal := false
	for _, each := range []*CombatTravelTransition{rooms.held, rooms.stored, rooms.encounter} {
		if each == nil {
			continue
		}
		if each.replaced {
			t = *each
		}
		unseal = unseal || each.UnsealDemonKing
	}
	t.ResponseSuppressed, t.UnsealDemonKing = false, unseal
	return t
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
			empty := ""
			a.HeldToolID = &empty
			a.ConsumedItems = append(a.ConsumedItems, corebattle.ConsumedItem{ID: definitionID, InstanceID: instanceID, Quantity: 1})
		}
		return facts, nil
	}
}

package runhost

import (
	"context"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// combatSetup is one operation's ports on the actor's projected combat
// state. Execution and discovery share it, so a listed choice is admitted by
// the same definitions that execute it. Ports hold pointers into the setup.
type combatSetup struct {
	def        job.Definition
	state      corebattle.LifetimeState
	seals      corebattle.WeaponSealTable
	reset      corebattle.EquipmentResetContext
	resetLevel int
	level      int
	room       RoomCombat
	choices    []Choice
	rewards    *corebattle.DefeatRewards
	ports      runPorts
	contexts   skill.PlayerCallbackContexts
	rooms      *roomPorts
	defs       skill.BattleDefinitions
	callback   corebattle.CallbackContext
}

func (su *combatSetup) operation(requester *corecharacter.Character) OperationContext {
	return OperationContext{Definitions: su.defs, Callback: su.callback, Requester: requester, Arithmetic: su.contexts.Arithmetic, Fields: su.contexts.Fields}
}

// setupCombat builds the ports for actor on run. Room callbacks mutate only
// run; fx receives defeat earnings when the operation executes.
func (s *Service[R, P]) setupCombat(ctx context.Context, run *R, actor string, requester *corecharacter.Character, assets *corebattle.JobAssets, fx *Effects) (*combatSetup, error) {
	r := P(run).Host()
	if s.cfg.Jobs == nil {
		return nil, ErrUnavailable
	}
	blessing := ""
	if s.cfg.Blessings != nil {
		var err error
		if blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actor); err != nil {
			return nil, err
		}
	}
	su := &combatSetup{resetLevel: requester.Level, level: requester.Level}
	var err error
	if su.def, err = s.cfg.Jobs.FindByID(requester.JobID); err != nil {
		return nil, err
	}
	var frame corebattle.TargetContext
	var inspection map[string]corebattle.InspectionContent
	if su.state, frame, inspection, err = r.ProjectCombat(actor); err != nil {
		return nil, err
	}
	if su.seals, err = battle.BuildWeaponSealTable(findActor(su.state.Actors, actor).Participant, s.cfg.RNG); err != nil {
		return nil, err
	}
	if su.reset, err = r.ResetContext(actor, &su.resetLevel); err != nil {
		return nil, err
	}
	if s.cfg.CombatRoom != nil {
		if su.room, err = s.cfg.CombatRoom.Combat(ctx, run, actor, &su.reset, assets != nil); err != nil {
			return nil, err
		}
		su.choices = s.cfg.CombatRoom.Choices(run, actor)
	}
	su.rewards = defeatRewards(r, su.room, *requester, blessing, s.cfg.RNG, fx)
	su.ports = newRunPorts(frame, &su.level, &su.seals, su.state.Fields.Current(), su.rewards, false)
	su.ports.damage = su.state.NativeDamageContext(su.ports.damage, su.ports.defeat, s.cfg.RNG)
	su.contexts = su.ports.callbacks(su.room, &su.state, requester, inspection)
	su.contexts.Tools.RequesterMaxHP = &requester.Stats.MaxHP
	su.rooms = bindRoomPorts(r, su.room, &su.state, frame, &su.reset, &su.contexts, su.ports, assets)
	var base skill.BattleDefinitions
	base.RegisterPlayerCallbacks(su.contexts, len(frame.PartyIDs))
	base.RegisterGemCallbacks(su.contexts, &su.ports.damage)
	su.defs = withHeldTravel(r, base, su.room.Tools)
	su.callback = corebattle.CallbackContext{TargetContext: frame, Damage: &su.ports.damage, Status: su.ports.status, Defeat: su.ports.defeat, Inspection: inspection, Assets: assets}
	return su, nil
}

// Observation is an actor's read-only view of its run: public encounter
// facts, the actor's own ledger and the operations it can submit now.
type Observation struct {
	ActorRun
	Speed int
	// ReadyAt is when the actor's action interval elapses; IdleAt is when the
	// room becomes due for idle cleanup. Both use the server clock.
	ReadyAt, IdleAt, Now time.Time
	Actors               []ObservedActor
	Boxes                []ObservedBox
	Choices              []OperationChoice
}

// ObservedActor carries only facts every room member sees, in member order.
type ObservedActor struct {
	ID, Name, TeamID     string
	HP, MaxHP, MP, MaxMP int
	NPC, Enemy, Leader   bool
}

// ObservedBox hides an unopened box's content.
type ObservedBox struct {
	ID, Name, OpenedBy, ItemName string
	Defeated                     bool
}

// ObserveRun reads actor's run without a lock and without changing it.
// Discovery runs on a decoded copy; nil means no owned run.
func (s *Service[R, P]) ObserveRun(ctx context.Context, player, actor string) (*Observation, error) {
	owned, err := s.GetActorRun(ctx, player, actor)
	if err != nil || owned == nil {
		return nil, err
	}
	obs := &Observation{ActorRun: *owned, Now: s.cfg.Clock.Now()}
	run, err := s.cfg.Store.FindRun(ctx, owned.RunID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return obs, nil
	}
	r := P(run).Host()
	// One snapshot supplies every encounter fact, so its version matches them.
	obs.Version, obs.Round, obs.LeaderID, obs.Speed = r.Version, r.Round, r.LeaderID, r.Admission.Speed
	obs.ReadyAt = r.LastActions[actor].Add(time.Duration(r.Admission.Speed) * time.Second)
	obs.IdleAt = r.LastActivity.Add(idleTimeout)
	for _, a := range r.Battle.Actors {
		obs.Actors = append(obs.Actors, ObservedActor{
			ID: a.ID, Name: a.Name, TeamID: a.TeamID, HP: a.HP, MaxHP: a.MaxHP, MP: a.MP, MaxMP: a.MaxMP,
			NPC: a.NPC, Enemy: slices.Contains(r.Enemies, a.ID), Leader: a.ID == r.LeaderID,
		})
	}
	for _, b := range r.Boxes {
		box := ObservedBox{ID: b.ID, Name: b.Name, OpenedBy: b.OpenedBy, Defeated: b.Defeated}
		if b.OpenedBy != "" {
			box.ItemName = b.ItemName
		}
		obs.Boxes = append(obs.Boxes, box)
	}
	if r.TimedOut || !slices.Contains(r.Roster, actor) {
		return obs, nil
	}
	requester, err := s.cfg.Characters.FindByID(ctx, actor)
	if err != nil {
		return nil, err
	}
	su, err := s.setupCombat(ctx, run, actor, &requester, nil, &Effects{})
	if err != nil {
		return nil, err
	}
	if obs.Choices, err = r.DiscoverOperations(Command{ActorID: actor}, su.operation(&requester), su.choices); err != nil {
		return nil, err
	}
	// Whisper, screenshot, invite and kick stay listed; other owners execute them.
	for i, c := range obs.Choices {
		obs.Choices[i].Submittable = !c.Gated && (c.Kind == OperationBattle || c.Action == "chest" || c.Action == "exit" || slices.Contains(s.cfg.Room.Actions(), c.Action))
	}
	return obs, nil
}

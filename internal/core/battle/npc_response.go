package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// NPCOperation receives the selected actor and ordered sides from the consumer.
// Damage.ActorID is the NPC. EnemyIDs contains the living opponents at entry;
// fallen opponents are in MemberOnlyIDs. Assets belong to the requester.
type NPCOperation struct {
	Damage DamageContext
	Defeat *DefeatContext
	Field  NamedFieldContext
	Assets *JobAssets
}

// NPCActionPlan freezes the weighted pool and its definitions before cures.
type NPCActionPlan struct {
	Choices  []NPCChoice
	NextSeed []CallbackBinding
	Catalog  CallbackCatalog
}

// NPCDefeat records the source of the fatal hit. The defeat context owns rewards.
type NPCDefeat struct {
	TargetID, KillerID string
}

type NPCOperationResult struct {
	NormalAction bool
	Choice       CallbackBinding
	NextSeed     []CallbackBinding
	Target       TargetContext
	Defeats      []NPCDefeat
	Consumed     map[string][]ConsumedItem
}

type npcResponseFacts struct {
	defeats []NPCDefeat
	end     EndProcessing
	effects EffectContext
}

func (s *LifetimeState) npcFacts() *npcResponseFacts {
	if s == nil {
		return nil
	}
	return s.npcResponse
}

// Native bodies can bind distinct damage/healing contexts. The NPC response
// owns counters and mirror activation across those bodies and equipment tails.
func mergeNPCEffects(dst, src *EffectContext) {
	dst.DamageCalls = max(dst.DamageCalls, src.DamageCalls)
	dst.AreaDamageCalls = max(dst.AreaDamageCalls, src.AreaDamageCalls)
	dst.HealCalls = max(dst.HealCalls, src.HealCalls)
	dst.PartyHealCalls = max(dst.PartyHealCalls, src.PartyHealCalls)
	dst.Item185Activated = dst.Item185Activated || src.Item185Activated
	dst.Item195Activated = dst.Item195Activated || src.Item195Activated
}

// ApplyNPCOperation executes one response on isolated state and publishes the
// result only after all tails succeed. prepare binds fresh, operation-local
// callbacks to the working frame. It must not execute effects.
func (s *LifetimeState) ApplyNPCOperation(op NPCOperation, seed []CallbackBinding, prepare func(*LifetimeState, *CallbackContext) (NPCActionPlan, error), rng random.Generator) (NPCOperationResult, error) {
	if s == nil || prepare == nil || !op.Damage.ActorNPC {
		return NPCOperationResult{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(op.Damage.TargetContext, rng); err != nil {
		return NPCOperationResult{}, err
	}
	a := s.actor(op.Damage.ActorID)
	if a.HP <= 0 {
		return NPCOperationResult{}, ErrInvalidParticipant
	}
	if err := a.ValidateOrdinaryNPC(); err != nil {
		return NPCOperationResult{}, err
	}
	work := s.callbackCopy()
	op.Damage.Item185Activated, op.Damage.Item195Activated = false, false
	op.Damage.DamageCalls, op.Damage.AreaDamageCalls = 0, 0
	op.Damage.HealCalls, op.Damage.PartyHealCalls, op.Damage.TensionCalls = 0, 0, 0
	op.Damage.End = EndProcessing{}
	op.Damage.Reset, op.Damage.Defeat = nil, nil
	frame := cloneCallbackContext(CallbackContext{TargetContext: op.Damage.TargetContext, Damage: &op.Damage, Defeat: op.Defeat, Assets: op.Assets})
	if len(frame.EnemyIDs) == 0 {
		return NPCOperationResult{Target: frame.TargetContext, NextSeed: slices.Clone(seed)}, nil
	}
	if frame.Defeat != nil {
		frame.Defeat.TargetContext = frame.TargetContext
		*frame.Damage = work.nativeDamageContext(*frame.Damage, frame.Defeat, rng)
		frame.Damage.responseDefeat = frame.Defeat
	}
	result, err := work.applyNPCResponse(&op.Field, &frame, prepare, rng)
	if err != nil {
		return NPCOperationResult{}, err
	}
	for _, a := range work.Actors {
		count := 0
		if previous := s.actor(a.ID); previous != nil {
			count = len(previous.ConsumedItems)
		}
		if len(a.ConsumedItems) > count {
			if result.Consumed == nil {
				result.Consumed = make(map[string][]ConsumedItem)
			}
			result.Consumed[a.ID] = slices.Clone(a.ConsumedItems[count:])
		}
	}
	work.npcResponse = nil
	*s = work
	if op.Assets != nil {
		*op.Assets = *frame.Assets
	}
	return result, nil
}

func (s *LifetimeState) applyNPCResponse(field *NamedFieldContext, frame *CallbackContext, prepare func(*LifetimeState, *CallbackContext) (NPCActionPlan, error), rng random.Generator) (NPCOperationResult, error) {
	result := NPCOperationResult{}
	c := EquipmentTailContext{DamageContext: *frame.Damage, Defeat: frame.Defeat}
	c.Completion = LifetimeCompletion{Accepted: true, NPC: true, ActorID: c.ActorID}
	snapshot, err := s.CaptureEquipmentTail(c.TargetContext)
	if err != nil {
		return result, err
	}
	c.Snapshot = snapshot
	tail := NamedFieldTail{Before: s.Fields.Current(), Grave: s.SnapshotGrave(), NPC: true}
	plan, err := prepare(s, frame)
	if err != nil {
		return result, err
	}
	if err := s.validateNPCResponse(&c, field, tail, frame, plan, rng); err != nil {
		return result, err
	}
	s.npcResponse = &npcResponseFacts{}
	decision, err := s.beginNPCAction(&c, rng)
	if err != nil {
		return result, err
	}
	result.NormalAction = decision.NormalAction
	result.NextSeed = plan.NextSeed
	if decision.NormalAction {
		choice, err := plan.SelectChoice(rng)
		if err != nil {
			return result, err
		}
		if err := s.actor(c.ActorID).BeginNormalAction(true, false, false, rng); err != nil {
			return result, err
		}
		if err := s.ApplyNPCChoice(frame, plan.Catalog, choice, rng); err != nil {
			return result, err
		}
		result.Choice = choice.CallbackBinding
		if frame.Damage != nil {
			c.DamageContext = *frame.Damage
		}
		c.Defeat = frame.Defeat
		c.TargetContext = frame.TargetContext
		mergeNPCEffects(&c.EffectContext, &s.npcResponse.effects)
		if c.Defeat != nil {
			c.Defeat.TargetContext = c.TargetContext
		}
		if err := s.FinishNPCNormalAction(&c, rng); err != nil {
			return result, err
		}
	}
	if err := s.FinishNPCTails(&c, field, tail, rng); err != nil {
		return result, err
	}
	result.Target, result.Defeats = c.TargetContext, s.npcResponse.defeats
	return result, nil
}

func (s *LifetimeState) validateNPCResponse(c *EquipmentTailContext, field *NamedFieldContext, tail NamedFieldTail, frame *CallbackContext, plan NPCActionPlan, rng random.Generator) error {
	a := s.actor(c.ActorID)
	validateChoice := func(status string) error {
		return s.validateNPCChoiceState(frame, plan, status, rng, func(state *LifetimeState, projected *CallbackContext) error {
			child := *c
			if projected.Damage != nil {
				child.DamageContext = *projected.Damage
			}
			child.Defeat = projected.Defeat
			child.TargetContext = projected.TargetContext
			if child.Defeat != nil {
				child.Defeat.TargetContext = child.TargetContext
			}
			return state.validateNPCNormalContinuation(&child, field, tail, rng)
		})
	}
	status := a.Status
	if a.Extended == ExtendedPurification {
		status = ""
	}
	if status != "" && (status == StatusConfusion || hasItem(a.ItemDefinitionIDs, "item-246") || hasAbility(a.Abilities, "state1") ||
		(a.statusCureAbility() != "" && hasAbility(a.Abilities, a.statusCureAbility()))) {
		if err := validateChoice(""); err != nil {
			return err
		}
	}
	if !hasItem(a.ItemDefinitionIDs, "item-204") {
		switch status {
		case StatusDofuu, StatusParalyze, StatusSleep:
			if status != StatusDofuu {
				if err := s.validateNPCIdleTails(c, field, tail, status, rng); err != nil {
					return err
				}
			}
			return s.validateNPCIdleTails(c, field, tail, "", rng)
		case StatusKinju:
			if err := s.validateNPCKinju(c, field, tail, rng); err != nil {
				return err
			}
		case StatusSabaku:
			for _, outcome := range []string{status, ""} {
				if err := s.validateNPCIdleTails(c, field, tail, outcome, rng); err != nil {
					return err
				}
			}
		}
	}
	return validateChoice(status)
}

func (s *LifetimeState) validateNPCChoiceState(frame *CallbackContext, plan NPCActionPlan, status string, rng random.Generator, after func(*LifetimeState, *CallbackContext) error) error {
	p := s.callbackCopy()
	a := p.actor(frame.ActorID)
	a.Status = status
	// The expiry draw can preserve a temporary effect that isn't defense/counter.
	if err := a.BeginNormalAction(true, false, false, resetBoundary(.9)); err != nil {
		return err
	}
	if a.Temporary != "" {
		if err := p.validateNPCChoices(frame, plan, rng, after); err != nil {
			return err
		}
		a.Temporary = ""
	}
	return p.validateNPCChoices(frame, plan, rng, after)
}

func (s *LifetimeState) validateNPCNormalContinuation(c *EquipmentTailContext, field *NamedFieldContext, tail NamedFieldTail, rng random.Generator) error {
	a := s.actor(c.ActorID)
	if a.HP > 0 {
		// A random poison, recovery or rage transition leaves the subsequent
		// tails to validate against their actual execution state.
		switch {
		case a.Status == StatusDeadlyPoison || a.Status == StatusVirulentPoison:
			return s.validateNPCPoison(c, rng)
		case a.Temporary == TemporaryRegeneration:
			return nil
		case a.Status == StatusPoison:
			return s.validateNPCPoison(c, rng)
		case a.Temporary == TemporaryRage:
			_, err := s.gemTensionCeiling(&c.EffectContext, a, 1)
			return err
		}
	}
	return s.validateNPCIdleTails(c, field, tail, a.Status, rng)
}

func (s *LifetimeState) validateNPCKinju(c *EquipmentTailContext, field *NamedFieldContext, tail NamedFieldTail, rng random.Generator) error {
	a := s.actor(c.ActorID)
	minDamage, maxDamage := npcPoisonBounds(a)
	for _, hp := range damageSurvivorHPs(a.HP, minDamage, maxDamage) {
		p := s.callbackCopy()
		p.actor(c.ActorID).HP = hp
		p.actor(c.ActorID).Status = ""
		if err := p.validateNPCIdleTails(c, field, tail, "", rng); err != nil {
			return err
		}
	}
	if a.HP <= maxDamage {
		if c.Defeat == nil {
			return ErrUnsupportedEffect
		}
		p := s.callbackCopy()
		p.actor(c.ActorID).HP = 0
		p.actor(c.ActorID).Status = ""
		defeat := *c.Defeat
		defeat.TargetContext = c.TargetContext
		continuations, err := p.projectDefeatContinuation(&defeat, c.ActorID, rng, map[string]bool{}, true)
		if err != nil {
			return err
		}
		for _, next := range continuations {
			if err := next.validateNPCIdleTails(c, field, tail, next.actor(c.ActorID).Status, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

// Check tails on a known continuation. Each runtime owner rechecks the
// actual state reached after recovery, random callbacks and roster changes.
func (s *LifetimeState) validateNPCIdleTails(c *EquipmentTailContext, field *NamedFieldContext, tail NamedFieldTail, status string, rng random.Generator) error {
	p := s.callbackCopy()
	a := p.actor(c.ActorID)
	a.Status = status
	for _, id := range npcTailsBeforeField {
		if err := p.ValidateEquipmentTail(c, id, rng); err != nil {
			return err
		}
		if id == "item-164-operation-tail" && p.equipmentTailReached(c, id) {
			a.Temporary = ""
		}
	}
	child := *field
	child.TargetContext = c.TargetContext
	if err := p.ValidateNamedFieldTail(&child, tail, rng); err != nil {
		return err
	}
	p.applyBurnField(c.ActorID, tail)
	for _, id := range npcTailsAfterField {
		if err := p.ValidateEquipmentTail(c, id, rng); err != nil {
			return err
		}
	}
	return nil
}

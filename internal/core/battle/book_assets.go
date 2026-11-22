package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// BookReturned moves the current durable held unit to the requester depot with
// Depot.Deliver; it has no stored row.
const BookReturned StoredToolResult = "returned"

// bookAssets reports whether job86's held-item branch runs. A player branch
// needs the requester as actor, matching hands and the depot; an NPC actor
// would reach another identity's durable hand or a missing depot.
func (s *LifetimeState) bookAssets(c *JobAssetContext, id string) (bool, error) {
	a := s.actor(c.ActorID)
	if a == nil || a.HeldToolID == nil {
		return false, ErrUnsupportedEffect
	}
	held := *a.HeldToolID
	run := heldBookKind(held) != "none"
	if id == "job-86-entry-0" && !run && s.Fields.Current() == "library" {
		// The library exception reads the requester's durable hand.
		if c.Assets.HeldToolID == nil {
			return false, ErrUnsupportedEffect
		}
		run = *c.Assets.HeldToolID == ""
	}
	if !run {
		return false, nil
	}
	if a.NPC || c.ActorID != c.Assets.RequesterID || c.Assets.HeldToolID == nil || *c.Assets.HeldToolID != held || c.Assets.StoredTools == nil {
		return false, ErrUnsupportedEffect
	}
	return true, nil
}

// bookTargets are double use's two scoped participants; the same one may be
// chosen twice. Malformed or foreign pairs reject instead of the source scan.
func (c *JobAssetContext) bookTargets() ([]string, error) {
	if len(c.Arguments) < 2 {
		return nil, ErrUnsupportedEffect
	}
	targets := c.Arguments[:2]
	for _, target := range targets {
		if !slices.Contains(c.PartyIDs, target) && !slices.Contains(c.EnemyIDs, target) {
			return nil, ErrUnsupportedEffect
		}
	}
	return targets, nil
}

func (c *JobAssetContext) bookFrame() (CallbackContext, error) {
	if c.Frame == nil {
		return CallbackContext{}, ErrUnsupportedEffect
	}
	frame := *c.Frame
	frame.TargetContext = c.TargetContext
	return frame, nil
}

func (s *LifetimeState) validateBookAssets(c *JobAssetContext, id string, rng random.Generator) error {
	run, err := s.bookAssets(c, id)
	if err != nil || !run {
		return err
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-86-entry-2":
		p := s.callbackCopy()
		return p.validateRecovery(&c.EffectContext, controlEffect(c.ActorID, float64(a.MaxMP)*.15, CategoryMagic), rng)
	case "job-86-entry-3":
		if float64(a.MaxHP)*.15 >= 1e15 {
			return ErrUnsupportedEffect
		}
	case "job-86-entry-5":
		targets, err := c.bookTargets()
		if err != nil {
			return err
		}
		entry, mp := bookEntry(*a.HeldToolID)
		if int(float64(mp)*1.5) > a.MP {
			return nil
		}
		frame, err := c.bookFrame()
		if err != nil {
			return err
		}
		// An unprojected first use leaves the second to execution-time validation.
		op := CallbackInvocation{Kind: "entry", EntryID: entry}
		return s.validateSequence(&frame, c.Catalog, []CallbackInvocation{op, op}, [][]string{targets[:1], targets[1:]}, rng)
	}
	return nil
}

// applyBookAssets is job86 positions0/2/3/5 (_skill.cgi:6806–6899,10489–10536).
// Withdrawal failure is a selection error with no asset or effect change.
func (s *LifetimeState) applyBookAssets(c *JobAssetContext, id string, rng random.Generator) error {
	run, err := s.bookAssets(c, id)
	if err != nil || !run {
		return err
	}
	a, assets := s.actor(c.ActorID), c.Assets
	switch id {
	case "job-86-entry-0", "job-86-entry-3":
		target := c.shift()
		index := slices.IndexFunc(assets.StoredTools, func(row StoredTool) bool {
			return row.DefinitionID == target && heldBookKind(row.DefinitionID) != "none"
		})
		if index < 0 {
			assets.SelectionError = true
			return nil
		}
		row := assets.StoredTools[index]
		// shortcut: the swapped-in hand has no instance ID here, so a later
		// withdrawal in the same operation cannot select it; add one if reachable.
		assets.StoredTools = slices.Delete(slices.Clone(assets.StoredTools), index, index+1)
		assets.StoredOutcomes = append(assets.StoredOutcomes, StoredToolOutcome{InstanceID: row.InstanceID, Result: StoredToolExchanged})
		a.setHeldTool(row.DefinitionID)
		*assets.HeldToolID = row.DefinitionID
		if id == "job-86-entry-0" {
			return nil
		}
		c.ResponseSuppressed = true
		if _, err := s.ApplyRecoil(&c.EffectContext, &c.End, float64(a.MaxHP)*.15, rng); err != nil {
			return err
		}
		if s.SkillBans == nil {
			s.SkillBans = make(map[string]int)
		}
		s.SkillBans[id] = 2 // `$npc_turn_skip` skips the tail tick (_battle.cgi:938,1137–1150).
		return nil
	case "job-86-entry-2":
		if _, err := s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(a.MaxMP)*.15, CategoryMagic), rng); err != nil {
			return err
		}
	case "job-86-entry-5":
		targets, err := c.bookTargets()
		if err != nil {
			return err
		}
		c.Arguments = c.Arguments[2:] // The pair is the source's single argument.
		entry, mp := bookEntry(*a.HeldToolID)
		cost := int(float64(mp) * 1.5)
		if cost > a.MP {
			return nil
		}
		frame, err := c.bookFrame()
		if err != nil {
			return err
		}
		for i, target := range targets {
			child := frame
			child.Arguments = []string{target}
			if err := s.invokeCallback(&child, c.Catalog, CallbackInvocation{Kind: "entry", EntryID: entry}, rng); err != nil {
				return err
			}
			frame.TargetContext, frame.Damage, frame.Status, frame.Defeat = child.TargetContext, child.Damage, child.Status, child.Defeat
			frame.ArithmeticDivisor = child.ArithmeticDivisor
			if i == 0 && s.actor(c.ActorID).HP <= 0 {
				c.adoptFrame(&frame)
				return nil
			}
		}
		c.adoptFrame(&frame)
		a = s.actor(c.ActorID)
		a.MP -= cost // The enclosing action clamps; children's MP changes stay.
	}
	// Return the current hands, including changes made by the children.
	if *assets.HeldToolID != "" {
		assets.StoredOutcomes = append(assets.StoredOutcomes, StoredToolOutcome{Result: BookReturned})
	}
	a.setHeldTool("")
	*assets.HeldToolID = ""
	return nil
}

// adoptFrame keeps a child frame's operation context in the enclosing call.
func (c *JobAssetContext) adoptFrame(frame *CallbackContext) {
	c.TargetContext = frame.TargetContext
	c.Frame.Damage, c.Frame.Status, c.Frame.Defeat = frame.Damage, frame.Status, frame.Defeat
	c.Frame.ArithmeticDivisor = frame.ArithmeticDivisor
	if frame.Damage != nil {
		c.DamageContext = *frame.Damage
	}
}

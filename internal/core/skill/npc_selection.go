package skill

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// NPCChoice preserves a weighted name position and its last admitted binding.
// Callbacks and definition ports are rebuilt per request, never cached on actors.
type NPCChoice struct {
	corebattle.CallbackBinding
	Custom      bool
	Invocations []corebattle.CallbackInvocation
}

// NPCActionResult carries the request-local table left by source construction.
// The next request starts from its mode seed; only a same-request repeat uses it.
type NPCActionResult struct {
	NPCChoice
	NextSeed []corebattle.CallbackBinding
}

// NPCDefaultSeed belongs to the request's mode, independently of job0's SP gates.
// A repeated response in the same request receives the previous table as seed.
func NPCDefaultSeed() []corebattle.CallbackBinding {
	return []corebattle.CallbackBinding{{CallbackEntry: corebattle.CallbackEntry{ID: "npc-normal-attack", Name: "normal-attack"}}}
}

func npcActor(s *corebattle.LifetimeState, c *corebattle.CallbackContext) (*corebattle.LifetimeActor, error) {
	if s == nil || c == nil {
		return nil, corebattle.ErrInvalidRequest
	}
	for i := range s.Actors {
		if s.Actors[i].ID == c.ActorID && s.Actors[i].NPC {
			return &s.Actors[i], nil
		}
	}
	return nil, corebattle.ErrInvalidParticipant
}

func (d BattleDefinitions) npcTable(s *corebattle.LifetimeState, c *corebattle.CallbackContext, id string) ([]corebattle.CallbackBinding, error) {
	job := 0
	if id != "starter" {
		var err error
		job, err = strconv.Atoi(strings.TrimPrefix(id, "job-"))
		if err != nil || id != fmt.Sprintf("job-%02d", job) {
			return nil, corebattle.ErrUnsupportedEffect
		}
	}
	return s.CallbackBindings(c, d.Invocations, job)
}

// NPCChoices applies NPC-only SP/MP/extension predicates in source order. Every
// eligible last binding must be supported: filtering missing bodies would alter
// the random pool. Primitive batch definitions cannot certify NPC callbacks.
// _npc_action.cgi:159–224,313–322; vs_monster.cgi:11–14.
func (d BattleDefinitions) NPCChoices(s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding, rng random.Generator) ([]NPCChoice, error) {
	a, err := npcActor(s, c)
	if err != nil || rng == nil {
		if err != nil {
			return nil, err
		}
		return nil, corebattle.ErrInvalidRequest
	}
	if a.HP <= 0 {
		return nil, corebattle.ErrInvalidParticipant
	}
	if err := a.ValidateOrdinaryNPC(); err != nil {
		return nil, err
	}
	var positions []string
	bindings := map[string]NPCChoice{}
	appendEntry := func(e corebattle.CallbackBinding, sp int, job string) {
		if e.RequiredSP > sp || e.MPCost > a.MP || (e.Extended && (!slices.Contains(a.ItemDefinitionIDs, "item-158") || job == "job-73")) {
			return
		}
		positions = append(positions, e.Name)
		bindings[e.Name] = NPCChoice{CallbackBinding: e}
	}
	current, err := d.npcTable(s, c, a.Jobs.JobID)
	if err != nil {
		return nil, err
	}
	for _, e := range append(slices.Clone(seed), current...) {
		appendEntry(e, a.Jobs.SP, a.Jobs.JobID)
	}
	previous, err := d.npcTable(s, c, a.Jobs.OldJobID)
	if err != nil {
		return nil, err
	}
	for _, e := range previous {
		appendEntry(e, a.Jobs.OldSP, a.Jobs.OldJobID)
	}
	if load := a.NPCLoadout; load != nil && load.CustomName != "" && a.CMP >= load.CustomCost {
		choice := NPCChoice{Custom: true, CallbackBinding: corebattle.CallbackBinding{CallbackEntry: corebattle.CallbackEntry{ID: "npc-custom", Name: load.CustomName}}}
		for _, id := range load.Gems {
			if id != "" {
				choice.Invocations = append(choice.Invocations, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
			}
		}
		if len(choice.Invocations) > 0 {
			positions = append(positions, load.CustomName)
			bindings[load.CustomName] = choice
		}
	}
	choices := make([]NPCChoice, 0, len(positions))
	for _, name := range positions {
		choice := bindings[name]
		frame := *c
		frame.Arguments = nil // NPC callbacks receive no player selector/extension list.
		frame.Custom = choice.Custom
		if choice.Custom {
			frame.Custom = true
			frame.Arguments = []string{a.ID}
		}
		if choice.ID == "npc-normal-attack" {
			if c.Damage == nil || !c.Damage.ActorNPC {
				return nil, corebattle.ErrUnsupportedEffect
			}
			child := *c.Damage
			child.TargetContext = frame.TargetContext
			err = s.ValidateNormalAttack(&child, "", rng)
		} else {
			ops := choice.Invocations
			if !choice.Custom {
				ops = []corebattle.CallbackInvocation{choice.Invocation}
			}
			err = s.ValidateInvocations(&frame, d.Invocations, ops, rng)
		}
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	if len(choices) == 0 {
		return nil, corebattle.ErrUnsupportedEffect
	}
	return choices, nil
}

// ApplyNPCAction derives execution from the same server-owned discovery pool.
// Costs are late, after a successful callback, including accepted no-effect.
// Status/temporary checks precede this boundary; outer tails follow it.
func (d BattleDefinitions) ApplyNPCAction(s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding, rng random.Generator) (NPCActionResult, error) {
	choices, err := d.NPCChoices(s, c, seed, rng)
	if err != nil {
		return NPCActionResult{}, err
	}
	a, err := npcActor(s, c)
	if err != nil {
		return NPCActionResult{}, err
	}
	nextSeed, err := d.npcTable(s, c, a.Jobs.OldJobID)
	if err != nil {
		return NPCActionResult{}, err
	}
	choice := choices[int(rng.Float64()*float64(len(choices)))]
	frame := *c
	frame.Arguments = nil
	frame.Custom = choice.Custom
	if choice.Custom {
		frame.Custom = true
		frame.Arguments = []string{c.ActorID}
	}
	if choice.ID == "npc-normal-attack" {
		c.Damage.TargetContext = frame.TargetContext
		_, err = s.ApplyNormalAttack(c.Damage, "", rng)
		frame.TargetContext = c.Damage.TargetContext
	} else {
		ops := choice.Invocations
		if !choice.Custom {
			ops = []corebattle.CallbackInvocation{choice.Invocation}
		}
		err = s.ApplyInvocations(&frame, d.Invocations, ops, rng)
	}
	if err != nil {
		return NPCActionResult{}, err
	}
	*c = frame
	a, err = npcActor(s, c)
	if err != nil {
		return NPCActionResult{}, err
	}
	a.MP = max(0, a.MP-choice.MPCost)
	if load := a.NPCLoadout; load != nil && choice.Name == load.CustomName {
		a.CMP -= load.CustomCost
	}
	return NPCActionResult{NPCChoice: choice, NextSeed: nextSeed}, nil
}

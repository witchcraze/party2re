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
type NPCChoice = corebattle.NPCChoice

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

func (d BattleDefinitions) npcBurnStrike(s *corebattle.LifetimeState, a *corebattle.LifetimeActor) bool {
	if d.lateBreathSong != nil {
		return d.lateBreathSong.BurnStrike
	}
	if s == nil || a == nil {
		return false
	}
	return a.LateBreathSongBurn(s.Fields.Current(), "m") || a.LateBreathSongBurn(s.Fields.Current(), "f")
}

// NPCChoices applies NPC-only SP/MP/extension predicates in source order. Every
// eligible last binding must be supported: filtering missing bodies would alter
// the random pool. Primitive batch definitions cannot certify NPC callbacks.
// _npc_action.cgi:159–224,313–322; vs_monster.cgi:11–14.
func (d BattleDefinitions) NPCChoices(s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding, rng random.Generator) ([]NPCChoice, error) {
	plan, err := d.npcChoicePool(s, c, seed)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateNPCChoices(c, plan, rng); err != nil {
		return nil, err
	}
	return plan.Choices, nil
}

func (d BattleDefinitions) npcChoicePool(s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding) (corebattle.NPCActionPlan, error) {
	a, err := npcActor(s, c)
	if err != nil {
		return corebattle.NPCActionPlan{}, err
	}
	if a.HP <= 0 {
		return corebattle.NPCActionPlan{}, corebattle.ErrInvalidParticipant
	}
	if err := a.ValidateOrdinaryNPC(); err != nil {
		return corebattle.NPCActionPlan{}, err
	}
	burn := d.npcBurnStrike(s, a)
	var positions []string
	bindings := map[string]NPCChoice{}
	byDisplay := map[string]string{}
	appendEntry := func(e corebattle.CallbackBinding, sp int, job string) {
		if e.RequiredSP > sp || e.MPCost > a.MP || (e.Extended && (!slices.Contains(a.ItemDefinitionIDs, "item-158") || job == "job-73")) {
			return
		}
		key := BindingSourceKey(e, burn)
		if key == "" {
			key = "id:" + e.ID
		}
		positions = append(positions, key)
		bindings[key] = NPCChoice{CallbackBinding: e}
		if prev, ok := byDisplay[e.Name]; !ok {
			byDisplay[e.Name] = key
		} else if prev != key {
			byDisplay[e.Name] = ""
		}
	}
	current, err := d.npcTable(s, c, a.Jobs.JobID)
	if err != nil {
		return corebattle.NPCActionPlan{}, err
	}
	for _, e := range append(slices.Clone(seed), current...) {
		appendEntry(e, a.Jobs.SP, a.Jobs.JobID)
	}
	previous, err := d.npcTable(s, c, a.Jobs.OldJobID)
	if err != nil {
		return corebattle.NPCActionPlan{}, err
	}
	for _, e := range previous {
		appendEntry(e, a.Jobs.OldSP, a.Jobs.OldJobID)
	}
	customKey := ""
	if load := a.NPCLoadout; load != nil && load.CustomName != "" {
		customKey = OperationNameKey(load.CustomName)
		if _, ok := bindings[customKey]; !ok {
			if displayKey := byDisplay[load.CustomName]; displayKey != "" {
				customKey = displayKey
			}
		}
		if a.CMP >= load.CustomCost {
			choice := NPCChoice{Custom: true, ChargeCustom: true, CallbackBinding: corebattle.CallbackBinding{CallbackEntry: corebattle.CallbackEntry{ID: "npc-custom", Name: load.CustomName}}}
			for _, id := range load.Gems {
				if id != "" {
					choice.Invocations = append(choice.Invocations, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
				}
			}
			if len(choice.Invocations) > 0 {
				positions = append(positions, customKey)
				bindings[customKey] = choice
			}
		}
	}
	choices := make([]NPCChoice, 0, len(positions))
	for _, key := range positions {
		choice := bindings[key]
		if customKey != "" && key == customKey {
			choice.ChargeCustom = true
		}
		choices = append(choices, choice)
	}
	return corebattle.NPCActionPlan{Choices: choices, NextSeed: previous, Catalog: d.Invocations}, nil
}

// ApplyNPCAction derives execution from the same server-owned discovery pool.
// Costs are late, after a successful callback, including accepted no-effect.
// Status/temporary checks precede this boundary; outer tails follow it.
func (d BattleDefinitions) ApplyNPCAction(s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding, rng random.Generator) (NPCActionResult, error) {
	plan, err := d.npcChoicePool(s, c, seed)
	if err != nil {
		return NPCActionResult{}, err
	}
	if err := s.ValidateNPCChoices(c, plan, rng); err != nil {
		return NPCActionResult{}, err
	}
	choice := plan.Choices[int(rng.Float64()*float64(len(plan.Choices)))]
	if err := s.ApplyNPCChoice(c, plan.Catalog, choice, rng); err != nil {
		return NPCActionResult{}, err
	}
	return NPCActionResult{NPCChoice: choice, NextSeed: plan.NextSeed}, nil
}

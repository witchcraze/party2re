package battle

import (
	"math"
	"reflect"

	"github.com/witchcraze/party2re/internal/core/random"
)

type StatusContinuation struct {
	State   LifetimeState
	Context StatusContext
}

func appendStatusContinuation(branches []StatusContinuation, s LifetimeState, c StatusContext) []StatusContinuation {
	facts := func(c StatusContext) StatusContext {
		c.Damage, c.targetResources, c.Defeat = nil, nil, nil
		return c
	}
	for _, old := range branches {
		cursor, previous := TargetCursor{}, TargetCursor{}
		if c.Defeat != nil {
			cursor = c.Defeat.Cursor
		}
		if old.Context.Defeat != nil {
			previous = old.Context.Defeat.Cursor
		}
		if cursor == previous && reflect.DeepEqual(facts(old.Context), facts(c)) && reflect.DeepEqual(old.State, s) {
			return branches
		}
	}
	f := cloneCallbackContext(CallbackContext{Status: &c})
	return append(branches, StatusContinuation{State: s.callbackCopy(), Context: *f.Status})
}

// ProjectStatusGemBranches supplies complete intrinsic status/cure alternatives.
// The job dispel body retains its independent reset owner and unknown projection.
func (s *LifetimeState) ProjectStatusGemBranches(c *StatusContext, id, targetID string) ([]StatusContinuation, error) {
	if s == nil || c == nil {
		return nil, ErrInvalidRequest
	}
	source := 0
	for _, e := range StatusEntries() {
		if e.ID == id {
			source = e.SourceID
			break
		}
	}
	if source == 0 {
		return nil, nil
	}
	if err := s.ValidateStatusEntry(c, nil, id, targetID, random.Default()); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	e, chance := statusEntryEffect(source, targetID)
	branches := appendStatusContinuation(nil, *s, *c)
	if source == 62 {
		first, err := s.projectSymbolicTargets(c.TargetContext, e, false)
		if err != nil {
			return nil, err
		}
		var results []StatusContinuation
		for _, branch := range first {
			if !branch.Resolution.Resolved {
				results = appendStatusContinuation(results, branch.State, *c)
				continue
			}
			child := branch.Resolution.TargetEffect
			child.Payload.Symbol = branch.State.actor(child.TargetID).Status
			next, err := branch.State.projectGemCure(*c, child)
			if err != nil {
				return nil, err
			}
			for _, n := range next {
				results = appendStatusContinuation(results, n.State, n.Context)
			}
		}
		return results, nil
	}
	ids := c.EnemyIDs
	if source < 62 {
		ids = c.PartyIDs
		branches[0].Context.PartyCureCalls++
		branches[0].Context.AdditionalEffect = true
	}
	if source == 70 {
		ids = []string{targetID}
	}
	var results []StatusContinuation
	if source >= 71 {
		if branches[0].State.actor(c.ActorID).categorySealed(e.Category) {
			results = appendStatusContinuation(results, branches[0].State, branches[0].Context)
			branches[0].State.actor(c.ActorID).Status = ""
		}
		if source == 71 {
			branches[0].Context.AreaDeathCalls++
		} else {
			branches[0].Context.AreaStatusCalls++
		}
	}
	for _, target := range ids {
		e.TargetID = target
		var next []StatusContinuation
		for _, branch := range branches {
			var children []StatusContinuation
			var err error
			if source < 62 {
				children, err = branch.State.projectGemCure(branch.Context, e)
			} else {
				children, err = branch.State.projectGemStatus(branch.Context, e, chance, source == 70 || source == 71)
			}
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				next = appendStatusContinuation(next, child.State, child.Context)
			}
		}
		branches = next
	}
	for _, branch := range branches {
		results = appendStatusContinuation(results, branch.State, branch.Context)
	}
	return results, nil
}

func (s *LifetimeState) projectGemCure(c StatusContext, e TargetEffect) ([]StatusContinuation, error) {
	targets, err := s.projectSymbolicTargets(c.TargetContext, e, false)
	if err != nil {
		return nil, err
	}
	var results []StatusContinuation
	for _, branch := range targets {
		child := c
		r := branch.Resolution
		if r.Resolved && r.Payload.Symbol != "" && branch.State.actor(r.TargetID).HP > 0 {
			child.CureCalls++
			t := branch.State.actor(r.TargetID)
			if t.Status == r.Payload.Symbol || r.Payload.Symbol == StatusDeadlyPoison && (t.Status == StatusPoison || t.Status == StatusVirulentPoison) {
				t.Status = ""
			}
		}
		results = appendStatusContinuation(results, branch.State, child)
	}
	return results, nil
}

func (s *LifetimeState) projectGemStatus(c StatusContext, e TargetEffect, chance float64, death bool) ([]StatusContinuation, error) {
	target := c.TargetContext
	target.symbolicConversion = true
	targets, err := s.projectSymbolicTargets(target, e, true)
	if err != nil {
		return nil, err
	}
	var results []StatusContinuation
	for _, branch := range targets {
		child := c
		r, p := branch.Resolution, branch.State
		if !r.Resolved || r.Payload.Symbol == "" || p.actor(r.TargetID).HP <= 0 {
			results = appendStatusContinuation(results, p, child)
			continue
		}
		if death {
			child.DeathCalls++
		} else {
			child.StatusCalls++
		}
		probability := TargetEffect{Category: r.Category, Payload: CallbackPayload{Number: chance}}
		power := p.finalEffectPower(&child.EffectContext, p.initialEffectPower(&child.EffectContext, probability, ModifierStatus))
		if !death && p.Fields.Current() == "den" {
			power = math.Trunc(power * 1.2)
		}
		t := p.actor(r.TargetID)
		if statusImmune(t, false, death, p.Fields.Current()) || (!death && power <= 0) {
			results = appendStatusContinuation(results, p, child)
			continue
		}
		if t.Gear == nil {
			return nil, ErrUnsupportedEffect
		}
		if power < 100 || t.Gear.WeaponID == "weapon-68" && t.Gear.ArmorID == "armor-53" {
			results = appendStatusContinuation(results, p, child)
		}
		if !death {
			if hasItem(t.ItemDefinitionIDs, "item-161") && t.Tension >= 5 {
				results = appendStatusContinuation(results, p, child)
				continue
			}
			t.Status, t.Tension = r.Payload.Symbol, 1
			results = appendStatusContinuation(results, p, child)
			continue
		}
		if child.Defeat == nil {
			return nil, ErrUnsupportedEffect
		}
		defeat := *child.Defeat
		defeat.TargetContext = child.TargetContext
		states, err := p.projectDefeatBranches(&defeat, t.ID, random.Default(), map[string]bool{}, true)
		if err != nil {
			return nil, err
		}
		for _, state := range states {
			next := child
			next.Defeat = &state.Context
			results = appendStatusContinuation(results, state.State, next)
		}
	}
	return results, nil
}

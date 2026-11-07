package battle

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

// StoredTool is one ordered tool row of the requesting player's depot.
type StoredTool struct {
	InstanceID   string
	DefinitionID string
	Category     item.UsageCategory
}

type StoredToolResult string

const (
	StoredToolNoMatch   StoredToolResult = "no_match"  // Selection error; the source also resets the interval to0.
	StoredToolRetained  StoredToolResult = "retained"  // A child selection error keeps the row and the history.
	StoredToolConsumed  StoredToolResult = "consumed"  // Category1 child returned normally, including no-effect.
	StoredToolExchanged StoredToolResult = "exchanged" // Other categories swap the row with the durable hand.
)

// StoredToolOutcome is applied by the asset owner in order; no-match has no row.
type StoredToolOutcome struct {
	InstanceID string
	Result     StoredToolResult
}

// StoredToolTravel is Adventure's operation-local travel port. Validate errors
// other than ErrUnsupportedEffect are source selection errors. Apply never
// consumes the stored instance; the outcome does.
type StoredToolTravel struct {
	Validate func(tool, target string) error
	Apply    func(tool, target string, frame TargetContext) (TargetContext, error)
}

// storedToolEntry maps a tool definition to its catalog callback identity.
func storedToolEntry(definitionID string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(definitionID, "item-"))
	if err != nil || !strings.HasPrefix(definitionID, "item-") {
		return ""
	}
	return fmt.Sprintf("tool-%d", n)
}

// storedToolTarget trims whitespace, then one leading separator before text.
func storedToolTarget(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 1 && text[0] == '>' {
		return text[1:]
	}
	return text
}

// storedTool selects the first matching row whose definition this color has not
// used. History excludes every copy, so a nested job50 never reselects it.
func (s *LifetimeState) storedTool(actorID string, a *JobAssets) (StoredTool, bool) {
	used := s.StoredToolHistory[s.actor(actorID).TeamID]
	for _, row := range a.StoredTools {
		if row.DefinitionID == a.StoredItem && !used[row.DefinitionID] {
			return row, true
		}
	}
	return StoredTool{}, false
}

func (s *LifetimeState) recordStoredTool(color, definitionID string) {
	if s.StoredToolHistory == nil {
		s.StoredToolHistory = make(map[string]map[string]bool)
	}
	if s.StoredToolHistory[color] == nil {
		s.StoredToolHistory[color] = make(map[string]bool)
	}
	s.StoredToolHistory[color][definitionID] = true
}

// storedChild binds the outer submitted target, not the forwarded argument.
// Travel and other tools absent from the catalog have no callback child.
func storedChild(c *JobAssetContext, definitionID, target string) (*CallbackContext, []CallbackInvocation) {
	entry := storedToolEntry(definitionID)
	if _, ok := c.Catalog.Entries[entry]; !ok || c.Frame == nil {
		return nil, nil
	}
	child := *c.Frame
	child.TargetContext, child.Arguments = c.TargetContext, []string{target}
	return &child, []CallbackInvocation{{Kind: "entry", EntryID: entry}}
}

func (s *LifetimeState) validateStoredTool(c *JobAssetContext, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	if p.actor(c.ActorID).NPC {
		child.Arguments = slices.Clone(c.Arguments)
		return p.validateRecovery(&child.EffectContext, controlEffect(child.shift(), 500, CategoryMagic), rng)
	}
	if c.Assets.StoredTools == nil {
		return ErrUnsupportedEffect
	}
	row, ok := p.storedTool(c.ActorID, c.Assets)
	if !ok {
		return nil
	}
	if row.Category != item.UsageCategoryCombatOnly {
		if c.Assets.HeldToolID == nil {
			return ErrUnsupportedEffect
		}
		return nil
	}
	p.recordStoredTool(p.actor(c.ActorID).TeamID, row.DefinitionID)
	target := storedToolTarget(c.Assets.StoredTarget)
	if frame, ops := storedChild(c, row.DefinitionID, target); frame != nil {
		return p.ValidateInvocations(frame, c.Catalog, ops, rng)
	}
	if c.Travel == nil {
		return ErrUnsupportedEffect
	}
	if err := c.Travel.Validate(row.DefinitionID, target); errors.Is(err, ErrUnsupportedEffect) {
		return err
	}
	return nil
}

// applyStoredTool is job50's extended body. History is written before the child,
// and a child selection error retains both the history and the row.
func (s *LifetimeState) applyStoredTool(c *JobAssetContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	if a.NPC {
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), 500, CategoryMagic), rng)
		return err
	}
	assets := c.Assets
	row, ok := s.storedTool(c.ActorID, assets)
	if !ok {
		assets.SelectionError = true
		assets.StoredOutcomes = append(assets.StoredOutcomes, StoredToolOutcome{Result: StoredToolNoMatch})
		return nil
	}
	s.recordStoredTool(a.TeamID, row.DefinitionID)
	outcome := StoredToolOutcome{InstanceID: row.InstanceID, Result: StoredToolExchanged}
	if row.Category != item.UsageCategoryCombatOnly {
		held := row.DefinitionID
		*assets.HeldToolID = held
		a.HeldToolID = &held
		assets.StoredOutcomes = append(assets.StoredOutcomes, outcome)
		return nil
	}
	target := storedToolTarget(assets.StoredTarget)
	if frame, ops := storedChild(c, row.DefinitionID, target); frame != nil {
		if err := s.ApplyInvocations(frame, c.Catalog, ops, rng); err != nil {
			return err
		}
		c.TargetContext = frame.TargetContext
		c.Frame.Damage, c.Frame.Status, c.Frame.Defeat = frame.Damage, frame.Status, frame.Defeat
		c.Frame.ArithmeticDivisor = frame.ArithmeticDivisor
		if frame.Damage != nil {
			c.DamageContext = *frame.Damage
		}
	} else if err := c.Travel.Validate(row.DefinitionID, target); errors.Is(err, ErrUnsupportedEffect) {
		return err
	} else if err != nil {
		assets.SelectionError = true
	} else if c.TargetContext, err = c.Travel.Apply(row.DefinitionID, target, c.TargetContext); err != nil {
		return err
	}
	// The source checks the operation-wide selection message, not only this child's.
	outcome.Result = StoredToolConsumed
	if assets.SelectionError {
		outcome.Result = StoredToolRetained
	}
	assets.StoredOutcomes = append(assets.StoredOutcomes, outcome)
	return nil
}

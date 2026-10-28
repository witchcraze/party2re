package battle

import (
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

var ErrNoBattleTarget = errors.New("no battle target available")

type OrdinaryToolEntry struct {
	CallbackEntry
	DefinitionID  string
	UsageCategory item.UsageCategory
}

// OrdinaryToolEntries is the finite subset at ff97fc5:_data.cgi:623–1889.
// These identities require explicit callbacks, never the batch scalar mapper.
func OrdinaryToolEntries() []OrdinaryToolEntry {
	var entries []OrdinaryToolEntry
	for _, source := range []int{1, 2, 3, 4, 5, 7, 8, 9, 10, 14, 15, 41, 42, 75, 76, 77, 79, 80, 81, 82, 86, 87, 102, 103, 130, 151, 213} {
		id := fmt.Sprintf("tool-%d", source)
		entries = append(entries, OrdinaryToolEntry{CallbackEntry: CallbackEntry{ID: id, Name: id}, DefinitionID: fmt.Sprintf("item-%03d", source), UsageCategory: item.UsageCategoryCombatOnly})
	}
	return entries
}

// OrdinaryToolInput is frozen with the run, independently of rebuilt ports.
// RequesterMaxHP is persistent requester data, not the active actor's MaxHP.
type OrdinaryToolInput struct {
	RequesterMaxHP *int                         `json:"requester_max_hp,omitempty"`
	Inspection     map[string]InspectionContent `json:"inspection,omitempty"`
}

type OrdinaryToolResult struct {
	ID      string           `json:"id"`
	Consume bool             `json:"consume"`
	Facts   []InspectionFact `json:"facts,omitempty"`
}

type OrdinaryToolContext struct {
	DamageContext
	OrdinaryToolInput
	EquipmentReset *EquipmentResetContext
	Arguments      []string
	CureCalls      int
	Results        []OrdinaryToolResult
}

func (c *OrdinaryToolContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	target := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return target
}

func (c *OrdinaryToolContext) toolTarget() string {
	if target := c.shift(); target != "" {
		return target
	}
	return c.ActorID
}

func ordinaryToolTemporary(id string) TemporaryEffect {
	switch id {
	case "tool-14":
		return TemporaryReduceAttack
	case "tool-15":
		return TemporaryReflectMagic
	case "tool-80":
		return TemporaryReduceMagic
	case "tool-81":
		return TemporaryAbsorbMagic
	case "tool-82":
		return TemporaryDouble
	case "tool-102":
		return TemporaryRegeneration
	case "tool-103":
		return TemporaryRevival
	}
	return ""
}

func ordinaryToolHealing(id string) float64 {
	switch id {
	case "tool-1":
		return 40
	case "tool-2":
		return 100
	case "tool-3":
		return 250
	case "tool-4":
		return 200
	case "tool-5":
		return 999
	}
	return 0
}

// ApplyOrdinaryTool records a consumption intent only after an accepted body.
// Durable instances, receipts and retries are owned by the consumer.
func (s *LifetimeState) ApplyOrdinaryTool(c *OrdinaryToolContext, id string, rng random.Generator) error {
	if inspectionEntry(id) {
		return ErrUnsupportedEffect
	}
	if err := s.ValidateOrdinaryTool(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	result := OrdinaryToolResult{ID: id, Consume: true}
	if power := ordinaryToolHealing(id); power != 0 {
		if id == "tool-4" || id == "tool-5" {
			if _, err := s.ApplyPartyHealing(&c.EffectContext, power, CategoryTool, rng); err != nil {
				return err
			}
		} else if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.toolTarget(), power, CategoryTool), rng); err != nil {
			return err
		}
	} else if tmp := ordinaryToolTemporary(id); tmp != "" {
		e := controlSymbol(c.toolTarget(), string(tmp), CategoryTool)
		var r TargetResolution
		var err error
		if id == "tool-82" {
			r, err = s.ResolveEnemyTarget(c.TargetContext, e, rng)
		} else {
			r, err = s.ResolvePartyTarget(c.TargetContext, e, rng)
		}
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved && (!(id == "tool-80" || id == "tool-81") || s.actor(r.TargetID).HP > 0) {
			s.actor(r.TargetID).Temporary = tmp
		}
	} else {
		switch id {
		case "tool-7", "tool-8", "tool-9", "tool-10":
			e := controlSymbol(c.toolTarget(), StatusDeadlyPoison, CategoryTool)
			if id == "tool-8" {
				e.Payload.Symbol = StatusParalyze
			} else if id == "tool-9" {
				e.Payload.Symbol = StatusConfusion
			} else if id == "tool-10" {
				r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(e.TargetID, "cure", CategoryTool), rng)
				if err != nil {
					return err
				}
				resources(r)
				if r.Resolved {
					e.TargetID, e.Payload.Symbol = r.TargetID, s.actor(r.TargetID).Status
				}
			}
			status := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
			if _, err := s.ApplyCure(&status, e, rng); err != nil {
				return err
			}
			c.CureCalls = status.CureCalls
		case "tool-41":
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.toolTarget(), 80, CategoryTool, true), rng); err != nil {
				return err
			}
		case "tool-42":
			if _, err := s.ApplyAreaDamage(&c.DamageContext, 60, CategoryTool, true, rng); err != nil {
				return err
			}
		case "tool-75", "tool-77", "tool-86":
			e := controlSymbol(c.Cursor.ActorID, "tension", CategoryTool)
			var r TargetResolution
			var err error
			if id == "tool-86" {
				r, err = s.ResolvePartyTarget(c.TargetContext, e, rng)
			} else {
				r, err = s.ResolveEnemyTarget(c.TargetContext, e, rng)
			}
			if err != nil {
				return err
			}
			resources(r)
			if id == "tool-75" {
				if r.Resolved {
					s.actor(r.TargetID).Tension = 1
				}
			} else if r.Resolved {
				if err := s.statTension(&c.EffectContext, s.actor(r.TargetID), false, rng); err != nil {
					return err
				}
			} else {
				// tenshon's absent-target fallback occurs after its binding guard.
				c.TensionCalls++
				if err := s.statTensionAfterBinding(&c.EffectContext, s.actor(c.ActorID), false, rng); err != nil {
					return err
				}
			}
		case "tool-76", "tool-79":
			stat, power := StatAttack, 1.0
			if id == "tool-79" {
				stat, power = StatDefense, .3
			}
			r, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.toolTarget(), power, CategoryTool), stat, rng)
			if err != nil {
				return err
			}
			if id == "tool-79" && r.Returned {
				s.actor(r.TargetID).Temporary = TemporaryReduceBreath
			}
		case "tool-87":
			if err := s.resetToolMembers(c, rng); err != nil {
				return err
			}
		case "tool-130":
		case "tool-151":
			for i, target := range c.EnemyIDs {
				if i == 10 {
					break
				}
				result.Facts = append(result.Facts, s.inspectMember(c, target, false, rng))
			}
		case "tool-213":
			r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.toolTarget(), "capture", CategoryTool), rng)
			if err != nil {
				return err
			}
			resources(r)
			if r.Resolved {
				t := s.actor(r.TargetID)
				if *c.RequesterMaxHP > t.HP || float64(t.MaxHP)*.2 > float64(t.HP) {
					t.Status = "capture"
				}
			}
		}
	}
	c.Results = append(c.Results, result)
	return nil
}

func (s *LifetimeState) resetToolMembers(c *OrdinaryToolContext, rng random.Generator) error {
	if err := s.ReviveOnAdvance(c.PartyIDs, rng); err != nil {
		return err
	}
	reset := *c.EquipmentReset
	reset.TargetContext, reset.Cursor = c.TargetContext, c.Cursor
	for _, member := range s.Actors {
		if err := s.ApplyEquipmentReset(&reset, member.ID, rng); err != nil {
			return err
		}
		a := s.actor(member.ID)
		if hasItem(a.ItemDefinitionIDs, "item-188") && a.HP > 0 {
			a.Temporary = TemporaryRevival
		}
	}
	c.Cursor = reset.Cursor
	*c.EquipmentReset = reset
	for i := range s.Actors {
		a := &s.Actors[i]
		a.HP, a.MP = a.MaxHP, a.MaxMP
	}
	return nil
}

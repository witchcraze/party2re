package battle

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

// Finite source tables: ff97fc5:_skill.cgi:10196–10201,10284–10291. These are
// distinct from the ordinary tool catalog and from the scavenging pool.
var (
	enhancedToolNumbers  = []int{1, 2, 3, 4, 5, 6, 9, 11, 12, 13, 14, 15, 18, 19, 20, 41, 42, 78, 80, 81, 84, 87}
	enhancedPurchasePool = []int{1, 1, 1, 1, 1, 2, 9, 11, 14, 14, 15, 15, 41, 80, 80, 81, 84, 84}
)

// Source helpers accept any true value here; the symbol only marks "not numeric".
const enhancedToolSymbol = "enhanced-tool"

func enhancedToolNumber(definitionID string) (int, bool) {
	kind, number, err := item.Definition{ID: definitionID}.LegacyStorageKey()
	return number, err == nil && kind == 3 && slices.Contains(enhancedToolNumbers, number)
}

func enhancedToolID(number int) string { return fmt.Sprintf("item-%03d", number) }

// enhancedPurchaseCost is floor(base price×2.5).
func enhancedPurchaseCost(number int) (int, error) {
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return 0, err
	}
	d, err := catalog.FindByID(enhancedToolID(number))
	if err != nil {
		return 0, ErrUnsupportedEffect
	}
	return d.Price * 5 / 2, nil
}

// numericMember binds the first shift, which reads the held ID rather than the
// submitted target. Only a participant actually named that number is retained;
// otherwise the target is absent and each helper applies its own fallback.
func (s *LifetimeState) numericMember(number int) string {
	name := strconv.Itoa(number)
	for _, a := range s.Actors {
		if a.Name == name {
			return a.ID
		}
	}
	return ""
}

// applyEnhancedTool leaves the outer arguments untouched: dispatch forwards only
// the numeric ID. Consumption follows the body's return value, never its effect.
func (s *LifetimeState) applyEnhancedTool(c *JobAssetContext, rng random.Generator) error {
	held := *s.actor(c.ActorID).HeldToolID
	if held == "" {
		return s.purchaseEnhancedTool(c, rng)
	}
	number, ok := enhancedToolNumber(held)
	if !ok {
		return nil
	}
	consumed, err := s.useEnhancedTool(c, number, rng)
	if err != nil || !consumed {
		return err
	}
	a := s.actor(c.ActorID) // Summons can reallocate Actors.
	*a.HeldToolID = ""
	if !a.NPC {
		*c.Assets.HeldToolID = ""
	}
	return nil
}

// purchaseEnhancedTool draws before checking money, without a redraw. The
// requester pays even when an NPC actor receives the tool.
func (s *LifetimeState) purchaseEnhancedTool(c *JobAssetContext, rng random.Generator) error {
	pool := slices.DeleteFunc(slices.Clone(enhancedPurchasePool), func(number int) bool {
		return slices.Contains(c.Assets.HelperToolIDs, number)
	})
	if len(pool) == 0 {
		return nil
	}
	selected := pool[int(rng.Float64()*float64(len(pool)))]
	cost, err := enhancedPurchaseCost(selected)
	if err != nil || c.Assets.Wallet < cost {
		return err
	}
	a := s.actor(c.ActorID)
	*a.HeldToolID = enhancedToolID(selected)
	if !a.NPC {
		*c.Assets.HeldToolID = *a.HeldToolID
	}
	c.Assets.Wallet -= cost
	return nil
}

func (s *LifetimeState) useEnhancedTool(c *JobAssetContext, number int, rng random.Generator) (bool, error) {
	argument := s.numericMember(number)
	self := func(power float64) TargetEffect { return controlEffect(argument, power, CategoryUntyped) }
	party := func(symbol string) (TargetResolution, error) {
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(argument, symbol, CategoryUntyped), rng)
		if err == nil {
			c.EffectContext.targetResources(r)
		}
		return r, err
	}
	enemy := func() (TargetResolution, error) {
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(argument, enhancedToolSymbol, CategoryUntyped), rng)
		if err == nil {
			c.EffectContext.targetResources(r)
		}
		return r, err
	}
	var err error
	switch number {
	case 1, 2, 3:
		_, err = s.ApplyHealing(&c.EffectContext, self(enhancedToolPower(number)), rng)
	case 4:
		_, err = s.ApplyPartyHealing(&c.EffectContext, 400, CategoryUntyped, rng)
	case 5:
		if _, err = s.ApplyPartyHealing(&c.EffectContext, 999, CategoryUntyped, rng); err == nil {
			for _, id := range c.PartyIDs {
				if _, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(id, 100, CategoryUntyped), rng); err != nil {
					break
				}
			}
		}
	case 6:
		r, err := party("revival")
		if err != nil || !r.Resolved || s.actor(r.TargetID).HP > 0 {
			return false, err // The only body that returns without consumption.
		}
		t := s.actor(r.TargetID)
		t.HP, t.Temporary = t.MaxHP, TemporaryRevival
	case 9:
		for _, id := range c.PartyIDs {
			t := s.actor(id)
			if t.HP <= 0 && rng.Float64()*2 < 1 {
				t.HP = int(float64(t.MaxHP) * .5)
			}
		}
	case 11, 12:
		_, err = s.ApplyMPHealing(&c.EffectContext, self(enhancedToolPower(number)), rng)
	case 13:
		var r TargetResolution
		if r, err = party(enhancedToolSymbol); err == nil {
			if _, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(r.TargetID, 999, CategoryUntyped), rng); err == nil {
				err = s.enhancedReset(c, r.TargetID, rng)
			}
		}
	case 14:
		var r TargetResolution
		if r, err = party(enhancedToolSymbol); err == nil {
			s.actor(r.TargetID).Temporary = TemporaryStrongDefense
		}
	case 15:
		var r TargetResolution
		if r, err = enemy(); err == nil {
			// An unresolved first check leaves undefined power, but the second
			// helper still runs its own target checks.
			target, power := "", 0.0
			if r.Resolved {
				target, power = r.TargetID, float64(s.actor(r.TargetID).Attack)
			}
			_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, power, CategoryUntyped, true), rng)
		}
	case 18, 19, 20:
		_, err = s.ApplyStatUp(&c.EffectContext, self(1), enhancedToolStat(number), rng)
	case 41:
		_, err = s.ApplyAreaDamage(&c.DamageContext, 400, CategoryUntyped, true, rng)
	case 42:
		status := StatusContext{EffectContext: c.EffectContext, Defeat: c.DefeatContext}
		_, err = s.ApplyAreaStatus(&status, controlSymbol("", "instant_death", CategoryUntyped), 40, false, true, rng)
		c.EffectContext = status.EffectContext
	case 78:
		roster := *c.Roster
		roster.TargetContext = c.TargetContext
		if _, err = s.summonEnemy(&roster, "tool-78", rng); err == nil && rng.Float64()*2 < 1 {
			_, err = s.summonEnemy(&roster, "tool-78", rng)
		}
		c.TargetContext = roster.TargetContext
	case 80:
		for _, id := range c.PartyIDs {
			s.actor(id).Temporary = TemporaryDefense
		}
	case 81:
		var r TargetResolution
		if r, err = enemy(); err == nil && r.Resolved {
			s.actor(r.TargetID).Temporary = ""
		}
	case 84:
		// The second shift is absent, so the checked target does not bind the theft.
		if _, err = enemy(); err == nil {
			var r EffectApplication
			r, err = s.ApplyStatDown(&c.EffectContext, controlEffect("", .25, CategoryUntyped), StatMP, rng)
			if err == nil && r.Returned {
				_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryPhysical), rng)
			}
		}
	case 87:
		target := argument
		if target == "" {
			target = s.randomMember(rng)
		}
		err = s.enhancedReset(c, target, rng)
	}
	return err == nil, err
}

func (s *LifetimeState) enhancedReset(c *JobAssetContext, target string, rng random.Generator) error {
	reset := &c.DefeatContext.EquipmentResetContext
	reset.TargetContext = c.TargetContext
	return s.ApplyEquipmentReset(reset, target, rng)
}

func enhancedToolPower(number int) float64 {
	switch number {
	case 1, 11:
		return 120
	case 2, 12:
		return 300
	}
	return 750
}

func enhancedToolStat(number int) BattleStat {
	switch number {
	case 18:
		return StatAttack
	case 19:
		return StatDefense
	}
	return StatAgility
}

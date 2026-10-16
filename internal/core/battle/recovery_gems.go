package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

type RecoveryStatGemEntry struct {
	ID, Name          string
	SourceID, CMPCost int
	TargetScope       string
	Category          CallbackCategory
}

// RecoveryStatGemEntries is the finite native subset at _data.cgi:2107–2184.
// Commerce/learning catalogs and batch Power payloads do not supply these bodies.
func RecoveryStatGemEntries() []RecoveryStatGemEntry {
	return []RecoveryStatGemEntry{
		{"gem_heal_1", "ally-heal-100", 19, 3, TargetScopeSingleAlly, CategoryMagic},
		{"gem_heal_sky_1", "self-heal-300", 20, 10, TargetScopeSelf, CategoryUntyped},
		{"gem_heal_sky_2", "ally-heal-300", 21, 16, TargetScopeSingleAlly, CategoryMagic},
		{"gem_all_heal_1", "party-heal-100", 22, 12, TargetScopeAllAllies, CategoryMagic},
		{"gem_all_heal_sky", "party-heal-300", 23, 22, TargetScopeAllAllies, CategoryMagic},
		{"gem_buff_atk_1", "self-attack-up", 28, 16, TargetScopeSelf, CategoryPhysical},
		{"gem_buff_atk_2", "ally-attack-up", 29, 28, TargetScopeSingleAlly, CategoryMagic},
		{"gem_buff_def_1", "self-defense-up", 30, 12, TargetScopeSelf, CategoryPhysical},
		{"gem_buff_def_2", "ally-defense-up", 31, 18, TargetScopeSingleAlly, CategoryMagic},
		{"gem_buff_spd_1", "self-agility-up", 32, 8, TargetScopeSelf, CategoryPhysical},
		{"gem_buff_spd_2", "ally-agility-up", 33, 13, TargetScopeSingleAlly, CategoryMagic},
		{"gem_mind_eye", "party-accuracy-reset", 54, 1, TargetScopeAllAllies, CategoryUntyped},
		{"gem_divine_tension_sky", "self-tension-twice", 67, 57, TargetScopeSelf, CategoryUntyped},
		{"gem_calm_orb", "enemy-tension-reset", 68, 20, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_calm_sky", "all-tension-reset", 69, 37, "all_members", CategoryUntyped},
		{"gem_inspire_sky", "ally-tension", 74, 11, TargetScopeSingleAlly, CategoryUntyped},
		{"gem_debuff_atk", "enemies-attack-down", 93, 12, TargetScopeAllEnemies, CategoryMagic},
		{"gem_debuff_def", "enemies-defense-down", 94, 9, TargetScopeAllEnemies, CategoryMagic},
		{"gem_debuff_spd", "enemies-agility-down", 95, 7, TargetScopeAllEnemies, CategoryMagic},
		{"gem_debuff_hit", "enemies-accuracy-down", 96, 12, TargetScopeAllEnemies, CategoryMagic},
	}
}

func (s *LifetimeState) gemTensionCeiling(c *EffectContext, t *LifetimeActor, calls int) (float64, error) {
	if t.Status == StatusSabaku {
		return t.Tension, nil
	}
	if hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-145") && t.ID == c.ActorID && !c.ActorNPC {
		calls *= 2
	}
	tension := t.Tension
	for range calls {
		if tension > 3 && tension < 8 && (c.RequesterLevel == nil || *c.RequesterLevel < 0) {
			return tension, ErrUnsupportedEffect
		}
		switch {
		case tension <= 1:
			tension = 1.7
		case tension <= 2:
			tension = 3
		case tension <= 3:
			tension = 5
		case tension < 8 && c.RequesterLevel != nil && *c.RequesterLevel >= 25:
			tension = 8
		}
	}
	return tension, nil
}

// ValidateRecoveryStatGems projects only reachable transitions, without draws,
// cost, callback execution or writes to the submitted state/context. Item201's
// admitted executor may reset arbitrary state; its later continuation requires
// that owner's transition projection and stays rejected here.
func (s *LifetimeState) ValidateRecoveryStatGems(ids []int, c *EffectContext, targetID string, rng random.Generator) error {
	if s == nil || c == nil || len(ids) == 0 {
		return ErrInvalidRequest
	}
	if len(ids) > 3 {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	context := *c
	// A category cure can fail in every slot; retain that branch for consumption.
	originalActor := s.actor(c.ActorID)
	minimumTension := make(map[string]float64, len(s.Actors))
	for _, a := range s.Actors {
		minimumTension[a.ID] = a.Tension
	}
	admission := statUpAdmission{mayReplace: make(map[string]float64)}
	for i, id := range ids {
		e := TargetEffect{TargetID: targetID, Category: CategoryMagic, Payload: CallbackPayload{Number: 1}}
		stat := StatAttack
		switch id {
		case 19, 20, 21, 22, 23:
			e.Payload.Number = 300
			if id == 19 || id == 22 {
				e.Payload.Number = 100
			}
			if id == 20 {
				e.TargetID, e.Category = c.ActorID, CategoryUntyped
			}
			targets := []string{e.TargetID}
			if id == 22 || id == 23 {
				targets = c.PartyIDs
				if projected.actor(c.ActorID).Extended == ExtendedAdditional {
					e.Payload.Number *= 1.5
				}
			}
			consumes, mayConsume := !originalActor.categorySealed(e.Category), false
			for _, target := range targets {
				e.TargetID = target
				if err := projected.validatePartyTarget(context.TargetContext, e, rng); err != nil {
					return err
				}
				for _, candidate := range projected.partyTargetCandidates(context.TargetContext, e, admission.mayReplace) {
					power := candidate
					power.Payload.Number = projected.healingBasePower(&context, candidate)
					value := projected.finalEffectPower(&context, projected.initialEffectPower(&context, power, ModifierHPHealing))
					if projected.actor(c.ActorID).Status == StatusSabaku {
						value *= .7
					}
					if projected.actor(candidate.TargetID).Status == StatusVirulentPoison {
						value = float64(int(value * .1))
					}
					if projected.actor(candidate.TargetID).HP <= 0 || candidate.Payload.Number <= 0 || value == 0 {
						consumes = false
					} else {
						mayConsume = true
					}
				}
			}
			if mayConsume {
				minimumTension[c.ActorID] = min(minimumTension[c.ActorID], 1)
			}
			if consumes {
				projected.actor(c.ActorID).Tension = 1
				minimumTension[c.ActorID] = 1
				admission.powerMayChange = false
			}
		case 28, 29, 30, 31, 32, 33:
			if id >= 32 {
				stat = StatAgility
			} else if id >= 30 {
				stat = StatDefense
			}
			if id%2 == 0 {
				e.TargetID, e.Category = c.ActorID, CategoryPhysical
			}
			if err := projected.validateStatUp(&context, e, stat, rng, admission); err != nil {
				return err
			}
			consumes, self205 := !originalActor.categorySealed(e.Category), false
			mayConsume := false
			for _, candidate := range projected.partyTargetCandidates(context.TargetContext, e, admission.mayReplace) {
				t := projected.actor(candidate.TargetID)
				power := projected.finalEffectPower(&context, projected.initialEffectPower(&context, candidate, ModifierStatUp))
				mayConsume = mayConsume || projected.statUpTailPossible(&context, candidate, stat, admission.powerMayChange)
				if !projected.statUpTailPossible(&context, candidate, stat, false) || int(float64(t.baseStat(stat))*power*min(1, projected.actor(c.ActorID).Tension)) == 0 {
					consumes = false
				}
				self205 = self205 || (stat != StatAttack && t.ID == c.ActorID && hasItem(t.ItemDefinitionIDs, "item-205"))
				if stat == StatAgility && hasItem(t.ItemDefinitionIDs, "item-201") && projected.statUpTailPossible(&context, candidate, stat, admission.powerMayChange) && i+1 < len(ids) {
					return ErrUnsupportedEffect
				}
			}
			projected.recordStatUp205(&context, e, stat, &admission)
			if mayConsume {
				minimumTension[c.ActorID] = min(minimumTension[c.ActorID], 1)
			}
			if consumes {
				projected.actor(c.ActorID).Tension = 1
				minimumTension[c.ActorID] = 1
				if self205 {
					projected.actor(c.ActorID).Tension = 3 // Upper bound after205 and optional145 recursion.
				}
				admission.powerMayChange = false
			}
		case 54:
			continue
		case 69:
			for i := range projected.Actors {
				projected.Actors[i].Tension = 1
				minimumTension[projected.Actors[i].ID] = 1
			}
			admission.powerMayChange = false
			continue
		case 67, 74:
			targets := []TargetEffect{{TargetID: c.ActorID}}
			calls := 2
			if id == 74 {
				calls, e.Category, e.Payload = 1, CategoryUntyped, CallbackPayload{Symbol: "encourage"}
				if err := projected.validatePartyTarget(context.TargetContext, e, rng); err != nil {
					return err
				}
				targets = projected.partyTargetCandidates(context.TargetContext, e, admission.mayReplace)
			}
			for _, target := range targets {
				t := projected.actor(target.TargetID)
				lower := *t
				lower.Tension = minimumTension[t.ID]
				if _, err := projected.gemTensionCeiling(&context, &lower, calls); err != nil {
					return err
				}
				ceiling, err := projected.gemTensionCeiling(&context, t, calls)
				if err != nil {
					return err
				}
				t.Tension = ceiling
				// Base stages always run;145 recursion and the high-stage roll may not.
				for range calls {
					if err := lower.AdvanceTension(0, nil); err != nil {
						return err
					}
				}
				if len(targets) == 1 {
					minimumTension[t.ID] = lower.Tension
				}
				if t.Status != StatusSabaku {
					//161 can replace covering/reflection/null with a defense.
					if hasItem(t.ItemDefinitionIDs, "item-161") && t.Temporary != TemporaryDefense && t.Temporary != TemporaryStrongDefense && t.Temporary != TemporaryOneHitDefense {
						admission.mayReplace[t.ID] = max(admission.mayReplace[t.ID], .5)
					}
				}
			}
			if id == 67 {
				continue // Direct tension invokes no category/target helper.
			}
		case 68, 93, 94, 95, 96:
			e.Payload.Number = .25
			if id == 96 {
				e.Payload.Number = .2
			}
			if id == 68 {
				e.Category, e.Payload = CategoryPhysical, CallbackPayload{Symbol: "destroy"}
			}
			converted, _, err := projected.actor(c.ActorID).ConvertEnemyCategory(context.TargetContext, e)
			if err != nil {
				return err
			}
			if id == 68 && len(c.EnemyIDs) > 0 {
				candidates := projected.damageCandidatesAfter(&DamageContext{EffectContext: context}, converted, admission.mayReplace)
				selfOnly := len(candidates) > 0
				for _, candidate := range candidates {
					selfOnly = selfOnly && candidate.TargetID == c.ActorID
					minimumTension[candidate.TargetID] = min(minimumTension[candidate.TargetID], 1)
				}
				if selfOnly && !originalActor.categorySealed(converted.Category) {
					projected.actor(c.ActorID).Tension = 1
					minimumTension[c.ActorID] = 1
					admission.powerMayChange = false
				}
			}
			if id >= 93 {
				if id == 94 {
					stat = StatDefense
				} else if id == 95 {
					stat = StatAgility
				} else if id == 96 {
					stat = StatHit
				}
				consumes := false
				for _, enemy := range c.EnemyIDs {
					converted.TargetID = enemy
					candidates := projected.damageCandidatesAfter(&DamageContext{EffectContext: context}, converted, admission.mayReplace)
					invocationConsumes := len(candidates) > 0
					for _, candidate := range candidates {
						t := projected.actor(candidate.TargetID)
						power := projected.finalEffectPower(&context, projected.initialEffectPower(&context, candidate, ModifierStatDown))
						eligible := power > 0 && t.HP > 0 && t.BaseStats.Defense <= 999 && !hasItem(t.ItemDefinitionIDs, "item-159") && (stat != StatAttack || t.HP <= 999) && float64(*t.currentStat(stat)) > float64(t.baseStat(stat))*.2 && (stat != StatHit || (*t.currentStat(stat) > 50 && !hasItem(t.ItemDefinitionIDs, "item-197")))
						invocationConsumes = invocationConsumes && eligible
						if eligible {
							minimumTension[c.ActorID] = min(minimumTension[c.ActorID], 1)
						}
						// A reduction can reopen a later stat-up cap guard.
						if eligible && (admission.powerMayChange || int(float64(*t.currentStat(stat))*power*projected.actor(c.ActorID).Tension) > 0) {
							*t.currentStat(stat) = min(*t.currentStat(stat), int(float64(t.baseStat(stat))*.2))
						}
					}
					consumes = consumes || invocationConsumes
				}
				if consumes && !originalActor.categorySealed(e.Category) {
					projected.actor(c.ActorID).Tension = 1
					minimumTension[c.ActorID] = 1
					admission.powerMayChange = false
				}
			}
		default:
			return ErrUnsupportedEffect
		}
		a := projected.actor(c.ActorID)
		if a.categorySealed(e.Category) {
			a.Status = ""
		}
		if context.Item195Activated && a.Status == "" {
			a.Status = StatusConfusion
		}
	}
	return nil
}

// ApplyRecoveryStatGems shares discovery's whole-slot gate. Returned facts are
// already applied; a consumer persists them with frozen input, never replays
// them to restore state or retries a committed callback.
func (s *LifetimeState) ApplyRecoveryStatGems(ids []int, c *EffectContext, targetID string, rng random.Generator) ([][]EffectApplication, error) {
	if err := s.ValidateRecoveryStatGems(ids, c, targetID, rng); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	var results [][]EffectApplication
	for _, id := range ids {
		r, err := s.ApplyRecoveryStatGem(id, c, targetID, rng)
		if err != nil {
			return results, err
		}
		results = append(results, r)
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
	}
	return results, nil
}

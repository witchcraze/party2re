package battle

import "github.com/witchcraze/party2re/internal/core/random"

type DefenseGemEntry struct {
	ID, Name          string
	SourceID, CMPCost int
	TargetScope       string
	Category          CallbackCategory
	temporary         TemporaryEffect
}

// DefenseGemEntries certifies only _data.cgi:2122–2141,2172–2173 bodies.
// CMP contributions are metadata; enclosing costs remain consumer-owned.
func DefenseGemEntries() []DefenseGemEntry {
	return []DefenseGemEntry{
		{"gem_guard_1", "self-defense", 34, 20, TargetScopeSelf, CategoryUntyped, TemporaryDefense},
		{"gem_guard_2", "ally-defense", 35, 33, TargetScopeSingleAlly, CategoryMagic, TemporaryDefense},
		{"gem_guard_sky", "party-defense", 36, 40, TargetScopeAllAllies, CategoryMagic, TemporaryDefense},
		{"gem_big_guard_sky", "self-strong-defense", 37, 37, TargetScopeSelf, CategoryUntyped, TemporaryStrongDefense},
		{"gem_mitigate_phys", "party-physical-reduction", 38, 24, TargetScopeAllAllies, CategoryMagic, TemporaryReduceAttack},
		{"gem_mitigate_magic", "party-magic-reduction", 39, 12, TargetScopeAllAllies, CategoryMagic, TemporaryReduceMagic},
		{"gem_mitigate_breath", "party-breath-reduction", 40, 7, TargetScopeAllAllies, CategoryMagic, TemporaryReduceBreath},
		{"gem_nullify_phys", "ally-physical-null", 41, 24, TargetScopeSingleAlly, CategoryMagic, TemporaryNullAttack},
		{"gem_nullify_magic_1", "ally-magic-null", 42, 10, TargetScopeSingleAlly, CategoryMagic, TemporaryNullMagic},
		{"gem_nullify_magic_2", "party-magic-null", 43, 19, TargetScopeAllAllies, CategoryMagic, TemporaryNullMagic},
		{"gem_counter_phys_1", "self-physical-reflection", 44, 30, TargetScopeSelf, CategoryMagic, TemporaryReflectAttack},
		{"gem_counter_phys_sky", "ally-physical-reflection", 45, 35, TargetScopeSingleAlly, CategoryMagic, TemporaryReflectAttack},
		{"gem_counter_magic_1", "self-magic-reflection", 46, 8, TargetScopeSelf, CategoryMagic, TemporaryReflectMagic},
		{"gem_counter_magic_2", "ally-magic-reflection", 47, 17, TargetScopeSingleAlly, CategoryMagic, TemporaryReflectMagic},
		{"gem_counter_magic_sky", "party-magic-reflection", 48, 28, TargetScopeAllAllies, CategoryMagic, TemporaryReflectMagic},
		{"gem_counter_breath", "party-breath-reflection", 49, 14, TargetScopeAllAllies, CategoryMagic, TemporaryReflectBreath},
		{"gem_regen_1", "ally-regeneration", 50, 16, TargetScopeSingleAlly, CategoryMagic, TemporaryRegeneration},
		{"gem_auto_revive_sky", "ally-revival-protection", 51, 36, TargetScopeSingleAlly, CategoryMagic, TemporaryRevival},
		{"gem_cover", "other-ally-cover", 52, 2, TargetScopeSingleAlly, CategoryPhysical, TemporaryCover},
		{"gem_magic_absorb_sky", "self-magic-absorption", 53, 40, TargetScopeSelf, CategoryMagic, TemporaryAbsorbMagic},
		{"gem_awakening_1", "self-awakening-eight", 84, 6, TargetScopeSelf, CategoryUntyped, ""},
		{"gem_awakening_sky", "ally-awakening-eight", 85, 13, TargetScopeSingleAlly, CategoryUntyped, ""},
	}
}

func defenseGemEntry(id int) (DefenseGemEntry, error) {
	for _, entry := range DefenseGemEntries() {
		if entry.SourceID == id {
			return entry, nil
		}
	}
	return DefenseGemEntry{}, ErrUnsupportedEffect
}

func (e DefenseGemEntry) target(id string) TargetEffect {
	symbol := "temporary:" + string(e.temporary)
	if e.SourceID == 85 {
		symbol = "awakening"
	}
	return TargetEffect{TargetID: id, Category: e.Category, Payload: CallbackPayload{Symbol: symbol}}
}

func (e DefenseGemEntry) write(s *LifetimeState, c TargetContext, targetID string) bool {
	if e.SourceID == 52 {
		if targetID == c.ActorID {
			return false
		}
		s.actor(c.ActorID).Temporary = TemporaryCovering
	}
	t := s.actor(targetID)
	if e.SourceID == 84 || e.SourceID == 85 {
		t.Extended, t.ExtendedTurns = ExtendedAwakening, 8
	} else {
		t.Temporary = e.temporary
	}
	return true
}

// ApplyDefenseGems preflights every occupied slot before any write or draw.
// Results are already applied facts, including incidental target MP recovery.
// Persist them with frozen input; restoring committed state never runs callbacks.
func (s *LifetimeState) ApplyDefenseGems(ids []int, c TargetContext, targetID string, rng random.Generator) ([]TemporaryApplication, error) {
	if err := s.ValidateDefenseGems(ids, c, targetID, rng); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	var results []TemporaryApplication
	for _, id := range ids {
		var r TemporaryApplication
		e, err := defenseGemEntry(id)
		if err != nil {
			return results, err
		}
		if id == 84 || id == 85 {
			target := c.ActorID
			if id == 85 {
				var resolved TargetResolution
				resolved, err = s.ResolvePartyTarget(c, e.target(targetID), rng)
				target = resolved.TargetID
			}
			if err == nil {
				r.Applied = e.write(s, c, target)
			}
		} else {
			r, err = s.ApplyTemporaryGem(id, c, targetID, rng)
		}
		if err != nil {
			return results, err
		}
		if r.MPRecoveryTarget != "" {
			t := s.actor(r.MPRecoveryTarget)
			t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
		}
		results = append(results, r)
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
	}
	return results, nil
}

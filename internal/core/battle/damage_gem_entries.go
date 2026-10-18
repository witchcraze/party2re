package battle

type DamageGemEntry struct {
	ID, Name          string
	SourceID, CMPCost int
	TargetScope       string
	Category          CallbackCategory
}

// DamageGemEntries binds existing native IDs to concrete source callbacks.
// CMP contributions are metadata, never an MP debit or a scalar batch effect.
func DamageGemEntries() []DamageGemEntry {
	return []DamageGemEntry{
		{"gem_atk_1", "physical-strike-140", 1, 4, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_atk_2", "physical-strike-180", 2, 10, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_sky_atk_1", "physical-strike-with-recoil", 3, 29, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_sky_atk_2", "physical-strike-250", 4, 40, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_burst_sky", "direct-physical-strike", 5, 46, TargetScopeSingleEnemy, CategoryPhysical},
		{"gem_combo_1", "physical-combo-3-4", 6, 22, "random_enemies", CategoryPhysical},
		{"gem_combo_sky", "physical-combo-7-8", 7, 31, "random_enemies", CategoryPhysical},
		{"gem_combo_burst_sky", "direct-physical-combo-2-3", 8, 46, "random_enemies", CategoryPhysical},
		{"gem_dance_combo", "dance-combo-2-4", 9, 27, "random_enemies", CategoryDance},
		{"gem_all_atk_1", "area-physical-80", 10, 7, TargetScopeAllEnemies, CategoryPhysical},
		{"gem_all_atk_2", "area-physical-100", 11, 16, TargetScopeAllEnemies, CategoryPhysical},
		{"gem_all_atk_sky", "area-physical-160", 12, 21, TargetScopeAllEnemies, CategoryPhysical},
		{"gem_magic_atk_1", "magic-strike-50", 13, 5, TargetScopeSingleEnemy, CategoryMagic},
		{"gem_magic_atk_2", "magic-strike-200", 14, 19, TargetScopeSingleEnemy, CategoryMagic},
		{"gem_magic_atk_sky", "magic-strike-500", 15, 26, TargetScopeSingleEnemy, CategoryMagic},
		{"gem_all_magic_1", "area-magic-50", 16, 14, TargetScopeAllEnemies, CategoryMagic},
		{"gem_all_magic_sky", "area-magic-200", 17, 25, TargetScopeAllEnemies, CategoryMagic},
		{"gem_all_breath_1", "area-breath-150", 18, 28, TargetScopeAllEnemies, CategoryBreath},
		{"gem_drain_hp_sky", "requested-hp-drain", 26, 26, TargetScopeSingleEnemy, CategoryMagic},
		{"gem_kamikaze_sky", "hp-sacrifice-strike", 72, 9, TargetScopeSingleEnemy, CategoryMagic},
		{"gem_crisis_strike", "missing-hp-strike", 73, 22, TargetScopeSingleEnemy, CategoryPhysical},
	}
}

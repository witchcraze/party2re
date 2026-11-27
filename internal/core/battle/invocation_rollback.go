package battle

import "maps"

type stateSnapshot struct {
	state             LifetimeState
	origActors        []LifetimeActor
	actorPtrs         []actorPtrSnapshot
	origSkillBans     map[string]int
	origSkillHistory  map[string]map[string]bool
	origSkillInners   map[string]map[string]bool
	origStoredHistory map[string]map[string]bool
	origStoredInners  map[string]map[string]bool
}

type actorPtrSnapshot struct {
	held         *string
	gear         *BattleGearStats
	weaponAttack *int
	custom       *CustomLoadout
	npcLoadout   *NPCLoadout
	jobHistory   map[string]*int
	jobSP        map[string]*int
	abilities    []string
	items        []string
	consumed     []ConsumedItem
	skills       []ActionSkill
	actionItems  []ActionItem
	customSkills []ActionCustomSkill
	gems         [][]GemEffect
}

func snapshotState(s *LifetimeState) stateSnapshot {
	ptrs := make([]actorPtrSnapshot, len(s.Actors))
	for i := range s.Actors {
		a := &s.Actors[i]
		var weaponAttack *int
		if a.Gear != nil {
			weaponAttack = a.Gear.WeaponAttack
		}
		ptrs[i] = actorPtrSnapshot{
			held:         a.HeldToolID,
			gear:         a.Gear,
			weaponAttack: weaponAttack,
			custom:       a.Custom,
			npcLoadout:   a.NPCLoadout,
			jobHistory:   a.JobHistory,
			jobSP:        maps.Clone(a.JobHistory),
			abilities:    a.Abilities,
			items:        a.ItemDefinitionIDs,
			consumed:     a.ConsumedItems,
			skills:       a.Skills,
			actionItems:  a.ActionItems,
			customSkills: a.CustomSkills,
		}
		for _, custom := range a.CustomSkills {
			ptrs[i].gems = append(ptrs[i].gems, custom.Gems)
		}
	}
	return stateSnapshot{
		state:             s.executionCopy(),
		origActors:        s.Actors,
		actorPtrs:         ptrs,
		origSkillBans:     s.SkillBans,
		origSkillHistory:  s.SkillHistory,
		origSkillInners:   maps.Clone(s.SkillHistory),
		origStoredHistory: s.StoredToolHistory,
		origStoredInners:  maps.Clone(s.StoredToolHistory),
	}
}

// restoreSlice writes saved values back into the caller's original backing
// array; saved was cloned from it, so both have the same length.
func restoreSlice[T any](orig, saved []T) []T {
	if orig == nil {
		return saved
	}
	copy(orig, saved)
	return orig
}

func restoreNestedBoolMaps(origOuter, savedOuter, origInners map[string]map[string]bool) map[string]map[string]bool {
	if origOuter == nil {
		return savedOuter
	}
	for k, origInner := range origInners {
		if origInner != nil {
			clear(origInner)
			maps.Copy(origInner, savedOuter[k])
		}
	}
	clear(origOuter)
	for k, savedInner := range savedOuter {
		if origInner := origInners[k]; origInner != nil {
			origOuter[k] = origInner
		} else {
			origOuter[k] = savedInner
		}
	}
	return origOuter
}

func (snap stateSnapshot) restore(s *LifetimeState) {
	saved := snap.state
	for i := range saved.Actors {
		a := &saved.Actors[i]
		ptr := snap.actorPtrs[i]
		if ptr.held != nil && a.HeldToolID != nil {
			*ptr.held = *a.HeldToolID
			a.HeldToolID = ptr.held
		}
		if ptr.gear != nil && a.Gear != nil {
			if ptr.weaponAttack != nil && a.Gear.WeaponAttack != nil {
				*ptr.weaponAttack = *a.Gear.WeaponAttack
				a.Gear.WeaponAttack = ptr.weaponAttack
			}
			*ptr.gear = *a.Gear
			a.Gear = ptr.gear
		}
		if ptr.custom != nil && a.Custom != nil {
			*ptr.custom = *a.Custom
			a.Custom = ptr.custom
		}
		if ptr.npcLoadout != nil && a.NPCLoadout != nil {
			*ptr.npcLoadout = *a.NPCLoadout
			a.NPCLoadout = ptr.npcLoadout
		}
		for id, sp := range a.JobHistory {
			if orig := ptr.jobSP[id]; orig != nil && sp != nil {
				*orig = *sp
				a.JobHistory[id] = orig
			}
		}
		a.Abilities = restoreSlice(ptr.abilities, a.Abilities)
		a.ItemDefinitionIDs = restoreSlice(ptr.items, a.ItemDefinitionIDs)
		a.ConsumedItems = restoreSlice(ptr.consumed, a.ConsumedItems)
		a.Skills = restoreSlice(ptr.skills, a.Skills)
		a.ActionItems = restoreSlice(ptr.actionItems, a.ActionItems)
		for j := range a.CustomSkills {
			a.CustomSkills[j].Gems = restoreSlice(ptr.gems[j], a.CustomSkills[j].Gems)
		}
		a.CustomSkills = restoreSlice(ptr.customSkills, a.CustomSkills)
		if ptr.jobHistory != nil {
			clear(ptr.jobHistory)
			maps.Copy(ptr.jobHistory, a.JobHistory)
			a.JobHistory = ptr.jobHistory
		}
	}
	if len(snap.origActors) == len(saved.Actors) {
		copy(snap.origActors, saved.Actors)
		saved.Actors = snap.origActors
	}
	if snap.origSkillBans != nil {
		clear(snap.origSkillBans)
		maps.Copy(snap.origSkillBans, saved.SkillBans)
		saved.SkillBans = snap.origSkillBans
	}
	saved.SkillHistory = restoreNestedBoolMaps(snap.origSkillHistory, saved.SkillHistory, snap.origSkillInners)
	saved.StoredToolHistory = restoreNestedBoolMaps(snap.origStoredHistory, saved.StoredToolHistory, snap.origStoredInners)
	saved.rollbacks = s.rollbacks
	*s = saved
}

type contextSnapshot struct {
	ctx              CallbackContext
	origAssets       *JobAssets
	origAssetsHeld   *string
	origDamage       *DamageContext
	origRespDefeat   *DefeatContext
	origStatus       *StatusContext
	origStatusDefeat *DefeatContext
	origDefeat       *DefeatContext
	origInspection   map[string]InspectionContent
	origPartyIDs     []string
	origEnemyIDs     []string
	origMemberIDs    []string
	origPhantomIDs   []string
	origArguments    []string
}

func snapshotContext(c *CallbackContext) contextSnapshot {
	var origAssetsHeld *string
	if c.Assets != nil {
		origAssetsHeld = c.Assets.HeldToolID
	}
	var origRespDefeat, origStatusDefeat *DefeatContext
	if c.Damage != nil {
		origRespDefeat = c.Damage.responseDefeat
	}
	if c.Status != nil {
		origStatusDefeat = c.Status.Defeat
	}
	return contextSnapshot{
		ctx:              cloneCallbackContext(*c),
		origAssets:       c.Assets,
		origAssetsHeld:   origAssetsHeld,
		origDamage:       c.Damage,
		origRespDefeat:   origRespDefeat,
		origStatus:       c.Status,
		origStatusDefeat: origStatusDefeat,
		origDefeat:       c.Defeat,
		origInspection:   c.Inspection,
		origPartyIDs:     c.PartyIDs,
		origEnemyIDs:     c.EnemyIDs,
		origMemberIDs:    c.MemberOnlyIDs,
		origPhantomIDs:   c.PhantomEnemyIDs,
		origArguments:    c.Arguments,
	}
}

func (snap contextSnapshot) restorePointers() {
	saved := snap.ctx
	if snap.origAssets != nil && saved.Assets != nil {
		if snap.origAssetsHeld != nil && saved.Assets.HeldToolID != nil {
			*snap.origAssetsHeld = *saved.Assets.HeldToolID
			saved.Assets.HeldToolID = snap.origAssetsHeld
		}
		*snap.origAssets = *saved.Assets
	}
	if snap.origDefeat != nil && saved.Defeat != nil {
		*snap.origDefeat = *saved.Defeat
	}
	if snap.origStatusDefeat != nil && snap.origStatusDefeat != snap.origDefeat && saved.Status != nil && saved.Status.Defeat != nil {
		*snap.origStatusDefeat = *saved.Status.Defeat
	}
	if snap.origRespDefeat != nil && snap.origRespDefeat != snap.origDefeat && snap.origRespDefeat != snap.origStatusDefeat && saved.Damage != nil && saved.Damage.responseDefeat != nil {
		*snap.origRespDefeat = *saved.Damage.responseDefeat
	}
	if snap.origStatus != nil && saved.Status != nil {
		*snap.origStatus = *saved.Status
		snap.origStatus.Defeat = snap.origStatusDefeat
	}
	if snap.origDamage != nil && saved.Damage != nil {
		*snap.origDamage = *saved.Damage
		snap.origDamage.responseDefeat = snap.origRespDefeat
	}
}

func (snap contextSnapshot) restore(c *CallbackContext) {
	snap.restorePointers()
	saved := snap.ctx
	if snap.origAssets != nil {
		saved.Assets = snap.origAssets
	}
	if snap.origDefeat != nil {
		saved.Defeat = snap.origDefeat
	}
	if snap.origStatus != nil {
		saved.Status = snap.origStatus
	}
	if snap.origDamage != nil {
		saved.Damage = snap.origDamage
	}
	if snap.origInspection != nil {
		clear(snap.origInspection)
		maps.Copy(snap.origInspection, saved.Inspection)
		saved.Inspection = snap.origInspection
	}
	if len(snap.origPartyIDs) == len(saved.PartyIDs) {
		copy(snap.origPartyIDs, saved.PartyIDs)
		saved.PartyIDs = snap.origPartyIDs
	}
	if len(snap.origEnemyIDs) == len(saved.EnemyIDs) {
		copy(snap.origEnemyIDs, saved.EnemyIDs)
		saved.EnemyIDs = snap.origEnemyIDs
	}
	if len(snap.origMemberIDs) == len(saved.MemberOnlyIDs) {
		copy(snap.origMemberIDs, saved.MemberOnlyIDs)
		saved.MemberOnlyIDs = snap.origMemberIDs
	}
	if len(snap.origPhantomIDs) == len(saved.PhantomEnemyIDs) {
		copy(snap.origPhantomIDs, saved.PhantomEnemyIDs)
		saved.PhantomEnemyIDs = snap.origPhantomIDs
	}
	if len(snap.origArguments) == len(saved.Arguments) {
		copy(snap.origArguments, saved.Arguments)
		saved.Arguments = snap.origArguments
	}
	saved.rollbacks = c.rollbacks
	saved.Invoke = c.Invoke
	*c = saved
}

func runRollbacks(fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

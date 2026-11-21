package battle

import "maps"

type stateSnapshot struct {
	state             LifetimeState
	origActors        []LifetimeActor
	actorPtrs         []actorPtrSnapshot
	origSkillBans     map[string]int
	origSkillHistory  map[string]map[string]bool
	origStoredHistory map[string]map[string]bool
}

type actorPtrSnapshot struct {
	held         *string
	gear         *BattleGearStats
	weaponAttack *int
	custom       *CustomLoadout
	npcLoadout   *NPCLoadout
	jobHistory   map[string]*int
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
		}
	}
	return stateSnapshot{
		state:             s.executionCopy(),
		origActors:        s.Actors,
		actorPtrs:         ptrs,
		origSkillBans:     s.SkillBans,
		origSkillHistory:  s.SkillHistory,
		origStoredHistory: s.StoredToolHistory,
	}
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
	if snap.origSkillHistory != nil {
		clear(snap.origSkillHistory)
		maps.Copy(snap.origSkillHistory, saved.SkillHistory)
		saved.SkillHistory = snap.origSkillHistory
	}
	if snap.origStoredHistory != nil {
		clear(snap.origStoredHistory)
		maps.Copy(snap.origStoredHistory, saved.StoredToolHistory)
		saved.StoredToolHistory = snap.origStoredHistory
	}
	saved.rollbacks = s.rollbacks
	*s = saved
}

type contextSnapshot struct {
	ctx            CallbackContext
	origAssets     *JobAssets
	origAssetsHeld *string
	origDamage     *DamageContext
	origStatus     *StatusContext
	origDefeat     *DefeatContext
	origInspection map[string]InspectionContent
	origPartyIDs   []string
	origEnemyIDs   []string
	origMemberIDs  []string
	origPhantomIDs []string
	origArguments  []string
}

func snapshotContext(c *CallbackContext) contextSnapshot {
	var origAssetsHeld *string
	if c.Assets != nil {
		origAssetsHeld = c.Assets.HeldToolID
	}
	return contextSnapshot{
		ctx:            cloneCallbackContext(*c),
		origAssets:     c.Assets,
		origAssetsHeld: origAssetsHeld,
		origDamage:     c.Damage,
		origStatus:     c.Status,
		origDefeat:     c.Defeat,
		origInspection: c.Inspection,
		origPartyIDs:   c.PartyIDs,
		origEnemyIDs:   c.EnemyIDs,
		origMemberIDs:  c.MemberOnlyIDs,
		origPhantomIDs: c.PhantomEnemyIDs,
		origArguments:  c.Arguments,
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
	if snap.origStatus != nil && saved.Status != nil {
		*snap.origStatus = *saved.Status
	}
	if snap.origDamage != nil && saved.Damage != nil {
		*snap.origDamage = *saved.Damage
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
		if saved.Damage != nil && snap.origDefeat != nil && saved.Damage.responseDefeat != nil && saved.Damage.responseDefeat == saved.Defeat {
			saved.Damage.responseDefeat = snap.origDefeat
		}
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

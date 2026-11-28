package adventure

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NPCPayload supplies an isolated source template to either ordinary stat rolls
// or the shared summon owner. It does not certify any selected callback.
func (m Monster) NPCPayload() corebattle.LifetimeActor {
	a := corebattle.LifetimeActor{Jobs: corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}}
	if m.Combat != nil {
		a = m.Combat.Clone()
	}
	if a.Jobs.JobID == "" {
		a.Jobs.JobID = "starter"
	}
	if a.Jobs.OldJobID == "" {
		a.Jobs.OldJobID = "starter"
	}
	a.ID, a.Name, a.HP, a.MP, a.CMP = m.ID, m.Name, m.HP, m.MP, m.CMP
	a.EncounterIdentity = m.EncounterIdentity
	if a.EncounterIdentity == "" {
		a.EncounterIdentity = m.ID
	}
	a.Attack, a.Defense, a.Agility = m.Attack, m.Defense, m.Agility
	a.ExperienceReward, a.GoldReward = m.ExperienceReward, m.GoldReward
	a.NPC = true
	if a.Hit == 0 {
		a.Hit = 95
	}
	if a.Tension == 0 {
		a.Tension = 1
	}
	if a.Gear == nil {
		a.Gear = &corebattle.BattleGearStats{}
	}
	if a.Icon == "" {
		a.Icon = "assets/images/placeholders/character/character-default.svg"
	}
	if a.NPCLoadout != nil {
		a.NPCLoadout.CustomCost = corebattle.CustomGemCost(a.NPCLoadout.Gems, a.NPCLoadout.GemCosts)
	}
	a.OriginalJobs = a.Jobs
	return a
}

// buildMonsterActor rolls one ordinary encounter actor from its catalog
// template (_npc_action.cgi add_monster/add_boss). rngFloat must return [0,1).
func buildMonsterActor(id string, m Monster, partyAdjustment float64, isBoss bool, mode string, rngFloat func() float64) corebattle.LifetimeActor {
	a := m.NPCPayload()
	hp := m.HP
	mp := m.MP
	atk := m.Attack
	def := m.Defense
	ag := m.Agility
	cmp := m.CMP
	expReward := m.ExperienceReward
	goldReward := m.GoldReward

	// Hard mode applies 1.5x multiplier to normal monster stats & rewards (lib/_npc_action.cgi:649-653)
	if !isBoss && mode == CrawlModeHard {
		hp = int(float64(hp) * 1.5)
		mp = int(float64(mp) * 1.5)
		atk = int(float64(atk) * 1.5)
		def = int(float64(def) * 1.5)
		ag = int(float64(ag) * 1.5)
		cmp = int(float64(cmp) * 1.5)
		expReward = int(float64(expReward) * 1.5)
		goldReward = int(float64(goldReward) * 1.5)
		a.Jobs = corebattle.BattleJobs{JobID: a.Jobs.JobID, OldJobID: a.Jobs.OldJobID, SP: int(float64(a.Jobs.SP) * 1.5), OldSP: int(float64(a.Jobs.OldSP) * 1.5)}
		a.CrystalReward = int(float64(a.CrystalReward) * 1.5)
	}

	statFactor := func() float64 {
		r := rngFloat()
		if r < 0 {
			r = 0
		}
		if r >= 1 {
			r = 0.9999999999999999
		}
		return 0.9 + r*0.3 + partyAdjustment
	}

	// Legacy stat scaling order: hp, mp, at, df, ag, c_mp (lib/_npc_action.cgi:612, 657)
	scaledHP := max(1, int(float64(hp)*statFactor()))
	scaledMP := max(0, int(float64(mp)*statFactor()))
	scaledAtk := max(0, int(float64(atk)*statFactor()))
	scaledDef := max(0, int(float64(def)*statFactor()))
	scaledAg := max(0, int(float64(ag)*statFactor()))
	scaledCMP := max(0, int(float64(cmp)*statFactor()))

	p := &a.Participant
	p.ID, p.HP, p.Attack, p.Defense = id, scaledHP, scaledAtk, scaledDef
	p.Name = m.Name
	p.Agility = scaledAg
	if p.MaxHP == 0 {
		p.MaxHP = scaledHP
	}
	p.MP = scaledMP
	if p.MaxMP == 0 {
		p.MaxMP = scaledMP
	}
	p.CMP = scaledCMP
	if p.MaxCMP == 0 {
		p.MaxCMP = scaledCMP
	}
	if p.BaseStats.Attack == 0 {
		p.BaseStats.Attack = scaledAtk
	}
	if p.BaseStats.Defense == 0 {
		p.BaseStats.Defense = scaledDef
	}
	if p.BaseStats.Agility == 0 {
		p.BaseStats.Agility = scaledAg
	}
	a.OriginalJobs = a.Jobs
	p.ExperienceReward = expReward
	p.GoldReward = goldReward

	return a
}

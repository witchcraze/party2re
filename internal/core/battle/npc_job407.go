package battle

import (
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Size classes by consumed MaxHP (≥2500, ≥1000, ≥500, else) choose the
// replacement pool positions and HP multipliers of _skill.cgi:_obgene.
var npc407Classes = [3]struct {
	positions  []int
	base, span float64
}{
	{[]int{0, 6, 7, 9}, 1.4, .7},
	{[]int{1, 3, 4, 10}, 1.3, .5},
	{[]int{2, 5, 8, 11}, 1.2, .3},
}

// ValidateNPCJob407 admits a body on a private projection without RNG or
// external hooks. The roster transform's branch is fixed by current members.
func (s *LifetimeState) ValidateNPCJob407(c *RosterContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	damage := p.nativeDamageContext(child.DamageContext, child.Death, rng)
	switch id {
	case "npc-job-407-position-0", "npc-job-407-position-1":
		return p.validateNPC407Roster(&child, rng)
	case "npc-job-407-position-2":
		// Up to three hits; the shared projection stops once the actor falls.
		for range 3 {
			if err := p.admitNPC405Area(&damage, child.Death, dualAgilityEffect("", 180, CategoryBreath, true), rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-407-position-3":
		for _, target := range child.EnemyIDs {
			p.actor(target).Status = StatusParalyze
			for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
				if err := p.ValidateStatDown(&child.EffectContext, controlEffect(target, .2, CategoryUntyped), stat, rng); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

// ApplyNPCJob407 admits the whole body before any draw or mutation.
func (s *LifetimeState) ApplyNPCJob407(c *RosterContext, id string, rng random.Generator) error {
	if err := s.ValidateNPCJob407(c, id, rng); err != nil {
		return err
	}
	previous := c.DamageContext
	native := s.nativeDamageContext(c.DamageContext, c.Death, rng)
	c.Reset, c.Defeat = native.Reset, native.Defeat
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() {
		c.Reset, c.Defeat = previous.Reset, previous.Defeat
		c.targetResources, c.EffectContext.targetResources = previous.targetResources, previous.EffectContext.targetResources
	}()
	switch id {
	case "npc-job-407-position-0", "npc-job-407-position-1":
		return s.applyNPC407Roster(c, rng)
	case "npc-job-407-position-2":
		hits := int(rng.Float64()*2 + 2)
		for range hits {
			if s.actor(c.ActorID).HP <= 0 {
				return nil
			}
			if _, err := s.ApplyAreaDamage(&c.DamageContext, 180, CategoryBreath, true, rng); err != nil {
				return err
			}
		}
	case "npc-job-407-position-3":
		// Unguarded: the status overwrites an existing one before the stat-downs.
		for _, target := range slices.Clone(c.EnemyIDs) {
			if rng.Float64()*4 >= 1 {
				continue
			}
			s.actor(target).Status = StatusParalyze
			for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
				if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(target, .2, CategoryUntyped), stat, rng); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// npc407Side is the source's NPC-turn party, the requester's opponent array:
// built from living members at load, it keeps those felled during the request
// (_battle.cgi:486–493; _npc_action.cgi:107–109). Earlier falls were pruned at
// the last save; a helper dismissed this request stays in the requester's array.
func (s *LifetimeState) npc407Side(c *RosterContext) []string {
	var ids []string
	for _, id := range c.PartyIDs {
		if a := s.actor(id); a != nil && !a.RosterDetached {
			ids = append(ids, id)
		}
	}
	return ids
}

// The source matches '@氷の女神模像' and '@アイスオブジェ' by name. Copies hold
// the statue payload's identity plus "-copy-"; ice objects share the stage
// pool's name-family identity, optionally with one encounter letter.
func npc407CopyPrefix(c *RosterContext) string {
	return c.EXPool[1].EncounterIdentity + "-copy-"
}

func npc407Ice(c *RosterContext, a *LifetimeActor) bool {
	family, id := c.EnemyPool[0].EncounterIdentity, a.EncounterIdentity
	return id == family || len(id) == len(family)+1 && strings.HasPrefix(id, family) && id[len(family)] >= 'A' && id[len(family)] <= 'Z'
}

func (s *LifetimeState) npc407Summoning(c *RosterContext) bool {
	for _, id := range s.npc407Side(c) {
		if id != c.ActorID && strings.HasPrefix(s.actor(id).RosterIdentity, npc407CopyPrefix(c)) {
			return true
		}
	}
	return false
}

func validNPC407Payload(a LifetimeActor) bool {
	for _, v := range []int{a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.Jobs.SP, a.Jobs.OldSP} {
		if v < 0 || v > 1e12 {
			return false
		}
	}
	return a.Name != "" && a.EncounterIdentity != ""
}

func (p *LifetimeState) validateNPC407Roster(c *RosterContext, rng random.Generator) error {
	if c.RequesterLevel == nil || *c.RequesterLevel < 0 || len(c.EnemyPool) < 12 || len(c.EXPool) < 2 || len(c.EXXPool) < 1 || c.ReloadDraws < 0 {
		return ErrUnsupportedEffect
	}
	for _, a := range append([]LifetimeActor{c.EXPool[1], c.EXXPool[0]}, c.EnemyPool...) {
		if !validNPC407Payload(a) {
			return ErrInvalidParticipant
		}
	}
	if p.RosterSequence >= 1e15 {
		return ErrInvalidParticipant
	}
	if p.npc407Summoning(c) {
		return nil
	}
	return p.validateRecovery(&c.EffectContext, controlEffect(c.ActorID, 0, CategoryUntyped), rng)
}

// applyNPC407Roster is _obgene: re-requiring the stage file first consumes its
// top-level draws, then a living copy summons the goddess in place of the
// whole side, otherwise every ice object is replaced by a class successor.
func (s *LifetimeState) applyNPC407Roster(c *RosterContext, rng random.Generator) error {
	s.reloadStage(c, rng)
	side := s.npc407Side(c)
	remove := func(id string) {
		a := s.actor(id)
		// Removal is not a defeat: no credit, and the save prunes the record.
		// The fallen actor keeps its frame slot, which the source also empties.
		a.HP, a.RosterIdentity = 0, ""
		if id != c.ActorID {
			c.PartyIDs = slices.DeleteFunc(c.PartyIDs, func(v string) bool { return v == id })
			c.MemberOnlyIDs = append(c.MemberOnlyIDs, id)
		}
	}
	if s.npc407Summoning(c) {
		for _, id := range side {
			remove(id)
		}
		g := c.EXXPool[0]
		// Only these nine fields pass; rewards, item and MaxMP follow _add_party.
		s.putHelper(c, HelperPayload{Identity: g.EncounterIdentity, EncounterIdentity: g.EncounterIdentity, Name: g.Name, Icon: g.Icon,
			Stats: [5]float64{float64(g.HP), float64(g.MP), float64(g.Attack), float64(g.Defense), float64(g.Agility)},
			Jobs:  BattleJobs{JobID: g.Jobs.JobID, SP: g.Jobs.SP, OldJobID: "starter"}}, rng)
		return nil
	}
	var classes [4][]float64
	for _, id := range side {
		a := s.actor(id)
		if !npc407Ice(c, a) {
			continue
		}
		hp := float64(a.MaxHP)
		remove(id)
		switch {
		case hp >= 2500:
			classes[3] = append(classes[3], hp)
		case hp >= 1000:
			classes[2] = append(classes[2], hp)
		case hp >= 500:
			classes[1] = append(classes[1], hp)
		default:
			classes[0] = append(classes[0], hp)
		}
	}
	classes[0] = append(classes[0], float64(int(100+rng.Float64()*300)))
	letter := 0
	add := func(a LifetimeActor, identity, name string, hp float64) {
		suffix := "" // Past Z the source name has no letter.
		if letter < 18 {
			suffix = string(rune('I' + letter))
		}
		letter++
		s.putHelper(c, HelperPayload{Identity: identity + suffix, EncounterIdentity: a.EncounterIdentity, Name: strings.TrimSpace(name + " " + suffix), Icon: a.Icon,
			Stats: [5]float64{hp, float64(a.MP), float64(a.Attack), float64(a.Defense), float64(a.Agility)}, Jobs: a.Jobs}, rng)
	}
	successor := func(positions []int, maxHP, base, span float64) {
		a := c.EnemyPool[positions[int(rng.Float64()*float64(len(positions)))]]
		add(a, a.EncounterIdentity+"-", a.Name, maxHP*(base+rng.Float64()*span))
	}
	statue := func() {
		a := c.EXPool[1]
		add(a, npc407CopyPrefix(c), "氷の女神模像", float64(a.HP))
	}
	for i, class := range npc407Classes {
		for _, hp := range classes[i] {
			successor(class.positions, hp, class.base, class.span)
		}
	}
	switch ex := classes[3]; {
	case len(ex) >= 2:
		statue()
	case len(ex) == 1 && ex[0] > 4000:
		statue()
	case len(ex) == 1:
		successor([]int{2, 5, 8}, ex[0], 1.2, .3)
	}
	heal := 150*len(classes[0]) + 300*len(classes[1]) + 600*len(classes[2]) + 1000*len(classes[3])
	_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(heal), CategoryUntyped), rng)
	return err
}

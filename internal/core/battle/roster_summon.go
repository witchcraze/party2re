package battle

import (
	"strconv"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Source-backed stat requirements, ordered by requester level <40, <70, >=70.
// Forms are native ordinal identities; legacy art and proper names are unused.
var callingStats = map[int][3][5]float64{
	59: {{70, 120, 50, 30, 120}, {180, 80, 80, 50, 200}, {5, 10, 10, 950, 950}},
	60: {{160, 70, 180, 100, 70}, {250, 50, 300, 200, 100}, {400, 30, 400, 300, 100}},
	61: {{120, 120, 120, 60, 120}, {300, 200, 280, 90, 240}, {444, 444, 666, 222, 444}},
	62: {{90, 80, 120, 10, 180}, {210, 270, 170, 30, 400}, {410, 350, 400, 50, 600}},
	63: {{100, 100, 80, 50, 80}, {300, 80, 150, 100, 100}, {400, 60, 200, 100, 100}},
	64: {{100, 100, 50, 100, 100}, {200, 80, 100, 150, 150}, {300, 60, 150, 200, 200}},
	65: {{100, 60, 100, 60, 180}, {210, 90, 300, 80, 280}, {400, 230, 500, 160, 400}},
	66: {{120, 110, 160, 90, 150}, {240, 210, 240, 120, 180}, {450, 280, 400, 240, 300}},
	67: {{90, 120, 90, 50, 90}, {180, 280, 160, 120, 180}, {8, 500, 80, 950, 950}},
	71: {{150, 70, 100, 50, 180}, {270, 50, 270, 70, 270}, {500, 30, 300, 300, 10}},
	87: {{180, 10, 180, 30, 350}, {300, 80, 280, 170, 450}, {650, 300, 450, 300, 600}},
}

func (s *LifetimeState) summonCalling(c *RosterContext, job int, rng random.Generator) (RosterResult, error) {
	p := HelperPayload{Identity: callingIdentity(job), Name: "援護者"}
	if s.rosterIdentity(p.Identity) != nil {
		return RosterResult{}, nil
	}
	level := *c.RequesterLevel
	p.Form = 0
	if level >= 70 {
		p.Form = 2
	} else if level >= 40 {
		p.Form = 1
	}
	p.Stats = callingStats[job][p.Form]
	a := s.actor(c.ActorID)
	base := [5]float64{float64(a.MaxHP), float64(a.MaxMP), float64(a.BaseStats.Attack), float64(a.BaseStats.Defense), float64(a.BaseStats.Agility)}
	switch job {
	case 68, 69:
		p.Form = 2
		if rng.Float64()*3 < 1 {
			p.Form = 0
		} else if rng.Float64()*3 < 1 {
			p.Form = 1
		}
		factors := [3][5]float64{{.8, .2, 3, 2, 1}, {.6, 1, 1, 1, 2}, {.7, 1, 1.5, 1.5, 1.5}}[p.Form]
		for i := range base {
			p.Stats[i] = base[i] * factors[i]
		}
	case 80:
		switch {
		case rng.Float64()*10 < 1:
			p.Form = 0
			for i, v := range [5]float64{.7, 2.3, 1.4, 1.5, 1.8} {
				p.Stats[i] = base[i] * v
			}
		case rng.Float64()*10 < 1:
			p.Form = 1
			for i := range base {
				if i < 2 {
					p.Stats[i] = base[i] * (2 + rng.Float64()) / 4
				} else {
					p.Stats[i] = base[i] * (3 + rng.Float64()*2) / 2
				}
			}
		case rng.Float64()*2 < 1:
			p.Form = 2
			for i, v := range [5]float64{.4, 1.6, .8, .8, 1.5} {
				p.Stats[i] = base[i] * v
			}
		default:
			p.Form = 3
			for i := range base {
				if i < 2 {
					p.Stats[i] = base[i] * (1 + rng.Float64()*2) / 4
				} else {
					p.Stats[i] = base[i] * (1 + rng.Float64()*2) / (1 + rng.Float64()*2)
				}
			}
		}
	case 87:
		p.Form = 0
		if level >= 70 && rng.Float64()*10 < 1 {
			p.Form = 2
		} else if level >= 40 && rng.Float64()*4 < 1 {
			p.Form = 1
		}
		p.Stats = callingStats[87][p.Form]
	}
	return s.AddHelper(c, p, rng)
}

func toolHelper(id string) (HelperPayload, bool) {
	p := HelperPayload{Identity: "dummy", Name: "援護人形"}
	switch id {
	case "tool-72":
		p.Stats = [5]float64{150, 999, 400, 50, 150}
	case "tool-73":
		p.Form = 1
		p.Stats = [5]float64{120, 300, 700, 900, 100}
	case "tool-74":
		p.Form = 2
		p.Stats = [5]float64{300, 100, 999, 200, 100}
	case "tool-216":
		p.Identity = "guardian"
		p.Name = "守護柱"
		p.Stats = [5]float64{350, 10, 5, 550, 10}
	case "extension-244":
		p.Identity = "snow"
		p.Name = "雪の援護者"
		p.Stats = [5]float64{100, 5, 5, 5, 5}
	default:
		return HelperPayload{}, false
	}
	return p, true
}

func (s *LifetimeState) duplicateHelper(c *RosterContext, rng random.Generator) (RosterResult, error) {
	one, two := s.rosterIdentity(callingIdentity(63)), s.rosterIdentity("calling-63-copy")
	count := 0
	if one != nil {
		count++
	}
	if two != nil {
		count++
	}
	var source *LifetimeActor
	identity := "calling-63-copy"
	if one != nil && one.HP > 0 && two == nil {
		source = one
	} else if two != nil && two.HP > 0 && one == nil {
		source = two
		identity = callingIdentity(63)
	}
	r := RosterResult{}
	if source != nil {
		saved := *source
		var err error
		r, err = s.AddHelper(c, HelperPayload{Identity: identity, Name: "援護者", Form: saved.SummonForm, Stats: [5]float64{float64(saved.MaxHP), float64(saved.MaxMP), float64(saved.Attack), float64(saved.Defense), float64(saved.Agility)}}, rng)
		if err != nil {
			return r, err
		}
		a := s.actor(r.HelperID)
		a.HP, a.MP = min(saved.HP, a.MaxHP), min(saved.MP, a.MaxMP)
		a.BaseStats, a.Temporary = saved.BaseStats, saved.Temporary
		a.Icon = saved.Icon
	}
	effects, err := s.ApplyPartyHealing(&c.EffectContext, float64(count*100), CategoryUntyped, rng)
	r.Effects = effects
	return r, err
}

func (s *LifetimeState) smallHelpers(c *RosterContext, helperID string, rng random.Generator) (RosterResult, error) {
	a := *s.actor(helperID)
	r := RosterResult{}
	for form := 0; form < 3; form++ {
		p := HelperPayload{Identity: "small-" + strconv.Itoa(form), Name: "小さな援護者", Form: form, Stats: [5]float64{float64(a.MaxHP) * .1, float64(a.MaxMP) * .1, float64(a.BaseStats.Attack) * .1, float64(a.BaseStats.Defense) * .1, float64(a.BaseStats.Agility) * .1}}
		child, err := s.AddHelper(c, p, rng)
		if err != nil {
			return r, err
		}
		r.Applied = r.Applied || child.Applied
	}
	return r, nil
}

func (s *LifetimeState) growHelper(c *RosterContext, id string, chance float64, rng random.Generator) {
	a := s.actor(id)
	if a.RosterIdentity != callingIdentity(87) || a.SummonForm >= 2 {
		return
	}
	if rng.Float64()*chance >= 1 {
		return
	}
	a.SummonForm++
	v := summonStats(callingStats[87][a.SummonForm], c.RequesterItemID == "item-165", rng)
	a.HP, a.MaxHP, a.MP, a.MaxMP = v[0], v[0], v[1], v[1]
	a.Attack, a.Defense, a.Agility = v[2], v[3], v[4]
	a.BaseStats = BattleBaseStats{v[2], v[3], v[4]}
	a.Icon = "assets/images/placeholders/battle/helpers.svg#" + callingIdentity(87) + "-" + strconv.Itoa(a.SummonForm)
	a.Hit, a.TeamID, a.Temporary = 95, s.actor(c.ActorID).TeamID, ""
	a.ExperienceReward, a.GoldReward = *c.RequesterLevel/2, *c.RequesterLevel/4
	// Growth preserves tension, abnormal state, jobs and extended effects.
}

func (s *LifetimeState) mergeHelper(c *RosterContext, helperID string, job int) RosterResult {
	a, h := s.actor(c.ActorID), s.actor(helperID)
	left := []*int{&a.HP, &a.MP, &a.Attack, &a.Defense, &a.Agility}
	right := []*int{&h.HP, &h.MP, &h.Attack, &h.Defense, &h.Agility}
	baselines := [5]int{a.MaxHP, a.MaxMP, a.BaseStats.Attack, a.BaseStats.Defense, a.BaseStats.Agility}
	for i := range left {
		cap := 999
		if a.OverLevel {
			if i < 2 {
				cap = max(999, baselines[i])
			} else {
				cap = max(999, min(1998, baselines[i]*3))
			}
		}
		sum := *left[i] + *right[i]
		if a.ID == h.ID {
			sum += *left[i]
		}
		*left[i] = min(sum, cap)
		*right[i] = min(sum, 999)
	}
	a.Icon = nativeFormIcon("job-"+strconv.Itoa(job)+"-mix", "")
	if h.NPC || h.RosterIdentity != "" {
		s.dismissHelper(c, helperID)
	} else {
		h.Icon = nativeFormIcon("job-0", "")
	}
	return RosterResult{Applied: true, HelperID: helperID}
}

func (s *LifetimeState) dismissHelper(c *RosterContext, id string) {
	a := s.actor(id)
	a.HP, a.TeamID, a.NPC = 0, c.EnemyTeamID, true
	a.RosterDetached = true
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BattleJobs belongs to the run, independently of permanent job progression.
type BattleJobs struct {
	JobID, OldJobID string
	SP, OldSP       int
}

// TransformationContext retains the requesting player's appearance, even when
// a delegated/NPC actor differs. Damage hooks must already admit their complete
// enclosing callback (#1363); nil keeps those callbacks wholly unsupported.
// MonsterIcons has exactly108 equally likely positions; missing art falls back.
type TransformationContext struct {
	EffectContext
	RequesterIcon, RequesterGender string
	MonsterIcons                   [108]string
	AreaDamage                     func(TargetContext, float64, CallbackCategory, bool, random.Generator) error
}

type TransformationResult struct {
	Applied  bool
	TargetID string
	Stats    []EffectApplication
}

// ValidateTransformation is shared by explicit discovery and execution. It
// admits payloads, not ordinary SP/cost/ownership or indirect closure (#1357).
// No random draw, counter, resource or lifetime fact changes during admission.
func (s *LifetimeState) ValidateTransformation(c *TransformationContext, id, targetID string, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-36-copy", "book-211-copy", "tool-214-copy", "gem-100-appearance", "job-300-form":
		return nil
	case "tool-168-memory":
		if targetID != a.Jobs.JobID && targetID != a.Jobs.OldJobID {
			if sp, recorded := a.JobHistory[targetID]; recorded {
				if sp == nil {
					return ErrUnsupportedEffect
				}
				if *sp < 0 {
					return ErrInvalidParticipant
				}
			}
		}
	case "job-82-form", "job-79-breath", "job-79-speed":
		if c.RequesterGender != "m" && c.RequesterGender != "f" {
			return ErrInvalidRequest
		}
		if id == "job-79-breath" && c.AreaDamage == nil {
			return ErrUnsupportedEffect
		}
		if id == "job-79-speed" {
			return s.ValidateStatUp(&c.EffectContext, TargetEffect{TargetID: a.ID, Payload: CallbackPayload{Number: 1.5}}, StatAgility, rng)
		}
	case "job-300-tune":
		return s.validateFormTune(c, rng)
	case "job-300-strike", "job-300-burst":
		// The unchecked outer helper result otherwise addresses an empty record.
		if c.Damage == nil || len(c.EnemyIDs) == 0 {
			return ErrUnsupportedEffect
		}
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

// ApplyTransformation reconstructs the concrete copy/form/memory callbacks at
// _skill.cgi:173–217,700–724,2944–2964,6278–6292,6593–6610,7465–7532;
// _data.cgi:1716–1748,1888–1912,2188–2193. Costs belong to the invoking owner:
// ordinary, random/delegated and custom invocations have different debit rules.
func (s *LifetimeState) ApplyTransformation(c *TransformationContext, id, targetID string, rng random.Generator) (TransformationResult, error) {
	r := TransformationResult{}
	if err := s.ValidateTransformation(c, id, targetID, rng); err != nil {
		return r, err
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-36-copy", "book-211-copy", "tool-214-copy":
		return s.copyParticipant(c, id, targetID, rng)
	case "tool-168-memory":
		sp, recorded := a.JobHistory[targetID]
		if targetID != "" && targetID != a.Jobs.JobID && targetID != a.Jobs.OldJobID && recorded {
			a.Jobs.OldJobID, a.Jobs.OldSP = targetID, *sp
			r.Applied = true
		}
	case "gem-100-appearance":
		a.Icon = c.MonsterIcons[int(rng.Float64()*108)]
		if a.Icon == "" {
			a.Icon = "assets/images/placeholders/battle/monster-default.svg"
		}
		r.Applied = true
	case "job-82-form":
		r.Applied = a.switchForm("job-82", "job-300")
		if r.Applied && !a.NPC {
			a.Icon = nativeFormIcon("mechanical", c.RequesterGender)
		}
	case "job-300-form":
		r.Applied = a.switchForm("job-300", "job-82")
	case "job-300-tune":
		for range 2 {
			stat := []BattleStat{StatAgility, StatDefense, StatAttack}[int(rng.Float64()*3)]
			effect, err := s.ApplyStatUp(&c.EffectContext, TargetEffect{TargetID: a.ID, Payload: CallbackPayload{Number: .5}}, stat, rng)
			if err != nil {
				return r, err
			}
			r.Stats = append(r.Stats, effect)
			r.Applied = r.Applied || effect.Applied
		}
	case "job-300-strike", "job-300-burst":
		return s.applyFormDamage(c, id, targetID, rng)
	case "job-79-breath":
		if err := c.AreaDamage(c.TargetContext, 150, CategoryBreath, true, rng); err != nil {
			return r, err
		}
		a = s.actor(c.ActorID)
		// Original predicate compares to the REQUESTER, including NPC actors.
		if a.Icon == c.RequesterIcon {
			a.Icon = nativeFormIcon("winged", c.RequesterGender)
			r.Applied = true
		}
	case "job-79-speed":
		effect, err := s.ApplyStatUp(&c.EffectContext, TargetEffect{TargetID: a.ID, Payload: CallbackPayload{Number: 1.5}}, StatAgility, rng)
		if err != nil {
			return r, err
		}
		r.Stats = []EffectApplication{effect}
		a = s.actor(c.ActorID)
		a.Icon, r.Applied = c.RequesterIcon, true
	}
	return r, nil
}

func (s *LifetimeState) copyParticipant(c *TransformationContext, id, targetID string, rng random.Generator) (TransformationResult, error) {
	r := TransformationResult{}
	a := s.actor(c.ActorID)
	if id == "book-211-copy" && (a.Jobs.JobID != a.OriginalJobs.JobID || a.Jobs.OldJobID != a.OriginalJobs.OldJobID) {
		return r, nil
	}
	if id == "tool-214-copy" {
		var others []string
		for _, member := range s.Actors {
			if member.ID != a.ID {
				others = append(others, member.ID)
			}
		}
		if len(others) == 0 {
			return r, nil
		}
		targetID = randomRoster(others, rng)
	} else {
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return r, err
		}
		if s.actor(targetID) == nil {
			targetID = s.randomMember(rng)
		}
	}
	target := s.actor(targetID)
	r.TargetID = targetID
	if target.HP <= 0 || target.MaxHP > 999 || target.BaseStats.Defense > 999 {
		return r, nil
	}
	a.HP, a.MaxHP, a.MP, a.MaxMP = target.HP, target.MaxHP, target.MP, target.MaxMP
	a.Attack, a.Defense, a.Agility, a.BaseStats = target.Attack, target.Defense, target.Agility, target.BaseStats
	a.Jobs, a.Icon = target.Jobs, target.Icon
	if id != "tool-214-copy" {
		a.MP = max(50, a.MP)
	}
	if id == "book-211-copy" {
		a.Status = StatusDofuu
		if s.SkillBans == nil {
			s.SkillBans = make(map[string]int)
		}
		s.SkillBans["copy"] = 999
	}
	r.Applied = true
	return r, nil
}

func (a *LifetimeActor) switchForm(from, to string) bool {
	if a.Jobs.JobID == from {
		a.Jobs.JobID = to
		return true
	}
	if a.Jobs.OldJobID == from {
		a.Jobs.OldJobID = to
		return true
	}
	return false
}

func (s *LifetimeState) validateFormTune(c *TransformationContext, rng random.Generator) error {
	e := TargetEffect{TargetID: c.ActorID, Payload: CallbackPayload{Number: .5}}
	admission := statUpAdmission{mayDouble: make(map[string]bool)}
	for _, stat := range []BattleStat{StatAgility, StatDefense, StatAttack} {
		if err := s.validateStatUp(&c.EffectContext, e, stat, rng, statUpAdmission{}); err != nil {
			return err
		}
		s.recordStatUp205(&c.EffectContext, e, stat, &admission)
	}
	for _, stat := range []BattleStat{StatAgility, StatDefense, StatAttack} {
		if err := s.validateStatUp(&c.EffectContext, e, stat, rng, admission); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) applyFormDamage(c *TransformationContext, id, targetID string, rng random.Generator) (TransformationResult, error) {
	r := TransformationResult{}
	target, err := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: targetID, Payload: CallbackPayload{Symbol: id}}, rng)
	if err != nil || !target.Resolved {
		return r, err
	}
	a := s.actor(c.ActorID)
	r.TargetID = target.TargetID
	if id == "job-300-strike" {
		t := s.actor(target.TargetID)
		saved := t.Temporary
		t.Temporary = ""
		err := c.Damage(c.TargetContext, TargetEffect{TargetID: t.ID, Category: CategoryPhysical, Payload: CallbackPayload{Number: float64(a.Attack)}, Arguments: []string{"1"}}, rng)
		s.actor(target.TargetID).Temporary = saved // Restore after defeat/reset or roster growth.
		return TransformationResult{Applied: err == nil, TargetID: t.ID}, err
	}
	hits, tension := 2+int(rng.Float64()*2), a.Tension
	for i := range hits {
		a = s.actor(c.ActorID)
		if a.HP <= 0 {
			break
		}
		if i > 0 {
			target.TargetID = randomRoster(c.EnemyIDs, rng)
		}
		a.Tension = tension
		power := 200.0
		if s.actor(target.TargetID).MaxHP > 999 {
			power = 500
		}
		if err := c.Damage(c.TargetContext, TargetEffect{TargetID: target.TargetID, Category: CategoryBreath, Payload: CallbackPayload{Number: power}, Arguments: []string{"1"}}, rng); err != nil {
			return r, err
		}
	}
	a = s.actor(c.ActorID)
	a.Tension, a.Status = 1, StatusDofuu
	a.switchForm("job-300", "job-82")
	r.Applied = true
	return r, nil
}

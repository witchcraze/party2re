package character

// ClampVitality ensures current HP and MP stay within valid bounds: [0, MaxHP] and [0, MaxMP].
func (s *Stats) ClampVitality() {
	if s == nil {
		return
	}
	if s.HP < 0 {
		s.HP = 0
	} else if s.MaxHP > 0 && s.HP > s.MaxHP {
		s.HP = s.MaxHP
	}
	if s.MP < 0 {
		s.MP = 0
	} else if s.MaxMP > 0 && s.MP > s.MaxMP {
		s.MP = s.MaxMP
	}
}

// RecoverVitality fully restores current HP and MP to their respective maximum values.
func (c *Character) RecoverVitality() {
	if c == nil {
		return
	}
	c.Stats.HP = c.Stats.MaxHP
	c.Stats.MP = c.Stats.MaxMP
	c.Stats.ClampVitality()
}

// ApplyCombatSurvival applies post-combat surviving HP and MP, enforcing the authentic
// 1 HP survival floor for fallen combatants (HP <= 0 or fallen == true) and clamping within vitality bounds.
// If survivingMP is negative, current MP is preserved and clamped.
func (c *Character) ApplyCombatSurvival(survivingHP, survivingMP int, fallen bool) {
	if c == nil {
		return
	}
	if fallen || survivingHP <= 0 {
		c.Stats.HP = 1
	} else {
		c.Stats.HP = survivingHP
	}
	if survivingMP >= 0 {
		c.Stats.MP = survivingMP
	}
	c.Stats.ClampVitality()
}

// ResetTired resets character fatigue to 0 upon sleep or full recovery.
func (c *Character) ResetTired() {
	if c != nil {
		c.Tired = 0
	}
}

// AddTired adds fatigue percentage to character, capping at 100%.
func (c *Character) AddTired(delta int) {
	if c == nil || delta <= 0 {
		return
	}
	c.Tired += delta
	if c.Tired > 100 {
		c.Tired = 100
	}
}

// ReduceTired decreases character fatigue by delta percentage (e.g. celestial wishes).
// Legacy parity: Celestial wishes (-150%) can push fatigue below 0% as a combat buffer.
func (c *Character) ReduceTired(delta int) {
	if c == nil || delta <= 0 {
		return
	}
	c.Tired -= delta
}

// IsExhausted returns true if character fatigue is 100% or higher.
func (c *Character) IsExhausted() bool {
	if c == nil {
		return false
	}
	return c.Tired >= 100
}

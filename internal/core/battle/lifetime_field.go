package battle

// LifetimeFields holds named fields, independently of batch elemental FieldState.
// Default survives temporary expiry and anti-field creation. Special boss-marker
// flags are separate encounter facts; they are never inferred from a field name.
type LifetimeFields struct {
	Temporary string `json:"temporary,omitempty"`
	Turns     int    `json:"turns,omitempty"`
	Default   string `json:"default,omitempty"`
	AntiTurns int    `json:"anti_turns,omitempty"`
}

// Current returns the temporary/default snapshot even during anti-field. Original
// get_field_info does not consult anti-field; effect predicates do so separately.
func (f LifetimeFields) Current() string {
	if f.Temporary != "" {
		return f.Temporary
	}
	return f.Default
}

// Create follows _check_field: anti-field and the same temporary name block
// creation; an identical default name does not block a temporary field.
func (f *LifetimeFields) Create(name string, turns int) bool {
	if name == "" || turns <= 0 || f.AntiTurns > 0 || f.Temporary == name {
		return false
	}
	f.Temporary, f.Turns = name, turns
	return true
}

func (f *LifetimeFields) CreateAnti(turns int) {
	if turns <= 0 {
		return
	}
	f.AntiTurns = turns
	f.Temporary, f.Turns = "", 0
}

// FinishOperation is the player operation's field tail, outside the normal-action
// block. Incapacitation does not skip it; suppression does. A newly-created field
// ticks immediately. NPC response does not tick fields a second time.
// _battle.cgi:913–914,1534–1557,1661–1694.
func (f *LifetimeFields) FinishOperation(enemiesRemain, suppressed bool) {
	if suppressed {
		return
	}
	if f.AntiTurns > 0 {
		f.AntiTurns--
		return
	}
	if enemiesRemain && f.Temporary != "" {
		f.Turns--
		if f.Turns <= 0 {
			f.Temporary, f.Turns = "", 0
		}
	}
}

// Exercises is the special-field hook predicate (_battle.cgi:1715–1720).
// before/after are operation snapshots, not a cached elemental multiplier.
func (f LifetimeFields) Exercises(name, before string, npcResponseEnabled, suppressed bool) bool {
	return (name == before || name == f.Current()) && npcResponseEnabled && !suppressed && f.AntiTurns == 0
}

// ResetFloor removes only temporary and anti-field state on explicit advance.
func (f *LifetimeFields) ResetFloor() {
	f.Temporary, f.Turns, f.AntiTurns = "", 0, 0
}

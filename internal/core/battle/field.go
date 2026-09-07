package battle

const (
	ElementFire  = "fire"
	ElementWater = "water"
	ElementWind  = "wind"
	ElementEarth = "earth"
	ElementLight = "light"
	ElementDark  = "dark"
)

// FieldState represents elemental field conditions and anti-field suppression during battle.
type FieldState struct {
	Element       string `json:"element,omitempty"`
	RemainingTurn int    `json:"remaining_turn,omitempty"`
	AntiFieldTurn int    `json:"anti_field_turn,omitempty"`
}

// Clone returns a deep copy of the field state.
func (f *FieldState) Clone() *FieldState {
	if f == nil {
		return nil
	}
	return &FieldState{
		Element:       f.Element,
		RemainingTurn: f.RemainingTurn,
		AntiFieldTurn: f.AntiFieldTurn,
	}
}

// IsActive reports whether a non-suppressed elemental field is active.
func (f *FieldState) IsActive() bool {
	return f != nil && f.Element != "" && f.RemainingTurn > 0 && f.AntiFieldTurn == 0
}

// IsAntiFieldActive reports whether an anti-field suppression barrier is active.
func (f *FieldState) IsAntiFieldActive() bool {
	return f != nil && f.AntiFieldTurn > 0
}

// CreateField sets the field element and duration unless blocked by an anti-field.
func (f *FieldState) CreateField(element string, duration int) {
	if f.IsAntiFieldActive() {
		return
	}
	f.Element = element
	f.RemainingTurn = duration
}

// CreateAntiField creates an anti-field that suppresses all fields for duration turns.
func (f *FieldState) CreateAntiField(duration int) {
	f.AntiFieldTurn = duration
	f.Element = ""
	f.RemainingTurn = 0
}

// EndTurn advances the field counters at the end of each battle round.
func (f *FieldState) EndTurn() {
	if f == nil {
		return
	}
	if f.AntiFieldTurn > 0 {
		f.AntiFieldTurn--
	}
	if f.RemainingTurn > 0 {
		f.RemainingTurn--
		if f.RemainingTurn == 0 {
			f.Element = ""
		}
	}
}

// DamageMultiplier returns the damage multiplier for an action with the given element.
func (f *FieldState) DamageMultiplier(element string) float64 {
	if !f.IsActive() || element == "" {
		return 1.0
	}
	if f.Element == element {
		return 1.3
	}
	if (f.Element == ElementFire && element == ElementWater) ||
		(f.Element == ElementWater && element == ElementFire) ||
		(f.Element == ElementWind && element == ElementEarth) ||
		(f.Element == ElementEarth && element == ElementWind) ||
		(f.Element == ElementLight && element == ElementDark) ||
		(f.Element == ElementDark && element == ElementLight) {
		return 0.8
	}
	return 1.0
}

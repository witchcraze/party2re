package battle

import (
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// NewParticipant validates and creates a new Participant.
func NewParticipant(id string, hp, attack, defense int) (Participant, error) {
	trimmedID := strings.TrimSpace(id)
	p := Participant{
		ID:      trimmedID,
		Name:    trimmedID,
		HP:      hp,
		Attack:  attack,
		Defense: defense,
	}
	if err := validateParticipant(p); err != nil {
		return Participant{}, err
	}
	return p, nil
}

// MustNewParticipant creates a Participant or panics if invalid (useful in tests/constants).
func MustNewParticipant(id string, hp, attack, defense int) Participant {
	p, err := NewParticipant(id, hp, attack, defense)
	if err != nil {
		panic(err)
	}
	return p
}

// NewParticipantFromCharacter creates a Participant from Character stats.
// If the character's HP is <= 0, it falls back to MaxHP if MaxHP > 0.
func NewParticipantFromCharacter(char corecharacter.Character) Participant {
	hp := char.Stats.HP
	if hp <= 0 && char.Stats.MaxHP > 0 {
		hp = char.Stats.MaxHP
	}
	return Participant{
		ID:      char.ID,
		Name:    char.Name,
		HP:      hp,
		Attack:  char.Stats.Attack,
		Defense: char.Stats.Defense,
	}
}

// NewParticipantFromCharacterWithHP creates a Participant from Character with a specific current HP override.
func NewParticipantFromCharacterWithHP(char corecharacter.Character, currentHP int) Participant {
	hp := currentHP
	if hp <= 0 {
		hp = char.Stats.HP
		if hp <= 0 && char.Stats.MaxHP > 0 {
			hp = char.Stats.MaxHP
		}
	}
	return Participant{
		ID:      char.ID,
		Name:    char.Name,
		HP:      hp,
		Attack:  char.Stats.Attack,
		Defense: char.Stats.Defense,
	}
}

// ParticipantBuilder provides a fluent builder pattern for constructing Participants.
type ParticipantBuilder struct {
	id      string
	name    string
	hp      int
	attack  int
	defense int
}

// NewParticipantBuilder initializes a new builder with an ID.
func NewParticipantBuilder(id string) *ParticipantBuilder {
	trimmedID := strings.TrimSpace(id)
	return &ParticipantBuilder{id: trimmedID, name: trimmedID}
}

// FromCharacter sets attributes based on a Character's stats.
func (b *ParticipantBuilder) FromCharacter(char corecharacter.Character) *ParticipantBuilder {
	b.id = char.ID
	b.name = char.Name
	b.hp = char.Stats.HP
	if b.hp <= 0 && char.Stats.MaxHP > 0 {
		b.hp = char.Stats.MaxHP
	}
	b.attack = char.Stats.Attack
	b.defense = char.Stats.Defense
	return b
}

// WithName sets the display name.
func (b *ParticipantBuilder) WithName(name string) *ParticipantBuilder {
	b.name = strings.TrimSpace(name)
	return b
}

// WithStats sets HP, Attack, and Defense values.
func (b *ParticipantBuilder) WithStats(hp, attack, defense int) *ParticipantBuilder {
	b.hp = hp
	b.attack = attack
	b.defense = defense
	return b
}

// WithCurrentHP overrides the HP attribute.
func (b *ParticipantBuilder) WithCurrentHP(hp int) *ParticipantBuilder {
	b.hp = hp
	return b
}

// Build validates and returns the constructed Participant.
func (b *ParticipantBuilder) Build() (Participant, error) {
	p, err := NewParticipant(b.id, b.hp, b.attack, b.defense)
	if err != nil {
		return Participant{}, err
	}
	if b.name != "" {
		p.Name = b.name
	}
	return p, nil
}

// MustBuild returns the constructed Participant or panics if invalid.
func (b *ParticipantBuilder) MustBuild() Participant {
	p, err := b.Build()
	if err != nil {
		panic(err)
	}
	return p
}

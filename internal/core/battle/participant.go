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
		MaxHP:   char.Stats.MaxHP,
		MP:      char.Stats.MP,
		MaxMP:   char.Stats.MaxMP,
		Attack:  char.Stats.Attack,
		Defense: char.Stats.Defense,
		Agility: char.Stats.Agility,
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
		MaxHP:   char.Stats.MaxHP,
		MP:      char.Stats.MP,
		MaxMP:   char.Stats.MaxMP,
		Attack:  char.Stats.Attack,
		Defense: char.Stats.Defense,
		Agility: char.Stats.Agility,
	}
}

// ParticipantBuilder provides a fluent builder pattern for constructing Participants.
type ParticipantBuilder struct {
	id           string
	name         string
	teamID       string
	hp           int
	maxHP        int
	mp           int
	maxMP        int
	cmp          int
	maxCMP       int
	attack       int
	defense      int
	agility      int
	abilities    []string
	skills       []ActionSkill
	customSkills []ActionCustomSkill
	defending    bool
	status       string
	items        []string
	actionItems  []ActionItem
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
	b.maxHP = char.Stats.MaxHP
	b.mp = char.Stats.MP
	b.maxMP = char.Stats.MaxMP
	b.attack = char.Stats.Attack
	b.defense = char.Stats.Defense
	b.agility = char.Stats.Agility
	return b
}

// WithName sets the display name.
func (b *ParticipantBuilder) WithName(name string) *ParticipantBuilder {
	b.name = strings.TrimSpace(name)
	return b
}

// WithTeamID sets the team or faction identifier.
func (b *ParticipantBuilder) WithTeamID(teamID string) *ParticipantBuilder {
	b.teamID = strings.TrimSpace(teamID)
	return b
}

// WithStats sets HP, Attack, and Defense values.
func (b *ParticipantBuilder) WithStats(hp, attack, defense int) *ParticipantBuilder {
	b.hp = hp
	if b.maxHP <= 0 {
		b.maxHP = hp
	}
	b.attack = attack
	b.defense = defense
	return b
}

// WithCurrentHP overrides the HP attribute.
func (b *ParticipantBuilder) WithCurrentHP(hp int) *ParticipantBuilder {
	b.hp = hp
	return b
}

// WithAgility sets the Agility value.
func (b *ParticipantBuilder) WithAgility(ag int) *ParticipantBuilder {
	b.agility = ag
	return b
}

// WithMP sets MP and MaxMP values.
func (b *ParticipantBuilder) WithMP(mp, maxMP int) *ParticipantBuilder {
	b.mp = mp
	b.maxMP = maxMP
	return b
}

// WithCMP sets CMP and MaxCMP values.
func (b *ParticipantBuilder) WithCMP(cmp, maxCMP int) *ParticipantBuilder {
	b.cmp = cmp
	b.maxCMP = maxCMP
	return b
}

// WithAbilities sets passive combat abilities.
func (b *ParticipantBuilder) WithAbilities(abilities ...string) *ParticipantBuilder {
	b.abilities = append(b.abilities, abilities...)
	return b
}

// WithSkills sets actionable job skills.
func (b *ParticipantBuilder) WithSkills(skills ...ActionSkill) *ParticipantBuilder {
	b.skills = append(b.skills, skills...)
	return b
}

// WithCustomSkills sets actionable custom skills.
func (b *ParticipantBuilder) WithCustomSkills(skills ...ActionCustomSkill) *ParticipantBuilder {
	b.customSkills = append(b.customSkills, skills...)
	return b
}

// WithDefending sets the initial defending stance.
func (b *ParticipantBuilder) WithDefending(defending bool) *ParticipantBuilder {
	b.defending = defending
	return b
}

// WithStatus sets the initial status affliction.
func (b *ParticipantBuilder) WithStatus(status string) *ParticipantBuilder {
	b.status = status
	return b
}

// WithItems sets equipped item definition IDs used by legacy battle synergies.
func (b *ParticipantBuilder) WithItems(itemIDs ...string) *ParticipantBuilder {
	b.items = append(b.items, itemIDs...)
	return b
}

// WithActionItems sets actionable combat items (@どうぐ).
func (b *ParticipantBuilder) WithActionItems(items ...ActionItem) *ParticipantBuilder {
	b.actionItems = append(b.actionItems, items...)
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
	p.TeamID = b.teamID
	p.MaxHP = b.maxHP
	p.MP = b.mp
	p.MaxMP = b.maxMP
	p.CMP = b.cmp
	p.MaxCMP = b.maxCMP
	p.Agility = b.agility
	p.Abilities = b.abilities
	p.Skills = b.skills
	p.CustomSkills = b.customSkills
	p.Defending = b.defending
	p.Status = b.status
	p.ItemDefinitionIDs = b.items
	p.ActionItems = b.actionItems
	if err := validateParticipant(p); err != nil {
		return Participant{}, err
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

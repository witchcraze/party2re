package character

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/id"
)

const maxNameLength = 32

const (
	DefaultJobID  = "starter"
	DefaultGender = "unspecified"
	DefaultColor  = "#ffffff"
	InitialMoney  = 200
	InitialLevel  = 1
)

const (
	MaxMoney       = 2_000_000_000
	MaxSmallMedals = 999_999_999
)

var (
	ErrInvalidName        = errors.New("character name must be between 1 and 32 characters")
	ErrNotFound           = errors.New("character not found")
	ErrInvalidAmount      = errors.New("amount must be non-negative")
	ErrInsufficientFunds  = errors.New("insufficient money")
	ErrInsufficientMedals = errors.New("insufficient small medals")
	ErrInvalidColor       = errors.New("invalid color format, must be #RRGGBB")
)

type Character struct {
	ID          string
	PlayerID    string
	Name        string
	JobID       string
	Gender      string
	Stats       Stats
	Money       int
	Level       int
	Experience  int
	SP          int // Skill Points: incremented on each level-up; used for SP-based skill learning.
	JobLevel    int // Number of completed job changes.
	OldJobID    string
	OldSP       int
	JobMemory   *JobMemory
	SmallMedals int
	HelpCount   int
	Orb         string // Orb collection status: string containing characters 's','r','b','g','y','p', or 'G' (Ramia awakened)
	Tired       int    // Fatigue percentage (疲労度 %): increases in combat, resets to 0 on sleep.
	OverLevel   bool
	OverDepot   int
	OverMonster int
	OverFuture  int
	OverFlea    int
	OverStore   int
	Color       string // Player chat/display color (HEX format: #RRGGBB, default: #ffffff)
}

// JobMemory is the temporary pair of job states used by the job exchange
// operation. It is intentionally part of character state so the exchange can
// be resumed safely after a process restart.
type JobMemory struct {
	JobID    string
	SP       int
	OldJobID string
	OldSP    int
}

// FutureMemory represents a saved snapshot of a character's state created via
// item-207 (未来のカケラ) and restored through the legacy "よびおこす" action.
type FutureMemory struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	JobID       string    `json:"job_id"`
	OldJobID    string    `json:"old_job_id"`
	Level       int       `json:"level"`
	Experience  int       `json:"experience"`
	MaxHP       int       `json:"max_hp"`
	MaxMP       int       `json:"max_mp"`
	Attack      int       `json:"attack"`
	Defense     int       `json:"defense"`
	Agility     int       `json:"agility"`
	Gender      string    `json:"gender"`
	OverLevel   bool      `json:"over_level"`
	CreatedAt   time.Time `json:"created_at"`
}

type Stats struct {
	MaxHP   int
	MaxMP   int
	HP      int
	MP      int
	Attack  int
	Defense int
	Agility int
}

// Clamp applies the legacy status ceilings. OverLevel characters above level 99
// receive the doubled ceilings used by the celestial level-break system.
func (s *Stats) Clamp(overLevel bool, level int) {
	if s == nil {
		return
	}
	multiplier := 1
	if overLevel && level > 99 {
		multiplier = 2
	}
	limitHPMP := 999 * multiplier
	limitCombat := 255 * multiplier
	for _, value := range []*int{&s.MaxHP, &s.HP, &s.MaxMP, &s.MP} {
		if *value > limitHPMP {
			*value = limitHPMP
		}
	}
	for _, value := range []*int{&s.Attack, &s.Defense, &s.Agility} {
		if *value > limitCombat {
			*value = limitCombat
		}
	}
}

type RandomSource interface {
	Intn(max int) (int, error)
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Intn(max int) (int, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func New(name string) (Character, error) {
	return NewWithOptions(name, DefaultJobID, DefaultGender, cryptoRandomSource{})
}

func NewWithOptions(name, jobID, gender string, random RandomSource) (Character, error) {
	if !utf8.ValidString(name) || containsControl(name) {
		return Character{}, ErrInvalidName
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Character{}, ErrInvalidName
	}
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(gender) == "" {
		return Character{}, errors.New("job and gender are required")
	}
	if random == nil {
		random = cryptoRandomSource{}
	}

	stats, err := initialStats(random)
	if err != nil {
		return Character{}, fmt.Errorf("generate initial stats: %w", err)
	}

	return Character{
		ID:     id.New(),
		Name:   name,
		JobID:  strings.TrimSpace(jobID),
		Gender: strings.TrimSpace(gender),
		Stats:  stats,
		Money:  InitialMoney,
		Level:  InitialLevel,
		Color:  DefaultColor,
	}, nil
}

func initialStats(random RandomSource) (Stats, error) {
	const randomRange = 3
	next := func(base int) (int, error) {
		value, err := random.Intn(randomRange)
		if err != nil {
			return 0, err
		}
		return base + value, nil
	}

	maxHP, err := next(30)
	if err != nil {
		return Stats{}, err
	}
	maxMP, err := next(6)
	if err != nil {
		return Stats{}, err
	}
	attack, err := next(6)
	if err != nil {
		return Stats{}, err
	}
	defense, err := next(6)
	if err != nil {
		return Stats{}, err
	}
	agility, err := next(6)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		MaxHP:   maxHP,
		MaxMP:   maxMP,
		HP:      maxHP,
		MP:      maxMP,
		Attack:  attack,
		Defense: defense,
		Agility: agility,
	}, nil
}

func containsControl(name string) bool {
	for _, r := range name {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validName(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}
	length := utf8.RuneCountInString(name)
	if length < 1 || length > maxNameLength {
		return false
	}
	return !containsControl(name)
}

// AddMoney safely credits currency to the character, capping at MaxMoney and guarding against negative amounts.
func (c *Character) AddMoney(amount int) error {
	if amount < 0 {
		return ErrInvalidAmount
	}
	if c.Money > MaxMoney-amount {
		c.Money = MaxMoney
		return nil
	}
	c.Money += amount
	return nil
}

// DeductMoney safely subtracts currency from the character, ensuring non-negative balance.
func (c *Character) DeductMoney(amount int) error {
	if amount < 0 {
		return ErrInvalidAmount
	}
	if c.Money < amount {
		return ErrInsufficientFunds
	}
	c.Money -= amount
	return nil
}

// HasMoney returns true if the character has at least the specified amount of money.
func (c *Character) HasMoney(amount int) bool {
	return amount >= 0 && c.Money >= amount
}

// AddSmallMedals safely credits small medals to the character, capping at MaxSmallMedals and guarding against negative amounts.
func (c *Character) AddSmallMedals(amount int) error {
	if amount < 0 {
		return ErrInvalidAmount
	}
	if c.SmallMedals > MaxSmallMedals-amount {
		c.SmallMedals = MaxSmallMedals
		return nil
	}
	c.SmallMedals += amount
	return nil
}

// DeductSmallMedals safely subtracts small medals from the character, ensuring non-negative balance.
func (c *Character) DeductSmallMedals(amount int) error {
	if amount < 0 {
		return ErrInvalidAmount
	}
	if c.SmallMedals < amount {
		return ErrInsufficientMedals
	}
	c.SmallMedals -= amount
	return nil
}

// HasSmallMedals returns true if the character has at least the specified amount of small medals.
func (c *Character) HasSmallMedals(amount int) bool {
	return amount >= 0 && c.SmallMedals >= amount
}

// AddSP safely credits skill points to the character, guarding against negative amounts.
func (c *Character) AddSP(amount int) error {
	if amount < 0 {
		return ErrInvalidAmount
	}
	c.SP += amount
	return nil
}

// ApplyJobChange applies the legacy job-change reset and resource transfer.
func (c *Character) ApplyJobChange(targetJobID string, targetSP int) error {
	if c == nil || strings.TrimSpace(c.JobID) == "" || strings.TrimSpace(targetJobID) == "" {
		return errors.New("job change requires current and target jobs")
	}
	if targetSP < 0 {
		return ErrInvalidAmount
	}
	if targetJobID != c.JobID {
		c.OldJobID = c.JobID
		c.OldSP = c.SP
		c.JobID = strings.TrimSpace(targetJobID)
		c.SP = targetSP
	}
	for _, value := range []*int{&c.Stats.MaxHP, &c.Stats.MaxMP, &c.Stats.Attack, &c.Stats.Defense, &c.Stats.Agility} {
		*value /= 2
		if *value < 10 {
			*value = 10
		}
	}
	c.Stats.HP = c.Stats.MaxHP
	c.Stats.MP = c.Stats.MaxMP
	c.Level = InitialLevel
	c.Experience = 0
	c.JobLevel++
	c.OverLevel = false
	return nil
}

// ApplyJobMemory switches to a remembered job pair without applying the
// level/stat penalty used by a normal job change.
func (c *Character) ApplyJobMemory(jobID string, sp int, oldJobID string, oldSP int) error {
	if c == nil || strings.TrimSpace(jobID) == "" || strings.TrimSpace(oldJobID) == "" ||
		sp < 0 || oldSP < 0 {
		return errors.New("job memory is invalid")
	}
	c.JobID, c.SP = strings.TrimSpace(jobID), sp
	c.OldJobID, c.OldSP = strings.TrimSpace(oldJobID), oldSP
	return nil
}

// CanSaveFutureMemory checks whether a future memory snapshot can be saved.
// It requires that no temporary job exchange is active, and that existing snapshots
// do not exceed the OverFuture capacity limit (0 allows 1 snapshot).
func (c *Character) CanSaveFutureMemory(currentSnapshotCount int) bool {
	if c == nil || c.JobMemory != nil {
		return false
	}
	return currentSnapshotCount <= c.OverFuture
}

// CreateFutureMemory builds a future memory snapshot from current character state.
func (c *Character) CreateFutureMemory(id string, createdAt time.Time) FutureMemory {
	return FutureMemory{
		ID:          id,
		CharacterID: c.ID,
		JobID:       c.JobID,
		OldJobID:    c.OldJobID,
		Level:       c.Level,
		Experience:  c.Experience,
		MaxHP:       c.Stats.MaxHP,
		MaxMP:       c.Stats.MaxMP,
		Attack:      c.Stats.Attack,
		Defense:     c.Stats.Defense,
		Agility:     c.Stats.Agility,
		Gender:      c.Gender,
		OverLevel:   c.OverLevel,
		CreatedAt:   createdAt,
	}
}

// ApplyFutureMemory restores the character's state from a future memory snapshot
// and resets current HP/MP to their restored maxima, setting SP and OldSP from mastery data.
func (c *Character) ApplyFutureMemory(memory FutureMemory, currentSP, oldSP int) error {
	if c == nil || strings.TrimSpace(memory.JobID) == "" {
		return errors.New("future memory is invalid")
	}
	if currentSP < 0 || oldSP < 0 {
		return ErrInvalidAmount
	}
	c.JobID = memory.JobID
	c.OldJobID = memory.OldJobID
	c.Level = memory.Level
	c.Experience = memory.Experience
	c.Stats.MaxHP = memory.MaxHP
	c.Stats.HP = memory.MaxHP
	c.Stats.MaxMP = memory.MaxMP
	c.Stats.MP = memory.MaxMP
	c.Stats.Attack = memory.Attack
	c.Stats.Defense = memory.Defense
	c.Stats.Agility = memory.Agility
	c.Gender = memory.Gender
	c.OverLevel = memory.OverLevel
	c.SP = currentSP
	c.OldSP = oldSP
	return nil
}

// ResetTired resets character fatigue to 0 upon sleep or full recovery.
func (c *Character) ResetTired() {
	if c != nil {
		c.Tired = 0
	}
}

// AddTired adds fatigue percentage to character.
func (c *Character) AddTired(delta int) {
	if c != nil {
		c.Tired += delta
	}
}

// ReduceTired decreases character fatigue by delta percentage (e.g. celestial wishes).
func (c *Character) ReduceTired(delta int) {
	if c != nil {
		c.Tired -= delta
	}
}

// IsExhausted returns true if character fatigue is 100% or higher.
func (c *Character) IsExhausted() bool {
	if c == nil {
		return false
	}
	return c.Tired >= 100
}

// RevertJobMemory restores character's original job and SP from temporary JobMemory if present.
func (c *Character) RevertJobMemory() bool {
	if c == nil || c.JobMemory == nil {
		return false
	}
	memory := *c.JobMemory
	_ = c.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP)
	c.JobMemory = nil
	return true
}

// IsValidColor checks if a string is a valid #RRGGBB hex color code.
func IsValidColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		b := color[i]
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
			return false
		}
	}
	return true
}

// SetColor updates the character's chat and display color (#RRGGBB).
func (c *Character) SetColor(color string) error {
	clean := strings.TrimSpace(color)
	if !IsValidColor(clean) {
		return ErrInvalidColor
	}
	c.Color = strings.ToLower(clean)
	return nil
}

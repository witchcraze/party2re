package character

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/id"
)

const maxNameLength = 32

const (
	DefaultJobID  = "starter"
	DefaultGender = "unspecified"
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
	SmallMedals int
	HelpCount   int
	Orb         string // Orb collection status: string containing characters 's','r','b','g','y','p', or 'G' (Ramia awakened)
	OverLevel   bool
	OverDepot   int
	OverMonster int
	OverFuture  int
	OverFlea    int
	OverStore   int
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

// Orb symbols conforming to legacy Party2 (reborn.cgi / _data.cgi).
const (
	OrbSilver = 's' // 月曜日 / シルバーオーブ
	OrbRed    = 'r' // 火曜日 / レッドオーブ
	OrbBlue   = 'b' // 水曜日 / ブルーオーブ
	OrbGreen  = 'g' // 木曜日 / グリーンオーブ
	OrbYellow = 'y' // 金曜日 / イエローオーブ
	OrbPurple = 'p' // 土曜日 / パープルオーブ
	OrbGold   = 'G' // 不死鳥ラーミァ復活完了状態
)

var (
	ErrInvalidOrbRune = errors.New("invalid orb rune")
)

var ValidOrbRunes = []rune{OrbSilver, OrbRed, OrbBlue, OrbGreen, OrbYellow, OrbPurple}

func IsValidOrbRune(r rune) bool {
	for _, orb := range ValidOrbRunes {
		if orb == r {
			return true
		}
	}
	return false
}

// HasOrb checks whether character has offered or collected the specified orb color.
func (c *Character) HasOrb(orb rune) bool {
	return strings.ContainsRune(c.Orb, orb)
}

// OrbCount returns number of distinct standard orbs (s, r, b, g, y, p) collected.
func (c *Character) OrbCount() int {
	count := 0
	for _, orb := range ValidOrbRunes {
		if strings.ContainsRune(c.Orb, orb) {
			count++
		}
	}
	return count
}

// HasAllOrbs returns true if all 6 standard orbs are collected.
func (c *Character) HasAllOrbs() bool {
	return c.OrbCount() == len(ValidOrbRunes)
}

// IsRamiaAwakened returns true if Ramia has awakened (Orb contains 'G').
func (c *Character) IsRamiaAwakened() bool {
	return strings.ContainsRune(c.Orb, OrbGold)
}

// AddOrb adds an orb symbol to character's collection. Returns false if already collected.
func (c *Character) AddOrb(orb rune) (bool, error) {
	if !IsValidOrbRune(orb) {
		return false, ErrInvalidOrbRune
	}
	if c.HasOrb(orb) {
		return false, nil
	}
	c.Orb += string(orb)
	return true, nil
}

// SetRamiaAwakened sets orb state to 'G', indicating Ramia is revived.
func (c *Character) SetRamiaAwakened() {
	c.Orb = string(OrbGold)
}

// ClearOrbs resets character's orb status to empty string.
func (c *Character) ClearOrbs() {
	c.Orb = ""
}

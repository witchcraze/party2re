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
	ID           string
	PlayerID     string
	Name         string
	JobID        string
	Gender       string
	Stats        Stats
	Money        int
	Level        int
	Experience   int
	RebirthCount int
	SmallMedals  int
	HelpCount    int
	OverLevel    bool
	OverDepot    int
	OverMonster  int
	OverFuture   int
	OverFlea     int
	OverStore    int
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

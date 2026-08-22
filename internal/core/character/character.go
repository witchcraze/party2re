package character

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNameLength = 32

const (
	DefaultJobID  = "starter"
	DefaultGender = "unspecified"
	InitialMoney  = 200
	InitialLevel  = 1
)

var (
	ErrInvalidName = errors.New("character name must be between 1 and 32 characters")
	ErrNotFound    = errors.New("character not found")
)

type Character struct {
	ID         string
	Name       string
	JobID      string
	Gender     string
	Stats      Stats
	Money      int
	Level      int
	Experience int
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

func (c *Character) AddExperience(value int) error {
	if value < 0 {
		return errors.New("experience cannot be negative")
	}
	c.Experience += value
	return nil
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

	id, err := newID()
	if err != nil {
		return Character{}, fmt.Errorf("generate character ID: %w", err)
	}

	return Character{
		ID:     id,
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

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

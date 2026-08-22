package character

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNameLength = 32

var (
	ErrInvalidName = errors.New("character name must be between 1 and 32 characters")
	ErrNotFound    = errors.New("character not found")
)

type Character struct {
	ID         string
	Name       string
	Level      int
	Experience int
}

func New(name string) (Character, error) {
	if !utf8.ValidString(name) || containsControl(name) {
		return Character{}, ErrInvalidName
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Character{}, ErrInvalidName
	}

	id, err := newID()
	if err != nil {
		return Character{}, fmt.Errorf("generate character ID: %w", err)
	}

	return Character{ID: id, Name: name, Level: 1}, nil
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

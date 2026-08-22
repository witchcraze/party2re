package item

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrInvalidDefinition = errors.New("item definition is invalid")
	ErrInvalidInstance   = errors.New("item instance is invalid")
)

type Definition struct {
	ID   string
	Name string
}

func NewDefinition(id, name string) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name)}, nil
}

type Instance struct {
	ID           string
	DefinitionID string
	Quantity     int
}

func NewInstance(definitionID string, quantity int) (Instance, error) {
	if strings.TrimSpace(definitionID) == "" || quantity <= 0 {
		return Instance{}, ErrInvalidInstance
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Instance{}, err
	}
	return Instance{
		ID:           hex.EncodeToString(id),
		DefinitionID: strings.TrimSpace(definitionID),
		Quantity:     quantity,
	}, nil
}

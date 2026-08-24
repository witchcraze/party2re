package collection

import (
	"context"
	"errors"
	"time"
)

const (
	DefaultTotalMonsters = 286
	DefaultTotalItems    = 150
)

var (
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
	ErrInvalidMonsterID   = errors.New("monster ID cannot be empty")
	ErrInvalidItemID      = errors.New("item ID cannot be empty")
)

type MonsterBookEntry struct {
	CharacterID     string    `json:"character_id"`
	MonsterID       string    `json:"monster_id"`
	MonsterName     string    `json:"monster_name"`
	Habitat         string    `json:"habitat"`
	DefeatedCount   int       `json:"defeated_count"`
	FirstDefeatedAt time.Time `json:"first_defeated_at"`
	LastDefeatedAt  time.Time `json:"last_defeated_at"`
}

type ItemCollectionEntry struct {
	CharacterID  string    `json:"character_id"`
	ItemID       string    `json:"item_id"`
	ItemName     string    `json:"item_name"`
	Category     string    `json:"category"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

type CompletionProgress struct {
	DiscoveredCount      int     `json:"discovered_count"`
	TotalCatalogCount    int     `json:"total_catalog_count"`
	CompletionPercentage float64 `json:"completion_percentage"`
}

type Repository interface {
	RecordMonsterDefeat(ctx context.Context, characterID, monsterID, monsterName, habitat string) error
	GetMonsterBook(ctx context.Context, characterID string) ([]MonsterBookEntry, error)
	GetMonsterBookCount(ctx context.Context, characterID string) (int, error)

	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
	GetItemCollection(ctx context.Context, characterID, category string) ([]ItemCollectionEntry, error)
	GetItemCollectionCount(ctx context.Context, characterID string) (int, error)
}

type Service struct {
	repo          Repository
	totalMonsters int
	totalItems    int
}

func NewService(repo Repository, totalMonsters, totalItems int) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	if totalMonsters <= 0 {
		totalMonsters = DefaultTotalMonsters
	}
	if totalItems <= 0 {
		totalItems = DefaultTotalItems
	}
	return &Service{
		repo:          repo,
		totalMonsters: totalMonsters,
		totalItems:    totalItems,
	}, nil
}

func (s *Service) RecordMonsterDefeat(ctx context.Context, characterID, monsterID, monsterName, habitat string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if monsterID == "" {
		return ErrInvalidMonsterID
	}
	return s.repo.RecordMonsterDefeat(ctx, characterID, monsterID, monsterName, habitat)
}

func (s *Service) GetMonsterBook(ctx context.Context, characterID string) ([]MonsterBookEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	entries, err := s.repo.GetMonsterBook(ctx, characterID)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	discovered := len(entries)
	percentage := 0.0
	if s.totalMonsters > 0 {
		percentage = (float64(discovered) / float64(s.totalMonsters)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      discovered,
		TotalCatalogCount:    s.totalMonsters,
		CompletionPercentage: percentage,
	}
	return entries, progress, nil
}

func (s *Service) RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if itemID == "" {
		return ErrInvalidItemID
	}
	return s.repo.RecordItemDiscovered(ctx, characterID, itemID, itemName, category)
}

func (s *Service) GetItemCollection(ctx context.Context, characterID, category string) ([]ItemCollectionEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	entries, err := s.repo.GetItemCollection(ctx, characterID, category)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	totalDiscovered, err := s.repo.GetItemCollectionCount(ctx, characterID)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	percentage := 0.0
	if s.totalItems > 0 {
		percentage = (float64(totalDiscovered) / float64(s.totalItems)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      totalDiscovered,
		TotalCatalogCount:    s.totalItems,
		CompletionPercentage: percentage,
	}
	return entries, progress, nil
}

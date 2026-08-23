package bank

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidPlayerID     = errors.New("invalid player ID")
	ErrInvalidCharacterID  = errors.New("invalid character ID")
	ErrInvalidAmount       = errors.New("amount must be positive")
	ErrInsufficientFunds   = errors.New("insufficient character funds")
	ErrInsufficientBalance = errors.New("insufficient bank balance")
	ErrSelfTransfer        = errors.New("cannot transfer gold to self")
	ErrAccountNotFound     = errors.New("bank account not found")
)

type Account struct {
	PlayerID  string    `json:"player_id"`
	Balance   int64     `json:"balance"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransferRecord struct {
	ID           string    `json:"id"`
	FromPlayerID string    `json:"from_player_id"`
	ToPlayerID   string    `json:"to_player_id"`
	Amount       int64     `json:"amount"`
	CreatedAt    time.Time `json:"created_at"`
}

type Repository interface {
	Deposit(ctx context.Context, playerID string, characterID string, amount int) (Account, corecharacter.Character, error)
	Withdraw(ctx context.Context, playerID string, characterID string, amount int) (Account, corecharacter.Character, error)
	Transfer(ctx context.Context, record TransferRecord) (from Account, to Account, err error)
	GetAccount(ctx context.Context, playerID string) (Account, error)
	ListTransfers(ctx context.Context, playerID string, limit int) ([]TransferRecord, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("bank repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) GetAccount(ctx context.Context, playerID string) (Account, error) {
	if strings.TrimSpace(playerID) == "" {
		return Account{}, ErrInvalidPlayerID
	}
	return s.repository.GetAccount(ctx, strings.TrimSpace(playerID))
}

func (s *Service) Deposit(ctx context.Context, playerID string, characterID string, amount int) (Account, corecharacter.Character, error) {
	if strings.TrimSpace(playerID) == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidPlayerID
	}
	if strings.TrimSpace(characterID) == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return Account{}, corecharacter.Character{}, ErrInvalidAmount
	}
	return s.repository.Deposit(ctx, strings.TrimSpace(playerID), strings.TrimSpace(characterID), amount)
}

func (s *Service) Withdraw(ctx context.Context, playerID string, characterID string, amount int) (Account, corecharacter.Character, error) {
	if strings.TrimSpace(playerID) == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidPlayerID
	}
	if strings.TrimSpace(characterID) == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return Account{}, corecharacter.Character{}, ErrInvalidAmount
	}
	return s.repository.Withdraw(ctx, strings.TrimSpace(playerID), strings.TrimSpace(characterID), amount)
}

func (s *Service) Transfer(ctx context.Context, fromPlayerID string, toPlayerID string, amount int64) (Account, Account, TransferRecord, error) {
	fromPlayerID = strings.TrimSpace(fromPlayerID)
	toPlayerID = strings.TrimSpace(toPlayerID)
	if fromPlayerID == "" || toPlayerID == "" {
		return Account{}, Account{}, TransferRecord{}, ErrInvalidPlayerID
	}
	if fromPlayerID == toPlayerID {
		return Account{}, Account{}, TransferRecord{}, ErrSelfTransfer
	}
	if amount <= 0 {
		return Account{}, Account{}, TransferRecord{}, ErrInvalidAmount
	}

	recordID, err := generateID()
	if err != nil {
		return Account{}, Account{}, TransferRecord{}, err
	}

	record := TransferRecord{
		ID:           recordID,
		FromPlayerID: fromPlayerID,
		ToPlayerID:   toPlayerID,
		Amount:       amount,
		CreatedAt:    time.Now().UTC(),
	}

	fromAcc, toAcc, err := s.repository.Transfer(ctx, record)
	if err != nil {
		return Account{}, Account{}, TransferRecord{}, err
	}

	return fromAcc, toAcc, record, nil
}

func (s *Service) ListTransfers(ctx context.Context, playerID string, limit int) ([]TransferRecord, error) {
	if strings.TrimSpace(playerID) == "" {
		return nil, ErrInvalidPlayerID
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repository.ListTransfers(ctx, strings.TrimSpace(playerID), limit)
}

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

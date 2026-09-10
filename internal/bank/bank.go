package bank

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	// MaxDeposit is the maximum deposit allowed in the bank (99兆9999億9999万9999 G).
	MaxDeposit int64 = 99_999_999_999_999
	// MaxWallet is the maximum gold held in the character wallet upon bank withdrawal (999,999 G).
	MaxWallet int = 999_999
	// NPCName is the Taxeed bank receptionist.
	NPCName = "@タクシード"
)

// NPCDialogues lists standard bank greeting and service dialogues.
var NPCDialogues = []string{
	"ゴールドをお預かりいたします",
	"24時間いつでも、手数料もございません",
	"99999999999999 Gまでお預かりいたします",
}

var (
	ErrInvalidCharacterID   = errors.New("invalid character ID")
	ErrInvalidAmount        = errors.New("amount must be positive")
	ErrInsufficientFunds    = errors.New("insufficient character funds")
	ErrInsufficientBalance  = errors.New("insufficient bank balance")
	ErrDepositLimitExceeded = errors.New("これ以上お預かりできません")
)

// State represents the current bank and wallet status of a character.
type State struct {
	CharacterID string   `json:"character_id"`
	Money       int      `json:"money"`
	Deposit     int64    `json:"deposit"`
	MaxDeposit  int64    `json:"max_deposit"`
	NPCName     string   `json:"npc_name"`
	Dialogues   []string `json:"dialogues"`
}

// DepositResult represents the result of depositing gold into the bank.
type DepositResult struct {
	CharacterID string `json:"character_id"`
	Money       int    `json:"money"`
	Deposit     int64  `json:"deposit"`
	Amount      int64  `json:"amount"`
	Message     string `json:"message"`
}

// WithdrawResult represents the result of withdrawing gold from the bank.
type WithdrawResult struct {
	CharacterID     string `json:"character_id"`
	Money           int    `json:"money"`
	Deposit         int64  `json:"deposit"`
	Amount          int64  `json:"amount"`
	ActualWithdrawn int    `json:"actual_withdrawn"`
	Refunded        int64  `json:"refunded"`
	Message         string `json:"message"`
}

// NPCInfo represents NPC receptionist metadata.
type NPCInfo struct {
	Name      string   `json:"name"`
	Dialogues []string `json:"dialogues"`
}

// Repository defines the storage contract for character bank accounts.
type Repository interface {
	GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error)
	Deposit(ctx context.Context, characterID string, amount int64) (corecharacter.Character, error)
	Withdraw(ctx context.Context, characterID string, amount int64) (char corecharacter.Character, actualWithdrawn int, refunded int64, err error)
}

// Service provides bank operations aligned with Party2 specifications.
type Service struct {
	repository Repository
}

// NewService creates a new bank Service instance.
func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("bank repository is nil")
	}
	return &Service{repository: repository}, nil
}

// CalculateDeposit performs pure calculation and validation for depositing gold.
func CalculateDeposit(currentMoney int, currentDeposit int64, amount int64) (newMoney int, newDeposit int64, err error) {
	if amount <= 0 {
		return 0, 0, ErrInvalidAmount
	}
	if int64(currentMoney) < amount {
		return 0, 0, ErrInsufficientFunds
	}
	if currentDeposit > MaxDeposit-amount {
		return 0, 0, ErrDepositLimitExceeded
	}
	return currentMoney - int(amount), currentDeposit + amount, nil
}

// CalculateWithdrawal performs pure calculation and validation for withdrawing gold,
// enforcing the 999,999 G wallet clamp and refunding excess gold back to deposit.
func CalculateWithdrawal(currentMoney int, currentDeposit int64, amount int64) (newMoney int, newDeposit int64, actualWithdrawn int, refunded int64, err error) {
	if amount <= 0 {
		return 0, 0, 0, 0, ErrInvalidAmount
	}
	if currentDeposit < amount {
		return 0, 0, 0, 0, ErrInsufficientBalance
	}

	needed := MaxWallet - currentMoney
	if needed <= 0 {
		clampedMoney := currentMoney
		if clampedMoney > MaxWallet {
			clampedMoney = MaxWallet
		}
		return clampedMoney, currentDeposit, 0, amount, nil
	}

	if amount > int64(needed) {
		actualWithdrawn = needed
		refunded = amount - int64(needed)
		newMoney = MaxWallet
		newDeposit = currentDeposit - int64(needed)
		return newMoney, newDeposit, actualWithdrawn, refunded, nil
	}

	actualWithdrawn = int(amount)
	refunded = 0
	newMoney = currentMoney + actualWithdrawn
	newDeposit = currentDeposit - amount
	return newMoney, newDeposit, actualWithdrawn, refunded, nil
}

// GetState returns the current bank and wallet status for the character.
func (s *Service) GetState(ctx context.Context, characterID string) (State, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return State{}, ErrInvalidCharacterID
	}
	char, err := s.repository.GetCharacter(ctx, characterID)
	if err != nil {
		return State{}, err
	}
	return State{
		CharacterID: char.ID,
		Money:       char.Money,
		Deposit:     char.Deposit,
		MaxDeposit:  MaxDeposit,
		NPCName:     NPCName,
		Dialogues:   NPCDialogues,
	}, nil
}

// Deposit deposits the specified amount of gold from the character's wallet into their bank savings.
func (s *Service) Deposit(ctx context.Context, characterID string, amount int64) (DepositResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return DepositResult{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return DepositResult{}, ErrInvalidAmount
	}
	char, err := s.repository.Deposit(ctx, characterID, amount)
	if err != nil {
		return DepositResult{}, err
	}
	return DepositResult{
		CharacterID: char.ID,
		Money:       char.Money,
		Deposit:     char.Deposit,
		Amount:      amount,
		Message:     fmt.Sprintf("%d Gお預かりいたしました", amount),
	}, nil
}

// Withdraw withdraws gold from the character's bank savings into their wallet,
// clamping wallet gold at 999,999 G and refunding any excess back to deposit.
func (s *Service) Withdraw(ctx context.Context, characterID string, amount int64) (WithdrawResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return WithdrawResult{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return WithdrawResult{}, ErrInvalidAmount
	}
	char, actualWithdrawn, refunded, err := s.repository.Withdraw(ctx, characterID, amount)
	if err != nil {
		return WithdrawResult{}, err
	}
	return WithdrawResult{
		CharacterID:     char.ID,
		Money:           char.Money,
		Deposit:         char.Deposit,
		Amount:          amount,
		ActualWithdrawn: actualWithdrawn,
		Refunded:        refunded,
		Message:         fmt.Sprintf("%d Gお返しいたします", amount),
	}, nil
}

// InspectNPC returns metadata regarding the Taxeed bank receptionist.
func (s *Service) InspectNPC() NPCInfo {
	return NPCInfo{
		Name:      NPCName,
		Dialogues: NPCDialogues,
	}
}

// TalkNPC returns a random greeting dialogue from the bank receptionist.
func (s *Service) TalkNPC() string {
	if len(NPCDialogues) == 0 {
		return ""
	}
	return NPCDialogues[rand.Intn(len(NPCDialogues))]
}

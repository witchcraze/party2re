package bank_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockRepository struct {
	accounts map[string]bank.Account
	chars    map[string]corecharacter.Character
	records  []bank.TransferRecord
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		accounts: make(map[string]bank.Account),
		chars:    make(map[string]corecharacter.Character),
		records:  make([]bank.TransferRecord, 0),
	}
}

func (m *mockRepository) GetAccount(_ context.Context, playerID string) (bank.Account, error) {
	acc, ok := m.accounts[playerID]
	if !ok {
		return bank.Account{PlayerID: playerID, Balance: 0, UpdatedAt: time.Now()}, nil
	}
	return acc, nil
}

func (m *mockRepository) Deposit(_ context.Context, playerID string, characterID string, amount int) (bank.Account, corecharacter.Character, error) {
	char, ok := m.chars[characterID]
	if !ok {
		return bank.Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if char.Money < amount {
		return bank.Account{}, corecharacter.Character{}, bank.ErrInsufficientFunds
	}
	char.Money -= amount
	m.chars[characterID] = char

	acc := m.accounts[playerID]
	acc.PlayerID = playerID
	acc.Balance += int64(amount)
	acc.UpdatedAt = time.Now()
	m.accounts[playerID] = acc

	return acc, char, nil
}

func (m *mockRepository) Withdraw(_ context.Context, playerID string, characterID string, amount int) (bank.Account, corecharacter.Character, error) {
	char, ok := m.chars[characterID]
	if !ok {
		return bank.Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
	}
	acc := m.accounts[playerID]
	if acc.Balance < int64(amount) {
		return bank.Account{}, corecharacter.Character{}, bank.ErrInsufficientBalance
	}
	acc.Balance -= int64(amount)
	acc.UpdatedAt = time.Now()
	m.accounts[playerID] = acc

	char.Money += amount
	m.chars[characterID] = char

	return acc, char, nil
}

func (m *mockRepository) Transfer(_ context.Context, record bank.TransferRecord) (bank.Account, bank.Account, error) {
	fromAcc, ok := m.accounts[record.FromPlayerID]
	if !ok || fromAcc.Balance < record.Amount {
		return bank.Account{}, bank.Account{}, bank.ErrInsufficientBalance
	}
	toAcc, ok := m.accounts[record.ToPlayerID]
	if !ok {
		return bank.Account{}, bank.Account{}, bank.ErrAccountNotFound
	}

	fromAcc.Balance -= record.Amount
	fromAcc.UpdatedAt = time.Now()
	m.accounts[record.FromPlayerID] = fromAcc

	toAcc.Balance += record.Amount
	toAcc.UpdatedAt = time.Now()
	m.accounts[record.ToPlayerID] = toAcc

	m.records = append(m.records, record)
	return fromAcc, toAcc, nil
}

func (m *mockRepository) ListTransfers(_ context.Context, playerID string, limit int) ([]bank.TransferRecord, error) {
	var result []bank.TransferRecord
	for _, r := range m.records {
		if r.FromPlayerID == playerID || r.ToPlayerID == playerID {
			result = append(result, r)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func TestBankServiceDeposit(t *testing.T) {
	repo := newMockRepository()
	char, _ := corecharacter.New("Depositor")
	char.Money = 500
	repo.chars[char.ID] = char

	service, _ := bank.NewService(repo)

	// Insufficient funds
	_, _, err := service.Deposit(context.Background(), "player-1", char.ID, 600)
	if !errors.Is(err, bank.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// Valid deposit
	acc, updatedChar, err := service.Deposit(context.Background(), "player-1", char.ID, 300)
	if err != nil {
		t.Fatalf("Deposit error: %v", err)
	}
	if acc.Balance != 300 {
		t.Errorf("bank balance = %d, want 300", acc.Balance)
	}
	if updatedChar.Money != 200 {
		t.Errorf("character wallet = %d, want 200", updatedChar.Money)
	}
}

func TestBankServiceWithdraw(t *testing.T) {
	repo := newMockRepository()
	char, _ := corecharacter.New("Withdrawer")
	char.Money = 100
	repo.chars[char.ID] = char
	repo.accounts["player-1"] = bank.Account{PlayerID: "player-1", Balance: 500}

	service, _ := bank.NewService(repo)

	// Overdraft
	_, _, err := service.Withdraw(context.Background(), "player-1", char.ID, 600)
	if !errors.Is(err, bank.ErrInsufficientBalance) {
		t.Errorf("expected ErrInsufficientBalance, got %v", err)
	}

	// Valid withdrawal
	acc, updatedChar, err := service.Withdraw(context.Background(), "player-1", char.ID, 200)
	if err != nil {
		t.Fatalf("Withdraw error: %v", err)
	}
	if acc.Balance != 300 {
		t.Errorf("bank balance = %d, want 300", acc.Balance)
	}
	if updatedChar.Money != 300 {
		t.Errorf("character wallet = %d, want 300", updatedChar.Money)
	}
}

func TestBankServiceTransfer(t *testing.T) {
	repo := newMockRepository()
	repo.accounts["player-alice"] = bank.Account{PlayerID: "player-alice", Balance: 1000}
	repo.accounts["player-bob"] = bank.Account{PlayerID: "player-bob", Balance: 100}

	service, _ := bank.NewService(repo)

	// Cannot transfer to self
	_, _, _, err := service.Transfer(context.Background(), "player-alice", "player-alice", 100)
	if !errors.Is(err, bank.ErrSelfTransfer) {
		t.Errorf("expected ErrSelfTransfer, got %v", err)
	}

	// Valid transfer
	fromAcc, toAcc, rec, err := service.Transfer(context.Background(), "player-alice", "player-bob", 400)
	if err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	if fromAcc.Balance != 600 {
		t.Errorf("from balance = %d, want 600", fromAcc.Balance)
	}
	if toAcc.Balance != 500 {
		t.Errorf("to balance = %d, want 500", toAcc.Balance)
	}
	if rec.Amount != 400 || rec.FromPlayerID != "player-alice" || rec.ToPlayerID != "player-bob" {
		t.Errorf("unexpected record: %#v", rec)
	}
}

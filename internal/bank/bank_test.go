package bank_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockRepository struct {
	chars map[string]corecharacter.Character
	err   error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		chars: make(map[string]corecharacter.Character),
	}
}

func (m *mockRepository) GetCharacter(_ context.Context, characterID string) (corecharacter.Character, error) {
	if m.err != nil {
		return corecharacter.Character{}, m.err
	}
	char, ok := m.chars[characterID]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return char, nil
}

func (m *mockRepository) Deposit(_ context.Context, characterID string, amount int64) (corecharacter.Character, error) {
	if m.err != nil {
		return corecharacter.Character{}, m.err
	}
	char, ok := m.chars[characterID]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	newMoney, newDeposit, err := bank.CalculateDeposit(char.Money, char.Deposit, amount)
	if err != nil {
		return corecharacter.Character{}, err
	}
	char.Money = newMoney
	char.Deposit = newDeposit
	m.chars[characterID] = char
	return char, nil
}

func (m *mockRepository) Withdraw(_ context.Context, characterID string, amount int64) (corecharacter.Character, int, int64, error) {
	if m.err != nil {
		return corecharacter.Character{}, 0, 0, m.err
	}
	char, ok := m.chars[characterID]
	if !ok {
		return corecharacter.Character{}, 0, 0, corecharacter.ErrNotFound
	}
	newMoney, newDeposit, actualWithdrawn, refunded, err := bank.CalculateWithdrawal(char.Money, char.Deposit, amount)
	if err != nil {
		return corecharacter.Character{}, 0, 0, err
	}
	char.Money = newMoney
	char.Deposit = newDeposit
	m.chars[characterID] = char
	return char, actualWithdrawn, refunded, nil
}

func TestCalculateDeposit(t *testing.T) {
	t.Run("valid deposit", func(t *testing.T) {
		newMoney, newDeposit, err := bank.CalculateDeposit(5000, 10000, 3000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newMoney != 2000 {
			t.Errorf("newMoney = %d, want 2000", newMoney)
		}
		if newDeposit != 13000 {
			t.Errorf("newDeposit = %d, want 13000", newDeposit)
		}
	})

	t.Run("invalid amount <= 0", func(t *testing.T) {
		_, _, err := bank.CalculateDeposit(5000, 10000, 0)
		if !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
		_, _, err = bank.CalculateDeposit(5000, 10000, -100)
		if !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		_, _, err := bank.CalculateDeposit(1000, 10000, 1500)
		if !errors.Is(err, bank.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
	})

	t.Run("exceeds max deposit", func(t *testing.T) {
		_, _, err := bank.CalculateDeposit(100000, bank.MaxDeposit-50, 100)
		if !errors.Is(err, bank.ErrDepositLimitExceeded) {
			t.Errorf("expected ErrDepositLimitExceeded, got %v", err)
		}
	})
}

func TestCalculateWithdrawal(t *testing.T) {
	t.Run("normal withdrawal without reaching cap", func(t *testing.T) {
		newMoney, newDeposit, actual, refunded, err := bank.CalculateWithdrawal(10000, 50000, 5000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newMoney != 15000 {
			t.Errorf("newMoney = %d, want 15000", newMoney)
		}
		if newDeposit != 45000 {
			t.Errorf("newDeposit = %d, want 45000", newDeposit)
		}
		if actual != 5000 {
			t.Errorf("actual = %d, want 5000", actual)
		}
		if refunded != 0 {
			t.Errorf("refunded = %d, want 0", refunded)
		}
	})

	t.Run("withdrawal clamped to 999999 with excess refunded", func(t *testing.T) {
		newMoney, newDeposit, actual, refunded, err := bank.CalculateWithdrawal(800000, 500000, 300000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newMoney != bank.MaxWallet {
			t.Errorf("newMoney = %d, want %d", newMoney, bank.MaxWallet)
		}
		if newDeposit != 300001 {
			t.Errorf("newDeposit = %d, want 300001", newDeposit)
		}
		if actual != 199999 {
			t.Errorf("actual = %d, want 199999", actual)
		}
		if refunded != 100001 {
			t.Errorf("refunded = %d, want 100001", refunded)
		}
		if int64(newMoney)+newDeposit != int64(800000)+500000 {
			t.Errorf("gold conservation violated: %d vs %d", int64(newMoney)+newDeposit, int64(800000)+500000)
		}
	})

	t.Run("wallet already full at 999999", func(t *testing.T) {
		newMoney, newDeposit, actual, refunded, err := bank.CalculateWithdrawal(bank.MaxWallet, 500000, 50000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newMoney != bank.MaxWallet {
			t.Errorf("newMoney = %d, want %d", newMoney, bank.MaxWallet)
		}
		if newDeposit != 500000 {
			t.Errorf("newDeposit = %d, want 500000", newDeposit)
		}
		if actual != 0 {
			t.Errorf("actual = %d, want 0", actual)
		}
		if refunded != 50000 {
			t.Errorf("refunded = %d, want 50000", refunded)
		}
	})

	t.Run("invalid amount <= 0", func(t *testing.T) {
		_, _, _, _, err := bank.CalculateWithdrawal(1000, 5000, 0)
		if !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
		_, _, _, _, err = bank.CalculateWithdrawal(1000, 5000, -50)
		if !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("insufficient deposit balance", func(t *testing.T) {
		_, _, _, _, err := bank.CalculateWithdrawal(1000, 5000, 6000)
		if !errors.Is(err, bank.ErrInsufficientBalance) {
			t.Errorf("expected ErrInsufficientBalance, got %v", err)
		}
	})
}

func TestService_GetState(t *testing.T) {
	repo := newMockRepository()
	repo.chars["c1"] = corecharacter.Character{
		ID:      "c1",
		Money:   15000,
		Deposit: 300000,
	}

	service, err := bank.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		state, err := service.GetState(ctx, "c1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state.CharacterID != "c1" || state.Money != 15000 || state.Deposit != 300000 {
			t.Errorf("unexpected state: %+v", state)
		}
		if state.MaxDeposit != bank.MaxDeposit {
			t.Errorf("expected max deposit %d, got %d", bank.MaxDeposit, state.MaxDeposit)
		}
		if state.NPCName != bank.NPCName {
			t.Errorf("expected npc name %s, got %s", bank.NPCName, state.NPCName)
		}
		if len(state.Dialogues) != len(bank.NPCDialogues) {
			t.Errorf("unexpected dialogues length")
		}
	})

	t.Run("empty character ID", func(t *testing.T) {
		_, err := service.GetState(ctx, "  ")
		if !errors.Is(err, bank.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
	})

	t.Run("character not found", func(t *testing.T) {
		_, err := service.GetState(ctx, "nonexistent")
		if !errors.Is(err, corecharacter.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestService_DepositAndWithdraw(t *testing.T) {
	repo := newMockRepository()
	repo.chars["c1"] = corecharacter.Character{
		ID:      "c1",
		Money:   500000,
		Deposit: 200000,
	}

	service, err := bank.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("deposit success", func(t *testing.T) {
		res, err := service.Deposit(ctx, "c1", 100000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Money != 400000 || res.Deposit != 300000 {
			t.Errorf("unexpected deposit result: %+v", res)
		}
		if res.Message != "100000 Gお預かりいたしました" {
			t.Errorf("unexpected message: %s", res.Message)
		}
	})

	t.Run("deposit invalid inputs", func(t *testing.T) {
		if _, err := service.Deposit(ctx, "", 100); !errors.Is(err, bank.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
		if _, err := service.Deposit(ctx, "c1", 0); !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("withdraw with clamp", func(t *testing.T) {
		res, err := service.Withdraw(ctx, "c1", 250000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Money != 650000 || res.Deposit != 50000 {
			t.Errorf("unexpected withdraw result: %+v", res)
		}
		if res.ActualWithdrawn != 250000 || res.Refunded != 0 {
			t.Errorf("unexpected actual/refunded: actual=%d, refunded=%d", res.ActualWithdrawn, res.Refunded)
		}
		if res.Message != "250000 Gお返しいたします" {
			t.Errorf("unexpected message: %s", res.Message)
		}
	})

	t.Run("withdraw invalid inputs", func(t *testing.T) {
		if _, err := service.Withdraw(ctx, "", 100); !errors.Is(err, bank.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
		if _, err := service.Withdraw(ctx, "c1", -10); !errors.Is(err, bank.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}
	})
}

func TestService_NPC(t *testing.T) {
	repo := newMockRepository()
	service, _ := bank.NewService(repo)

	npc := service.InspectNPC()
	if npc.Name != bank.NPCName || len(npc.Dialogues) != 3 {
		t.Errorf("unexpected NPC info: %+v", npc)
	}

	talk := service.TalkNPC()
	if talk == "" {
		t.Error("expected non-empty talk dialogue")
	}
}

func TestNewService_NilRepository(t *testing.T) {
	_, err := bank.NewService(nil)
	if err == nil {
		t.Fatal("expected error for nil repository")
	}
}

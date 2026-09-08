package wishingwell_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

type mockCharacterRepo struct {
	chars     map[string]corecharacter.Character
	updateErr error
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{
		chars: make(map[string]corecharacter.Character),
	}
}

func (m *mockCharacterRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(_ context.Context, value corecharacter.Character) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.chars[value.ID] = value
	return nil
}

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestWishingWell_GetStatus(t *testing.T) {
	repo := newMockCharacterRepo()
	char, _ := corecharacter.New("Warrior")
	char.ID = "char-1"
	char.SP = 15
	repo.chars[char.ID] = char

	svc, err := wishingwell.NewService(repo, wishingwell.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	status, err := svc.GetStatus(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.CharacterID != char.ID {
		t.Errorf("got ID %s, want %s", status.CharacterID, char.ID)
	}
	if status.SP != 15 {
		t.Errorf("got SP %d, want 15", status.SP)
	}
	if !status.CanExchange {
		t.Errorf("expected CanExchange to be true")
	}
	if len(status.Dialogues) != 8 {
		t.Errorf("got %d dialogues, want 8", len(status.Dialogues))
	}

	// Test JobMemory active
	char.JobMemory = &corecharacter.JobMemory{JobID: "job-02", SP: 5}
	repo.chars[char.ID] = char
	status, err = svc.GetStatus(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CanExchange {
		t.Errorf("expected CanExchange to be false when JobMemory is active")
	}

	// Test OverLevel active
	char.JobMemory = nil
	char.OverLevel = true
	repo.chars[char.ID] = char
	status, err = svc.GetStatus(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CanExchange {
		t.Errorf("expected CanExchange to be false when OverLevel is active")
	}
}

func TestWishingWell_Exchange_Success(t *testing.T) {
	repo := newMockCharacterRepo()
	char, _ := corecharacter.New("Hero")
	char.ID = "char-hero"
	char.SP = 25
	repo.chars[char.ID] = char

	svc, err := wishingwell.NewService(repo, wishingwell.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Exchange 5 SP for MaxHP (+10 HP)
	initialMaxHP := char.Stats.MaxHP
	res, err := svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: char.ID,
		Stat:        "mhp",
		SP:          5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Stat != corecharacter.SPExchangeMaxHP {
		t.Errorf("got stat %s, want %s", res.Stat, corecharacter.SPExchangeMaxHP)
	}
	if res.StatName != "ＨＰ" {
		t.Errorf("got stat name %s, want ＨＰ", res.StatName)
	}
	if res.SPConsumed != 5 {
		t.Errorf("got sp consumed %d, want 5", res.SPConsumed)
	}
	if res.StatIncrease != 10 {
		t.Errorf("got stat increase %d, want 10", res.StatIncrease)
	}
	if res.RemainingSP != 20 {
		t.Errorf("got remaining sp %d, want 20", res.RemainingSP)
	}
	if !strings.Contains(res.Message, "SP 5 のかわりに ＨＰ を 10 あたえましょう") {
		t.Errorf("unexpected message: %s", res.Message)
	}
	if res.Character.Stats.MaxHP != initialMaxHP+10 {
		t.Errorf("got max HP %d, want %d", res.Character.Stats.MaxHP, initialMaxHP+10)
	}

	// Exchange 3 SP for Attack (+3 AT)
	initialAT := res.Character.Stats.Attack
	resAT, err := svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: char.ID,
		Stat:        "attack",
		SP:          3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resAT.StatIncrease != 3 {
		t.Errorf("got attack increase %d, want 3", resAT.StatIncrease)
	}
	if resAT.RemainingSP != 17 {
		t.Errorf("got remaining sp %d, want 17", resAT.RemainingSP)
	}
	if resAT.Character.Stats.Attack != initialAT+3 {
		t.Errorf("got attack %d, want %d", resAT.Character.Stats.Attack, initialAT+3)
	}
}

func TestWishingWell_Exchange_Errors(t *testing.T) {
	repo := newMockCharacterRepo()
	char, _ := corecharacter.New("Hero")
	char.ID = "char-hero"
	char.SP = 10
	repo.chars[char.ID] = char

	svc, err := wishingwell.NewService(repo, wishingwell.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. SP < 1
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: char.ID,
		Stat:        "mhp",
		SP:          0,
	})
	if !errors.Is(err, wishingwell.ErrInvalidSPAmount) {
		t.Errorf("expected ErrInvalidSPAmount, got %v", err)
	}

	// 2. SP > char.SP
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: char.ID,
		Stat:        "mhp",
		SP:          11,
	})
	if !errors.Is(err, wishingwell.ErrInsufficientSP) {
		t.Errorf("expected ErrInsufficientSP, got %v", err)
	}

	// 3. JobMemory active
	charMemory := char
	charMemory.ID = "char-mem"
	charMemory.JobMemory = &corecharacter.JobMemory{JobID: "job-01", SP: 10}
	repo.chars[charMemory.ID] = charMemory
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: charMemory.ID,
		Stat:        "mhp",
		SP:          2,
	})
	if !errors.Is(err, wishingwell.ErrJobMemoryActive) {
		t.Errorf("expected ErrJobMemoryActive, got %v", err)
	}

	// 4. OverLevel active
	charOver := char
	charOver.ID = "char-over"
	charOver.OverLevel = true
	repo.chars[charOver.ID] = charOver
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: charOver.ID,
		Stat:        "mhp",
		SP:          2,
	})
	if !errors.Is(err, wishingwell.ErrOverLevelRestricted) {
		t.Errorf("expected ErrOverLevelRestricted, got %v", err)
	}

	// 5. Invalid stat
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: char.ID,
		Stat:        "magic_power",
		SP:          2,
	})
	if !errors.Is(err, wishingwell.ErrInvalidTargetStat) {
		t.Errorf("expected ErrInvalidTargetStat, got %v", err)
	}

	// 6. Character not found
	_, err = svc.Exchange(context.Background(), wishingwell.ExchangeRequest{
		CharacterID: "nonexistent",
		Stat:        "mhp",
		SP:          1,
	})
	if err == nil {
		t.Errorf("expected error for nonexistent character, got nil")
	}
}

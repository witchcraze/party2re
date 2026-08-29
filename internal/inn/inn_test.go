package inn

import (
	"context"
	"errors"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// --- stub repository ---

type stubRepo struct {
	mu        sync.Mutex
	character corecharacter.Character
	findErr   error
	updateErr error
	updated   *corecharacter.Character
}

func (r *stubRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.findErr != nil {
		return corecharacter.Character{}, r.findErr
	}
	if id != r.character.ID {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.character, nil
}

func (r *stubRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *stubRepo) Update(_ context.Context, value corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateErr != nil {
		return r.updateErr
	}
	r.character = value
	r.updated = &value
	return nil
}

// --- helpers ---

func newTestCharacter(hp, mp, maxHP, maxMP, money, level int) corecharacter.Character {
	return corecharacter.Character{
		ID:    "char-1",
		Name:  "Hero",
		JobID: "starter",
		Level: level,
		Money: money,
		Stats: corecharacter.Stats{
			MaxHP:   maxHP,
			MaxMP:   maxMP,
			HP:      hp,
			MP:      mp,
			Attack:  10,
			Defense: 5,
			Agility: 6,
		},
	}
}

// --- tests ---

func TestNewServiceRejectsNilRepository(t *testing.T) {
	_, err := NewService(nil)
	if err == nil {
		t.Fatal("expected error for nil repository, got nil")
	}
}

func TestNewServiceWithFeeRejectsNegativeFee(t *testing.T) {
	repo := &stubRepo{}
	_, err := NewServiceWithFee(repo, -1)
	if err == nil {
		t.Fatal("expected error for negative fee, got nil")
	}
}

func TestCalculateFee(t *testing.T) {
	svc, _ := NewService(&stubRepo{})
	tests := []struct {
		level int
		want  int
	}{
		{1, 5},   // 1 * 5 = 5 (equals MinFee)
		{2, 10},  // 2 * 5 = 10
		{10, 50}, // 10 * 5 = 50
		{0, 5},   // clamped to level 1 -> 5
	}
	for _, tc := range tests {
		got := svc.CalculateFee(tc.level)
		if got != tc.want {
			t.Errorf("CalculateFee(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestRestRestoresHPAndMPAndDeductsFee(t *testing.T) {
	char := newTestCharacter(5, 1, 30, 10, 100, 3) // level 3 → fee = 15
	repo := &stubRepo{character: char}
	svc, _ := NewService(repo)

	result, err := svc.Rest(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("Rest() error = %v", err)
	}
	if result.Stats.HP != result.Stats.MaxHP {
		t.Errorf("HP = %d, want MaxHP %d", result.Stats.HP, result.Stats.MaxHP)
	}
	if result.Stats.MP != result.Stats.MaxMP {
		t.Errorf("MP = %d, want MaxMP %d", result.Stats.MP, result.Stats.MaxMP)
	}
	wantMoney := 100 - 3*DefaultFeePerLevel // 100 - 15 = 85
	if result.Money != wantMoney {
		t.Errorf("Money = %d, want %d", result.Money, wantMoney)
	}
	if repo.updated == nil {
		t.Fatal("Update was not called")
	}
}

func TestRestRejectsInsufficientFunds(t *testing.T) {
	char := newTestCharacter(10, 5, 30, 10, 4, 1) // money=4, fee=5
	repo := &stubRepo{character: char}
	svc, _ := NewService(repo)

	_, err := svc.Rest(context.Background(), char.ID)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("Rest() error = %v, want ErrInsufficientFunds", err)
	}
	if repo.updated != nil {
		t.Fatal("Update must not be called when funds are insufficient")
	}
}

func TestRestRejectsEmptyCharacterID(t *testing.T) {
	repo := &stubRepo{}
	svc, _ := NewService(repo)

	_, err := svc.Rest(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty character ID")
	}
}

func TestRestPropagatesFindError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &stubRepo{
		character: newTestCharacter(10, 5, 30, 10, 100, 1),
		findErr:   sentinel,
	}
	svc, _ := NewService(repo)

	_, err := svc.Rest(context.Background(), "char-1")
	if !errors.Is(err, sentinel) {
		t.Errorf("Rest() error = %v, want sentinel", err)
	}
}

func TestRestPropagatesUpdateError(t *testing.T) {
	sentinel := errors.New("write failed")
	char := newTestCharacter(5, 1, 30, 10, 100, 1)
	repo := &stubRepo{character: char, updateErr: sentinel}
	svc, _ := NewService(repo)

	_, err := svc.Rest(context.Background(), char.ID)
	if !errors.Is(err, sentinel) {
		t.Errorf("Rest() error = %v, want sentinel", err)
	}
}

func TestRestAlreadyFullHP(t *testing.T) {
	char := newTestCharacter(30, 10, 30, 10, 100, 1) // already full
	repo := &stubRepo{character: char}
	svc, _ := NewService(repo)

	result, err := svc.Rest(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("Rest() error = %v", err)
	}
	if result.Stats.HP != 30 || result.Stats.MP != 10 {
		t.Errorf("got HP=%d MP=%d, want 30 and 10", result.Stats.HP, result.Stats.MP)
	}
}

func TestNewServiceWithFeeCustomFee(t *testing.T) {
	char := newTestCharacter(5, 1, 30, 10, 50, 2) // level 2, fee=2*3=6
	repo := &stubRepo{character: char}
	svc, _ := NewServiceWithFee(repo, 3)

	result, err := svc.Rest(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("Rest() error = %v", err)
	}
	if result.Money != 44 { // 50 - 6
		t.Errorf("Money = %d, want 44", result.Money)
	}
}

type dummyTxProvider struct{}

func (d dummyTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestRest_ConcurrentRestPreventsOverdraft(t *testing.T) {
	char := newTestCharacter(5, 1, 30, 10, 8, 1) // level 1, fee = 5, char has 8G (only enough for 1 rest)
	repo := &stubRepo{character: char}
	svc, _ := NewService(repo, WithTransactionProvider(dummyTxProvider{}))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Rest(context.Background(), char.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	finalChar, _ := repo.FindByID(context.Background(), char.ID)
	if finalChar.Money < 0 {
		t.Fatalf("character money went negative: %d", finalChar.Money)
	}
	if finalChar.Money != 3 { // 8 - 5 = 3
		t.Errorf("character money = %d, want 3", finalChar.Money)
	}
}

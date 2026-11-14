package character

import (
	"context"
	"errors"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type fakeMemories struct {
	t       *testing.T
	entries []string
	limit   int
	err     error
}

type memoryTxKey struct{}

func (f *fakeMemories) AppendMemory(ctx context.Context, id string, kind corecharacter.MemoryKind, content string) error {
	if ctx.Value(memoryTxKey{}) != true {
		f.t.Fatal("memory must be written in the operation's transaction")
	}
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, id+"|"+string(kind)+"|"+content)
	return nil
}

func (f *fakeMemories) ListMemories(_ context.Context, _ string, limit int) ([]Memory, error) {
	f.limit = limit
	return []Memory{{Content: "newest"}}, nil
}

func newMemoryService(t *testing.T) (*Service, *mockRepository, *fakeMemories) {
	t.Helper()
	repo, memories := newMockRepository(), &fakeMemories{t: t}
	tx := &mockTransactionProvider{fn: func(ctx context.Context, fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, memoryTxKey{}, true))
	}}
	service, err := NewService(repo, WithTransactionProvider(tx), WithMemoryRepository(memories))
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, memories
}

func TestService_RecordsLifecycleMemories(t *testing.T) {
	ctx := context.Background()
	service, repo, memories := newMemoryService(t)
	created, err := service.Create(ctx, "player-1", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	created.Money = NameChangeCost + 2*GenderChangeCost
	repo.characters[created.ID] = created
	if _, err := service.ChangeName(ctx, created.ID, "Hera"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeGender(ctx, created.ID, "f"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeGender(ctx, created.ID, "m"); err != nil {
		t.Fatal(err)
	}
	// Rejected changes are not milestones.
	if _, err := service.ChangeGender(ctx, created.ID, "m"); !errors.Is(err, ErrSameGender) {
		t.Fatalf("same gender err = %v", err)
	}
	want := []string{
		created.ID + "|birth|冒険者 Hero 誕生！",
		created.ID + "|name_change|Hera に名前を変更する",
		created.ID + "|gender_change|男をやめて女として生まれ変わる",
		created.ID + "|gender_change|女をやめて男として生まれ変わる",
	}
	if !slices.Equal(memories.entries, want) {
		t.Fatalf("memories = %q, want %q", memories.entries, want)
	}
}

func TestService_MemoryFailureFailsTheOperation(t *testing.T) {
	service, _, memories := newMemoryService(t)
	memories.err = errors.New("memory store down")
	if _, err := service.Create(context.Background(), "player-1", "Hero"); !errors.Is(err, memories.err) {
		t.Fatalf("Create err = %v, want the memory failure", err)
	}
}

func TestService_Trajectory(t *testing.T) {
	ctx := context.Background()
	service, _, memories := newMemoryService(t)
	if _, err := service.Trajectory(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown character err = %v, want ErrNotFound", err)
	}
	created, err := service.Create(ctx, "player-1", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Trajectory(ctx, created.ID)
	if err != nil || len(got) != 1 || memories.limit != MaxMemories {
		t.Fatalf("Trajectory = %v, %v with limit %d, want the newest %d", got, err, memories.limit, MaxMemories)
	}
}

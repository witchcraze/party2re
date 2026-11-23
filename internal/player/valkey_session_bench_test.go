package player_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

func BenchmarkMemorySessionSave(b *testing.B) {
	ctx := context.Background()
	repo := newMemorySessionRepository()
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sess := coreplayer.Session{
			ID:        fmt.Sprintf("token-%d", i),
			PlayerID:  "player-benchmark",
			CreatedAt: now,
			ExpiresAt: now.Add(player.SessionDuration),
		}
		if err := repo.Save(ctx, sess); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

func BenchmarkMemorySessionFind(b *testing.B) {
	ctx := context.Background()
	repo := newMemorySessionRepository()
	now := time.Now().UTC()
	sess := coreplayer.Session{
		ID:        "token-constant",
		PlayerID:  "player-benchmark",
		CreatedAt: now,
		ExpiresAt: now.Add(player.SessionDuration),
	}
	if err := repo.Save(ctx, sess); err != nil {
		b.Fatalf("Save failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := repo.FindByID(ctx, sess.ID)
		if err != nil {
			b.Fatalf("FindByID failed: %v", err)
		}
	}
}

func BenchmarkValkeySessionSave_LiveValkey(b *testing.B) {
	client := testutil.OpenValkey(b)

	ctx := context.Background()
	repo := player.NewValkeySessionRepository(client)
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sess := coreplayer.Session{
			ID:        fmt.Sprintf("bench-token-%d", i),
			PlayerID:  "player-benchmark",
			CreatedAt: now,
			ExpiresAt: now.Add(player.SessionDuration),
		}
		if err := repo.Save(ctx, sess); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

func BenchmarkValkeySessionFind_LiveValkey(b *testing.B) {
	client := testutil.OpenValkey(b)

	ctx := context.Background()
	repo := player.NewValkeySessionRepository(client)
	now := time.Now().UTC()
	sess := coreplayer.Session{
		ID:        "bench-token-find",
		PlayerID:  "player-benchmark",
		CreatedAt: now,
		ExpiresAt: now.Add(player.SessionDuration),
	}
	if err := repo.Save(ctx, sess); err != nil {
		b.Fatalf("Save failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := repo.FindByID(ctx, sess.ID)
		if err != nil {
			b.Fatalf("FindByID failed: %v", err)
		}
	}
}

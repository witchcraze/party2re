package player_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/player"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestValkeySessionRepository_InMemoryFallback(t *testing.T) {
	ctx := context.Background()
	repo := player.NewValkeySessionRepository(nil)

	now := time.Now().UTC()
	sess := coreplayer.Session{
		ID:        "test-token-123",
		PlayerID:  "player-456",
		CreatedAt: now,
		ExpiresAt: now.Add(player.SessionDuration),
	}

	// 1. Find non-existent session
	_, err := repo.FindByID(ctx, sess.ID)
	if !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}

	// 2. Save session
	if err := repo.Save(ctx, sess); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 3. Find saved session
	got, err := repo.FindByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if got.ID != sess.ID || got.PlayerID != sess.PlayerID {
		t.Fatalf("FindByID mismatch: got %+v, want %+v", got, sess)
	}

	// 4. Revoke session
	if err := repo.Revoke(ctx, sess.ID, now); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// 5. Find revoked session
	_, err = repo.FindByID(ctx, sess.ID)
	if !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("expected ErrInvalidSession after revoke, got %v", err)
	}

	// 6. Revoking already revoked session returns ErrInvalidSession
	if err := repo.Revoke(ctx, sess.ID, now); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("expected ErrInvalidSession on duplicate revoke, got %v", err)
	}
}

func TestValkeySessionRepository_DeleteByPlayerID_InMemory(t *testing.T) {
	ctx := context.Background()
	repo := player.NewValkeySessionRepository(nil)

	now := time.Now().UTC()
	sess1 := coreplayer.Session{ID: "token-1", PlayerID: "target-player", CreatedAt: now, ExpiresAt: now.Add(player.SessionDuration)}
	sess2 := coreplayer.Session{ID: "token-2", PlayerID: "target-player", CreatedAt: now, ExpiresAt: now.Add(player.SessionDuration)}
	sess3 := coreplayer.Session{ID: "token-3", PlayerID: "other-player", CreatedAt: now, ExpiresAt: now.Add(player.SessionDuration)}

	_ = repo.Save(ctx, sess1)
	_ = repo.Save(ctx, sess2)
	_ = repo.Save(ctx, sess3)

	if err := repo.DeleteByPlayerID(ctx, "target-player"); err != nil {
		t.Fatalf("DeleteByPlayerID failed: %v", err)
	}

	// sess1 and sess2 must be deleted
	if _, err := repo.FindByID(ctx, "token-1"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("token-1 should be deleted, got err: %v", err)
	}
	if _, err := repo.FindByID(ctx, "token-2"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("token-2 should be deleted, got err: %v", err)
	}

	// sess3 must remain
	got, err := repo.FindByID(ctx, "token-3")
	if err != nil || got.PlayerID != "other-player" {
		t.Errorf("token-3 should remain, got %+v, err: %v", got, err)
	}
}

func TestValkeySessionRepository_RealValkeyIntegration(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("connect to valkey: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	testSessionPrefix := "party2:test:session:"
	testPlayerPrefix := "party2:test:player:sessions:"

	repo := player.NewValkeySessionRepository(client,
		player.WithSessionKeyPrefix(testSessionPrefix),
		player.WithPlayerSessionsKeyPrefix(testPlayerPrefix),
	)

	now := time.Now().UTC()
	sess1 := coreplayer.Session{
		ID:        "vk-token-1",
		PlayerID:  "vk-player-1",
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	sess2 := coreplayer.Session{
		ID:        "vk-token-2",
		PlayerID:  "vk-player-1",
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
	}

	defer func() {
		_ = repo.DeleteByPlayerID(ctx, "vk-player-1")
	}()

	// 1. Save sessions
	if err := repo.Save(ctx, sess1); err != nil {
		t.Fatalf("Save(sess1) failed: %v", err)
	}
	if err := repo.Save(ctx, sess2); err != nil {
		t.Fatalf("Save(sess2) failed: %v", err)
	}

	// 2. Verify FindByID returns the session
	got1, err := repo.FindByID(ctx, sess1.ID)
	if err != nil {
		t.Fatalf("FindByID(sess1) failed: %v", err)
	}
	if got1.ID != sess1.ID || got1.PlayerID != sess1.PlayerID {
		t.Errorf("got session %+v, want %+v", got1, sess1)
	}

	// Verify TTL on Valkey key
	ttl, err := client.Do(ctx, client.B().Ttl().Key(testSessionPrefix+sess1.ID).Build()).AsInt64()
	if err != nil {
		t.Fatalf("TTL error: %v", err)
	}
	if ttl <= 0 || ttl > int64(7*24*time.Hour/time.Second) {
		t.Errorf("expected positive TTL up to 7 days, got %d", ttl)
	}

	// Verify Player session set has both tokens
	members, err := client.Do(ctx, client.B().Smembers().Key(testPlayerPrefix+"vk-player-1").Build()).AsStrSlice()
	if err != nil {
		t.Fatalf("smembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 sessions in player set, got %v", members)
	}

	// 3. Test string-only token mapping fallback in Valkey
	rawTokenKey := testSessionPrefix + "raw-token"
	if err := client.Do(ctx, client.B().Set().Key(rawTokenKey).Value("raw-player-id").ExSeconds(60).Build()).Error(); err != nil {
		t.Fatalf("set raw token failed: %v", err)
	}
	defer client.Do(ctx, client.B().Del().Key(rawTokenKey).Build())

	rawSess, err := repo.FindByID(ctx, "raw-token")
	if err != nil {
		t.Fatalf("FindByID(raw-token) failed: %v", err)
	}
	if rawSess.PlayerID != "raw-player-id" {
		t.Errorf("expected player_id 'raw-player-id', got %s", rawSess.PlayerID)
	}

	// 4. Revoke sess1
	if err := repo.Revoke(ctx, sess1.ID, now); err != nil {
		t.Fatalf("Revoke(sess1) failed: %v", err)
	}
	if _, err := repo.FindByID(ctx, sess1.ID); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession after revoke, got %v", err)
	}

	// Ensure sess1 removed from player set
	membersAfterRevoke, _ := client.Do(ctx, client.B().Smembers().Key(testPlayerPrefix+"vk-player-1").Build()).AsStrSlice()
	if len(membersAfterRevoke) != 1 || membersAfterRevoke[0] != sess2.ID {
		t.Errorf("expected only sess2 in player set, got %v", membersAfterRevoke)
	}

	// 5. DeleteByPlayerID cleans up remaining sess2 and the set
	if err := repo.DeleteByPlayerID(ctx, "vk-player-1"); err != nil {
		t.Fatalf("DeleteByPlayerID failed: %v", err)
	}
	if _, err := repo.FindByID(ctx, sess2.ID); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected sess2 deleted by player deletion, got %v", err)
	}
	exists, _ := client.Do(ctx, client.B().Exists().Key(testPlayerPrefix+"vk-player-1").Build()).AsInt64()
	if exists != 0 {
		t.Errorf("expected player sessions set deleted, exists = %d", exists)
	}
}

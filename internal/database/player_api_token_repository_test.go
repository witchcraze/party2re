package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestNewPlayerAPITokenRepositoryNilDB(t *testing.T) {
	repo, err := NewPlayerAPITokenRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewPlayerAPITokenRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestPlayerAPITokenRepositoryOperations(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tokenRepo, err := NewPlayerAPITokenRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Create test player
	p1, err := coreplayer.New("tok_test_p1_"+now.Format("150405"), "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p1); err != nil {
		t.Fatal(err)
	}
	defer playerRepo.Delete(ctx, p1.ID)

	p2, err := coreplayer.New("tok_test_p2_"+now.Format("150405"), "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p2); err != nil {
		t.Fatal(err)
	}
	defer playerRepo.Delete(ctx, p2.ID)

	// Create tokens for p1
	expires := now.Add(48 * time.Hour)
	tok1, plaintext1, err := coreplayer.NewAPIToken(p1.ID, "Agent 1", &expires, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tokenRepo.Save(ctx, tok1); err != nil {
		t.Fatalf("Save(tok1) failed: %v", err)
	}

	tok2, plaintext2, err := coreplayer.NewAPIToken(p1.ID, "Agent 2", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tokenRepo.Save(ctx, tok2); err != nil {
		t.Fatalf("Save(tok2) failed: %v", err)
	}

	// 1. FindByTokenHash
	hash1 := coreplayer.HashAPIToken(plaintext1)
	gotTok1, err := tokenRepo.FindByTokenHash(ctx, hash1)
	if err != nil {
		t.Fatalf("FindByTokenHash(hash1) failed: %v", err)
	}
	if gotTok1.ID != tok1.ID || gotTok1.PlayerID != p1.ID || gotTok1.Name != "Agent 1" {
		t.Errorf("FindByTokenHash mismatch: got %+v, want %+v", gotTok1, tok1)
	}
	if gotTok1.ExpiresAt == nil || !gotTok1.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt mismatch: got %v, want %v", gotTok1.ExpiresAt, expires)
	}
	if gotTok1.LastUsedAt != nil {
		t.Errorf("expected nil LastUsedAt, got %v", gotTok1.LastUsedAt)
	}

	// 2. TouchLastUsed
	usedTime := now.Add(time.Hour)
	if err := tokenRepo.TouchLastUsed(ctx, tok1.ID, usedTime); err != nil {
		t.Fatalf("TouchLastUsed failed: %v", err)
	}
	touchedTok1, err := tokenRepo.FindByTokenHash(ctx, hash1)
	if err != nil {
		t.Fatal(err)
	}
	if touchedTok1.LastUsedAt == nil || !touchedTok1.LastUsedAt.Equal(usedTime) {
		t.Errorf("LastUsedAt not updated: got %v, want %v", touchedTok1.LastUsedAt, usedTime)
	}

	// 3. FindByPlayerID
	tokensP1, err := tokenRepo.FindByPlayerID(ctx, p1.ID)
	if err != nil {
		t.Fatalf("FindByPlayerID(p1) failed: %v", err)
	}
	if len(tokensP1) != 2 {
		t.Fatalf("expected 2 tokens for p1, got %d", len(tokensP1))
	}

	// 4. Revoke unauthorized (p2 trying to revoke p1's token)
	err = tokenRepo.Revoke(ctx, p2.ID, tok1.ID)
	if !errors.Is(err, ErrAPITokenForbidden) {
		t.Errorf("Revoke with wrong playerID: got error %v, want %v", err, ErrAPITokenForbidden)
	}

	// 5. Revoke authorized
	if err := tokenRepo.Revoke(ctx, p1.ID, tok1.ID); err != nil {
		t.Fatalf("Revoke(p1, tok1) failed: %v", err)
	}

	// FindByTokenHash for revoked token must return ErrAPITokenNotFound
	_, err = tokenRepo.FindByTokenHash(ctx, hash1)
	if !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("FindByTokenHash revoked token: got error %v, want %v", err, ErrAPITokenNotFound)
	}

	// 6. DeleteByPlayerID (cascading cleanup)
	_ = plaintext2
	if err := tokenRepo.DeleteByPlayerID(ctx, p1.ID); err != nil {
		t.Fatalf("DeleteByPlayerID failed: %v", err)
	}
	tokensAfterDel, err := tokenRepo.FindByPlayerID(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokensAfterDel) != 0 {
		t.Errorf("expected 0 tokens after DeleteByPlayerID, got %d", len(tokensAfterDel))
	}
}

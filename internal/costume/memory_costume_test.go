package costume_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/costume"
)

func TestMemoryCostumeRepository(t *testing.T) {
	ctx := context.Background()
	repo := costume.NewMemoryCostumeRepository()

	// 1. Initially nil
	c, err := repo.GetActiveCostume(ctx, "char-1")
	if err != nil || c != nil {
		t.Fatalf("expected nil, nil initially, got: %v, %v", c, err)
	}

	// 2. Save and Get
	now := time.Now().UTC()
	active := costume.ActiveCostume{
		CharacterID: "char-1",
		ItemNo:      46,
		ItemName:    "チョビヒゲタクシード",
		Icon:        "chr/012.gif",
		RentedAt:    now,
		ExpiresAt:   now.Add(2 * time.Hour),
	}
	if err := repo.SaveActiveCostume(ctx, active, time.Hour); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	got, err := repo.GetActiveCostume(ctx, "char-1")
	if err != nil || got == nil {
		t.Fatalf("expected costume, got nil or error: %v", err)
	}
	if got.CharacterID != "char-1" || got.ItemNo != 46 {
		t.Fatalf("unexpected costume data: %+v", got)
	}

	// 3. Expired costume returns nil
	expired := costume.ActiveCostume{
		CharacterID: "char-2",
		ItemNo:      44,
		ExpiresAt:   now.Add(-10 * time.Minute),
	}
	_ = repo.SaveActiveCostume(ctx, expired, 0)
	gotExpired, err := repo.GetActiveCostume(ctx, "char-2")
	if err != nil || gotExpired != nil {
		t.Fatalf("expected nil for expired costume, got: %v, %v", gotExpired, err)
	}

	// 4. Clear active costume
	if err := repo.ClearActiveCostume(ctx, "char-1"); err != nil {
		t.Fatalf("unexpected clear error: %v", err)
	}
	cleared, err := repo.GetActiveCostume(ctx, "char-1")
	if err != nil || cleared != nil {
		t.Fatalf("expected nil after clear, got: %v, %v", cleared, err)
	}
}

func TestMemoryCostumeRepository_ClearActiveCostumeWithClaim(t *testing.T) {
	ctx := context.Background()
	repo := costume.NewMemoryCostumeRepository()

	active := costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "cat-ears",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	_ = repo.SaveActiveCostume(ctx, active, time.Hour)

	// 1. Without claim checker: refuses unsafe deletion
	reset, err := repo.ClearActiveCostumeWithClaim(ctx, "c1", "waking", "any-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reset {
		t.Errorf("expected reset=false when claimChecker is nil")
	}
	got, _ := repo.GetActiveCostume(ctx, "c1")
	if got == nil || got.ItemNo != 46 {
		t.Fatalf("costume should not be deleted when claimChecker is nil")
	}

	// 2. Token mismatch preserves costume
	validToken := "token-123"
	repo.SetClaimChecker(func(_ context.Context, category, targetID, token string) bool {
		return category == "waking" && targetID == "c1" && token == validToken
	})

	reset, err = repo.ClearActiveCostumeWithClaim(ctx, "c1", "waking", "wrong-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reset {
		t.Errorf("expected reset=false on token mismatch")
	}
	got, _ = repo.GetActiveCostume(ctx, "c1")
	if got == nil || got.ItemNo != 46 {
		t.Fatalf("costume should NOT be deleted on token mismatch")
	}

	// 3. Matching token clears costume
	reset, err = repo.ClearActiveCostumeWithClaim(ctx, "c1", "waking", validToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reset {
		t.Errorf("expected reset=true on matching token")
	}
	got, _ = repo.GetActiveCostume(ctx, "c1")
	if got != nil {
		t.Fatalf("costume should be cleared on matching token")
	}
}

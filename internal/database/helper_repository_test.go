package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helper"
)

func TestHelperRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	q := helper.Quest{
		ID:            "test-q-" + now.Format("150405"),
		Title:         "テスト依頼その1",
		Kind:          helper.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 3,
		RewardItemID:  "item-128",
		IsRare:        false,
		IsGuild:       false,
		ExpiresAt:     now.Add(6 * 24 * time.Hour),
		CreatedAt:     now,
	}

	// 1. Save
	if err := repo.Save(ctx, q); err != nil {
		t.Fatalf("Save quest failed: %v", err)
	}

	// 2. FindByID
	found, err := repo.FindByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.Title != q.Title || found.RequiredCount != 3 {
		t.Errorf("unexpected found quest: %+v", found)
	}

	// 3. ListActive
	active, err := repo.ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}
	hasQuest := false
	for _, a := range active {
		if a.ID == q.ID {
			hasQuest = true
			break
		}
	}
	if !hasQuest {
		t.Errorf("expected quest %s to be in active list", q.ID)
	}

	// 4. Update (Mark completed)
	completedAt := now.Add(1 * time.Hour)
	q.CompletedAt = &completedAt
	q.CompletedBy = "char-tester"
	if err := repo.Save(ctx, q); err != nil {
		t.Fatalf("Save completed quest failed: %v", err)
	}

	foundCompleted, err := repo.FindByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("FindByID after completion failed: %v", err)
	}
	if foundCompleted.CompletedBy != "char-tester" || foundCompleted.CompletedAt == nil {
		t.Errorf("unexpected completed quest: %+v", foundCompleted)
	}
}

package park_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/park"
)

func TestParkServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "ParkIntegrationChar")
	if err != nil {
		t.Fatal(err)
	}

	parkRepo, err := database.NewParkRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	curTime := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	svc, err := park.NewService(
		parkRepo,
		charRepo,
		park.WithRateLimit(time.Second),
		park.WithNowFunc(func() time.Time { return curTime }),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Post message
	post, err := svc.PostMessage(ctx, char.ID, "初めまして！", "#0088ff", "町娘")
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	if post.CharacterName != "ParkIntegrationChar" {
		t.Errorf("expected ParkIntegrationChar, got %s", post.CharacterName)
	}

	// 2. Immediate post should trigger rate limit
	_, err = svc.PostMessage(ctx, char.ID, "連投チェック", "", "")
	if err != park.ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}

	// 3. Advance clock past rate limit window
	curTime = curTime.Add(2 * time.Second)

	// 4. Retrieve recent posts
	posts, total, err := svc.GetRecentPosts(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetRecentPosts failed: %v", err)
	}
	if total < 1 || len(posts) == 0 {
		t.Fatalf("expected at least 1 post")
	}

	// 5. NPC interactions
	talkLine, err := svc.TalkToNPC(ctx, char.ID)
	if err != nil || talkLine == "" {
		t.Errorf("TalkToNPC failed: %v, line: %s", err, talkLine)
	}

	divination, err := svc.Divinate(ctx, char.ID)
	if err != nil || divination.Fortune == "" {
		t.Errorf("Divinate failed: %v, divination: %+v", err, divination)
	}

	inspectLine := svc.InspectNPC()
	if inspectLine == "" {
		t.Errorf("InspectNPC returned empty line")
	}
}

package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/park"
)

func TestParkRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "ParkTester")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewParkRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	postID := fmt.Sprintf("post_%d", time.Now().UnixNano())
	if len(postID) > 32 {
		postID = postID[:32]
	}

	post1 := park.Post{
		ID:            postID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		Content:       "Hello from DB test 1",
		Color:         "#123456",
		RecipientName: "All",
		CreatedAt:     now,
	}

	err = repo.CreatePost(ctx, post1)
	if err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	latestTime, err := repo.GetLatestPostTimeByCharacter(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetLatestPostTimeByCharacter failed: %v", err)
	}
	if latestTime.IsZero() {
		t.Fatalf("expected non-zero latest post time")
	}

	posts, total, err := repo.GetRecentPosts(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetRecentPosts failed: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected total >= 1, got %d", total)
	}

	found := false
	for _, p := range posts {
		if p.ID == post1.ID {
			found = true
			if p.Content != post1.Content || p.Color != post1.Color {
				t.Errorf("mismatched post data: %+v", p)
			}
			break
		}
	}
	if !found {
		t.Errorf("created post not found in recent posts")
	}
}

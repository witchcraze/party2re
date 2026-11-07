package park_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/park"
)

func parkIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated park tests")
	}

	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("failed to close source DB: %v", err)
		}
	})
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("failed to close admin DB: %v", err)
		}
	})

	name := "party2_park_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("failed to drop isolated database %s: %v", name, err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close isolated DB: %v", err)
		}
	})

	for _, table := range []string{"players", "characters", "character_depots", "depot_items", "news_articles", "park_posts"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestParkServiceIntegration(t *testing.T) {
	db := parkIsolatedDB(t)

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "ParkChar")
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
	if post.CharacterName != char.Name {
		t.Errorf("expected %s, got %s", char.Name, post.CharacterName)
	}

	// 2. An immediate second post is accepted
	if _, err = svc.PostMessage(ctx, char.ID, "連投チェック", "", ""); err != nil {
		t.Errorf("expected consecutive post to succeed, got %v", err)
	}

	// 3. Retrieve recent posts
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

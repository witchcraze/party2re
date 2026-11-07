package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/testutil"
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

func TestParkRepository_Database(t *testing.T) {
	db := parkIsolatedDB(t)

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

	err = repo.CreatePost(ctx, post1, park.DefaultLogLimit)
	if err != nil {
		t.Fatalf("CreatePost failed: %v", err)
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

	// Test GetRecentPostsByCursor
	cursorPosts, err := repo.GetRecentPostsByCursor(ctx, 10, time.Time{}, "")
	if err != nil {
		t.Fatalf("GetRecentPostsByCursor failed: %v", err)
	}
	if len(cursorPosts) < 1 {
		t.Fatalf("expected cursorPosts len >= 1, got %d", len(cursorPosts))
	}
	if cursorPosts[0].ID != post1.ID {
		t.Errorf("expected newest post %s, got %s", post1.ID, cursorPosts[0].ID)
	}

	// Paginate after post1
	nextCursorPosts, err := repo.GetRecentPostsByCursor(ctx, 10, post1.CreatedAt, post1.ID)
	if err != nil {
		t.Fatalf("GetRecentPostsByCursor with cursor failed: %v", err)
	}
	for _, p := range nextCursorPosts {
		if p.ID == post1.ID {
			t.Errorf("post1 %s should not appear in subsequent page", post1.ID)
		}
	}
}

func TestParkBoardRetention(t *testing.T) {
	db := parkIsolatedDB(t)
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "ParkTrim")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewParkRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	offset := 0
	post := func(t *testing.T, limit int) park.Post {
		t.Helper()
		offset++
		p := park.Post{
			ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
			Content: "hello", Color: "#000000", CreatedAt: t0.Add(time.Duration(offset) * time.Second),
		}
		if err := repo.CreatePost(ctx, p, limit); err != nil {
			t.Fatal(err)
		}
		return p
	}
	listIDs := func(t *testing.T) []string {
		t.Helper()
		posts, total, err := repo.GetRecentPosts(ctx, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != len(posts) {
			t.Fatalf("total %d does not match %d posts", total, len(posts))
		}
		ids := make([]string, 0, len(posts))
		for _, p := range posts {
			ids = append(ids, p.ID)
		}
		return ids
	}

	t.Run("keeps the newest posts", func(t *testing.T) {
		post(t, 2)
		middle := post(t, 2)
		newest := post(t, 2)
		if got := listIDs(t); !slices.Equal(got, []string{newest.ID, middle.ID}) {
			t.Fatalf("unexpected board: %v", got)
		}
	})

	t.Run("a lowered limit trims on the next post", func(t *testing.T) {
		post(t, 10)
		post(t, 10)
		newest := post(t, 1)
		if got := listIDs(t); !slices.Equal(got, []string{newest.ID}) {
			t.Fatalf("unexpected board: %v", got)
		}
	})

	t.Run("stores a body that grows when escaped", func(t *testing.T) {
		p := park.Post{
			ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
			Content: strings.Repeat("&lt;", park.DefaultMaxContentLength), Color: "#000000", CreatedAt: t0.Add(time.Hour),
		}
		if err := repo.CreatePost(ctx, p, park.DefaultLogLimit); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("concurrent posts stay within the limit", func(t *testing.T) {
		const limit = 3
		fns := make([]func() error, 20)
		for i := range fns {
			at := t0.Add(2*time.Hour + time.Duration(i)*time.Second)
			fns[i] = func() error {
				return repo.CreatePost(ctx, park.Post{
					ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
					Content: "race", Color: "#000000", CreatedAt: at,
				}, limit)
			}
		}
		for _, err := range testutil.RunRace(fns...) {
			if err != nil {
				t.Fatal(err)
			}
		}
		if got := listIDs(t); len(got) != limit {
			t.Fatalf("expected %d posts, got %d", limit, len(got))
		}
	})
}

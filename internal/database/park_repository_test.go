package database_test

import (
	"context"
	"database/sql"
	"errors"
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

func TestParkRepository_IsDeadlockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"mysql 1213 deadlock", &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"}, true},
		{"mysql 1205 lock wait timeout", &mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"}, true},
		{"mysql 1062 duplicate key", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, false},
		{"string 1213 deadlock", errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction"), true},
		{"string 1205 lock wait", errors.New("Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction"), true},
		{"generic deadlock text", errors.New("transaction aborted due to deadlock"), true},
		{"generic lock timeout text", errors.New("query failed: lock wait timeout"), true},
		{"unrelated error", errors.New("syntax error in SQL statement"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := database.IsDeadlockForTesting(tt.err); got != tt.want {
				t.Errorf("IsDeadlock(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParkBoardTrimDeadlockRecovery(t *testing.T) {
	db := parkIsolatedDB(t)
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "TrimRetryChar")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewParkRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("retries on deadlock and trims successfully", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			p := park.Post{
				ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
				Content: fmt.Sprintf("initial %d", i), Color: "#000000", CreatedAt: t0.Add(time.Duration(i) * time.Minute),
			}
			if err := repo.CreatePost(ctx, p, 10); err != nil {
				t.Fatalf("seeding initial post failed: %v", err)
			}
		}

		var attempts int
		deadlockErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"}
		repo.SetTrimFnForTesting(func(txCtx context.Context, logLimit int) error {
			attempts++
			if attempts < 3 {
				return deadlockErr
			}
			return repo.TrimBoardForTesting(txCtx, logLimit)
		})
		defer repo.SetTrimFnForTesting(nil)

		newestPost := park.Post{
			ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
			Content: "retry test", Color: "#000000", CreatedAt: t0.Add(10 * time.Minute),
		}
		const targetLimit = 2
		if err := repo.CreatePost(ctx, newestPost, targetLimit); err != nil {
			t.Fatalf("CreatePost failed: %v", err)
		}
		if attempts != 3 {
			t.Errorf("expected 3 trim attempts, got %d", attempts)
		}

		posts, total, err := repo.GetRecentPosts(ctx, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != targetLimit {
			t.Errorf("expected total %d posts after trim, got %d", targetLimit, total)
		}
		if len(posts) != targetLimit {
			t.Fatalf("expected len %d posts after trim, got %d", targetLimit, len(posts))
		}
		if posts[0].ID != newestPost.ID {
			t.Errorf("expected newest post %s, got %s", newestPost.ID, posts[0].ID)
		}
	})

	t.Run("persists post even if all trim retries fail with deadlock", func(t *testing.T) {
		var attempts int
		deadlockErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"}
		repo.SetTrimFnForTesting(func(txCtx context.Context, logLimit int) error {
			attempts++
			return deadlockErr
		})
		defer repo.SetTrimFnForTesting(nil)

		postID := id.New()
		p := park.Post{
			ID: postID, CharacterID: char.ID, CharacterName: char.Name,
			Content: "persisted despite trim failure", Color: "#000000", CreatedAt: t0.Add(2 * time.Minute),
		}
		if err := repo.CreatePost(ctx, p, 3); err != nil {
			t.Fatalf("CreatePost should succeed even when trim fails: %v", err)
		}
		if attempts != 4 {
			t.Errorf("expected 4 trim attempts (1 initial + 3 retries), got %d", attempts)
		}

		posts, _, err := repo.GetRecentPosts(ctx, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, post := range posts {
			if post.ID == postID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("post %s was not persisted after trim failure", postID)
		}
	})

	t.Run("does not retry on non-deadlock error", func(t *testing.T) {
		var attempts int
		syntaxErr := errors.New("Error 1064 (42000): You have an error in your SQL syntax")
		repo.SetTrimFnForTesting(func(txCtx context.Context, logLimit int) error {
			attempts++
			return syntaxErr
		})
		defer repo.SetTrimFnForTesting(nil)

		p := park.Post{
			ID: id.New(), CharacterID: char.ID, CharacterName: char.Name,
			Content: "non-retry error test", Color: "#000000", CreatedAt: t0.Add(3 * time.Minute),
		}
		if err := repo.CreatePost(ctx, p, 3); err != nil {
			t.Fatalf("CreatePost should succeed even when trim fails: %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected exactly 1 trim attempt for non-deadlock error, got %d", attempts)
		}
	})

	t.Run("aborts retry when context is canceled during backoff", func(t *testing.T) {
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var attempts int
		deadlockErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"}
		repo.SetTrimFnForTesting(func(txCtx context.Context, logLimit int) error {
			attempts++
			if attempts == 1 {
				go func() {
					time.Sleep(1 * time.Millisecond)
					cancel()
				}()
			}
			return deadlockErr
		})
		defer repo.SetTrimFnForTesting(nil)

		postID := id.New()
		p := park.Post{
			ID: postID, CharacterID: char.ID, CharacterName: char.Name,
			Content: "canceled context test", Color: "#000000", CreatedAt: t0.Add(4 * time.Minute),
		}
		if err := repo.CreatePost(runCtx, p, 3); err != nil {
			t.Fatalf("CreatePost should succeed even when trim is canceled: %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected exactly 1 trim attempt when canceled during backoff, got %d", attempts)
		}

		posts, _, err := repo.GetRecentPosts(ctx, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, post := range posts {
			if post.ID == postID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("post %s was not persisted after canceled retry", postID)
		}
	})
}

package character_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/notification"
)

func TestCharacterJoiningNews_Integration(t *testing.T) {
	for _, stage := range []string{"success", "ambient_commit", "ambient_rollback", "character_failure", "depot_failure", "publisher_failure", "commit_failure"} {
		t.Run(stage, func(t *testing.T) {
			db := characterIsolatedDB(t)
			ctx := context.Background()
			player, err := database.CreateTestPlayer(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			chars, err := database.NewCharacterRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			newsRepo, err := database.NewNotificationRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			news, err := notification.NewService(newsRepo, newsRepo)
			if err != nil {
				t.Fatal(err)
			}
			service, err := character.NewService(chars,
				character.WithTransactionProvider(database.NewTransactionProvider(db)),
				character.WithNewsPublisher(character.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, at time.Time) error {
					_, err := news.PublishNews(ctx, cat, title, content, author, at)
					return err
				})),
			)
			if err != nil {
				t.Fatal(err)
			}
			failingTable := map[string]string{
				"character_failure": "characters", "depot_failure": "character_depots", "publisher_failure": "news_articles",
			}[stage]
			if failingTable != "" {
				if _, err := db.ExecContext(ctx, "CREATE TRIGGER fail_creation BEFORE INSERT ON "+failingTable+" FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected creation failure'"); err != nil {
					t.Fatal(err)
				}
			}
			var created corecharacter.Character
			create := func(ctx context.Context) error {
				var err error
				created, err = service.Create(ctx, player.ID, "Hero")
				return err
			}
			rollback := errors.New("caller rollback")
			if strings.HasPrefix(stage, "ambient_") || stage == "commit_failure" {
				err = database.RunInTx(ctx, db, func(txCtx context.Context) error {
					if err := create(txCtx); err != nil {
						return err
					}
					inside, err := news.ListNews(txCtx, 20, 0)
					if err != nil || inside.Total != 1 {
						t.Fatalf("inside transaction: news=%+v err=%v", inside, err)
					}
					outside, err := news.ListNews(ctx, 20, 0)
					if err != nil || outside.Total != 0 {
						t.Fatalf("news visible before commit: news=%+v err=%v", outside, err)
					}
					if stage == "ambient_rollback" {
						return rollback
					}
					if stage == "commit_failure" {
						// Force the real outer Commit to fail without a successful commit.
						return database.TxFromContext(txCtx).Rollback()
					}
					return nil
				})
			} else {
				err = create(ctx)
			}
			wantCount := 0
			if stage == "success" || stage == "ambient_commit" {
				wantCount = 1
				if err != nil {
					t.Fatal(err)
				}
				// Ordinary Save retries and Update must not append joining news.
				if err := chars.Save(ctx, created); err == nil {
					t.Fatal("duplicate Save unexpectedly succeeded")
				}
				created.Name = "Renamed"
				if err := chars.Update(ctx, created); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("expected creation or enclosing transaction failure")
			}
			if stage == "ambient_rollback" && !errors.Is(err, rollback) {
				t.Fatalf("rollback error=%v", err)
			}
			if stage == "commit_failure" && !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("commit error=%v", err)
			}
			for _, table := range []string{"characters", "character_depots", "news_articles"} {
				var count int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != wantCount {
					t.Fatalf("%s count=%d, want %d", table, count, wantCount)
				}
			}
			if wantCount == 1 {
				articles, err := news.ListNews(ctx, 20, 0)
				if err != nil || len(articles.Items) != 1 {
					t.Fatalf("news=%+v err=%v", articles, err)
				}
				article := articles.Items[0]
				if article.Category != "character" || article.Author != "System" || !strings.Contains(article.Content, created.ID) || !strings.Contains(article.Title, "Hero") {
					t.Fatalf("joining news=%+v", article)
				}
			}
		})
	}
}

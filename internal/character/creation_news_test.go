package character

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestService_CreatePublishesJoiningNews(t *testing.T) {
	repo := newMockRepository()
	var category, title, content, author string
	var publishedAt time.Time
	calls := 0
	type txKey struct{}
	txProvider := &mockTransactionProvider{fn: func(ctx context.Context, fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, txKey{}, true))
	}}
	service, err := NewService(repo, WithTransactionProvider(txProvider), WithNewsPublisher(NewsPublisherFunc(func(ctx context.Context, cat, heading, body, by string, at time.Time) error {
		if ctx.Value(txKey{}) != true || len(repo.characters) != 1 {
			t.Fatal("news must follow saving and use the same transaction")
		}
		calls++
		category, title, content, author, publishedAt = cat, heading, body, by, at
		return nil
	})))
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), "player-1", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || category != "character" || author != "System" || publishedAt.IsZero() {
		t.Fatalf("joining news: calls=%d category=%q author=%q publishedAt=%v", calls, category, author, publishedAt)
	}
	if !strings.Contains(title, created.Name) || !strings.Contains(content, created.ID) || strings.Contains(content, created.PlayerID) {
		t.Fatalf("news must identify the created character without account identity: title=%q content=%q", title, content)
	}
	if strings.ContainsAny(title+content, "<>") {
		t.Fatalf("news contains HTML: title=%q content=%q", title, content)
	}
	if err := repo.Save(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	created.Name = "NewHero"
	if err := repo.Update(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("ordinary persistence published joining news: calls=%d", calls)
	}
}

func TestService_CreateNewsFailures(t *testing.T) {
	failure := errors.New("injected failure")
	for _, stage := range []string{"validation", "save", "publisher", "commit"} {
		t.Run(stage, func(t *testing.T) {
			repo := newMockRepository()
			if stage == "save" {
				repo.err = failure
			}
			txProvider := &mockTransactionProvider{fn: func(ctx context.Context, fn func(context.Context) error) error {
				if err := fn(ctx); err != nil {
					return err
				}
				if stage == "commit" {
					return failure
				}
				return nil
			}}
			calls := 0
			service, err := NewService(repo, WithTransactionProvider(txProvider), WithNewsPublisher(NewsPublisherFunc(func(context.Context, string, string, string, string, time.Time) error {
				calls++
				if stage == "publisher" {
					return failure
				}
				return nil
			})))
			if err != nil {
				t.Fatal(err)
			}
			name := "Hero"
			wantErr, wantCalls := failure, 1
			if stage == "validation" {
				name, wantErr, wantCalls = "", ErrInvalidName, 0
			} else if stage == "save" {
				wantCalls = 0
			}
			created, err := service.Create(context.Background(), "player-1", name)
			if !errors.Is(err, wantErr) || created.ID != "" || calls != wantCalls {
				t.Fatalf("created=%+v err=%v calls=%d; want error=%v calls=%d", created, err, calls, wantErr, wantCalls)
			}
		})
	}
}

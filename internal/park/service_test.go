package park_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/ratelimit"
)

type mockRepository struct {
	posts          []park.Post
	latestPostTime map[string]time.Time
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		posts:          make([]park.Post, 0),
		latestPostTime: make(map[string]time.Time),
	}
}

func (m *mockRepository) CreatePost(ctx context.Context, post park.Post) error {
	m.posts = append([]park.Post{post}, m.posts...)
	m.latestPostTime[post.CharacterID] = post.CreatedAt
	return nil
}

func (m *mockRepository) GetRecentPosts(ctx context.Context, limit int, offset int) ([]park.Post, int, error) {
	total := len(m.posts)
	if offset >= total {
		return []park.Post{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return m.posts[offset:end], total, nil
}

func (m *mockRepository) GetLatestPostTimeByCharacter(ctx context.Context, characterID string) (time.Time, error) {
	t, ok := m.latestPostTime[characterID]
	if !ok {
		return time.Time{}, nil
	}
	return t, nil
}

type mockCharacterReader struct {
	characters map[string]corecharacter.Character
}

func (m *mockCharacterReader) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func TestService_PostMessage(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	charReader := &mockCharacterReader{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "アリス", JobID: "hero"},
		},
	}

	fixedTime := time.Date(2026, 8, 27, 20, 0, 0, 0, time.UTC)
	svc, err := park.NewService(
		repo,
		charReader,
		park.WithRateLimit(3*time.Second),
		park.WithNowFunc(func() time.Time { return fixedTime }),
	)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	t.Run("Successful post", func(t *testing.T) {
		post, err := svc.PostMessage(ctx, "char-1", "こんにちは！", "#ff0000", "")
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if post.CharacterID != "char-1" || post.CharacterName != "アリス" {
			t.Errorf("unexpected post metadata: %+v", post)
		}
		if post.Content != "こんにちは！" {
			t.Errorf("unexpected content: %s", post.Content)
		}
		if post.Color != "#ff0000" {
			t.Errorf("unexpected color: %s", post.Color)
		}
	})

	t.Run("Rate limit enforced", func(t *testing.T) {
		// Attempting immediate next post with same time
		_, err := svc.PostMessage(ctx, "char-1", "連投テスト", "#000000", "")
		if err != park.ErrRateLimited {
			t.Fatalf("expected ErrRateLimited, got %v", err)
		}
	})

	t.Run("Post after rate limit window", func(t *testing.T) {
		// Advance time by 4 seconds
		laterTime := fixedTime.Add(4 * time.Second)
		laterSvc, _ := park.NewService(
			repo,
			charReader,
			park.WithRateLimit(3*time.Second),
			park.WithNowFunc(func() time.Time { return laterTime }),
		)

		post, err := laterSvc.PostMessage(ctx, "char-1", "2回目の投稿", "", "ボブ")
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if post.RecipientName != "ボブ" {
			t.Errorf("expected recipient ボブ, got %s", post.RecipientName)
		}
		if post.Color != park.DefaultColor {
			t.Errorf("expected default color, got %s", post.Color)
		}
	})

	t.Run("Character not found", func(t *testing.T) {
		_, err := svc.PostMessage(ctx, "non-existent", "test", "", "")
		if err != park.ErrCharacterNotFound {
			t.Fatalf("expected ErrCharacterNotFound, got %v", err)
		}
	})
}

func TestService_PostMessage_WithLimiter(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	charReader := &mockCharacterReader{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "アリス", JobID: "hero"},
		},
	}
	limiter := ratelimit.NewMemoryLimiter()
	svc, err := park.NewService(
		repo,
		charReader,
		park.WithRateLimiter(limiter),
		park.WithRateLimit(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	// First post succeeds
	_, err = svc.PostMessage(ctx, "char-1", "First post", "", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Immediate second post blocked by limiter
	_, err = svc.PostMessage(ctx, "char-1", "Spam post", "", "")
	if err != park.ErrRateLimited {
		t.Fatalf("expected ErrRateLimited from limiter, got %v", err)
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)
	_, err = svc.PostMessage(ctx, "char-1", "Post after window", "", "")
	if err != nil {
		t.Fatalf("expected post allowed after window, got %v", err)
	}
}

func TestService_GetRecentPosts(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	charReader := &mockCharacterReader{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "アリス"},
		},
	}

	svc, _ := park.NewService(repo, charReader)

	// Create 5 posts
	for i := 0; i < 5; i++ {
		_ = repo.CreatePost(ctx, park.Post{
			ID:            string(rune('A' + i)),
			CharacterID:   "char-1",
			CharacterName: "アリス",
			Content:       "msg",
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	posts, total, err := svc.GetRecentPosts(ctx, 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
}

func TestService_NPCInteractions(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	charReader := &mockCharacterReader{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "アリス", JobID: "hero"},
		},
	}

	svc, _ := park.NewService(repo, charReader, park.WithNPCRNG(rand.New(rand.NewSource(123))))

	t.Run("Talk to NPC", func(t *testing.T) {
		dialogue, err := svc.TalkToNPC(ctx, "char-1")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if dialogue == "" {
			t.Errorf("expected non-empty dialogue")
		}
	})

	t.Run("Divinate with NPC", func(t *testing.T) {
		res, err := svc.Divinate(ctx, "char-1")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if res.Fortune == "" || res.LuckyColor == "" || res.Message == "" {
			t.Errorf("incomplete divination result: %+v", res)
		}
	})

	t.Run("Inspect NPC", func(t *testing.T) {
		res := svc.InspectNPC()
		if res == "" {
			t.Errorf("expected non-empty inspect dialogue")
		}
	})
}

func TestConcurrentNPCInteractions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMockRepository()
	charReader := &mockCharacterReader{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "アリス", JobID: "hero"},
		},
	}

	svc, err := park.NewService(repo, charReader)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	const goroutines = 100
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch (id + i) % 3 {
				case 0:
					d, err := svc.TalkToNPC(ctx, "char-1")
					if err != nil || d == "" {
						t.Errorf("unexpected TalkToNPC error: %v", err)
					}
				case 1:
					res, err := svc.Divinate(ctx, "char-1")
					if err != nil || res.Fortune == "" {
						t.Errorf("unexpected Divinate error: %v", err)
					}
				case 2:
					insp := svc.InspectNPC()
					if insp == "" {
						t.Errorf("unexpected InspectNPC empty result")
					}
				}
			}
		}(g)
	}

	wg.Wait()
}

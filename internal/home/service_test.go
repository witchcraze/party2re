package home

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCharReader struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharReader) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

type mockHomeRepo struct {
	homes   map[string]CharacterHome
	letters map[string]Letter
	phrases map[string][]CompanionPhrase
	notices map[string][]DeliveryNotice
}

func newMockHomeRepo() *mockHomeRepo {
	return &mockHomeRepo{
		homes:   make(map[string]CharacterHome),
		letters: make(map[string]Letter),
		phrases: make(map[string][]CompanionPhrase),
		notices: make(map[string][]DeliveryNotice),
	}
}

func (m *mockHomeRepo) GetHome(ctx context.Context, characterID string) (CharacterHome, error) {
	h, ok := m.homes[characterID]
	if !ok {
		return CharacterHome{
			CharacterID:   characterID,
			Theme:         DefaultTheme,
			Motto:         "",
			CompanionName: DefaultCompanionName,
			VisitorCount:  0,
			UpdatedAt:     time.Now().UTC(),
		}, nil
	}
	return h, nil
}

func (m *mockHomeRepo) SaveHome(ctx context.Context, home CharacterHome) error {
	m.homes[home.CharacterID] = home
	return nil
}

func (m *mockHomeRepo) IncrementVisitorCount(ctx context.Context, characterID string, visitedAt time.Time) error {
	h, _ := m.GetHome(ctx, characterID)
	h.VisitorCount++
	h.LastVisitedAt = &visitedAt
	h.UpdatedAt = visitedAt
	m.homes[characterID] = h
	return nil
}

func (m *mockHomeRepo) CreateLetter(ctx context.Context, letter Letter) error {
	m.letters[letter.ID] = letter
	return nil
}

func (m *mockHomeRepo) GetLetterByID(ctx context.Context, id string) (Letter, error) {
	l, ok := m.letters[id]
	if !ok {
		return Letter{}, ErrLetterNotFound
	}
	return l, nil
}

func (m *mockHomeRepo) ListInboxLetters(ctx context.Context, recipientID string, limit, offset int) ([]Letter, int, error) {
	var list []Letter
	for _, l := range m.letters {
		if l.RecipientCharacterID == recipientID {
			list = append(list, l)
		}
	}
	return list, len(list), nil
}

func (m *mockHomeRepo) ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]Letter, int, error) {
	var list []Letter
	for _, l := range m.letters {
		if l.SenderCharacterID == senderID {
			list = append(list, l)
		}
	}
	return list, len(list), nil
}

func (m *mockHomeRepo) GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error) {
	var count int
	for _, l := range m.letters {
		if l.RecipientCharacterID == recipientID && !l.IsRead {
			count++
		}
	}
	return count, nil
}

func (m *mockHomeRepo) MarkLetterAsRead(ctx context.Context, id, recipientID string, readAt time.Time) error {
	l, ok := m.letters[id]
	if !ok {
		return ErrLetterNotFound
	}
	if l.RecipientCharacterID != recipientID {
		return ErrForbidden
	}
	l.IsRead = true
	l.ReadAt = &readAt
	m.letters[id] = l
	return nil
}

func (m *mockHomeRepo) DeleteLetter(ctx context.Context, id, characterID string) error {
	l, ok := m.letters[id]
	if !ok {
		return ErrLetterNotFound
	}
	if l.RecipientCharacterID != characterID && l.SenderCharacterID != characterID {
		return ErrForbidden
	}
	delete(m.letters, id)
	return nil
}

func (m *mockHomeRepo) AddCompanionPhrase(ctx context.Context, phrase CompanionPhrase) error {
	m.phrases[phrase.CharacterID] = append(m.phrases[phrase.CharacterID], phrase)
	return nil
}

func (m *mockHomeRepo) DeleteCompanionPhrase(ctx context.Context, id, characterID string) error {
	list := m.phrases[characterID]
	var updated []CompanionPhrase
	found := false
	for _, p := range list {
		if p.ID == id {
			found = true
			continue
		}
		updated = append(updated, p)
	}
	if !found {
		return ErrPhraseNotFound
	}
	m.phrases[characterID] = updated
	return nil
}

func (m *mockHomeRepo) ListCompanionPhrases(ctx context.Context, characterID string) ([]CompanionPhrase, error) {
	return m.phrases[characterID], nil
}

func (m *mockHomeRepo) AddDeliveryNotice(ctx context.Context, notice DeliveryNotice) error {
	m.notices[notice.CharacterID] = append(m.notices[notice.CharacterID], notice)
	return nil
}

func (m *mockHomeRepo) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]DeliveryNotice, error) {
	var list []DeliveryNotice
	for _, n := range m.notices[characterID] {
		if unclearedOnly && n.IsCleared {
			continue
		}
		list = append(list, n)
	}
	return list, nil
}

func (m *mockHomeRepo) ClearDeliveryNotices(ctx context.Context, characterID string) error {
	for i := range m.notices[characterID] {
		m.notices[characterID][i].IsCleared = true
	}
	return nil
}

func TestHomeService(t *testing.T) {
	ctx := context.Background()
	chars := &mockCharReader{
		chars: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", PlayerID: "player-1", Name: "Hero", Money: 1000},
			"char-2": {ID: "char-2", PlayerID: "player-2", Name: "Mage", Money: 500},
		},
	}
	repo := newMockHomeRepo()
	fixedTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(42))

	service, err := NewService(repo, chars, WithNowFunc(func() time.Time { return fixedTime }), WithRNG(rng))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("visit home and update home settings", func(t *testing.T) {
		// Owner visit -> does not increment visitor count
		view, err := service.GetHomeView(ctx, "char-1", "char-1")
		if err != nil {
			t.Fatalf("GetHomeView failed: %v", err)
		}
		if view.Home.VisitorCount != 0 {
			t.Errorf("expected 0 visitor count, got %d", view.Home.VisitorCount)
		}

		// Visitor visit -> increments visitor count
		view, err = service.GetHomeView(ctx, "char-1", "char-2")
		if err != nil {
			t.Fatalf("GetHomeView failed: %v", err)
		}
		if view.Home.VisitorCount != 1 {
			t.Errorf("expected 1 visitor count, got %d", view.Home.VisitorCount)
		}

		// Update home settings
		updated, err := service.UpdateHome(ctx, "char-1", "#ff9933", "Welcome to my crib!", "スライム")
		if err != nil {
			t.Fatalf("UpdateHome failed: %v", err)
		}
		if updated.Theme != "#ff9933" || updated.CompanionName != "スライム" {
			t.Errorf("unexpected updated home: %+v", updated)
		}
	})

	t.Run("send and read letter", func(t *testing.T) {
		letter, err := service.SendLetter(ctx, "char-1", "char-2", "Let's party!", "#0088ff")
		if err != nil {
			t.Fatalf("SendLetter failed: %v", err)
		}
		if letter.SenderName != "Hero" || letter.RecipientName != "Mage" {
			t.Errorf("unexpected letter: %+v", letter)
		}

		// Recipient unread count
		count, err := service.GetUnreadLetterCount(ctx, "char-2")
		if err != nil || count != 1 {
			t.Errorf("expected 1 unread letter, got count=%d, err=%v", count, err)
		}

		// Recipient marks as read
		err = service.ReadLetter(ctx, letter.ID, "char-2")
		if err != nil {
			t.Fatalf("ReadLetter failed: %v", err)
		}

		count, _ = service.GetUnreadLetterCount(ctx, "char-2")
		if count != 0 {
			t.Errorf("expected 0 unread letters, got %d", count)
		}

		// Wrong recipient marks as read
		err = service.ReadLetter(ctx, letter.ID, "char-1")
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}

		// Delete letter
		err = service.DeleteLetter(ctx, letter.ID, "char-2")
		if err != nil {
			t.Fatalf("DeleteLetter failed: %v", err)
		}
	})

	t.Run("companion phrases teaching and talking", func(t *testing.T) {
		// Companion talks when no phrases taught -> default fallback
		talk, err := service.TalkToCompanion(ctx, "char-1")
		if err != nil || talk == "" {
			t.Errorf("TalkToCompanion failed: %v, talk=%s", err, talk)
		}

		// Teach phrase
		p1, err := service.TeachCompanionPhrase(ctx, "char-1", "クエッ！")
		if err != nil {
			t.Fatalf("TeachCompanionPhrase failed: %v", err)
		}

		// List phrases
		phrases, err := service.ListCompanionPhrases(ctx, "char-1")
		if err != nil || len(phrases) != 1 {
			t.Fatalf("expected 1 phrase, got %d", len(phrases))
		}

		// Companion talks taught phrase
		talk, err = service.TalkToCompanion(ctx, "char-1")
		if err != nil || talk != "クエッ！" {
			t.Errorf("expected 'クエッ！', got %s", talk)
		}

		// Forget phrase
		err = service.ForgetCompanionPhrase(ctx, p1.ID, "char-1")
		if err != nil {
			t.Fatalf("ForgetCompanionPhrase failed: %v", err)
		}

		phrases, _ = service.ListCompanionPhrases(ctx, "char-1")
		if len(phrases) != 0 {
			t.Errorf("expected 0 phrases after forget, got %d", len(phrases))
		}
	})

	t.Run("delivery notices", func(t *testing.T) {
		err := service.AddDeliveryNotice(ctx, "char-1", "money_remittance", "1,000 G has arrived from Player 2!")
		if err != nil {
			t.Fatalf("AddDeliveryNotice failed: %v", err)
		}

		notices, err := service.ListDeliveryNotices(ctx, "char-1", true)
		if err != nil || len(notices) != 1 {
			t.Fatalf("expected 1 notice, got %d", len(notices))
		}

		err = service.ClearDeliveryNotices(ctx, "char-1")
		if err != nil {
			t.Fatalf("ClearDeliveryNotices failed: %v", err)
		}

		notices, _ = service.ListDeliveryNotices(ctx, "char-1", true)
		if len(notices) != 0 {
			t.Errorf("expected 0 uncleared notices, got %d", len(notices))
		}
	})
}

func TestConcurrentTalkToCompanion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chars := &mockCharReader{
		chars: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero"},
		},
	}
	repo := newMockHomeRepo()
	service, err := NewService(repo, chars)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	for i := 0; i < 5; i++ {
		_, _ = service.TeachCompanionPhrase(ctx, "char-1", fmt.Sprintf("phrase-%d", i))
	}

	const goroutines = 100
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				phrase, err := service.TalkToCompanion(ctx, "char-1")
				if err != nil || phrase == "" {
					t.Errorf("unexpected TalkToCompanion result: %v, %s", err, phrase)
				}
			}
		}()
	}

	wg.Wait()
}

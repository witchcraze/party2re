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

type mockCharUpdater struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharUpdater) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharUpdater) Update(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type mockHomeRepo struct {
	homes   map[string]CharacterHome
	letters map[string]Letter
	phrases map[string][]CompanionPhrase
	notices map[string][]DeliveryNotice
	chars   map[string]corecharacter.Character
}

func newMockHomeRepo(chars ...map[string]corecharacter.Character) *mockHomeRepo {
	var c map[string]corecharacter.Character
	if len(chars) > 0 {
		c = chars[0]
	} else {
		c = make(map[string]corecharacter.Character)
	}
	return &mockHomeRepo{
		homes:   make(map[string]CharacterHome),
		letters: make(map[string]Letter),
		phrases: make(map[string][]CompanionPhrase),
		notices: make(map[string][]DeliveryNotice),
		chars:   c,
	}
}

func (m *mockHomeRepo) GetHome(ctx context.Context, characterID string) (CharacterHome, error) {
	h, ok := m.homes[characterID]
	if !ok {
		return CharacterHome{
			CharacterID:   characterID,
			TownID:        "",
			HouseStyle:    "",
			ExpiresAt:     nil,
			CompanionName: DefaultCompanionName,
			UpdatedAt:     time.Now().UTC(),
		}, nil
	}
	return h, nil
}

func (m *mockHomeRepo) SaveHome(ctx context.Context, home CharacterHome) error {
	m.homes[home.CharacterID] = home
	return nil
}

func (m *mockHomeRepo) CountActiveTownHouses(ctx context.Context, townID string, now time.Time) (int, error) {
	count := 0
	for _, h := range m.homes {
		if h.TownID == townID && h.IsActive(now) {
			count++
		}
	}
	return count, nil
}

func (m *mockHomeRepo) FindActiveHomeByCharacterID(ctx context.Context, characterID string, now time.Time) (CharacterHome, error) {
	h, ok := m.homes[characterID]
	if !ok || !h.IsActive(now) {
		return CharacterHome{}, ErrHouseNotFound
	}
	return h, nil
}

func (m *mockHomeRepo) FindActiveHomeByCharacterName(ctx context.Context, characterName string, now time.Time) (CharacterHome, corecharacter.Character, error) {
	for _, h := range m.homes {
		if h.IsActive(now) {
			if c, ok := m.chars[h.CharacterID]; ok && c.Name == characterName {
				return h, c, nil
			}
		}
	}
	return CharacterHome{}, corecharacter.Character{}, ErrHouseNotFound
}

func (m *mockHomeRepo) ListActiveTownHouses(ctx context.Context, townID string, now time.Time) ([]CharacterHome, error) {
	var list []CharacterHome
	for _, h := range m.homes {
		if h.TownID == townID && h.IsActive(now) {
			list = append(list, h)
		}
	}
	return list, nil
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
		if l.RecipientCharacterID == recipientID && !l.IsDeletedByRecipient {
			list = append(list, l)
		}
	}
	return list, len(list), nil
}

func (m *mockHomeRepo) ListInboxLettersByCursor(ctx context.Context, recipientID string, limit int, beforeTime time.Time, beforeID string) ([]Letter, error) {
	var list []Letter
	for _, l := range m.letters {
		if l.RecipientCharacterID == recipientID && !l.IsDeletedByRecipient {
			if beforeTime.IsZero() && beforeID == "" {
				list = append(list, l)
			} else if !beforeTime.IsZero() && beforeID != "" {
				if l.CreatedAt.Before(beforeTime) || (l.CreatedAt.Equal(beforeTime) && l.ID < beforeID) {
					list = append(list, l)
				}
			} else if !beforeTime.IsZero() {
				if l.CreatedAt.Before(beforeTime) {
					list = append(list, l)
				}
			} else {
				if l.ID < beforeID {
					list = append(list, l)
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockHomeRepo) ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]Letter, int, error) {
	var list []Letter
	for _, l := range m.letters {
		if l.SenderCharacterID == senderID && !l.IsDeletedBySender {
			list = append(list, l)
		}
	}
	return list, len(list), nil
}

func (m *mockHomeRepo) ListOutboxLettersByCursor(ctx context.Context, senderID string, limit int, beforeTime time.Time, beforeID string) ([]Letter, error) {
	var list []Letter
	for _, l := range m.letters {
		if l.SenderCharacterID == senderID && !l.IsDeletedBySender {
			if beforeTime.IsZero() && beforeID == "" {
				list = append(list, l)
			} else if !beforeTime.IsZero() && beforeID != "" {
				if l.CreatedAt.Before(beforeTime) || (l.CreatedAt.Equal(beforeTime) && l.ID < beforeID) {
					list = append(list, l)
				}
			} else if !beforeTime.IsZero() {
				if l.CreatedAt.Before(beforeTime) {
					list = append(list, l)
				}
			} else {
				if l.ID < beforeID {
					list = append(list, l)
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockHomeRepo) GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error) {
	var count int
	for _, l := range m.letters {
		if l.RecipientCharacterID == recipientID && !l.IsRead && !l.IsDeletedByRecipient {
			count++
		}
	}
	return count, nil
}

func (m *mockHomeRepo) MarkLetterAsRead(ctx context.Context, id, recipientID string, readAt time.Time) error {
	l, ok := m.letters[id]
	if !ok || l.IsDeletedByRecipient {
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

	if l.SenderCharacterID == characterID {
		if l.IsDeletedBySender && !l.IsDeletedByRecipient {
			return ErrLetterNotFound
		}
		l.IsDeletedBySender = true
	}
	if l.RecipientCharacterID == characterID {
		if l.IsDeletedByRecipient && !l.IsDeletedBySender {
			return ErrLetterNotFound
		}
		l.IsDeletedByRecipient = true
	}

	if l.IsDeletedBySender && l.IsDeletedByRecipient {
		delete(m.letters, id)
	} else {
		m.letters[id] = l
	}
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
	repo := newMockHomeRepo(chars.chars)
	fixedTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(42))
	charUpdater := &mockCharUpdater{chars: chars.chars}

	service, err := NewService(
		repo,
		chars,
		WithNowFunc(func() time.Time { return fixedTime }),
		WithRNG(rng),
		WithCharacterUpdater(charUpdater),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("visit home and update companion name", func(t *testing.T) {
		// Owner visit
		view, err := service.GetHomeView(ctx, "char-1", "char-1")
		if err != nil {
			t.Fatalf("GetHomeView failed: %v", err)
		}
		if !view.IsOwner {
			t.Errorf("expected isOwner true")
		}

		// Visitor visit
		view, err = service.GetHomeView(ctx, "char-1", "char-2")
		if err != nil {
			t.Fatalf("GetHomeView failed: %v", err)
		}
		if view.IsOwner {
			t.Errorf("expected isOwner false")
		}

		// Update companion name
		updated, err := service.UpdateHome(ctx, "char-1", "スライム")
		if err != nil {
			t.Fatalf("UpdateHome failed: %v", err)
		}
		if updated.CompanionName != "スライム" {
			t.Errorf("unexpected updated home companion: %+v", updated)
		}
	})

	t.Run("build house estate cycle, check house, and town limits", func(t *testing.T) {
		// Town 1 (メケメケ村: 500G, 5 days, max 10 houses, styles 001-004)
		buildRes, err := service.BuildHouse(ctx, "char-1", "town1", "001")
		if err != nil {
			t.Fatalf("BuildHouse failed: %v", err)
		}
		if buildRes.TownID != "town1" || buildRes.HouseStyle != "001" {
			t.Errorf("unexpected build result: %+v", buildRes)
		}
		if chars.chars["char-1"].Money != 500 {
			t.Errorf("expected money 500 after building, got %d", chars.chars["char-1"].Money)
		}

		// Second house attempt -> ErrAlreadyOwnsHouse
		_, err = service.BuildHouse(ctx, "char-1", "town2", "005")
		if !errors.Is(err, ErrAlreadyOwnsHouse) {
			t.Errorf("expected ErrAlreadyOwnsHouse, got %v", err)
		}

		// Invalid style -> ErrInvalidHouseStyle
		_, err = service.BuildHouse(ctx, "char-2", "town1", "099")
		if !errors.Is(err, ErrInvalidHouseStyle) {
			t.Errorf("expected ErrInvalidHouseStyle, got %v", err)
		}

		// Insufficient funds (Mage has 500G, town 2 costs 1500G)
		_, err = service.BuildHouse(ctx, "char-2", "town2", "005")
		if !errors.Is(err, ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}

		// Check house by char ID
		checkRes, err := service.CheckHouse(ctx, "char-1")
		if err != nil {
			t.Fatalf("CheckHouse failed: %v", err)
		}
		if checkRes.OwnerName != "Hero" {
			t.Errorf("expected owner Hero, got %s", checkRes.OwnerName)
		}

		// Check house by character Name
		checkResName, err := service.CheckHouse(ctx, "Hero")
		if err != nil {
			t.Fatalf("CheckHouse by name failed: %v", err)
		}
		if checkResName.CharacterID != "char-1" {
			t.Errorf("expected char-1, got %s", checkResName.CharacterID)
		}

		// List town houses
		list, err := service.ListTownHouses(ctx, "town1")
		if err != nil || len(list) != 1 {
			t.Errorf("expected 1 house in town1, got len=%d, err=%v", len(list), err)
		}
	})

	t.Run("set character color", func(t *testing.T) {
		if err := service.SetCharacterColor(ctx, "char-1", "#123456"); err != nil {
			t.Fatalf("SetCharacterColor failed: %v", err)
		}
		if chars.chars["char-1"].Color != "#123456" {
			t.Errorf("expected color #123456, got %s", chars.chars["char-1"].Color)
		}

		if err := service.SetCharacterColor(ctx, "char-1", "invalid"); err == nil {
			t.Errorf("expected error for invalid color")
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

		// Delete letter by recipient -> recipient inbox is empty, but sender outbox still has it
		err = service.DeleteLetter(ctx, letter.ID, "char-2")
		if err != nil {
			t.Fatalf("DeleteLetter failed: %v", err)
		}

		inbox, err := service.ListInbox(ctx, "char-2", 10, 0)
		if err != nil || inbox.Total != 0 || len(inbox.Items) != 0 {
			t.Errorf("expected empty inbox after recipient deletion, got total=%d, len=%d", inbox.Total, len(inbox.Items))
		}

		outbox, err := service.ListOutbox(ctx, "char-1", 10, 0)
		if err != nil || outbox.Total != 1 || len(outbox.Items) != 1 {
			t.Errorf("expected sender outbox still retained, got total=%d, len=%d", outbox.Total, len(outbox.Items))
		}

		// Delete letter by sender -> sender outbox now empty as well
		err = service.DeleteLetter(ctx, letter.ID, "char-1")
		if err != nil {
			t.Fatalf("DeleteLetter by sender failed: %v", err)
		}

		outbox, err = service.ListOutbox(ctx, "char-1", 10, 0)
		if err != nil || outbox.Total != 0 || len(outbox.Items) != 0 {
			t.Errorf("expected empty outbox after sender deletion, got total=%d, len=%d", outbox.Total, len(outbox.Items))
		}
	})

	t.Run("sender deletes letter first without affecting recipient inbox", func(t *testing.T) {
		letter, err := service.SendLetter(ctx, "char-1", "char-2", "Secret mission!", "#ff5500")
		if err != nil {
			t.Fatalf("SendLetter failed: %v", err)
		}

		// Sender deletes letter from outbox immediately
		err = service.DeleteLetter(ctx, letter.ID, "char-1")
		if err != nil {
			t.Fatalf("DeleteLetter by sender failed: %v", err)
		}

		// Sender outbox should be 0
		outbox, err := service.ListOutbox(ctx, "char-1", 10, 0)
		if err != nil || outbox.Total != 0 {
			t.Errorf("expected 0 letters in sender outbox, got total=%d", outbox.Total)
		}

		// Recipient still has unread letter count 1 and can read inbox
		count, err := service.GetUnreadLetterCount(ctx, "char-2")
		if err != nil || count != 1 {
			t.Errorf("expected 1 unread letter for recipient, got count=%d, err=%v", count, err)
		}

		inbox, err := service.ListInbox(ctx, "char-2", 10, 0)
		if err != nil || inbox.Total != 1 || len(inbox.Items) != 1 {
			t.Fatalf("expected 1 letter in recipient inbox, got total=%d", inbox.Total)
		}

		// Recipient marks as read
		err = service.ReadLetter(ctx, letter.ID, "char-2")
		if err != nil {
			t.Fatalf("ReadLetter failed: %v", err)
		}

		// Recipient deletes letter
		err = service.DeleteLetter(ctx, letter.ID, "char-2")
		if err != nil {
			t.Fatalf("DeleteLetter by recipient failed: %v", err)
		}

		inbox, err = service.ListInbox(ctx, "char-2", 10, 0)
		if err != nil || inbox.Total != 0 {
			t.Errorf("expected 0 letters in recipient inbox, got total=%d", inbox.Total)
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

func TestListInboxByCursor(t *testing.T) {
	ctx := context.Background()
	chars := &mockCharReader{
		chars: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero"},
			"char-2": {ID: "char-2", Name: "Friend"},
		},
	}
	repo := newMockHomeRepo()
	service, err := NewService(repo, chars)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		letID := fmt.Sprintf("let-%d", i)
		repo.letters[letID] = Letter{
			ID:                   letID,
			SenderCharacterID:    "char-2",
			SenderName:           "Friend",
			RecipientCharacterID: "char-1",
			RecipientName:        "Hero",
			Content:              fmt.Sprintf("Hello %d", i),
			CreatedAt:            now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := service.ListInboxByCursor(ctx, "char-1", 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}

func TestListOutboxByCursor(t *testing.T) {
	ctx := context.Background()
	chars := &mockCharReader{
		chars: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero"},
			"char-2": {ID: "char-2", Name: "Friend"},
		},
	}
	repo := newMockHomeRepo()
	service, err := NewService(repo, chars)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		letID := fmt.Sprintf("out-%d", i)
		repo.letters[letID] = Letter{
			ID:                   letID,
			SenderCharacterID:    "char-1",
			SenderName:           "Hero",
			RecipientCharacterID: "char-2",
			RecipientName:        "Friend",
			Content:              fmt.Sprintf("Sent %d", i),
			CreatedAt:            now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := service.ListOutboxByCursor(ctx, "char-1", 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}

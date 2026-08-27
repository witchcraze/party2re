package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char1, err := database.CreateTestCharacter(ctx, db, "HomeHero1")
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacter(ctx, db, "HomeHero2")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Home Settings & Visitor Count
	t.Run("home settings and visitor count", func(t *testing.T) {
		h, err := repo.GetHome(ctx, char1.ID)
		if err != nil {
			t.Fatalf("GetHome failed: %v", err)
		}
		if h.VisitorCount != 0 {
			t.Errorf("expected 0 visitor count, got %d", h.VisitorCount)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		err = repo.IncrementVisitorCount(ctx, char1.ID, now)
		if err != nil {
			t.Fatalf("IncrementVisitorCount failed: %v", err)
		}

		h, err = repo.GetHome(ctx, char1.ID)
		if err != nil || h.VisitorCount != 1 {
			t.Errorf("expected 1 visitor count, got %d, err=%v", h.VisitorCount, err)
		}

		h.Theme = "#112233"
		h.Motto = "Greetings travelers!"
		h.CompanionName = "モモンガ"
		err = repo.SaveHome(ctx, h)
		if err != nil {
			t.Fatalf("SaveHome failed: %v", err)
		}

		h, _ = repo.GetHome(ctx, char1.ID)
		if h.Theme != "#112233" || h.CompanionName != "モモンガ" {
			t.Errorf("unexpected home settings: %+v", h)
		}
	})

	// 2. Letters (Mailbox)
	t.Run("letters lifecycle", func(t *testing.T) {
		letterID := fmt.Sprintf("letter_%d", time.Now().UnixNano())
		if len(letterID) > 32 {
			letterID = letterID[:32]
		}
		now := time.Now().UTC().Truncate(time.Microsecond)

		l := home.Letter{
			ID:                   letterID,
			SenderCharacterID:    char1.ID,
			SenderName:           char1.Name,
			RecipientCharacterID: char2.ID,
			RecipientName:        char2.Name,
			Content:              "Join our dungeon party!",
			Color:                "#ff0000",
			IsRead:               false,
			CreatedAt:            now,
		}

		err = repo.CreateLetter(ctx, l)
		if err != nil {
			t.Fatalf("CreateLetter failed: %v", err)
		}

		count, err := repo.GetUnreadLetterCount(ctx, char2.ID)
		if err != nil || count != 1 {
			t.Errorf("expected 1 unread letter, got %d, err=%v", count, err)
		}

		inbox, total, err := repo.ListInboxLetters(ctx, char2.ID, 10, 0)
		if err != nil || total != 1 || len(inbox) != 1 {
			t.Errorf("ListInboxLetters failed: total=%d, len=%d, err=%v", total, len(inbox), err)
		}

		outbox, total, err := repo.ListOutboxLetters(ctx, char1.ID, 10, 0)
		if err != nil || total != 1 || len(outbox) != 1 {
			t.Errorf("ListOutboxLetters failed: total=%d, len=%d, err=%v", total, len(outbox), err)
		}

		// Mark read with wrong character
		err = repo.MarkLetterAsRead(ctx, l.ID, char1.ID, now)
		if !errors.Is(err, home.ErrForbidden) {
			t.Errorf("expected ErrForbidden when wrong char marks as read, got %v", err)
		}

		// Mark read with recipient
		err = repo.MarkLetterAsRead(ctx, l.ID, char2.ID, now)
		if err != nil {
			t.Fatalf("MarkLetterAsRead failed: %v", err)
		}

		count, _ = repo.GetUnreadLetterCount(ctx, char2.ID)
		if count != 0 {
			t.Errorf("expected 0 unread letters, got %d", count)
		}

		// Delete letter
		err = repo.DeleteLetter(ctx, l.ID, char2.ID)
		if err != nil {
			t.Fatalf("DeleteLetter failed: %v", err)
		}
	})

	// 3. Companion Phrases
	t.Run("companion phrases CRUD", func(t *testing.T) {
		phraseID := fmt.Sprintf("phrase_%d", time.Now().UnixNano())
		if len(phraseID) > 32 {
			phraseID = phraseID[:32]
		}
		now := time.Now().UTC().Truncate(time.Microsecond)

		cp := home.CompanionPhrase{
			ID:          phraseID,
			CharacterID: char1.ID,
			Phrase:      "お宝みっけ！",
			CreatedAt:   now,
		}

		err = repo.AddCompanionPhrase(ctx, cp)
		if err != nil {
			t.Fatalf("AddCompanionPhrase failed: %v", err)
		}

		phrases, err := repo.ListCompanionPhrases(ctx, char1.ID)
		if err != nil || len(phrases) != 1 {
			t.Fatalf("expected 1 phrase, got %d, err=%v", len(phrases), err)
		}

		err = repo.DeleteCompanionPhrase(ctx, phraseID, char1.ID)
		if err != nil {
			t.Fatalf("DeleteCompanionPhrase failed: %v", err)
		}

		phrases, _ = repo.ListCompanionPhrases(ctx, char1.ID)
		if len(phrases) != 0 {
			t.Errorf("expected 0 phrases, got %d", len(phrases))
		}
	})

	// 4. Delivery Notices
	t.Run("delivery notices CRUD", func(t *testing.T) {
		noticeID := fmt.Sprintf("notice_%d", time.Now().UnixNano())
		if len(noticeID) > 32 {
			noticeID = noticeID[:32]
		}
		now := time.Now().UTC().Truncate(time.Microsecond)

		n := home.DeliveryNotice{
			ID:          noticeID,
			CharacterID: char1.ID,
			NoticeType:  "item_transfer",
			Message:     "500 G sent to depot",
			IsCleared:   false,
			CreatedAt:   now,
		}

		err = repo.AddDeliveryNotice(ctx, n)
		if err != nil {
			t.Fatalf("AddDeliveryNotice failed: %v", err)
		}

		notices, err := repo.ListDeliveryNotices(ctx, char1.ID, true)
		if err != nil || len(notices) != 1 {
			t.Fatalf("expected 1 notice, got %d, err=%v", len(notices), err)
		}

		err = repo.ClearDeliveryNotices(ctx, char1.ID)
		if err != nil {
			t.Fatalf("ClearDeliveryNotices failed: %v", err)
		}

		notices, _ = repo.ListDeliveryNotices(ctx, char1.ID, true)
		if len(notices) != 0 {
			t.Errorf("expected 0 uncleared notices, got %d", len(notices))
		}
	})
}

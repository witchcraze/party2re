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

	char1Name := fmt.Sprintf("Hero1_%d", time.Now().UnixNano()%1000000)
	char2Name := fmt.Sprintf("Hero2_%d", time.Now().UnixNano()%1000000)
	char1, err := database.CreateTestCharacter(ctx, db, char1Name)
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacter(ctx, db, char2Name)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Home Estate Lifecycle
	t.Run("home estate lifecycle", func(t *testing.T) {
		h, err := repo.GetHome(ctx, char1.ID)
		if err != nil {
			t.Fatalf("GetHome failed: %v", err)
		}
		if h.CompanionName != home.DefaultCompanionName {
			t.Errorf("expected default companion name, got %s", h.CompanionName)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		expires := now.Add(5 * 24 * time.Hour)
		testTown := fmt.Sprintf("town_repo_%d", time.Now().UnixNano())

		h.TownID = testTown
		h.HouseStyle = "001"
		h.ExpiresAt = &expires
		h.CompanionName = "モモンガ"
		err = repo.SaveHome(ctx, h)
		if err != nil {
			t.Fatalf("SaveHome failed: %v", err)
		}

		h, err = repo.GetHome(ctx, char1.ID)
		if err != nil || h.TownID != testTown || h.HouseStyle != "001" || h.CompanionName != "モモンガ" {
			t.Fatalf("unexpected home settings: %+v, err=%v", h, err)
		}

		count, err := repo.CountActiveTownHouses(ctx, testTown, now)
		if err != nil || count != 1 {
			t.Errorf("expected 1 active house in %s, got %d, err=%v", testTown, count, err)
		}

		activeHome, err := repo.FindActiveHomeByCharacterID(ctx, char1.ID, now)
		if err != nil || activeHome.CharacterID != char1.ID {
			t.Errorf("FindActiveHomeByCharacterID failed: %+v, err=%v", activeHome, err)
		}

		activeHomeName, charFound, err := repo.FindActiveHomeByCharacterName(ctx, char1.Name, now)
		if err != nil || activeHomeName.CharacterID != char1.ID || charFound.ID != char1.ID {
			t.Errorf("FindActiveHomeByCharacterName failed: %+v, char=%+v, err=%v", activeHomeName, charFound, err)
		}

		list, err := repo.ListActiveTownHouses(ctx, testTown, now)
		if err != nil || len(list) != 1 {
			t.Errorf("ListActiveTownHouses failed: len=%d, err=%v", len(list), err)
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

		// Delete letter by recipient -> outbox for sender still has it
		err = repo.DeleteLetter(ctx, l.ID, char2.ID)
		if err != nil {
			t.Fatalf("DeleteLetter by recipient failed: %v", err)
		}

		inbox, total, err = repo.ListInboxLetters(ctx, char2.ID, 10, 0)
		if err != nil || total != 0 || len(inbox) != 0 {
			t.Errorf("expected 0 inbox letters after recipient deletion, got total=%d, len=%d", total, len(inbox))
		}

		outbox, total, err = repo.ListOutboxLetters(ctx, char1.ID, 10, 0)
		if err != nil || total != 1 || len(outbox) != 1 {
			t.Errorf("expected 1 outbox letter retained for sender, got total=%d, len=%d", total, len(outbox))
		}

		// Delete letter by sender -> outbox for sender now empty (both deleted -> purged)
		err = repo.DeleteLetter(ctx, l.ID, char1.ID)
		if err != nil {
			t.Fatalf("DeleteLetter by sender failed: %v", err)
		}

		outbox, total, err = repo.ListOutboxLetters(ctx, char1.ID, 10, 0)
		if err != nil || total != 0 || len(outbox) != 0 {
			t.Errorf("expected 0 outbox letters after both deleted, got total=%d, len=%d", total, len(outbox))
		}

		// Letter is now fully purged
		_, err = repo.GetLetterByID(ctx, l.ID)
		if !errors.Is(err, home.ErrLetterNotFound) {
			t.Errorf("expected ErrLetterNotFound after both deleted, got %v", err)
		}

		// Sender deletes first scenario
		letterID2 := fmt.Sprintf("letter2_%d", time.Now().UnixNano())
		if len(letterID2) > 32 {
			letterID2 = letterID2[:32]
		}
		l2 := home.Letter{
			ID:                   letterID2,
			SenderCharacterID:    char1.ID,
			SenderName:           char1.Name,
			RecipientCharacterID: char2.ID,
			RecipientName:        char2.Name,
			Content:              "Sender deletes first test",
			Color:                "#00ff00",
			IsRead:               false,
			CreatedAt:            now,
		}
		if err := repo.CreateLetter(ctx, l2); err != nil {
			t.Fatalf("CreateLetter 2 failed: %v", err)
		}

		// Sender deletes
		if err := repo.DeleteLetter(ctx, l2.ID, char1.ID); err != nil {
			t.Fatalf("DeleteLetter by sender failed: %v", err)
		}

		// Outbox is 0 for sender
		outbox, total, err = repo.ListOutboxLetters(ctx, char1.ID, 10, 0)
		if err != nil || total != 0 {
			t.Errorf("expected 0 outbox letters, got total=%d", total)
		}

		// Recipient still has unread count 1 and can view inbox
		count, err = repo.GetUnreadLetterCount(ctx, char2.ID)
		if err != nil || count != 1 {
			t.Errorf("expected 1 unread letter for recipient, got %d", count)
		}

		inbox, total, err = repo.ListInboxLetters(ctx, char2.ID, 10, 0)
		if err != nil || total != 1 {
			t.Errorf("expected 1 inbox letter for recipient, got total=%d", total)
		}

		// Recipient deletes
		if err := repo.DeleteLetter(ctx, l2.ID, char2.ID); err != nil {
			t.Fatalf("DeleteLetter by recipient failed: %v", err)
		}

		inbox, total, err = repo.ListInboxLetters(ctx, char2.ID, 10, 0)
		if err != nil || total != 0 {
			t.Errorf("expected 0 inbox letters for recipient, got total=%d", total)
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

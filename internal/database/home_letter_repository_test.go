package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeRepository_DeleteLetter_ConcurrentMutualDeletion(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char1Name := fmt.Sprintf("Sender_%d", time.Now().UnixNano()%1000000)
	char2Name := fmt.Sprintf("Recipient_%d", time.Now().UnixNano()%1000000)
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

	const letterCount = 20
	letters := make([]home.Letter, letterCount)
	for i := 0; i < letterCount; i++ {
		l := home.Letter{
			ID:                   fmt.Sprintf("ltr-%d-%d", time.Now().UnixNano()%100000000, i),
			SenderCharacterID:    char1.ID,
			SenderName:           char1.Name,
			RecipientCharacterID: char2.ID,
			RecipientName:        char2.Name,
			Content:              fmt.Sprintf("Hello %d", i),
			Color:                "#000000",
			CreatedAt:            time.Now().UTC(),
		}
		if err := repo.CreateLetter(ctx, l); err != nil {
			t.Fatalf("failed to create test letter: %v", err)
		}
		letters[i] = l
	}

	// Concurrently delete each letter from sender and recipient simultaneously
	var wg sync.WaitGroup
	errs := make(chan error, letterCount*2)

	for _, l := range letters {
		letterID := l.ID

		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := repo.DeleteLetter(ctx, letterID, char1.ID); err != nil {
				errs <- fmt.Errorf("sender delete failed for %s: %w", letterID, err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := repo.DeleteLetter(ctx, letterID, char2.ID); err != nil {
				errs <- fmt.Errorf("recipient delete failed for %s: %w", letterID, err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent DeleteLetter returned error: %v", err)
	}

	// Verify that all letters have been physically purged without lost update
	for _, l := range letters {
		found, err := repo.GetLetterByID(ctx, l.ID)
		if !errors.Is(err, home.ErrLetterNotFound) {
			t.Errorf("letter %s was not purged after concurrent mutual deletion! err=%v, found letter: sender_del=%v, recipient_del=%v",
				l.ID, err, found.IsDeletedBySender, found.IsDeletedByRecipient)
		}
	}
}

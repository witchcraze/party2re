package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/notification"
)

func TestNotificationRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Test News Articles
	t.Run("news articles CRUD", func(t *testing.T) {
		articleID := fmt.Sprintf("news_%d", time.Now().UnixNano())
		if len(articleID) > 32 {
			articleID = articleID[:32]
		}
		now := time.Now().UTC().Truncate(time.Microsecond)

		article := notification.NewsArticle{
			ID:          articleID,
			Category:    notification.CategoryUpdate,
			Title:       "Big Update Released",
			Content:     "New dungeons and items available!",
			Author:      "Admin",
			PublishedAt: now,
			CreatedAt:   now,
		}

		err = repo.CreateNews(ctx, article)
		if err != nil {
			t.Fatalf("CreateNews failed: %v", err)
		}

		fetched, err := repo.GetNewsByID(ctx, articleID)
		if err != nil {
			t.Fatalf("GetNewsByID failed: %v", err)
		}
		if fetched.Title != article.Title || fetched.Category != article.Category {
			t.Errorf("mismatched news article: %+v", fetched)
		}

		list, total, err := repo.ListNews(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListNews failed: %v", err)
		}
		if total < 1 {
			t.Fatalf("expected total >= 1, got %d", total)
		}
		found := false
		for _, a := range list {
			if a.ID == articleID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("created article not found in list")
		}
	})

	// 2. Test Player Notifications
	t.Run("player notifications lifecycle", func(t *testing.T) {
		player1, err := database.CreateTestPlayer(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		player2, err := database.CreateTestPlayer(ctx, db)
		if err != nil {
			t.Fatal(err)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		notif1ID := fmt.Sprintf("notif1_%d", time.Now().UnixNano())
		if len(notif1ID) > 32 {
			notif1ID = notif1ID[:32]
		}
		notif2ID := fmt.Sprintf("notif2_%d", time.Now().UnixNano())
		if len(notif2ID) > 32 {
			notif2ID = notif2ID[:32]
		}

		n1 := notification.PlayerNotification{
			ID:        notif1ID,
			PlayerID:  player1.ID,
			Category:  notification.NotificationCategorySystem,
			Title:     "Welcome Player 1",
			Body:      "Enjoy the game!",
			Link:      "/welcome",
			IsRead:    false,
			CreatedAt: now,
		}
		n2 := notification.PlayerNotification{
			ID:        notif2ID,
			PlayerID:  player1.ID,
			Category:  notification.NotificationCategoryAuction,
			Title:     "Auction Won",
			Body:      "You won the auction!",
			Link:      "/auction",
			IsRead:    false,
			CreatedAt: now,
		}

		// Batch create
		err = repo.CreateBatchNotifications(ctx, []notification.PlayerNotification{n1, n2})
		if err != nil {
			t.Fatalf("CreateBatchNotifications failed: %v", err)
		}

		// Get unread count
		count, err := repo.GetUnreadCount(ctx, player1.ID)
		if err != nil {
			t.Fatalf("GetUnreadCount failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected unread count 2, got %d", count)
		}

		// List notifications
		list, total, err := repo.ListNotificationsByPlayer(ctx, player1.ID, false, 10, 0)
		if err != nil {
			t.Fatalf("ListNotificationsByPlayer failed: %v", err)
		}
		if total != 2 || len(list) != 2 {
			t.Errorf("expected 2 notifications, got total=%d len=%d", total, len(list))
		}

		// Get single notification
		fetched, err := repo.GetNotificationByID(ctx, n1.ID)
		if err != nil {
			t.Fatalf("GetNotificationByID failed: %v", err)
		}
		if fetched.Title != n1.Title {
			t.Errorf("expected title %s, got %s", n1.Title, fetched.Title)
		}

		// Mark n1 as read by player2 -> should fail with ErrForbidden
		err = repo.MarkAsRead(ctx, n1.ID, player2.ID, now)
		if !errors.Is(err, notification.ErrForbidden) {
			t.Errorf("expected ErrForbidden when player2 marks player1's notif, got %v", err)
		}

		// Mark n1 as read by player1
		err = repo.MarkAsRead(ctx, n1.ID, player1.ID, now)
		if err != nil {
			t.Fatalf("MarkAsRead failed: %v", err)
		}

		count, _ = repo.GetUnreadCount(ctx, player1.ID)
		if count != 1 {
			t.Errorf("expected unread count 1, got %d", count)
		}

		// Mark all as read
		err = repo.MarkAllAsRead(ctx, player1.ID, now)
		if err != nil {
			t.Fatalf("MarkAllAsRead failed: %v", err)
		}

		count, _ = repo.GetUnreadCount(ctx, player1.ID)
		if count != 0 {
			t.Errorf("expected unread count 0, got %d", count)
		}

		// Delete notification with wrong player
		err = repo.DeleteNotification(ctx, n1.ID, player2.ID)
		if !errors.Is(err, notification.ErrForbidden) {
			t.Errorf("expected ErrForbidden when deleting other player's notif, got %v", err)
		}

		// Delete notification with owner
		err = repo.DeleteNotification(ctx, n1.ID, player1.ID)
		if err != nil {
			t.Fatalf("DeleteNotification failed: %v", err)
		}

		// Verify deleted
		_, err = repo.GetNotificationByID(ctx, n1.ID)
		if !errors.Is(err, notification.ErrNotificationNotFound) {
			t.Errorf("expected ErrNotificationNotFound, got %v", err)
		}
	})
}

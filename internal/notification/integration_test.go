package notification_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/notification"
)

func TestNotificationServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := notification.NewService(repo, repo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Publish and retrieve news
	article, err := svc.PublishNews(ctx, notification.CategoryUpdate, "Grand Opening", "Welcome to Party2 Reconstructed!", "Admin", time.Now().UTC())
	if err != nil {
		t.Fatalf("PublishNews failed: %v", err)
	}

	fetchedArticle, err := svc.GetNews(ctx, article.ID)
	if err != nil {
		t.Fatalf("GetNews failed: %v", err)
	}
	if fetchedArticle.Title != "Grand Opening" {
		t.Errorf("expected Grand Opening, got %s", fetchedArticle.Title)
	}

	newsList, err := svc.ListNews(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListNews failed: %v", err)
	}
	if newsList.Total < 1 {
		t.Errorf("expected at least 1 news article")
	}

	// 2. Player notification lifecycle
	notif, err := svc.NotifyPlayer(ctx, player.ID, notification.NotificationCategorySystem, "Welcome Gift", "You received 500 gold as a starting bonus.", "/rewards")
	if err != nil {
		t.Fatalf("NotifyPlayer failed: %v", err)
	}

	unreadCount, err := svc.GetUnreadCount(ctx, player.ID)
	if err != nil {
		t.Fatalf("GetUnreadCount failed: %v", err)
	}
	if unreadCount < 1 {
		t.Errorf("expected unread count >= 1, got %d", unreadCount)
	}

	notifList, err := svc.GetPlayerNotifications(ctx, player.ID, false, 10, 0)
	if err != nil {
		t.Fatalf("GetPlayerNotifications failed: %v", err)
	}
	if notifList.Total < 1 {
		t.Errorf("expected at least 1 notification")
	}

	// 3. Mark as read
	err = svc.MarkAsRead(ctx, notif.ID, player.ID)
	if err != nil {
		t.Fatalf("MarkAsRead failed: %v", err)
	}

	// 4. Delete notification
	err = svc.DeleteNotification(ctx, notif.ID, player.ID)
	if err != nil {
		t.Fatalf("DeleteNotification failed: %v", err)
	}
}

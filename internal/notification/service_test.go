package notification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockNewsRepo struct {
	articles map[string]NewsArticle
	listErr  error
	getErr   error
	saveErr  error
}

func newMockNewsRepo() *mockNewsRepo {
	return &mockNewsRepo{
		articles: make(map[string]NewsArticle),
	}
}

func (m *mockNewsRepo) CreateNews(ctx context.Context, article NewsArticle) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.articles[article.ID] = article
	return nil
}

func (m *mockNewsRepo) GetNewsByID(ctx context.Context, id string) (NewsArticle, error) {
	if m.getErr != nil {
		return NewsArticle{}, m.getErr
	}
	a, ok := m.articles[id]
	if !ok {
		return NewsArticle{}, ErrNewsNotFound
	}
	return a, nil
}

func (m *mockNewsRepo) ListNews(ctx context.Context, limit, offset int) ([]NewsArticle, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	res := make([]NewsArticle, 0, len(m.articles))
	for _, a := range m.articles {
		res = append(res, a)
	}
	return res, len(res), nil
}

type mockNotificationRepo struct {
	notifs  map[string]PlayerNotification
	listErr error
	getErr  error
	saveErr error
	delErr  error
	markErr error
}

func newMockNotificationRepo() *mockNotificationRepo {
	return &mockNotificationRepo{
		notifs: make(map[string]PlayerNotification),
	}
}

func (m *mockNotificationRepo) CreateNotification(ctx context.Context, notif PlayerNotification) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.notifs[notif.ID] = notif
	return nil
}

func (m *mockNotificationRepo) CreateBatchNotifications(ctx context.Context, notifs []PlayerNotification) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	for _, n := range notifs {
		m.notifs[n.ID] = n
	}
	return nil
}

func (m *mockNotificationRepo) GetNotificationByID(ctx context.Context, id string) (PlayerNotification, error) {
	if m.getErr != nil {
		return PlayerNotification{}, m.getErr
	}
	n, ok := m.notifs[id]
	if !ok {
		return PlayerNotification{}, ErrNotificationNotFound
	}
	return n, nil
}

func (m *mockNotificationRepo) ListNotificationsByPlayer(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) ([]PlayerNotification, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var res []PlayerNotification
	for _, n := range m.notifs {
		if n.PlayerID == playerID {
			if unreadOnly && n.IsRead {
				continue
			}
			res = append(res, n)
		}
	}
	return res, len(res), nil
}

func (m *mockNotificationRepo) GetUnreadCount(ctx context.Context, playerID string) (int, error) {
	var count int
	for _, n := range m.notifs {
		if n.PlayerID == playerID && !n.IsRead {
			count++
		}
	}
	return count, nil
}

func (m *mockNotificationRepo) MarkAsRead(ctx context.Context, id, playerID string, readAt time.Time) error {
	if m.markErr != nil {
		return m.markErr
	}
	n, ok := m.notifs[id]
	if !ok {
		return ErrNotificationNotFound
	}
	if n.PlayerID != playerID {
		return ErrForbidden
	}
	n.IsRead = true
	n.ReadAt = &readAt
	m.notifs[id] = n
	return nil
}

func (m *mockNotificationRepo) MarkAllAsRead(ctx context.Context, playerID string, readAt time.Time) error {
	if m.markErr != nil {
		return m.markErr
	}
	for id, n := range m.notifs {
		if n.PlayerID == playerID && !n.IsRead {
			n.IsRead = true
			n.ReadAt = &readAt
			m.notifs[id] = n
		}
	}
	return nil
}

func (m *mockNotificationRepo) DeleteNotification(ctx context.Context, id, playerID string) error {
	if m.delErr != nil {
		return m.delErr
	}
	n, ok := m.notifs[id]
	if !ok {
		return ErrNotificationNotFound
	}
	if n.PlayerID != playerID {
		return ErrForbidden
	}
	delete(m.notifs, id)
	return nil
}

func (m *mockNotificationRepo) DeleteExpiredNotifications(ctx context.Context, olderThan time.Time) (int64, error) {
	var count int64
	for id, n := range m.notifs {
		if n.CreatedAt.Before(olderThan) {
			delete(m.notifs, id)
			count++
		}
	}
	return count, nil
}

func TestNewsService(t *testing.T) {
	ctx := context.Background()
	newsRepo := newMockNewsRepo()
	notifRepo := newMockNotificationRepo()
	fixedTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	service, err := NewService(newsRepo, notifRepo, WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("publish and get news", func(t *testing.T) {
		article, err := service.PublishNews(ctx, CategoryAnnouncement, "Maintenance Notice", "Server will undergo scheduled maintenance.", "Admin", time.Time{})
		if err != nil {
			t.Fatalf("unexpected error publishing news: %v", err)
		}
		if article.ID == "" {
			t.Errorf("expected non-empty ID")
		}
		if article.Title != "Maintenance Notice" {
			t.Errorf("expected title 'Maintenance Notice', got %s", article.Title)
		}
		if article.Author != "Admin" {
			t.Errorf("expected author 'Admin', got %s", article.Author)
		}

		fetched, err := service.GetNews(ctx, article.ID)
		if err != nil {
			t.Fatalf("unexpected error fetching news: %v", err)
		}
		if fetched.ID != article.ID {
			t.Errorf("expected ID %s, got %s", article.ID, fetched.ID)
		}
	})

	t.Run("get non-existent news", func(t *testing.T) {
		_, err := service.GetNews(ctx, "nonexistent")
		if !errors.Is(err, ErrNewsNotFound) {
			t.Errorf("expected ErrNewsNotFound, got %v", err)
		}
	})

	t.Run("list news with pagination", func(t *testing.T) {
		list, err := service.ListNews(ctx, 10, 0)
		if err != nil {
			t.Fatalf("unexpected error listing news: %v", err)
		}
		if list.Total == 0 {
			t.Errorf("expected non-zero total")
		}
	})
}

func TestNotificationService(t *testing.T) {
	ctx := context.Background()
	newsRepo := newMockNewsRepo()
	notifRepo := newMockNotificationRepo()
	fixedTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	service, err := NewService(newsRepo, notifRepo, WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("notify player and unread count", func(t *testing.T) {
		n1, err := service.NotifyPlayer(ctx, "player-1", NotificationCategorySystem, "Welcome!", "Welcome to Party2!", "/home")
		if err != nil {
			t.Fatalf("unexpected error notifying player: %v", err)
		}
		if n1.IsRead {
			t.Errorf("expected notification to be unread")
		}

		n2, err := service.NotifyPlayer(ctx, "player-1", NotificationCategoryAuction, "Item Sold", "Your Iron Sword was sold.", "/auction")
		if err != nil {
			t.Fatalf("unexpected error notifying player: %v", err)
		}

		unreadCount, err := service.GetUnreadCount(ctx, "player-1")
		if err != nil {
			t.Fatalf("unexpected error getting unread count: %v", err)
		}
		if unreadCount != 2 {
			t.Errorf("expected unread count 2, got %d", unreadCount)
		}

		// Mark single as read
		err = service.MarkAsRead(ctx, n1.ID, "player-1")
		if err != nil {
			t.Fatalf("unexpected error marking as read: %v", err)
		}

		unreadCount, _ = service.GetUnreadCount(ctx, "player-1")
		if unreadCount != 1 {
			t.Errorf("expected unread count 1, got %d", unreadCount)
		}

		// Mark forbidden if different player
		err = service.MarkAsRead(ctx, n2.ID, "player-2")
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}

		// Mark all as read
		err = service.MarkAllAsRead(ctx, "player-1")
		if err != nil {
			t.Fatalf("unexpected error marking all as read: %v", err)
		}

		unreadCount, _ = service.GetUnreadCount(ctx, "player-1")
		if unreadCount != 0 {
			t.Errorf("expected unread count 0, got %d", unreadCount)
		}

		// Delete notification
		err = service.DeleteNotification(ctx, n1.ID, "player-1")
		if err != nil {
			t.Fatalf("unexpected error deleting notification: %v", err)
		}

		// Delete forbidden
		err = service.DeleteNotification(ctx, n2.ID, "player-2")
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("broadcast notifications", func(t *testing.T) {
		err := service.BroadcastNotification(ctx, []string{"player-A", "player-B", "player-C"}, NotificationCategoryEvent, "Festival Started", "The Grand Festival is live!", "/event")
		if err != nil {
			t.Fatalf("unexpected error broadcasting: %v", err)
		}

		countA, _ := service.GetUnreadCount(ctx, "player-A")
		countB, _ := service.GetUnreadCount(ctx, "player-B")
		countC, _ := service.GetUnreadCount(ctx, "player-C")
		if countA != 1 || countB != 1 || countC != 1 {
			t.Errorf("expected 1 unread each, got A=%d, B=%d, C=%d", countA, countB, countC)
		}
	})
}

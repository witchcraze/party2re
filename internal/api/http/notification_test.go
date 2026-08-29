package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/notification"
)

type mockNotificationService struct {
	publishNewsFn            func(ctx context.Context, category, title, content, author string, publishedAt time.Time) (notification.NewsArticle, error)
	getNewsFn                func(ctx context.Context, id string) (notification.NewsArticle, error)
	listNewsFn               func(ctx context.Context, limit, offset int) (notification.NewsListResult, error)
	getPlayerNotificationsFn func(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) (notification.NotificationListResult, error)
	getUnreadCountFn         func(ctx context.Context, playerID string) (int, error)
	markAsReadFn             func(ctx context.Context, id, playerID string) error
	markAllAsReadFn          func(ctx context.Context, playerID string) error
	deleteNotificationFn     func(ctx context.Context, id, playerID string) error
}

func (m *mockNotificationService) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) (notification.NewsArticle, error) {
	if m.publishNewsFn != nil {
		return m.publishNewsFn(ctx, category, title, content, author, publishedAt)
	}
	return notification.NewsArticle{
		ID:          "news-1",
		Category:    category,
		Title:       title,
		Content:     content,
		Author:      author,
		PublishedAt: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (m *mockNotificationService) GetNews(ctx context.Context, id string) (notification.NewsArticle, error) {
	if m.getNewsFn != nil {
		return m.getNewsFn(ctx, id)
	}
	if id == "news-1" {
		return notification.NewsArticle{
			ID:          "news-1",
			Category:    "announcement",
			Title:       "Server Update",
			Content:     "Update notes",
			Author:      "Admin",
			PublishedAt: time.Now().UTC(),
			CreatedAt:   time.Now().UTC(),
		}, nil
	}
	return notification.NewsArticle{}, notification.ErrNewsNotFound
}

func (m *mockNotificationService) ListNews(ctx context.Context, limit, offset int) (notification.NewsListResult, error) {
	if m.listNewsFn != nil {
		return m.listNewsFn(ctx, limit, offset)
	}
	return notification.NewsListResult{
		Articles: []notification.NewsArticle{
			{
				ID:          "news-1",
				Category:    "announcement",
				Title:       "Server Update",
				Content:     "Update notes",
				Author:      "Admin",
				PublishedAt: time.Now().UTC(),
				CreatedAt:   time.Now().UTC(),
			},
		},
		Total:  1,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (m *mockNotificationService) GetPlayerNotifications(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) (notification.NotificationListResult, error) {
	if m.getPlayerNotificationsFn != nil {
		return m.getPlayerNotificationsFn(ctx, playerID, unreadOnly, limit, offset)
	}
	return notification.NotificationListResult{
		Notifications: []notification.PlayerNotification{
			{
				ID:        "notif-1",
				PlayerID:  playerID,
				Category:  "system",
				Title:     "Reward",
				Body:      "You received 100 gold",
				IsRead:    false,
				CreatedAt: time.Now().UTC(),
			},
		},
		Total:  1,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (m *mockNotificationService) GetUnreadCount(ctx context.Context, playerID string) (int, error) {
	if m.getUnreadCountFn != nil {
		return m.getUnreadCountFn(ctx, playerID)
	}
	return 1, nil
}

func (m *mockNotificationService) MarkAsRead(ctx context.Context, id, playerID string) error {
	if m.markAsReadFn != nil {
		return m.markAsReadFn(ctx, id, playerID)
	}
	if id == "notif-not-found" {
		return notification.ErrNotificationNotFound
	}
	if id == "notif-other" {
		return notification.ErrForbidden
	}
	return nil
}

func (m *mockNotificationService) MarkAllAsRead(ctx context.Context, playerID string) error {
	if m.markAllAsReadFn != nil {
		return m.markAllAsReadFn(ctx, playerID)
	}
	return nil
}

func (m *mockNotificationService) DeleteNotification(ctx context.Context, id, playerID string) error {
	if m.deleteNotificationFn != nil {
		return m.deleteNotificationFn(ctx, id, playerID)
	}
	if id == "notif-not-found" {
		return notification.ErrNotificationNotFound
	}
	if id == "notif-other" {
		return notification.ErrForbidden
	}
	return nil
}

func TestNotificationEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	chars := &stubCharacterService{}
	advs := &stubAdventureService{}
	shops := &stubShopService{}
	notifSvc := &mockNotificationService{}

	const adminKey = "test-admin-key"
	handler, err := apihttp.NewHandler(
		players,
		chars,
		advs,
		shops,
		apihttp.WithNotification(notifSvc),
		apihttp.WithAdminAPIKey(adminKey),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := handler.Router()

	t.Run("GET /news - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/news?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var res notification.NewsListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.Total != 1 || len(res.Articles) != 1 {
			t.Errorf("unexpected news list result: %+v", res)
		}
	})

	t.Run("GET /news/{id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/news/news-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /news/{id} - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/news/nonexistent", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /news - missing credentials returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"category": "update",
			"title":    "New Feature",
			"content":  "News and Notifications implemented",
			"author":   "Admin",
		})
		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /news - invalid admin key returns 403", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"category": "update",
			"title":    "New Feature",
			"content":  "News and Notifications implemented",
			"author":   "Admin",
		})
		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "invalid-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /news - success with X-Admin-Key", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"category": "update",
			"title":    "New Feature",
			"content":  "News and Notifications implemented",
			"author":   "Admin",
		})
		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /news - success with Bearer authorization", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"category": "update",
			"title":    "New Feature",
			"content":  "News and Notifications implemented",
			"author":   "Admin",
		})
		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /notifications - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /notifications - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/notifications?unread_only=true", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var res notification.NotificationListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.Total != 1 || len(res.Notifications) != 1 {
			t.Errorf("unexpected notification list result: %+v", res)
		}
	})

	t.Run("GET /notifications/unread-count - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /notifications/{id}/read - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/notifications/notif-1/read", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /notifications/{id}/read - forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/notifications/notif-other/read", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /notifications/read-all - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/notifications/read-all", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /notifications/{id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/notifications/notif-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /notifications/{id} - forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/notifications/notif-other", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

package notification

import (
	"context"
	"time"
)

type NewsRepository interface {
	CreateNews(ctx context.Context, article NewsArticle) error
	GetNewsByID(ctx context.Context, id string) (NewsArticle, error)
	ListNews(ctx context.Context, limit, offset int) ([]NewsArticle, int, error)
}

type NotificationRepository interface {
	CreateNotification(ctx context.Context, notif PlayerNotification) error
	CreateBatchNotifications(ctx context.Context, notifs []PlayerNotification) error
	GetNotificationByID(ctx context.Context, id string) (PlayerNotification, error)
	ListNotificationsByPlayer(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) ([]PlayerNotification, int, error)
	GetUnreadCount(ctx context.Context, playerID string) (int, error)
	MarkAsRead(ctx context.Context, id, playerID string, readAt time.Time) error
	MarkAllAsRead(ctx context.Context, playerID string, readAt time.Time) error
	DeleteNotification(ctx context.Context, id, playerID string) error
	DeleteExpiredNotifications(ctx context.Context, olderThan time.Time) (int64, error)
}

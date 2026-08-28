package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

type NewsListResult struct {
	Articles []NewsArticle `json:"articles"`
	Total    int           `json:"total"`
	Limit    int           `json:"limit"`
	Offset   int           `json:"offset"`
}

type NotificationListResult struct {
	Notifications []PlayerNotification `json:"notifications"`
	Total         int                  `json:"total"`
	Limit         int                  `json:"limit"`
	Offset        int                  `json:"offset"`
}

type Service struct {
	newsRepo  NewsRepository
	notifRepo NotificationRepository
	nowFunc   func() time.Time
}

type ServiceOption func(*Service)

func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func NewService(newsRepo NewsRepository, notifRepo NotificationRepository, opts ...ServiceOption) (*Service, error) {
	if newsRepo == nil {
		return nil, errors.New("news repository is required")
	}
	if notifRepo == nil {
		return nil, errors.New("notification repository is required")
	}
	s := &Service{
		newsRepo:  newsRepo,
		notifRepo: notifRepo,
		nowFunc:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// PublishNews creates and publishes a new system-wide news article.
func (s *Service) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) (NewsArticle, error) {
	if err := ValidateNewsArticle(category, title, content, author); err != nil {
		return NewsArticle{}, err
	}

	cleanCategory := strings.TrimSpace(category)
	if cleanCategory == "" {
		cleanCategory = CategoryAnnouncement
	}
	cleanAuthor := strings.TrimSpace(author)
	if cleanAuthor == "" {
		cleanAuthor = "System"
	}

	now := s.nowFunc().UTC()
	if publishedAt.IsZero() {
		publishedAt = now
	} else {
		publishedAt = publishedAt.UTC()
	}

	article := NewsArticle{
		ID:          id.New(),
		Category:    cleanCategory,
		Title:       strings.TrimSpace(title),
		Content:     strings.TrimSpace(content),
		Author:      cleanAuthor,
		PublishedAt: publishedAt,
		CreatedAt:   now,
	}

	if err := s.newsRepo.CreateNews(ctx, article); err != nil {
		return NewsArticle{}, err
	}

	return article, nil
}

// GetNews retrieves a news article by ID.
func (s *Service) GetNews(ctx context.Context, id string) (NewsArticle, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return NewsArticle{}, ErrNewsNotFound
	}
	return s.newsRepo.GetNewsByID(ctx, id)
}

// ListNews retrieves a paginated list of news articles ordered by publication date descending.
func (s *Service) ListNews(ctx context.Context, limit, offset int) (NewsListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	articles, total, err := s.newsRepo.ListNews(ctx, limit, offset)
	if err != nil {
		return NewsListResult{}, err
	}

	return NewsListResult{
		Articles: articles,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	}, nil
}

// NotifyPlayer dispatches a notification to a specific player's inbox.
func (s *Service) NotifyPlayer(ctx context.Context, playerID, category, title, body, link string) (PlayerNotification, error) {
	if err := ValidateNotification(playerID, category, title, body); err != nil {
		return PlayerNotification{}, err
	}

	cleanCategory := strings.TrimSpace(category)
	if cleanCategory == "" {
		cleanCategory = NotificationCategorySystem
	}

	now := s.nowFunc().UTC()
	notif := PlayerNotification{
		ID:        id.New(),
		PlayerID:  strings.TrimSpace(playerID),
		Category:  cleanCategory,
		Title:     strings.TrimSpace(title),
		Body:      strings.TrimSpace(body),
		Link:      strings.TrimSpace(link),
		IsRead:    false,
		ReadAt:    nil,
		CreatedAt: now,
	}

	if err := s.notifRepo.CreateNotification(ctx, notif); err != nil {
		return PlayerNotification{}, err
	}

	return notif, nil
}

// BroadcastNotification sends a notification to multiple players simultaneously.
func (s *Service) BroadcastNotification(ctx context.Context, playerIDs []string, category, title, body, link string) error {
	if len(playerIDs) == 0 {
		return nil
	}

	cleanCategory := strings.TrimSpace(category)
	if cleanCategory == "" {
		cleanCategory = NotificationCategorySystem
	}
	cleanTitle := strings.TrimSpace(title)
	cleanBody := strings.TrimSpace(body)
	cleanLink := strings.TrimSpace(link)

	now := s.nowFunc().UTC()
	notifs := make([]PlayerNotification, 0, len(playerIDs))

	for _, pid := range playerIDs {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		notifs = append(notifs, PlayerNotification{
			ID:        id.New(),
			PlayerID:  pid,
			Category:  cleanCategory,
			Title:     cleanTitle,
			Body:      cleanBody,
			Link:      cleanLink,
			IsRead:    false,
			ReadAt:    nil,
			CreatedAt: now,
		})
	}

	if len(notifs) == 0 {
		return nil
	}

	return s.notifRepo.CreateBatchNotifications(ctx, notifs)
}

// GetPlayerNotifications retrieves a paginated list of notifications for a player.
func (s *Service) GetPlayerNotifications(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) (NotificationListResult, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return NotificationListResult{}, ErrInvalidPlayerID
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	notifs, total, err := s.notifRepo.ListNotificationsByPlayer(ctx, playerID, unreadOnly, limit, offset)
	if err != nil {
		return NotificationListResult{}, err
	}

	return NotificationListResult{
		Notifications: notifs,
		Total:         total,
		Limit:         limit,
		Offset:        offset,
	}, nil
}

// GetUnreadCount retrieves the count of unread notifications for a player.
func (s *Service) GetUnreadCount(ctx context.Context, playerID string) (int, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return 0, ErrInvalidPlayerID
	}
	return s.notifRepo.GetUnreadCount(ctx, playerID)
}

// MarkAsRead marks a single notification as read, validating player ownership.
func (s *Service) MarkAsRead(ctx context.Context, id, playerID string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotificationNotFound
	}
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return ErrInvalidPlayerID
	}

	now := s.nowFunc().UTC()
	return s.notifRepo.MarkAsRead(ctx, id, playerID, now)
}

// MarkAllAsRead marks all unread notifications for a player as read.
func (s *Service) MarkAllAsRead(ctx context.Context, playerID string) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return ErrInvalidPlayerID
	}

	now := s.nowFunc().UTC()
	return s.notifRepo.MarkAllAsRead(ctx, playerID, now)
}

// DeleteNotification deletes a notification, validating player ownership.
func (s *Service) DeleteNotification(ctx context.Context, id, playerID string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotificationNotFound
	}
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return ErrInvalidPlayerID
	}

	return s.notifRepo.DeleteNotification(ctx, id, playerID)
}

// PruneExpired deletes notifications older than the given number of retention days.
func (s *Service) PruneExpired(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	threshold := s.nowFunc().UTC().AddDate(0, 0, -retentionDays)
	return s.notifRepo.DeleteExpiredNotifications(ctx, threshold)
}

package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/pagination"
)

// NotificationService defines the news and notification operations exposed over HTTP.
type NotificationService interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) (notification.NewsArticle, error)
	GetNews(ctx context.Context, id string) (notification.NewsArticle, error)
	ListNews(ctx context.Context, limit, offset int) (notification.NewsListResult, error)
	GetPlayerNotifications(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) (notification.NotificationListResult, error)
	GetUnreadCount(ctx context.Context, playerID string) (int, error)
	MarkAsRead(ctx context.Context, id, playerID string) error
	MarkAllAsRead(ctx context.Context, playerID string) error
	DeleteNotification(ctx context.Context, id, playerID string) error
}

// WithNotification configures the notification service for the Handler.
func WithNotification(n NotificationService) Option {
	return func(h *Handler) {
		h.notifications = n
	}
}

type createNewsRequest struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Author   string `json:"author"`
}

type unreadCountResponse struct {
	UnreadCount int `json:"unread_count"`
}

func (h *Handler) handleListNews(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	params := pagination.ParseRequest(r)

	result, err := h.notifications.ListNews(r.Context(), params.Limit, params.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleGetNews(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	id := r.PathValue("id")
	article, err := h.notifications.GetNews(r.Context(), id)
	if err != nil {
		if errors.Is(err, notification.ErrNewsNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, article)
}

func (h *Handler) handleCreateNews(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	if !h.authenticateAdmin(w, r) {
		return
	}

	var req createNewsRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	article, err := h.notifications.PublishNews(r.Context(), req.Category, req.Title, req.Content, req.Author, time.Time{})
	if err != nil {
		if errors.Is(err, notification.ErrEmptyTitle) || errors.Is(err, notification.ErrTitleTooLong) ||
			errors.Is(err, notification.ErrEmptyContent) || errors.Is(err, notification.ErrContentTooLong) {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, article)
}

func (h *Handler) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	params := pagination.ParseRequest(r)
	unreadOnly := r.URL.Query().Get("unread_only") == "true"

	result, err := h.notifications.GetPlayerNotifications(r.Context(), player.ID, unreadOnly, params.Limit, params.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleGetUnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	count, err := h.notifications.GetUnreadCount(r.Context(), player.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: count})
}

func (h *Handler) handleMarkNotificationAsRead(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	err := h.notifications.MarkAsRead(r.Context(), id, player.ID)
	if err != nil {
		if errors.Is(err, notification.ErrNotificationNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, notification.ErrForbidden) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleMarkAllNotificationsAsRead(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	err := h.notifications.MarkAllAsRead(r.Context(), player.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeError(w, http.StatusNotImplemented, errors.New("notification service not configured"))
		return
	}

	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	err := h.notifications.DeleteNotification(r.Context(), id, player.ID)
	if err != nil {
		if errors.Is(err, notification.ErrNotificationNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, notification.ErrForbidden) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

package home

import (
	"context"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
)

// AddDeliveryNotice records an incoming delivery notice for a character.
func (s *Service) AddDeliveryNotice(ctx context.Context, characterID, noticeType, message string) error {
	cleanType := strings.TrimSpace(noticeType)
	if cleanType == "" {
		cleanType = "item_transfer"
	}
	notice := DeliveryNotice{
		ID:          id.New(),
		CharacterID: characterID,
		NoticeType:  cleanType,
		Message:     strings.TrimSpace(message),
		IsCleared:   false,
		CreatedAt:   s.nowFunc().UTC(),
	}
	return s.repo.AddDeliveryNotice(ctx, notice)
}

// AddDeliveryFlag records a notice unless one of the same type is uncleared,
// as legacy money_flag.cgi shows one message for any number of remittances.
// Callers serialize on the recipient's character lock.
func (s *Service) AddDeliveryFlag(ctx context.Context, characterID, noticeType, message string) error {
	pending, err := s.repo.HasUnclearedDeliveryNotice(ctx, characterID, noticeType)
	if err != nil || pending {
		return err
	}
	return s.AddDeliveryNotice(ctx, characterID, noticeType, message)
}

// ListDeliveryNotices returns delivery notices for a character.
func (s *Service) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]DeliveryNotice, error) {
	return s.repo.ListDeliveryNotices(ctx, characterID, unclearedOnly)
}

// ClearDeliveryNotices marks the owner's listed notices as cleared; IDs that
// are unknown, already cleared or another character's are ignored.
func (s *Service) ClearDeliveryNotices(ctx context.Context, characterID string, noticeIDs []string) error {
	if len(noticeIDs) == 0 {
		return nil
	}
	return s.repo.ClearDeliveryNotices(ctx, characterID, noticeIDs)
}

// ListDeliveryNoticesPage returns the owner's uncleared notices and a stable continuation.
func (s *Service) ListDeliveryNoticesPage(ctx context.Context, characterID, playerID string, limit, offset int, cursor string) (pagination.CursorPage[DeliveryNotice], error) {
	owner, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		return pagination.CursorPage[DeliveryNotice]{}, err
	}
	if playerID == "" || owner.PlayerID != playerID || owner.ID != characterID {
		return pagination.CursorPage[DeliveryNotice]{}, ErrForbidden
	}
	limit, offset = pagination.Normalize(limit, offset)
	beforeTime, beforeID := pagination.DecodeCursorParts(cursor)
	if cursor != "" {
		offset = 0
	}
	notices, err := s.repo.ListDeliveryNoticesPage(ctx, characterID, limit+1, offset, beforeTime, beforeID)
	if err != nil {
		return pagination.CursorPage[DeliveryNotice]{}, err
	}
	return pagination.BuildCursorPage(notices, limit, cursor, func(n DeliveryNotice) (time.Time, string) {
		return n.CreatedAt, n.ID
	}), nil
}

package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/validation"
)

// HomeEstateService defines estate, item, and color operations.
type HomeEstateService interface {
	GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID, playerID string) (home.HomeView, error)
	BuildHouse(ctx context.Context, characterID, townID, houseStyle string) (*home.HomeCheckResult, error)
	CheckHouse(ctx context.Context, targetNameOrID string) (*home.HomeCheckResult, error)
	ListTownHouses(ctx context.Context, townID string) ([]home.HomeCheckResult, error)
	SetCharacterColor(ctx context.Context, characterID, color string) error
	ListHomeItems(ctx context.Context, characterID string) ([]home.HomeUsableItem, error)
	UseHomeItem(ctx context.Context, characterID, instanceID, source string) (*home.UseHomeItemResult, error)
}

// HomeMailboxService defines player-to-player letter and mailbox operations.
type HomeMailboxService interface {
	SendLetter(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error)
	ReadLetter(ctx context.Context, letterID, recipientID string) error
	ListInbox(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error)
	ListInboxByCursor(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	ListOutbox(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error)
	ListOutboxByCursor(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error)
	DeleteLetter(ctx context.Context, letterID, characterID string) error
}

// HomeCompanionService defines companion pet dialogue and phrase operations.
type HomeCompanionService interface {
	TeachCompanionPhrase(ctx context.Context, characterID, phrase string) (home.CompanionPhrase, error)
	ForgetCompanionPhrase(ctx context.Context, phraseID, characterID string) error
	TalkToCompanion(ctx context.Context, characterID string) (home.CompanionTalkResult, error)
}

// HomeLivingService defines resting, waking, and delivery notice operations.
type HomeLivingService interface {
	ClearDeliveryNotices(ctx context.Context, characterID string, noticeIDs []string) error
	Sleep(ctx context.Context, characterID, targetHomeID string) (home.SleepResult, error)
	GetSleepStatus(ctx context.Context, characterID string) (home.SleepStatus, error)
	Wake(ctx context.Context, characterID string) (home.WakeResult, error)
}

// HomeService defines the home, mailbox, companion, and estate operations exposed over HTTP.
type HomeService interface {
	HomeEstateService
	HomeMailboxService
	HomeCompanionService
	HomeLivingService
}

type sendLetterRequest struct {
	SenderCharacterID    string `json:"sender_character_id"`
	RecipientCharacterID string `json:"recipient_character_id"`
	Content              string `json:"content"`
	Color                string `json:"color"`
}

type readLetterRequest struct {
	CharacterID string `json:"character_id"`
}

func (h *Handler) handleGetHomeView(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	homeCharID := r.PathValue("id")
	visitorCharID := r.URL.Query().Get("visitor_id")

	read := func(visitor, player string) {
		view, err := h.homes.GetHomeView(r.Context(), homeCharID, visitor, player)
		if err != nil {
			if errors.Is(err, home.ErrForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, home.ErrCharacterNotFound) || errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, view)
	}
	if visitorCharID == "" {
		read("", "")
		return
	}
	h.withAuthenticatedCharacter(w, r, visitorCharID, func(player coreplayer.Player, visitor corecharacter.Character) {
		read(visitor.ID, player.ID)
	})
}

func (h *Handler) handleSendLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *sendLetterRequest) string {
		return req.SenderCharacterID
	}, func(_ coreplayer.Player, sender corecharacter.Character, req sendLetterRequest) {
		letter, err := h.homes.SendLetter(r.Context(), sender.ID, req.RecipientCharacterID, req.Content, req.Color)
		if err != nil {
			if errors.Is(err, home.ErrCharacterNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, home.ErrLetterThrottled) {
				writeError(w, http.StatusTooManyRequests, err)
				return
			}
			if errors.Is(err, home.ErrInvalidSender) || errors.Is(err, home.ErrInvalidRecipient) ||
				errors.Is(err, home.ErrEmptyContent) ||
				errors.Is(err, home.ErrContentTooLong) || isTextValidationError(err) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, letter)
	})
}

func (h *Handler) handleListInbox(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		if r.URL != nil && r.URL.Query().Has("cursor") {
			cursorParams := pagination.ParseCursorRequest(r)
			res, err := h.homes.ListInboxByCursor(r.Context(), char.ID, cursorParams.Limit, cursorParams.Cursor)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, res)
			return
		}

		params := pagination.ParseRequest(r)

		res, err := h.homes.ListInbox(r.Context(), char.ID, params.Limit, params.Offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleListOutbox(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		if r.URL != nil && r.URL.Query().Has("cursor") {
			cursorParams := pagination.ParseCursorRequest(r)
			res, err := h.homes.ListOutboxByCursor(r.Context(), char.ID, cursorParams.Limit, cursorParams.Cursor)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, res)
			return
		}

		params := pagination.ParseRequest(r)

		res, err := h.homes.ListOutbox(r.Context(), char.ID, params.Limit, params.Offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGetUnreadLetterCount(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		count, err := h.homes.GetUnreadLetterCount(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: count})
	})
}

func (h *Handler) handleReadLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	letterID := r.PathValue("id")
	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *readLetterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req readLetterRequest) {
		err := h.homes.ReadLetter(r.Context(), letterID, char.ID)
		if err != nil {
			if errors.Is(err, home.ErrLetterNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, home.ErrForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

func (h *Handler) handleDeleteLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	letterID := r.PathValue("id")
	h.withAuthenticatedActionCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.homes.DeleteLetter(r.Context(), letterID, char.ID)
		if err != nil {
			if errors.Is(err, home.ErrLetterNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, home.ErrForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

func isTextValidationError(err error) bool {
	return errors.Is(err, validation.ErrControlCharacter) ||
		errors.Is(err, validation.ErrZeroWidth) ||
		errors.Is(err, validation.ErrBidiOverride) ||
		errors.Is(err, validation.ErrZalgo) ||
		errors.Is(err, validation.ErrEmpty) ||
		errors.Is(err, validation.ErrTooLong)
}

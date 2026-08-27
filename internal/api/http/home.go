package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
)

// HomeService defines the home and mailbox operations exposed over HTTP.
type HomeService interface {
	GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID string) (home.HomeView, error)
	UpdateHome(ctx context.Context, characterID, theme, motto, companionName string) (home.CharacterHome, error)
	SendLetter(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error)
	ReadLetter(ctx context.Context, letterID, recipientID string) error
	ListInbox(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error)
	ListOutbox(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error)
	GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error)
	DeleteLetter(ctx context.Context, letterID, characterID string) error
	TeachCompanionPhrase(ctx context.Context, characterID, phrase string) (home.CompanionPhrase, error)
	ForgetCompanionPhrase(ctx context.Context, phraseID, characterID string) error
	ListCompanionPhrases(ctx context.Context, characterID string) ([]home.CompanionPhrase, error)
	TalkToCompanion(ctx context.Context, characterID string) (string, error)
	ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]home.DeliveryNotice, error)
	ClearDeliveryNotices(ctx context.Context, characterID string) error
}

// WithHome configures the HomeService for the Handler.
func WithHome(h HomeService) Option {
	return func(handler *Handler) {
		handler.homes = h
	}
}

type updateHomeRequest struct {
	Theme         string `json:"theme"`
	Motto         string `json:"motto"`
	CompanionName string `json:"companion_name"`
}

type sendLetterRequest struct {
	SenderCharacterID    string `json:"sender_character_id"`
	RecipientCharacterID string `json:"recipient_character_id"`
	Content              string `json:"content"`
	Color                string `json:"color"`
}

type teachPhraseRequest struct {
	Phrase string `json:"phrase"`
}

type companionTalkResponse struct {
	Dialogue string `json:"dialogue"`
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

	view, err := h.homes.GetHomeView(r.Context(), homeCharID, visitorCharID)
	if err != nil {
		if errors.Is(err, home.ErrCharacterNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) handleUpdateHomeSettings(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	var req updateHomeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	updated, err := h.homes.UpdateHome(r.Context(), charID, req.Theme, req.Motto, req.CompanionName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) handleSendLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	var req sendLetterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	sender, err := h.characters.Get(r.Context(), req.SenderCharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if sender.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	letter, err := h.homes.SendLetter(r.Context(), req.SenderCharacterID, req.RecipientCharacterID, req.Content, req.Color)
	if err != nil {
		if errors.Is(err, home.ErrCharacterNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, home.ErrInvalidSender) || errors.Is(err, home.ErrInvalidRecipient) ||
			errors.Is(err, home.ErrCannotSendToSelf) || errors.Is(err, home.ErrEmptyContent) ||
			errors.Is(err, home.ErrContentTooLong) {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, letter)
}

func (h *Handler) handleListInbox(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	res, err := h.homes.ListInbox(r.Context(), charID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleListOutbox(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	res, err := h.homes.ListOutbox(r.Context(), charID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleGetUnreadLetterCount(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	count, err := h.homes.GetUnreadLetterCount(r.Context(), charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: count})
}

func (h *Handler) handleReadLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	var req readLetterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	char, err := h.characters.Get(r.Context(), req.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	letterID := r.PathValue("id")
	err = h.homes.ReadLetter(r.Context(), letterID, req.CharacterID)
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
}

func (h *Handler) handleDeleteLetter(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.URL.Query().Get("character_id")
	if strings.TrimSpace(charID) == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("character_id query parameter is required"))
		return
	}

	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	letterID := r.PathValue("id")
	err = h.homes.DeleteLetter(r.Context(), letterID, charID)
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
}

func (h *Handler) handleTeachCompanionPhrase(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	var req teachPhraseRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	phrase, err := h.homes.TeachCompanionPhrase(r.Context(), charID, req.Phrase)
	if err != nil {
		if errors.Is(err, home.ErrEmptyPhrase) || errors.Is(err, home.ErrPhraseTooLong) || errors.Is(err, home.ErrMaxPhrasesReached) {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, phrase)
}

func (h *Handler) handleForgetCompanionPhrase(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	phraseID := r.PathValue("phrase_id")
	err = h.homes.ForgetCompanionPhrase(r.Context(), phraseID, charID)
	if err != nil {
		if errors.Is(err, home.ErrPhraseNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleTalkToCompanion(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	dialogue, err := h.homes.TalkToCompanion(r.Context(), charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, companionTalkResponse{Dialogue: dialogue})
}

func (h *Handler) handleListDeliveryNotices(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	unclearedOnly := r.URL.Query().Get("uncleared_only") == "true"
	notices, err := h.homes.ListDeliveryNotices(r.Context(), charID, unclearedOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, notices)
}

func (h *Handler) handleClearDeliveryNotices(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	err = h.homes.ClearDeliveryNotices(r.Context(), charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

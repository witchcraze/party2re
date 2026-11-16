package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/validation"
)

// HomeEstateService defines estate, item, and color operations.
type HomeEstateService interface {
	GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID, playerID string) (home.HomeView, error)
	BuildHouse(ctx context.Context, characterID, townID, houseStyle string) (*home.TownHouse, error)
	ListTownHouses(ctx context.Context, townID string) ([]home.TownHouse, error)
	SetCharacterColor(ctx context.Context, characterID, color string) error
	ListHomeItems(ctx context.Context, characterID string) ([]home.HomeUsableItem, error)
	UseHomeItem(ctx context.Context, characterID, instanceID, source string) (*home.UseHomeItemResult, error)
}

// HomeMailboxService defines player-to-player letter and mailbox operations.
type HomeMailboxService interface {
	SendLetter(ctx context.Context, senderID, recipientID, content string) (home.Letter, error)
	ReadLetter(ctx context.Context, letterID, recipientID string) error
	ListInbox(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error)
	ListInboxByCursor(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	ListOutbox(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error)
	ListOutboxByCursor(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
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
	ListDeliveryNoticesPage(ctx context.Context, characterID, playerID string, limit, offset int, cursor string) (pagination.CursorPage[home.DeliveryNotice], error)
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

func isTextValidationError(err error) bool {
	return errors.Is(err, validation.ErrControlCharacter) ||
		errors.Is(err, validation.ErrZeroWidth) ||
		errors.Is(err, validation.ErrBidiOverride) ||
		errors.Is(err, validation.ErrZalgo) ||
		errors.Is(err, validation.ErrEmpty) ||
		errors.Is(err, validation.ErrTooLong)
}

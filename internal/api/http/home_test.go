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
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/validation"
)

type mockHomeService struct {
	getHomeViewFn          func(ctx context.Context, homeCharacterID, visitorCharacterID, playerID string) (home.HomeView, error)
	buildHouseFn           func(ctx context.Context, characterID, townID, houseStyle string) (*home.TownHouse, error)
	sendLetterFn           func(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error)
	readLetterFn           func(ctx context.Context, letterID, recipientID string) error
	listInboxFn            func(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error)
	listInboxByCursorFn    func(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	listOutboxFn           func(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error)
	listOutboxByCursorFn   func(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	getUnreadLetterCountFn func(ctx context.Context, recipientID string) (int, error)
	getSleepStatusFn       func(ctx context.Context, characterID string) (home.SleepStatus, error)
	deleteLetterFn         func(ctx context.Context, letterID, characterID string) error
}

func (m *mockHomeService) GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID, playerID string) (home.HomeView, error) {
	if m.getHomeViewFn != nil {
		return m.getHomeViewFn(ctx, homeCharacterID, visitorCharacterID, playerID)
	}
	if homeCharacterID == "char-not-found" {
		return home.HomeView{}, home.ErrCharacterNotFound
	}
	return home.HomeView{
		Owner: home.HomeOwner{ID: homeCharacterID, Name: "Hero"},
		Home: home.CharacterHome{
			CharacterID: homeCharacterID,
		},
		IsOwner: (homeCharacterID == visitorCharacterID),
	}, nil
}

func (m *mockHomeService) BuildHouse(ctx context.Context, characterID, townID, houseStyle string) (*home.TownHouse, error) {
	if m.buildHouseFn != nil {
		return m.buildHouseFn(ctx, characterID, townID, houseStyle)
	}
	return &home.TownHouse{CharacterID: characterID, TownID: townID, HouseStyle: houseStyle}, nil
}

func (m *mockHomeService) ListTownHouses(context.Context, string) ([]home.TownHouse, error) {
	return []home.TownHouse{}, nil
}

func (m *mockHomeService) SetCharacterColor(context.Context, string, string) error { return nil }

func (m *mockHomeService) ListHomeItems(context.Context, string) ([]home.HomeUsableItem, error) {
	return nil, nil
}

func (m *mockHomeService) UseHomeItem(context.Context, string, string, string) (*home.UseHomeItemResult, error) {
	return &home.UseHomeItemResult{}, nil
}

func (m *mockHomeService) SendLetter(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error) {
	if m.sendLetterFn != nil {
		return m.sendLetterFn(ctx, senderID, recipientID, content, color)
	}
	return home.Letter{
		ID:                   "letter-1",
		SenderCharacterID:    senderID,
		RecipientCharacterID: recipientID,
		Content:              content,
		Color:                color,
		CreatedAt:            time.Now().UTC(),
	}, nil
}

func (m *mockHomeService) ReadLetter(ctx context.Context, letterID, recipientID string) error {
	if m.readLetterFn != nil {
		return m.readLetterFn(ctx, letterID, recipientID)
	}
	if letterID == "not-found" {
		return home.ErrLetterNotFound
	}
	return nil
}

func (m *mockHomeService) ListInbox(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error) {
	if m.listInboxFn != nil {
		return m.listInboxFn(ctx, recipientID, limit, offset)
	}
	return pagination.NewPage([]home.Letter{{ID: "letter-1", RecipientCharacterID: recipientID}}, 1, limit, offset), nil
}

func (m *mockHomeService) ListInboxByCursor(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error) {
	if m.listInboxByCursorFn != nil {
		return m.listInboxByCursorFn(ctx, recipientID, limit, cursor)
	}
	return pagination.NewCursorPage([]home.Letter{{ID: "letter-1", RecipientCharacterID: recipientID}}, "", "", limit, false), nil
}

func (m *mockHomeService) ListOutbox(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error) {
	if m.listOutboxFn != nil {
		return m.listOutboxFn(ctx, senderID, limit, offset)
	}
	return pagination.NewPage([]home.Letter{{ID: "letter-1", SenderCharacterID: senderID}}, 1, limit, offset), nil
}

func (m *mockHomeService) ListOutboxByCursor(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error) {
	if m.listOutboxByCursorFn != nil {
		return m.listOutboxByCursorFn(ctx, senderID, limit, cursor)
	}
	return pagination.NewCursorPage([]home.Letter{{ID: "letter-1", SenderCharacterID: senderID}}, "", "", limit, false), nil
}

func (m *mockHomeService) GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error) {
	if m.getUnreadLetterCountFn != nil {
		return m.getUnreadLetterCountFn(ctx, recipientID)
	}
	return 1, nil
}

func (m *mockHomeService) DeleteLetter(ctx context.Context, letterID, characterID string) error {
	if m.deleteLetterFn != nil {
		return m.deleteLetterFn(ctx, letterID, characterID)
	}
	return nil
}

func (m *mockHomeService) TeachCompanionPhrase(context.Context, string, string) (home.CompanionPhrase, error) {
	return home.CompanionPhrase{}, nil
}

func (m *mockHomeService) ForgetCompanionPhrase(context.Context, string, string) error { return nil }

func (m *mockHomeService) TalkToCompanion(context.Context, string) (home.CompanionTalkResult, error) {
	return home.CompanionTalkResult{}, nil
}

func (m *mockHomeService) ClearDeliveryNotices(context.Context, string, []string) error { return nil }

func (m *mockHomeService) ListDeliveryNoticesPage(context.Context, string, string, int, int, string) (pagination.CursorPage[home.DeliveryNotice], error) {
	return pagination.CursorPage[home.DeliveryNotice]{}, nil
}

func (m *mockHomeService) Sleep(context.Context, string, string) (home.SleepResult, error) {
	return home.SleepResult{}, nil
}

func (m *mockHomeService) GetSleepStatus(ctx context.Context, characterID string) (home.SleepStatus, error) {
	if m.getSleepStatusFn != nil {
		return m.getSleepStatusFn(ctx, characterID)
	}
	return home.SleepStatus{}, nil
}

func (m *mockHomeService) Wake(context.Context, string) (home.WakeResult, error) {
	return home.WakeResult{}, nil
}

func TestHomeEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			if id == "char-2" {
				return otherChar, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	advs := &stubAdventureService{}
	shops := &stubShopService{}
	homeSvc := &mockHomeService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		advs,
		shops,
		apihttp.WithHome(homeSvc),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := handler.Router()

	t.Run("GET /homes/{id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/homes/char-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var res home.HomeView
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.Owner.ID != "char-1" {
			t.Errorf("unexpected home owner: %+v", res.Owner)
		}
	})

	t.Run("POST /letters - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"sender_character_id":    "char-1",
			"recipient_character_id": "char-2",
			"content":                "Let's play!",
			"color":                  "#0000ff",
		})
		req := httptest.NewRequest(http.MethodPost, "/letters", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /letters - throttled sender returns 429", func(t *testing.T) {
		throttledSvc := &mockHomeService{
			sendLetterFn: func(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error) {
				return home.Letter{}, home.ErrLetterThrottled
			},
		}
		h, _ := apihttp.NewHandler(players, chars, advs, shops, apihttp.WithHome(throttledSvc))
		body, _ := json.Marshal(map[string]string{
			"sender_character_id":    "char-1",
			"recipient_character_id": "char-2",
			"content":                "Hello",
		})
		req := httptest.NewRequest(http.MethodPost, "/letters", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /letters - invalid text rejection returns 422", func(t *testing.T) {
		invalidSvc := &mockHomeService{
			sendLetterFn: func(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error) {
				return home.Letter{}, validation.ErrControlCharacter
			},
		}
		h, _ := apihttp.NewHandler(players, chars, advs, shops, apihttp.WithHome(invalidSvc))
		body, _ := json.Marshal(map[string]string{
			"sender_character_id":    "char-1",
			"recipient_character_id": "char-2",
			"content":                "Hello\x00World",
		})
		req := httptest.NewRequest(http.MethodPost, "/letters", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /letters/inbox - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/letters/inbox?character_id=char-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /letters/inbox - cursor success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/letters/inbox?character_id=char-1&cursor=cur-1&limit=10", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /letters/outbox - cursor success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/letters/outbox?character_id=char-1&cursor=cur-1&limit=10", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /letters/unread-count - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/letters/unread-count?character_id=char-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /letters/{id}/read - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"character_id": "char-1",
		})
		req := httptest.NewRequest(http.MethodPost, "/letters/letter-1/read", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

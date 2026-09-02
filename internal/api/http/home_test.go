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
)

type mockHomeService struct {
	getHomeViewFn           func(ctx context.Context, homeCharacterID, visitorCharacterID string) (home.HomeView, error)
	updateHomeFn            func(ctx context.Context, characterID, theme, motto, companionName string) (home.CharacterHome, error)
	sendLetterFn            func(ctx context.Context, senderID, recipientID, content, color string) (home.Letter, error)
	readLetterFn            func(ctx context.Context, letterID, recipientID string) error
	listInboxFn             func(ctx context.Context, recipientID string, limit, offset int) (home.LetterListResult, error)
	listInboxByCursorFn     func(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	listOutboxFn            func(ctx context.Context, senderID string, limit, offset int) (home.LetterListResult, error)
	listOutboxByCursorFn    func(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[home.Letter], error)
	getUnreadLetterCountFn  func(ctx context.Context, recipientID string) (int, error)
	deleteLetterFn          func(ctx context.Context, letterID, characterID string) error
	teachCompanionPhraseFn  func(ctx context.Context, characterID, phrase string) (home.CompanionPhrase, error)
	forgetCompanionPhraseFn func(ctx context.Context, phraseID, characterID string) error
	listCompanionPhrasesFn  func(ctx context.Context, characterID string) ([]home.CompanionPhrase, error)
	talkToCompanionFn       func(ctx context.Context, characterID string) (string, error)
	listDeliveryNoticesFn   func(ctx context.Context, characterID string, unclearedOnly bool) ([]home.DeliveryNotice, error)
	clearDeliveryNoticesFn  func(ctx context.Context, characterID string) error
}

func (m *mockHomeService) GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID string) (home.HomeView, error) {
	if m.getHomeViewFn != nil {
		return m.getHomeViewFn(ctx, homeCharacterID, visitorCharacterID)
	}
	if homeCharacterID == "char-not-found" {
		return home.HomeView{}, home.ErrCharacterNotFound
	}
	return home.HomeView{
		Owner: corecharacter.Character{ID: homeCharacterID, Name: "Hero"},
		Home: home.CharacterHome{
			CharacterID:   homeCharacterID,
			Theme:         "#ffffff",
			CompanionName: "ペット",
		},
		IsOwner: (homeCharacterID == visitorCharacterID),
	}, nil
}

func (m *mockHomeService) UpdateHome(ctx context.Context, characterID, theme, motto, companionName string) (home.CharacterHome, error) {
	if m.updateHomeFn != nil {
		return m.updateHomeFn(ctx, characterID, theme, motto, companionName)
	}
	return home.CharacterHome{
		CharacterID:   characterID,
		Theme:         theme,
		Motto:         motto,
		CompanionName: companionName,
		UpdatedAt:     time.Now().UTC(),
	}, nil
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

func (m *mockHomeService) TeachCompanionPhrase(ctx context.Context, characterID, phrase string) (home.CompanionPhrase, error) {
	if m.teachCompanionPhraseFn != nil {
		return m.teachCompanionPhraseFn(ctx, characterID, phrase)
	}
	return home.CompanionPhrase{
		ID:          "phrase-1",
		CharacterID: characterID,
		Phrase:      phrase,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (m *mockHomeService) ForgetCompanionPhrase(ctx context.Context, phraseID, characterID string) error {
	if m.forgetCompanionPhraseFn != nil {
		return m.forgetCompanionPhraseFn(ctx, phraseID, characterID)
	}
	return nil
}

func (m *mockHomeService) ListCompanionPhrases(ctx context.Context, characterID string) ([]home.CompanionPhrase, error) {
	if m.listCompanionPhrasesFn != nil {
		return m.listCompanionPhrasesFn(ctx, characterID)
	}
	return []home.CompanionPhrase{{ID: "phrase-1", CharacterID: characterID, Phrase: "Hi"}}, nil
}

func (m *mockHomeService) TalkToCompanion(ctx context.Context, characterID string) (string, error) {
	if m.talkToCompanionFn != nil {
		return m.talkToCompanionFn(ctx, characterID)
	}
	return "クエッ！", nil
}

func (m *mockHomeService) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]home.DeliveryNotice, error) {
	if m.listDeliveryNoticesFn != nil {
		return m.listDeliveryNoticesFn(ctx, characterID, unclearedOnly)
	}
	return []home.DeliveryNotice{{ID: "notice-1", CharacterID: characterID, Message: "Gold received"}}, nil
}

func (m *mockHomeService) ClearDeliveryNotices(ctx context.Context, characterID string) error {
	if m.clearDeliveryNoticesFn != nil {
		return m.clearDeliveryNoticesFn(ctx, characterID)
	}
	return nil
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
		req := httptest.NewRequest(http.MethodGet, "/homes/char-1?visitor_id=char-2", nil)
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

	t.Run("POST /homes/{id}/settings - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"theme":          "#123456",
			"motto":          "Welcome to my palace",
			"companion_name": "スライム",
		})
		req := httptest.NewRequest(http.MethodPost, "/homes/char-1/settings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /homes/{id}/settings - forbidden other player", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"theme": "#123456",
		})
		req := httptest.NewRequest(http.MethodPost, "/homes/char-2/settings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
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

	t.Run("POST /homes/{id}/companion/phrases - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"phrase": "おかえり！",
		})
		req := httptest.NewRequest(http.MethodPost, "/homes/char-1/companion/phrases", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /homes/{id}/companion/talk - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/homes/char-1/companion/talk", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /homes/{id}/notices - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/homes/char-1/notices", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /homes/{id}/notices/clear - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/homes/char-1/notices/clear", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

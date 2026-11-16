package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
)

type mockHomeService struct {
	getHomeViewFn    func(ctx context.Context, homeCharacterID, visitorCharacterID, playerID string) (home.HomeView, error)
	buildHouseFn     func(ctx context.Context, characterID, townID, houseStyle string) (*home.TownHouse, error)
	getSleepStatusFn func(ctx context.Context, characterID string) (home.SleepStatus, error)
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

func (m *mockHomeService) SendLetter(context.Context, string, string, string) (home.Letter, error) {
	return home.Letter{}, nil
}

func (m *mockHomeService) ReadLetter(context.Context, string, string) error { return nil }

func (m *mockHomeService) ListInbox(context.Context, string, int, int) (home.LetterListResult, error) {
	return home.LetterListResult{}, nil
}

func (m *mockHomeService) ListInboxByCursor(context.Context, string, int, string) (pagination.CursorPage[home.Letter], error) {
	return pagination.CursorPage[home.Letter]{}, nil
}

func (m *mockHomeService) ListOutbox(context.Context, string, int, int) (home.LetterListResult, error) {
	return home.LetterListResult{}, nil
}

func (m *mockHomeService) ListOutboxByCursor(context.Context, string, int, string) (pagination.CursorPage[home.Letter], error) {
	return pagination.CursorPage[home.Letter]{}, nil
}

func (m *mockHomeService) DeleteLetter(context.Context, string, string) error { return nil }

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
}

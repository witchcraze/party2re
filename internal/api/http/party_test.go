package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/party"
)

type stubPartyService struct {
	parties []party.PartySummary
	detail  party.PartyDetail
	advRes  party.PartyAdventureResult
	err     error
}

func (s *stubPartyService) CreateParty(_ context.Context, leaderCharID string, req party.CreatePartyRequest) (party.PartyDetail, error) {
	if s.err != nil {
		return party.PartyDetail{}, s.err
	}
	return s.detail, nil
}

func (s *stubPartyService) GetParty(_ context.Context, partyID string) (party.PartyDetail, error) {
	if s.err != nil {
		return party.PartyDetail{}, s.err
	}
	return s.detail, nil
}

func (s *stubPartyService) ListParties(_ context.Context, status string, limit, offset int) ([]party.PartySummary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.parties, nil
}

func (s *stubPartyService) JoinParty(_ context.Context, partyID, characterID, password string) (party.PartyDetail, error) {
	if s.err != nil {
		return party.PartyDetail{}, s.err
	}
	return s.detail, nil
}

func (s *stubPartyService) LeaveParty(_ context.Context, partyID, characterID string) error {
	return s.err
}

func (s *stubPartyService) KickMember(_ context.Context, partyID, leaderCharID, targetCharID string) error {
	return s.err
}

func (s *stubPartyService) DisbandParty(_ context.Context, partyID, leaderCharID string) error {
	return s.err
}

func (s *stubPartyService) SetReady(_ context.Context, partyID, characterID string, ready bool) (party.PartyDetail, error) {
	if s.err != nil {
		return party.PartyDetail{}, s.err
	}
	return s.detail, nil
}

func (s *stubPartyService) StartPartyAdventure(_ context.Context, partyID, leaderCharID string) (party.PartyAdventureResult, error) {
	if s.err != nil {
		return party.PartyAdventureResult{}, s.err
	}
	return s.advRes, nil
}

func TestPartyHTTPHandlers(t *testing.T) {
	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "sess-1" {
				return coreplayer.Player{ID: "player-1", Username: "player1"}, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	charSvc := &stubCharacterServiceExtended{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{
				ID:       id,
				PlayerID: "player-1",
				Name:     "Hero",
				Level:    10,
				Stats:    corecharacter.Stats{HP: 50, MaxHP: 50},
			}, nil
		},
	}

	partySvc := &stubPartyService{
		parties: []party.PartySummary{
			{ID: "party-1", Name: "TestParty", LeaderName: "Hero", CurrentMembers: 1, MaxMembers: 4, Status: party.StatusRecruiting},
		},
		detail: party.PartyDetail{
			Party: party.Party{ID: "party-1", Name: "TestParty", LeaderCharacterID: "c1", Status: party.StatusRecruiting, MaxMembers: 4},
			Members: []party.Member{
				{PartyID: "party-1", CharacterID: "c1", CharacterName: "Hero", IsLeader: true, ReadyState: true},
			},
		},
		advRes: party.PartyAdventureResult{
			PartyID:             "party-1",
			StageID:             "forest",
			Outcome:             "win",
			Turns:               3,
			TotalEXP:            110,
			TotalGold:           55,
			SynergyBonusPercent: 10,
		},
	}

	handler, err := apihttp.NewHandler(
		playerSvc,
		charSvc,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithParty(partySvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	router := handler.Router()

	// 1. GET /parties
	req := httptest.NewRequest(http.MethodGet, "/parties", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /parties returned %d, want 200", w.Code)
	}

	// 2. GET /parties/party-1
	req = httptest.NewRequest(http.MethodGet, "/parties/party-1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /parties/party-1 returned %d, want 200", w.Code)
	}

	// 3. POST /parties (create)
	body, _ := json.Marshal(map[string]any{
		"character_id": "c1",
		"name":         "TestParty",
		"stage_id":     "forest",
		"max_members":  4,
	})
	req = httptest.NewRequest(http.MethodPost, "/parties", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /parties returned %d (body: %s), want 201", w.Code, w.Body.String())
	}

	// 4. POST /parties/party-1/join
	body, _ = json.Marshal(map[string]any{
		"character_id": "c1",
		"password":     "",
	})
	req = httptest.NewRequest(http.MethodPost, "/parties/party-1/join", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /parties/party-1/join returned %d, want 200", w.Code)
	}

	// 5. POST /parties/party-1/ready
	body, _ = json.Marshal(map[string]any{
		"character_id": "c1",
		"ready":        true,
	})
	req = httptest.NewRequest(http.MethodPost, "/parties/party-1/ready", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /parties/party-1/ready returned %d, want 200", w.Code)
	}

	// 6. POST /parties/party-1/kick
	body, _ = json.Marshal(map[string]any{
		"character_id":        "c1",
		"target_character_id": "c2",
	})
	req = httptest.NewRequest(http.MethodPost, "/parties/party-1/kick", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /parties/party-1/kick returned %d, want 200", w.Code)
	}

	// 7. POST /parties/party-1/start
	body, _ = json.Marshal(map[string]any{
		"character_id": "c1",
	})
	req = httptest.NewRequest(http.MethodPost, "/parties/party-1/start", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /parties/party-1/start returned %d, want 200", w.Code)
	}

	// 8. POST /parties/party-1/leave
	body, _ = json.Marshal(map[string]any{
		"character_id": "c1",
	})
	req = httptest.NewRequest(http.MethodPost, "/parties/party-1/leave", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /parties/party-1/leave returned %d, want 200", w.Code)
	}

	// 9. DELETE /parties/party-1
	body, _ = json.Marshal(map[string]any{
		"character_id": "c1",
	})
	req = httptest.NewRequest(http.MethodDelete, "/parties/party-1", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sess-1")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /parties/party-1 returned %d, want 200", w.Code)
	}
}

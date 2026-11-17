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
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubCharacterServiceExtended struct {
	createFn                func(ctx context.Context, playerID, name string) (corecharacter.Character, error)
	getFn                   func(ctx context.Context, id string) (corecharacter.Character, error)
	changeNameFn            func(ctx context.Context, characterID, newName string) (corecharacter.Character, error)
	changeGenderFn          func(ctx context.Context, characterID, newGender string) (corecharacter.Character, error)
	getProfileFn            func(ctx context.Context, characterID string) (character.ProfileView, error)
	getNamingHallDialogueFn func() character.NamingHallDialogue
	deleteFn                func(ctx context.Context, playerID, characterID string) error
}

func (s *stubCharacterServiceExtended) Create(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
	if s.createFn != nil {
		return s.createFn(ctx, playerID, name)
	}
	return corecharacter.Character{ID: "char-1", PlayerID: playerID, Name: name}, nil
}

func (s *stubCharacterServiceExtended) CreateWithOptions(ctx context.Context, playerID, name string, options character.CreationOptions) (corecharacter.Character, error) {
	if s.createFn != nil {
		return s.createFn(ctx, playerID, name)
	}
	return corecharacter.Character{ID: "char-1", PlayerID: playerID, Name: name, JobID: options.JobID, Gender: options.Gender}, nil
}

func (s *stubCharacterServiceExtended) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return corecharacter.Character{ID: id, PlayerID: "player-1", Name: "Hero", Gender: "m", Money: 999999}, nil
}

func (s *stubCharacterServiceExtended) ChangeName(ctx context.Context, characterID, newName string) (corecharacter.Character, error) {
	if s.changeNameFn != nil {
		return s.changeNameFn(ctx, characterID, newName)
	}
	return corecharacter.Character{ID: characterID, PlayerID: "player-1", Name: newName, Money: 500000}, nil
}

func (s *stubCharacterServiceExtended) ChangeGender(ctx context.Context, characterID, newGender string) (corecharacter.Character, error) {
	if s.changeGenderFn != nil {
		return s.changeGenderFn(ctx, characterID, newGender)
	}
	return corecharacter.Character{ID: characterID, PlayerID: "player-1", Gender: newGender, Money: 990000}, nil
}

func (s *stubCharacterServiceExtended) Trajectory(context.Context, string) ([]character.Memory, error) {
	return nil, nil
}

func (s *stubCharacterServiceExtended) GetProfile(ctx context.Context, characterID string) (character.ProfileView, error) {
	if s.getProfileFn != nil {
		return s.getProfileFn(ctx, characterID)
	}
	return character.ProfileView{
		Character: corecharacter.Character{ID: characterID, Name: "Hero"},
		Profile: character.Profile{
			CharacterID: characterID,
			Comment:     "Mighty adventurer",
			AvatarURL:   "https://i.imgur.com/AvATest.gif",
			AuraEffect:  3,
		},
	}, nil
}

func (s *stubCharacterServiceExtended) UpdateProfile(context.Context, string, map[string]string) (character.Profile, error) {
	panic("profile writes are a Gateway command")
}

func (s *stubCharacterServiceExtended) GetNamingHallDialogue() character.NamingHallDialogue {
	if s.getNamingHallDialogueFn != nil {
		return s.getNamingHallDialogueFn()
	}
	return character.NamingHallDialogue{
		NPCName:          "@マリナン",
		LocationTitle:    "命名の館",
		Phrases:          []string{"Welcome"},
		NameChangeCost:   500000,
		GenderChangeCost: 10000,
	}
}

func (s *stubCharacterServiceExtended) Delete(ctx context.Context, playerID, characterID string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, playerID, characterID)
	}
	return nil
}

func TestCharacterCustomizationHTTP(t *testing.T) {
	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return coreplayer.Player{ID: "player-1", Username: "player1"}, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	charSvc := &stubCharacterServiceExtended{}

	handler, err := apihttp.NewHandler(playerSvc, charSvc, &stubAdventureService{}, &stubShopService{})
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	client := &http.Client{}

	// 1. GET /naming-hall/dialogue (Public)
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/naming-hall/dialogue", nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for dialogue, got %d (err: %v)", resp.StatusCode, err)
	}

	// 2. GET /characters/{id}/profile (Public)
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/characters/char-1/profile", nil)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for profile view, got %d (err: %v)", resp.StatusCode, err)
	}

	var profileView character.ProfileView
	if err := json.NewDecoder(resp.Body).Decode(&profileView); err != nil {
		t.Fatalf("failed to decode profile view response: %v", err)
	}
	resp.Body.Close()
	if profileView.Profile.AuraEffect != 3 {
		t.Fatalf("expected profile.aura_effect == 3 in GET response, got %d", profileView.Profile.AuraEffect)
	}

	// 3. POST /characters/{id}/name - Unauthenticated -> 401
	body, _ := json.Marshal(map[string]string{"name": "NewHero"})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/name", bytes.NewReader(body))
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 4. POST /characters/{id}/name - Authenticated -> 200 OK
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/name", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for rename, got %d (err: %v)", resp.StatusCode, err)
	}
	resp.Body.Close()

	// 4b. POST /characters/{id}/name - Invalid name -> 400 Bad Request
	charSvc.changeNameFn = func(ctx context.Context, characterID, newName string) (corecharacter.Character, error) {
		return corecharacter.Character{}, character.ErrInvalidName
	}
	invBody, _ := json.Marshal(map[string]string{"name": "TooLongName"})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/name", bytes.NewReader(invBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid rename, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	charSvc.changeNameFn = nil

	// 5. POST /characters/{id}/gender - Authenticated -> 200 OK
	genderBody, _ := json.Marshal(map[string]string{"gender": "f"})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/gender", bytes.NewReader(genderBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for gender change, got %d (err: %v)", resp.StatusCode, err)
	}
	resp.Body.Close()

	// 5b. POST /characters/{id}/gender - Incompatible with job -> 400 Bad Request
	charSvc.changeGenderFn = func(ctx context.Context, characterID, newGender string) (corecharacter.Character, error) {
		return corecharacter.Character{}, character.ErrGenderIncompatibleWithJob
	}
	genderBody, _ = json.Marshal(map[string]string{"gender": "m"})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/gender", bytes.NewReader(genderBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for incompatible gender, got %d (err: %v)", resp.StatusCode, err)
	}
	var errResp map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()
	if errResp["error"] != character.ErrGenderIncompatibleWithJob.Error() {
		t.Fatalf("expected error message %q, got %q", character.ErrGenderIncompatibleWithJob.Error(), errResp["error"])
	}
	charSvc.changeGenderFn = nil

	// 6. Profile writes moved to the Home home_profile_update command.
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		req, _ = http.NewRequest(method, server.URL+"/characters/char-1/profile", bytes.NewReader([]byte(`{"bio_data":{}}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s profile: expected retired route, got %d (err: %v)", method, resp.StatusCode, err)
		}
		resp.Body.Close()
	}

	// 7. The data-URI avatar upload route is retired (legacy has no upload).
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/avatar", bytes.NewReader([]byte(`{"image_data":"x"}`)))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || (resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed) {
		t.Fatalf("expected retired avatar route, got %d (err: %v)", resp.StatusCode, err)
	}
	resp.Body.Close()
}

func TestCreateCharacterEndpoint(t *testing.T) {
	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return coreplayer.Player{ID: "player-1", Username: "player1"}, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	charSvc := &stubCharacterServiceExtended{}

	handler, err := apihttp.NewHandler(playerSvc, charSvc, &stubAdventureService{}, &stubShopService{})
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	client := &http.Client{}

	// 1. Unauthenticated -> 401
	body, _ := json.Marshal(map[string]string{
		"name":   "Hero",
		"job_id": "job-01",
		"gender": "m",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters", bytes.NewReader(body))
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 2. Authenticated valid creation -> 201
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("failed to decode 201 response: %v", err)
	}
	resp.Body.Close()
	if created.ID != "char-1" || created.Name != "Hero" {
		t.Fatalf("unexpected created character payload: %+v", created)
	}

	// 3. Error from character service (e.g. invalid job/gender) -> 422
	charSvc.createFn = func(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
		return corecharacter.Character{}, errors.New("invalid job")
	}
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got %d", resp.StatusCode)
	}
}

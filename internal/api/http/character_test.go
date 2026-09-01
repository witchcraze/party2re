package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
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
	rebirthFn               func(ctx context.Context, id string) (corecharacter.Character, error)
	changeNameFn            func(ctx context.Context, characterID, newName string) (corecharacter.Character, error)
	changeGenderFn          func(ctx context.Context, characterID, newGender string) (corecharacter.Character, error)
	getProfileFn            func(ctx context.Context, characterID string) (character.ProfileView, error)
	updateProfileFn         func(ctx context.Context, characterID string, req character.UpdateProfileRequest) (character.Profile, error)
	uploadAvatarFn          func(ctx context.Context, characterID string, filename string, contentType string, data []byte) (string, error)
	getNamingHallDialogueFn func() character.NamingHallDialogue
	deleteFn                func(ctx context.Context, playerID, characterID string) error
}

func (s *stubCharacterServiceExtended) Create(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
	if s.createFn != nil {
		return s.createFn(ctx, playerID, name)
	}
	return corecharacter.Character{ID: "char-1", PlayerID: playerID, Name: name}, nil
}

func (s *stubCharacterServiceExtended) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return corecharacter.Character{ID: id, PlayerID: "player-1", Name: "Hero", Gender: "m", Money: 1000000}, nil
}

func (s *stubCharacterServiceExtended) Rebirth(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.rebirthFn != nil {
		return s.rebirthFn(ctx, id)
	}
	return corecharacter.Character{ID: id, RebirthCount: 1}, nil
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

func (s *stubCharacterServiceExtended) GetProfile(ctx context.Context, characterID string) (character.ProfileView, error) {
	if s.getProfileFn != nil {
		return s.getProfileFn(ctx, characterID)
	}
	return character.ProfileView{
		Character: corecharacter.Character{ID: characterID, Name: "Hero"},
		Profile: character.Profile{
			CharacterID: characterID,
			Comment:     "Mighty adventurer",
			AvatarURL:   "https://example.com/avatar.png",
		},
	}, nil
}

func (s *stubCharacterServiceExtended) UpdateProfile(ctx context.Context, characterID string, req character.UpdateProfileRequest) (character.Profile, error) {
	if s.updateProfileFn != nil {
		return s.updateProfileFn(ctx, characterID, req)
	}
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}
	avatarURL := ""
	if req.AvatarURL != nil {
		avatarURL = *req.AvatarURL
	}
	return character.Profile{
		CharacterID: characterID,
		Comment:     comment,
		AvatarURL:   avatarURL,
		BioData:     req.BioData,
	}, nil
}

func (s *stubCharacterServiceExtended) UploadAvatar(ctx context.Context, characterID string, filename string, contentType string, data []byte) (string, error) {
	if s.uploadAvatarFn != nil {
		return s.uploadAvatarFn(ctx, characterID, filename, contentType, data)
	}
	return "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", nil
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
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for rename, got %d (err: %v)", resp.StatusCode, err)
	}

	// 5. POST /characters/{id}/gender - Authenticated -> 200 OK
	genderBody, _ := json.Marshal(map[string]string{"gender": "f"})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/gender", bytes.NewReader(genderBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for gender change, got %d (err: %v)", resp.StatusCode, err)
	}

	// 6. PUT /characters/{id}/profile - Authenticated -> 200 OK
	profBody, _ := json.Marshal(map[string]interface{}{
		"comment":    "New bio comment",
		"avatar_url": "https://example.com/avatar2.png",
	})
	req, _ = http.NewRequest(http.MethodPut, server.URL+"/characters/char-1/profile", bytes.NewReader(profBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for update profile, got %d (err: %v)", resp.StatusCode, err)
	}

	// 7. POST /characters/{id}/avatar - Multipart Upload -> 200 OK
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("avatar", "test.png")
	part.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4"))
	writer.Close()

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/avatar", &buf)
	req.Header.Set("Authorization", "Bearer valid-session")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for avatar upload, got %d (err: %v)", resp.StatusCode, err)
	}
}

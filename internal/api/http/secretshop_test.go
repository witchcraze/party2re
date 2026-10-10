package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type stubSecretShopService struct{ apihttp.SecretShopService }

func (*stubSecretShopService) Talk(context.Context, string) (string, error) {
	return "メェ〜", nil
}

func (*stubSecretShopService) Inspect(context.Context, string) (string, error) {
	return secretshop.InspectDialogue, nil
}

func (*stubSecretShopService) PuffPuff(_ context.Context, characterID string) (*secretshop.PuffPuffResult, error) {
	return &secretshop.PuffPuffResult{
		CharacterID: characterID,
		NPCName:     secretshop.NPCName,
		Message:     secretshop.PuffPuffDialogue,
	}, nil
}

func TestSecretShopNPCEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 20, Money: 50000}
	h := newTestHandler(t,
		&stubPlayerService{authenticateFn: alwaysAuthPlayer(player)},
		&stubCharacterService{getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == char.ID {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		}},
		&stubAdventureService{}, &stubShopService{}, apihttp.WithSecretShop(&stubSecretShopService{}),
	)
	router := h.Router()
	for _, tc := range []struct{ path, message string }{
		{"talk", "メェ〜"},
		{"inspect", secretshop.InspectDialogue},
		{"puffpuff", secretshop.PuffPuffDialogue},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/characters/c1/secretshop/"+tc.path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
			}
			var response struct {
				CharacterID string `json:"character_id"`
				NPCName     string `json:"npc_name"`
				Message     string `json:"message"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.CharacterID != char.ID || response.NPCName != secretshop.NPCName || response.Message != tc.message {
				t.Fatalf("lost NPC response: %+v", response)
			}
		})
	}
}

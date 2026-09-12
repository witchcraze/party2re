package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
)

type stubAlchemyService struct {
	getStatusFn     func(ctx context.Context, characterID string) (alchemy.Synthesis, error)
	getCompendiumFn func(ctx context.Context, characterID string) (alchemy.Compendium, error)
	synthesizeFn    func(ctx context.Context, characterID string, recipeID string) (alchemy.SynthesisResult, error)
	claimFn         func(ctx context.Context, characterID string) (alchemy.ClaimResult, error)
	learnRecipeFn   func(ctx context.Context, characterID string, pool []string) (alchemy.Recipe, error)
}

func (s *stubAlchemyService) GetStatus(ctx context.Context, characterID string) (alchemy.Synthesis, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return alchemy.Synthesis{
		CharacterID: characterID,
		State:       alchemy.StateNone,
	}, nil
}

func (s *stubAlchemyService) GetCompendium(ctx context.Context, characterID string) (alchemy.Compendium, error) {
	if s.getCompendiumFn != nil {
		return s.getCompendiumFn(ctx, characterID)
	}
	return alchemy.Compendium{
		TotalRecipes:         112,
		LearnedCount:         1,
		CraftedCount:         0,
		CompletionPercentage: 0,
		CompAlc:              false,
	}, nil
}

func (s *stubAlchemyService) Synthesize(ctx context.Context, characterID string, recipeID string) (alchemy.SynthesisResult, error) {
	if s.synthesizeFn != nil {
		return s.synthesizeFn(ctx, characterID, recipeID)
	}
	r, _ := alchemy.NewRecipe(recipeID, "Synthesize", "item-002", 1, []alchemy.Ingredient{{DefinitionID: "item-001", Quantity: 2}})
	now := time.Now()
	return alchemy.SynthesisResult{
		Recipe:    r,
		State:     alchemy.StateOngoing,
		StartedAt: now,
		MaturesAt: now.Add(12 * time.Hour),
		Message:   "一晩たてば完成するじゃろう",
	}, nil
}

func (s *stubAlchemyService) Claim(ctx context.Context, characterID string) (alchemy.ClaimResult, error) {
	if s.claimFn != nil {
		return s.claimFn(ctx, characterID)
	}
	r, _ := alchemy.NewRecipe("rec-1", "Synthesize", "item-002", 1, []alchemy.Ingredient{{DefinitionID: "item-001", Quantity: 2}})
	itemInst, _ := coreitem.NewInstance("item-002", 1)
	return alchemy.ClaimResult{
		Recipe:         r,
		CreatedItem:    itemInst,
		TotalCrafts:    1,
		CompAlcAwarded: false,
		Message:        "完成したぞい！",
	}, nil
}

func (s *stubAlchemyService) LearnRecipe(ctx context.Context, characterID string, pool []string) (alchemy.Recipe, error) {
	if s.learnRecipeFn != nil {
		return s.learnRecipeFn(ctx, characterID, pool)
	}
	return alchemy.NewRecipe("rec-learned", "Learned Recipe", "item-002", 1, []alchemy.Ingredient{{DefinitionID: "item-001", Quantity: 2}})
}

func setupAlchemyTestHandler(t *testing.T, alchemySvc *stubAlchemyService) (*apihttp.Handler, string) {
	player := coreplayer.Player{ID: "p1", Username: "alchemist"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "AlchemistChar", Level: 10, Money: 1000}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAlchemy(alchemySvc),
	)
	return h, "c1"
}

func TestHTTP_GetCharacterAlchemy(t *testing.T) {
	h, charID := setupAlchemyTestHandler(t, &stubAlchemyService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/characters/"+charID+"/alchemy", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Status     alchemy.Synthesis  `json:"status"`
		Compendium alchemy.Compendium `json:"compendium"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if body.Compendium.TotalRecipes != 112 {
		t.Errorf("expected 112 total recipes, got %d", body.Compendium.TotalRecipes)
	}
}

func TestHTTP_AlchemySynthesize(t *testing.T) {
	h, charID := setupAlchemyTestHandler(t, &stubAlchemyService{})
	router := h.Router()

	payload := `{"recipe_id":"recipe-001"}`
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/alchemy/synthesize", bytes.NewReader([]byte(payload)))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res alchemy.SynthesisResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if res.State != alchemy.StateOngoing {
		t.Errorf("expected StateOngoing, got %v", res.State)
	}
}

func TestHTTP_AlchemySynthesizeErrors(t *testing.T) {
	stub := &stubAlchemyService{
		synthesizeFn: func(_ context.Context, _ string, _ string) (alchemy.SynthesisResult, error) {
			return alchemy.SynthesisResult{}, alchemy.ErrInsufficientMaterials
		},
	}
	h, charID := setupAlchemyTestHandler(t, stub)
	router := h.Router()

	payload := `{"recipe_id":"recipe-001"}`
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/alchemy/synthesize", bytes.NewReader([]byte(payload)))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422 for ErrInsufficientMaterials, got %d", rec.Code)
	}
}

func TestHTTP_AlchemyClaim(t *testing.T) {
	h, charID := setupAlchemyTestHandler(t, &stubAlchemyService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/alchemy/claim", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res alchemy.ClaimResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if res.TotalCrafts != 1 {
		t.Errorf("expected TotalCrafts == 1, got %d", res.TotalCrafts)
	}
}

func TestHTTP_AlchemyClaimDepotFullConflict(t *testing.T) {
	stub := &stubAlchemyService{
		claimFn: func(_ context.Context, _ string) (alchemy.ClaimResult, error) {
			return alchemy.ClaimResult{}, depot.ErrDepotFull
		},
	}
	h, charID := setupAlchemyTestHandler(t, stub)
	router := h.Router()

	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/alchemy/claim", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409 Conflict for ErrDepotFull, got %d", rec.Code)
	}
}

func TestHTTP_AlchemyLearn(t *testing.T) {
	h, charID := setupAlchemyTestHandler(t, &stubAlchemyService{})
	router := h.Router()

	payload, _ := json.Marshal(map[string]any{"pool": []string{"item-001"}})
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/alchemy/learn", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var r alchemy.Recipe
	if err := json.NewDecoder(rec.Body).Decode(&r); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if r.ID != "rec-learned" {
		t.Errorf("expected rec-learned, got %s", r.ID)
	}
}

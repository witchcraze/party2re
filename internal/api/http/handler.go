// Package http exposes game application services as HTTP JSON API endpoints.
// Handlers contain no domain business logic; they delegate strictly to
// application services injected at construction time.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/shop"
)

// PlayerService defines the player account operations exposed over HTTP.
type PlayerService interface {
	Register(ctx context.Context, username, password string) (coreplayer.Player, error)
	Login(ctx context.Context, username, password string) (coreplayer.Session, error)
	Logout(ctx context.Context, sessionID string) error
	Authenticate(ctx context.Context, sessionID string) (coreplayer.Player, error)
}

// CharacterService defines the character operations exposed over HTTP.
type CharacterService interface {
	Create(ctx context.Context, playerID string, name string) (corecharacter.Character, error)
	Get(ctx context.Context, id string) (corecharacter.Character, error)
}

// AdventureService defines the adventure operations exposed over HTTP.
type AdventureService interface {
	StartStage(ctx context.Context, characterID string, stageID string) (adventure.Adventure, error)
	Claim(ctx context.Context, id string) (adventure.Adventure, error)
}

// ShopService defines the shop operations exposed over HTTP.
type ShopService interface {
	Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (shop.PurchaseResult, error)
	Sell(ctx context.Context, characterID string, itemInstanceID string, quantity int) (shop.SaleResult, error)
}

// Handler holds all HTTP handlers for the game API.
type Handler struct {
	players    PlayerService
	characters CharacterService
	adventures AdventureService
	shops      ShopService
}

// NewHandler constructs an HTTP Handler with the required application services.
func NewHandler(
	players PlayerService,
	characters CharacterService,
	adventures AdventureService,
	shops ShopService,
) (*Handler, error) {
	if players == nil {
		return nil, errors.New("player service is nil")
	}
	if characters == nil {
		return nil, errors.New("character service is nil")
	}
	if adventures == nil {
		return nil, errors.New("adventure service is nil")
	}
	if shops == nil {
		return nil, errors.New("shop service is nil")
	}
	return &Handler{
		players:    players,
		characters: characters,
		adventures: adventures,
		shops:      shops,
	}, nil
}

// Router returns an http.ServeMux wired to all API endpoints.
func (h *Handler) Router() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", h.handleHealth)

	mux.HandleFunc("POST /players", h.handleRegisterPlayer)
	mux.HandleFunc("POST /sessions", h.handleLogin)
	mux.HandleFunc("DELETE /sessions", h.handleLogout)

	mux.HandleFunc("POST /characters", h.handleCreateCharacter)
	mux.HandleFunc("GET /characters/{id}", h.handleGetCharacter)

	mux.HandleFunc("POST /adventures", h.handleStartAdventure)
	mux.HandleFunc("POST /adventures/{id}/claim", h.handleClaimAdventure)

	mux.HandleFunc("POST /shop/purchase", h.handlePurchase)
	mux.HandleFunc("POST /shop/sell", h.handleSell)

	return mux
}

// -------------------------------------------------------------------
// Health
// -------------------------------------------------------------------

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// -------------------------------------------------------------------
// Player / Session
// -------------------------------------------------------------------

type registerPlayerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type playerResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) handleRegisterPlayer(w http.ResponseWriter, r *http.Request) {
	var req registerPlayerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.players.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, playerResponse{
		ID:        p.ID,
		Username:  p.Username,
		CreatedAt: p.CreatedAt,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	ID        string    `json:"id"`
	PlayerID  string    `json:"player_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	session, err := h.players.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, coreplayer.ErrAuthentication) {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{
		ID:        session.ID,
		PlayerID:  session.PlayerID,
		ExpiresAt: session.ExpiresAt,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	if err := h.players.Logout(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// -------------------------------------------------------------------
// Character
// -------------------------------------------------------------------

type createCharacterRequest struct {
	Name string `json:"name"`
}

type characterResponse struct {
	ID           string        `json:"id"`
	PlayerID     string        `json:"player_id"`
	Name         string        `json:"name"`
	JobID        string        `json:"job_id"`
	Gender       string        `json:"gender"`
	Level        int           `json:"level"`
	Experience   int           `json:"experience"`
	Money        int           `json:"money"`
	RebirthCount int           `json:"rebirth_count"`
	Stats        statsResponse `json:"stats"`
}

type statsResponse struct {
	MaxHP   int `json:"max_hp"`
	MaxMP   int `json:"max_mp"`
	HP      int `json:"hp"`
	MP      int `json:"mp"`
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Agility int `json:"agility"`
}

func (h *Handler) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	// Require authentication.
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

	var req createCharacterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	char, err := h.characters.Create(r.Context(), player.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCharacterResponse(char))
}

func (h *Handler) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	// Require authentication.
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

	id := r.PathValue("id")
	char, err := h.characters.Get(r.Context(), id)
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
	writeJSON(w, http.StatusOK, toCharacterResponse(char))
}

func toCharacterResponse(char corecharacter.Character) characterResponse {
	return characterResponse{
		ID:           char.ID,
		PlayerID:     char.PlayerID,
		Name:         char.Name,
		JobID:        char.JobID,
		Gender:       char.Gender,
		Level:        char.Level,
		Experience:   char.Experience,
		Money:        char.Money,
		RebirthCount: char.RebirthCount,
		Stats: statsResponse{
			MaxHP:   char.Stats.MaxHP,
			MaxMP:   char.Stats.MaxMP,
			HP:      char.Stats.HP,
			MP:      char.Stats.MP,
			Attack:  char.Stats.Attack,
			Defense: char.Stats.Defense,
			Agility: char.Stats.Agility,
		},
	}
}

// -------------------------------------------------------------------
// Adventure
// -------------------------------------------------------------------

type startAdventureRequest struct {
	CharacterID string `json:"character_id"`
	StageID     string `json:"stage_id"`
}

type adventureResponse struct {
	ID               string    `json:"id"`
	CharacterID      string    `json:"character_id"`
	StageID          string    `json:"stage_id"`
	StartedAt        time.Time `json:"started_at"`
	AvailableAt      time.Time `json:"available_at"`
	Resolved         bool      `json:"resolved"`
	Claimed          bool      `json:"claimed"`
	ExperienceReward int       `json:"experience_reward"`
}

func (h *Handler) handleStartAdventure(w http.ResponseWriter, r *http.Request) {
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

	var req startAdventureRequest
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
	adv, err := h.adventures.StartStage(r.Context(), req.CharacterID, req.StageID)
	if err != nil {
		if errors.Is(err, adventure.ErrLevelRequirementNotMet) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAdventureResponse(adv))
}

func (h *Handler) handleClaimAdventure(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	if _, err := h.players.Authenticate(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	id := r.PathValue("id")
	adv, err := h.adventures.Claim(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, adventure.ErrNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, adventure.ErrNotReady):
			writeError(w, http.StatusConflict, err)
		case errors.Is(err, adventure.ErrAlreadyClaimed):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, toAdventureResponse(adv))
}

func toAdventureResponse(adv adventure.Adventure) adventureResponse {
	return adventureResponse{
		ID:               adv.ID,
		CharacterID:      adv.CharacterID,
		StageID:          adv.StageID,
		StartedAt:        adv.StartedAt,
		AvailableAt:      adv.AvailableAt,
		Resolved:         adv.Resolved,
		Claimed:          adv.Claimed,
		ExperienceReward: adv.ExperienceReward,
	}
}

// -------------------------------------------------------------------
// Shop
// -------------------------------------------------------------------

type purchaseRequest struct {
	CharacterID      string `json:"character_id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Quantity         int    `json:"quantity"`
}

type purchaseResponse struct {
	CharacterID      string `json:"character_id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Quantity         int    `json:"quantity"`
	TotalCost        int    `json:"total_cost"`
}

type sellRequest struct {
	CharacterID    string `json:"character_id"`
	ItemInstanceID string `json:"item_instance_id"`
	Quantity       int    `json:"quantity"`
}

type saleResponse struct {
	CharacterID string `json:"character_id"`
	InstanceID  string `json:"item_instance_id"`
	Quantity    int    `json:"quantity"`
	TotalPayout int    `json:"total_payout"`
}

func (h *Handler) handlePurchase(w http.ResponseWriter, r *http.Request) {
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

	var req purchaseRequest
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
	result, err := h.shops.Purchase(r.Context(), req.CharacterID, req.ItemDefinitionID, req.Quantity)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, purchaseResponse{
		CharacterID:      result.Character.ID,
		ItemDefinitionID: result.ItemInstance.DefinitionID,
		Quantity:         result.ItemInstance.Quantity,
		TotalCost:        result.TotalPrice,
	})
}

func (h *Handler) handleSell(w http.ResponseWriter, r *http.Request) {
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

	var req sellRequest
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
	result, err := h.shops.Sell(r.Context(), req.CharacterID, req.ItemInstanceID, req.Quantity)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, saleResponse{
		CharacterID: result.Character.ID,
		InstanceID:  result.SoldInstance.ID,
		Quantity:    result.SoldInstance.Quantity,
		TotalPayout: result.TotalPayout,
	})
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

// sessionIDFromRequest extracts the session ID from the Authorization header.
// Expected format: "Bearer <session-id>"
func sessionIDFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		return auth[len(prefix):]
	}
	return ""
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// maxRequestBodyBytes is the maximum accepted request body size (64 KiB).
// No legitimate game API call requires more than this.
const maxRequestBodyBytes = 64 * 1024

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("Content-Type must be application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return false
	}
	return true
}

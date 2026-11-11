// Package http exposes game application services as HTTP JSON API endpoints.
// Handlers contain no domain business logic; they delegate strictly to
// application services injected at construction time.
package http

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/ratelimit"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/shop"
)

// RateLimiter defines the rate limiting interface required by HTTP middleware.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error)
}

// RateLimitConfig configures rate limiting thresholds and windows.
type RateLimitConfig struct {
	PublicLimit   int64
	PublicWindow  time.Duration
	GeneralLimit  int64
	GeneralWindow time.Duration
	IPLimit       int64
	IPWindow      time.Duration
}

// DefaultRateLimitConfig returns standard default rate limits.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  60,
		GeneralWindow: time.Minute,
		IPLimit:       300,
		IPWindow:      time.Minute,
	}
}

// AdminPlayerService defines administrative player management operations.
type AdminPlayerService interface {
	ListPlayers(ctx context.Context, sort string) ([]coreplayer.Player, error)
	BanPlayer(ctx context.Context, playerID string) error
}

// PlayerService defines the player account operations exposed over HTTP.
type PlayerService interface {
	AdminPlayerService
	Register(ctx context.Context, username, password string) (coreplayer.Player, error)
	Login(ctx context.Context, username, password string, clientIP ...string) (coreplayer.Session, error)
	Logout(ctx context.Context, sessionID string) error
	Authenticate(ctx context.Context, sessionID string) (coreplayer.Player, error)
	DeleteAccount(ctx context.Context, playerID, password string) error
	CreateAPIToken(ctx context.Context, playerID, name string, expiresAt *time.Time) (coreplayer.APIToken, string, error)
	ListAPITokens(ctx context.Context, playerID string) ([]coreplayer.APIToken, error)
	RevokeAPIToken(ctx context.Context, playerID, tokenID string) error
}

// CharacterService defines the character operations exposed over HTTP.
type CharacterService interface {
	Create(ctx context.Context, playerID string, name string) (corecharacter.Character, error)
	CreateWithOptions(ctx context.Context, playerID string, name string, options character.CreationOptions) (corecharacter.Character, error)
	Get(ctx context.Context, id string) (corecharacter.Character, error)
	ChangeName(ctx context.Context, characterID, newName string) (corecharacter.Character, error)
	ChangeGender(ctx context.Context, characterID, newGender string) (corecharacter.Character, error)
	GetProfile(ctx context.Context, characterID string) (character.ProfileView, error)
	UpdateProfile(ctx context.Context, characterID string, req character.UpdateProfileRequest) (character.Profile, error)
	GetNamingHallDialogue() character.NamingHallDialogue
	Delete(ctx context.Context, playerID, characterID string) error
}

// AdventureService defines the adventure operations exposed over HTTP.
type AdventureService interface {
	StartStage(ctx context.Context, characterID string, stageID string) (adventure.Adventure, error)
	Get(ctx context.Context, id string) (adventure.Adventure, error)
	ListHistory(ctx context.Context, characterID string, limit, offset int) (adventure.PaginatedAdventures, error)
	ListHistoryByCursor(ctx context.Context, characterID string, limit int, cursor string) (pagination.CursorPage[adventure.AdventureHistoryEntry], error)
	GetChronicle(ctx context.Context, characterID string) (adventure.AdventureChronicle, error)
}

// ShopService defines the shop operations exposed over HTTP.
type ShopService interface {
	PurchaseInShop(ctx context.Context, characterID string, shopType shop.ShopType, itemDefinitionID string, quantity int) (shop.PurchaseResult, error)
	Sell(ctx context.Context, characterID string, shopType shop.ShopType, itemInstanceID string, quantity int) (shop.SaleResult, error)
	GetCatalog(ctx context.Context, shopType shop.ShopType, characterID string) (shop.ShopCatalog, error)
	BatchPurchase(ctx context.Context, characterID string, shopType shop.ShopType, items []shop.BatchPurchaseItemRequest) (shop.BatchPurchaseResult, error)
	InspectNPC(ctx context.Context, shopType shop.ShopType, characterID string) (shop.NPCInspectResult, error)
	GetSaleOffer(ctx context.Context, shopType shop.ShopType, characterID string) (*shop.SaleOffer, error)
	Synthesize(ctx context.Context, characterID string, recipeTarget string) (shop.SynthesisResult, error)
}

// HelperService defines the helper quest operations exposed over HTTP.
type HelperService interface {
	ListQuests(ctx context.Context, now time.Time) ([]helperquest.Quest, error)
	CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (helperquest.CompletionResult, error)
}

// RescueService defines the emergency rescue operations exposed over HTTP.
type RescueService interface {
	EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error)
	IsUnderPenalty(ctx context.Context, characterID string, now time.Time) (bool, time.Duration, error)
	Unstuck(ctx context.Context, characterID string) error
}

// Handler holds all HTTP handlers for the game API.
type Handler struct {
	playerContext  *playercontext.Service
	sceneAdapters  map[string]sceneAdapter
	placeLog       PlaceLog
	actionCommands map[string]actionCommand
	players        PlayerService
	characters     CharacterService
	adventures     AdventureService
	shops          ShopService
	medals         MedalService
	park           ParkService
	helpers        HelperService
	rescues        RescueService
	notifications  NotificationService
	homes          HomeService
	rankings       RankingService
	jobs           JobService
	customSkills   CustomSkillService
	chapel         ChapelService
	exile          ExileService
	collections    CollectionService
	depot          DepotService
	lottery        LotteryService
	casino         CasinoService
	challenges     ChallengeService
	bosses         BossService
	dungeons       DungeonService
	pvp            PvPService
	gvg            GvGService
	guild          GuildService
	eventplaza     EventPlazaService
	secretshop     SecretShopService
	tavern         TavernService
	alchemy        AlchemyService
	plantation     PlantationService
	blackmarket    BlackMarketService
	fleamarket     FleaMarketService
	gemstore       GemStoreService
	stores         StoreService
	oracle         OracleShopService
	god            GodService
	monster        MonsterService
	contest        ContestService
	parties        PartyService
	altar          AltarService
	wishingWell    WishingWellService
	bank           BankService
	blacksmith     BlacksmithService
	replay         ReplayService
	maintenance    MaintenanceService
	limiter        RateLimiter
	rateLimitCfg   RateLimitConfig
	allowedOrigins map[string]struct{}
	trustedProxies []netip.Prefix
	adminAPIKey    string
}

// Option configures optional parameters for the Handler.
type Option func(*Handler)

// WithMaintenance configures the maintenance service for the Handler.
func WithMaintenance(maintenance MaintenanceService) Option {
	return func(h *Handler) {
		h.maintenance = maintenance
	}
}

// WithAdminAPIKey configures the secret API key required for administrative endpoints (e.g. POST /news, POST /rankings/refresh).
func WithAdminAPIKey(key string) Option {
	return func(h *Handler) {
		h.adminAPIKey = strings.TrimSpace(key)
	}
}

// WithAdminAPIKeyFromEnv loads the administrator API key from an environment variable (default: "PARTY2_ADMIN_API_KEY", falling back to "ADMIN_API_KEY").
func WithAdminAPIKeyFromEnv(envKey string) Option {
	return func(h *Handler) {
		if envKey != "" {
			//lint:ignore env-docs caller-supplied override; the default names below are documented
			if val := os.Getenv(envKey); val != "" {
				h.adminAPIKey = strings.TrimSpace(val)
				return
			}
		}
		if val := os.Getenv("PARTY2_ADMIN_API_KEY"); val != "" {
			h.adminAPIKey = strings.TrimSpace(val)
		} else if val := os.Getenv("ADMIN_API_KEY"); val != "" {
			h.adminAPIKey = strings.TrimSpace(val)
		}
	}
}

// WithRateLimiter configures rate limiting for the Handler.
func WithRateLimiter(limiter RateLimiter, cfg ...RateLimitConfig) Option {
	return func(h *Handler) {
		h.limiter = limiter
		if len(cfg) > 0 {
			h.rateLimitCfg = cfg[0]
		} else {
			h.rateLimitCfg = DefaultRateLimitConfig()
		}
	}
}

// WithHelper configures the helper quest service for the Handler.
func WithHelper(helpers HelperService) Option {
	return func(h *Handler) {
		h.helpers = helpers
	}
}

// WithAllowedOrigins configures the whitelist of allowed CORS origins.
// Any wildcard ("*") or empty entries are ignored/discarded.
func WithAllowedOrigins(origins ...string) Option {
	return func(h *Handler) {
		h.setAllowedOrigins(origins)
	}
}

// WithAllowedOriginsFromEnv loads allowed CORS origins from an environment variable (default: "PARTY2_CORS_ORIGINS").
func WithAllowedOriginsFromEnv(envKey string) Option {
	return func(h *Handler) {
		if envKey == "" {
			envKey = "PARTY2_CORS_ORIGINS"
		}
		raw := os.Getenv(envKey)
		if raw != "" {
			h.setAllowedOrigins(ParseCORSOrigins(raw))
		}
	}
}

// ParseCORSOrigins splits a comma-separated origins string, trimming whitespace and ignoring "*" and empty entries.
func ParseCORSOrigins(s string) []string {
	var origins []string
	for _, part := range strings.Split(s, ",") {
		origin := strings.TrimSpace(part)
		if origin != "" && origin != "*" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func (h *Handler) setAllowedOrigins(origins []string) {
	if h.allowedOrigins == nil {
		h.allowedOrigins = make(map[string]struct{})
	}
	for _, o := range origins {
		trimmed := strings.TrimSpace(o)
		if trimmed != "" && trimmed != "*" {
			h.allowedOrigins[trimmed] = struct{}{}
		}
	}
}

// NewHandler constructs an HTTP Handler with the required application services.
func NewHandler(
	players PlayerService,
	characters CharacterService,
	adventures AdventureService,
	shops ShopService,
	opts ...Option,
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
	h := &Handler{
		players:        players,
		characters:     characters,
		adventures:     adventures,
		shops:          shops,
		allowedOrigins: make(map[string]struct{}),
	}
	for _, opt := range append([]Option{adventureStartCommand(adventures), shopCommands()}, opts...) {
		if opt != nil {
			opt(h)
		}
	}
	return h, nil
}

// Router returns an http.Handler wired to all API endpoints with standard security headers and CORS policy applied.
func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("GET /openapi.json", h.handleOpenAPI)
	mux.HandleFunc("GET /maintenance", h.handleGetMaintenance)
	mux.HandleFunc("POST /admin/maintenance", h.handleAdminSetMaintenance)
	mux.HandleFunc("PUT /admin/maintenance", h.handleAdminSetMaintenance)
	mux.HandleFunc("GET /admin/players", h.handleAdminListPlayers)
	mux.HandleFunc("POST /admin/players/{id}/ban", h.handleAdminBanPlayer)
	mux.HandleFunc("POST /admin/characters/{id}/unstuck", h.handleAdminUnstuckCharacter)
	mux.HandleFunc("POST /admin/guilds/decay-points", h.handleAdminDecayGuildPoints)

	mux.HandleFunc("POST /players", h.handleRegisterPlayer)
	mux.HandleFunc("DELETE /players/me", h.handleDeletePlayerMe)
	mux.HandleFunc("DELETE /players/{id}", h.handleDeletePlayerByID)
	mux.HandleFunc("POST /sessions", h.handleLogin)
	mux.HandleFunc("DELETE /sessions", h.handleLogout)
	mux.HandleFunc("POST /player/tokens", h.handleCreateAPIToken)
	mux.HandleFunc("GET /player/tokens", h.handleListAPITokens)
	mux.HandleFunc("DELETE /player/tokens/{id}", h.handleRevokeAPIToken)

	mux.HandleFunc("POST /characters", h.handleCreateCharacter)
	mux.HandleFunc("GET /characters/{id}", h.handleGetCharacter)
	mux.HandleFunc("GET /api/v1/characters/{id}/context", h.handleGetPlayerContext)
	mux.HandleFunc("POST /api/v1/characters/{id}/actions", h.handleCharacterAction)
	mux.HandleFunc("DELETE /characters/{id}", h.handleDeleteCharacter)
	mux.HandleFunc("GET /characters/{id}/profile", h.handleGetCharacterProfile)
	mux.HandleFunc("POST /characters/{id}/profile", h.handleUpdateCharacterProfile)
	mux.HandleFunc("PUT /characters/{id}/profile", h.handleUpdateCharacterProfile)
	mux.HandleFunc("POST /characters/{id}/name", h.handleChangeCharacterName)
	mux.HandleFunc("POST /characters/{id}/gender", h.handleChangeCharacterGender)
	mux.HandleFunc("GET /naming-hall/dialogue", h.handleNamingHallDialogue)

	mux.HandleFunc("POST /adventures", h.handleStartAdventure)
	mux.HandleFunc("GET /characters/{id}/adventures", h.handleListCharacterAdventures)
	mux.HandleFunc("GET /characters/{id}/adventure-chronicle", h.handleGetAdventureChronicle)

	mux.HandleFunc("GET /park/posts", h.handleGetParkPosts)
	mux.HandleFunc("POST /park/posts", h.handlePostParkMessage)
	mux.HandleFunc("POST /park/npc/talk", h.handleParkNPCTalk)
	mux.HandleFunc("POST /park/npc/divinate", h.handleParkNPCDivinate)
	mux.HandleFunc("GET /park/npc/inspect", h.handleParkNPCInspect)

	mux.HandleFunc("GET /eventplaza", h.handleGetEventPlaza)
	mux.HandleFunc("POST /eventplaza/presence", h.handlePostEventPlazaPresence)
	mux.HandleFunc("GET /eventplaza/merchant/items", h.handleGetEventPlazaMerchantItems)
	mux.HandleFunc("POST /eventplaza/merchant/purchase", h.handlePostEventPlazaMerchantPurchase)

	mux.HandleFunc("GET /medals/rewards", h.handleGetMedalRewards)
	mux.HandleFunc("POST /medals/claim", h.handleClaimMedalReward)

	mux.HandleFunc("GET /helpers/quests", h.handleListHelperQuests)
	mux.HandleFunc("POST /helpers/complete", h.handleCompleteHelperQuest)

	mux.HandleFunc("GET /rescues/penalty", h.handleGetRescuePenalty)

	mux.HandleFunc("GET /news", h.handleListNews)
	mux.HandleFunc("GET /news/{id}", h.handleGetNews)
	mux.HandleFunc("POST /news", h.handleCreateNews)

	mux.HandleFunc("GET /notifications", h.handleListNotifications)
	mux.HandleFunc("GET /notifications/unread-count", h.handleGetUnreadNotificationCount)
	mux.HandleFunc("POST /notifications/{id}/read", h.handleMarkNotificationAsRead)
	mux.HandleFunc("POST /notifications/read-all", h.handleMarkAllNotificationsAsRead)
	mux.HandleFunc("DELETE /notifications/{id}", h.handleDeleteNotification)

	mux.HandleFunc("GET /homes/{id}", h.handleGetHomeView)
	mux.HandleFunc("POST /homes/{id}/settings", h.handleUpdateHomeSettings)
	mux.HandleFunc("GET /homes/{id}/companion/phrases", h.handleListCompanionPhrases)
	mux.HandleFunc("POST /homes/{id}/companion/phrases", h.handleTeachCompanionPhrase)
	mux.HandleFunc("DELETE /homes/{id}/companion/phrases/{phrase_id}", h.handleForgetCompanionPhrase)
	mux.HandleFunc("GET /homes/{id}/companion/talk", h.handleTalkToCompanion)
	mux.HandleFunc("GET /homes/{id}/notices", h.handleListDeliveryNotices)
	mux.HandleFunc("POST /homes/{id}/notices/clear", h.handleClearDeliveryNotices)
	mux.HandleFunc("POST /characters/{id}/home/sleep", h.handleHomeSleep)
	mux.HandleFunc("GET /characters/{id}/home/sleep", h.handleGetHomeSleep)
	mux.HandleFunc("POST /characters/{id}/home/wake", h.handleHomeWake)
	mux.HandleFunc("POST /towns/{town_id}/houses", h.handleBuildHouse)
	mux.HandleFunc("GET /towns/{town_id}/houses", h.handleListTownHouses)
	mux.HandleFunc("GET /houses/check", h.handleCheckHouse)
	mux.HandleFunc("POST /characters/{id}/color", h.handleSetCharacterColor)
	mux.HandleFunc("GET /characters/{id}/home/items", h.handleListHomeItems)
	mux.HandleFunc("POST /characters/{id}/home/items/use", h.handleUseHomeItem)

	mux.HandleFunc("POST /letters", h.handleSendLetter)
	mux.HandleFunc("GET /letters/inbox", h.handleListInbox)
	mux.HandleFunc("GET /letters/outbox", h.handleListOutbox)
	mux.HandleFunc("GET /letters/unread-count", h.handleGetUnreadLetterCount)
	mux.HandleFunc("POST /letters/{id}/read", h.handleReadLetter)
	mux.HandleFunc("DELETE /letters/{id}", h.handleDeleteLetter)

	mux.HandleFunc("GET /rankings/levels", h.handleGetLevelRanking)
	mux.HandleFunc("GET /rankings/wealth", h.handleGetPlayerWealthRanking)
	mux.HandleFunc("GET /rankings/characters-wealth", h.handleGetCharacterWealthRanking)
	mux.HandleFunc("GET /rankings/monster-kills", h.handleGetMonsterKillsRanking)
	mux.HandleFunc("GET /rankings/mao-count", h.handleGetMaoCountRanking)
	mux.HandleFunc("GET /rankings/hero-count", h.handleGetHeroCountRanking)
	mux.HandleFunc("GET /rankings/job-mastery", h.handleGetJobMasteryRanking)
	mux.HandleFunc("GET /rankings/job-popularity", h.handleGetJobPopularityRanking)
	mux.HandleFunc("GET /rankings/helpers", h.handleGetHelperRanking)
	mux.HandleFunc("GET /rankings/medals", h.handleGetSmallMedalRanking)
	mux.HandleFunc("GET /rankings/casino-wins", h.handleGetCasinoWinsRanking)
	mux.HandleFunc("GET /rankings/alchemy", h.handleGetAlchemyRanking)
	mux.HandleFunc("GET /rankings/weekly-job-change", h.handleGetWeeklyJobChangeRanking)
	mux.HandleFunc("GET /rankings/{type}", h.handleGetRankingByType)
	mux.HandleFunc("POST /rankings/refresh", h.handleRefreshRankings)

	// Hall of Fame (legend.cgi)
	mux.HandleFunc("GET /legends", h.handleGetLegends)
	mux.HandleFunc("GET /legends/{category}", h.handleGetLegendCategory)

	// Jobs
	mux.HandleFunc("GET /jobs", h.handleListJobs)
	mux.HandleFunc("GET /characters/{id}/job-mastery", h.handleGetCharacterJobMastery)
	mux.HandleFunc("POST /characters/{id}/change-job", h.handleChangeJob)
	mux.HandleFunc("POST /characters/{id}/exchange-job", h.handleExchangeJob)
	mux.HandleFunc("GET /characters/{id}/future-memories", h.handleListFutureMemories)
	mux.HandleFunc("POST /characters/{id}/future-memories", h.handleSaveFutureMemory)
	mux.HandleFunc("POST /characters/{id}/recall-future", h.handleRecallFutureMemory)

	// Custom Skills
	mux.HandleFunc("GET /characters/{id}/custom-skills", h.handleGetCustomSkills)
	mux.HandleFunc("POST /characters/{id}/custom-skills", h.handleSetCustomSkill)

	// Chapel

	// Altar of Rebirth
	mux.HandleFunc("GET /characters/{id}/altar", h.handleGetAltar)
	mux.HandleFunc("POST /characters/{id}/altar/pray", h.handleAltarPray)
	mux.HandleFunc("POST /characters/{id}/altar/wish", h.handleAltarWish)
	mux.HandleFunc("POST /characters/{id}/altar/offer", h.handleAltarOffer)

	// Wishing Well
	mux.HandleFunc("GET /characters/{id}/wishing-well", h.handleGetWishingWell)
	mux.HandleFunc("POST /characters/{id}/wishing-well/exchange", h.handleWishingWellExchange)

	// Tavern
	mux.HandleFunc("GET /tavern/menu", h.handleGetTavernMenu)
	mux.HandleFunc("GET /characters/{id}/tavern", h.handleGetCharacterTavernStatus)
	mux.HandleFunc("POST /characters/{id}/tavern/order", h.handleTavernOrder)
	mux.HandleFunc("POST /characters/{id}/tavern/delivery", h.handleTavernReserveDelivery)
	mux.HandleFunc("GET /characters/{id}/tavern/delivery", h.handleGetTavernDelivery)
	mux.HandleFunc("DELETE /characters/{id}/tavern/delivery", h.handleTavernCancelDelivery)
	mux.HandleFunc("POST /characters/{id}/tavern/talk", h.handleTavernTalk)

	// Plantation
	mux.HandleFunc("GET /characters/{id}/plantation", h.handleGetCharacterPlantation)
	mux.HandleFunc("POST /characters/{id}/plantation/sow", h.handlePlantationSow)
	mux.HandleFunc("POST /characters/{id}/plantation/fertilize", h.handlePlantationFertilize)
	mux.HandleFunc("POST /characters/{id}/plantation/harvest", h.handlePlantationHarvest)

	// Black Market

	// Collections
	mux.HandleFunc("GET /characters/{id}/collections/monsters", h.handleGetMonsterBook)
	mux.HandleFunc("GET /characters/{id}/collections/items", h.handleGetItemCollection)
	mux.HandleFunc("GET /characters/{id}/collections/weapons", h.handleGetWeaponCollection)
	mux.HandleFunc("GET /characters/{id}/collections/armors", h.handleGetArmorCollection)

	// Achievements & Commemorative Medals
	mux.HandleFunc("GET /characters/{id}/achievements", h.handleGetCharacterAchievements)
	mux.HandleFunc("POST /characters/{id}/achievements/{achievement_id}/claim", h.handleClaimAchievement)
	mux.HandleFunc("GET /characters/{id}/medals", h.handleGetCharacterMedals)

	// Lottery & Raffle
	mux.HandleFunc("GET /lottery/takarakuji", h.handleGetTakarakujiStatus)
	mux.HandleFunc("GET /characters/{id}/lottery/tickets", h.handleGetLotteryTickets)
	mux.HandleFunc("POST /characters/{id}/lottery/raffle", h.handlePlayRaffle)
	mux.HandleFunc("POST /characters/{id}/lottery/takarakuji/buy", h.handleBuyTakarakujiTicket)
	mux.HandleFunc("GET /characters/{id}/lottery/takarakuji/ticket", h.handleGetCharacterTakarakujiTicket)

	// Casino
	mux.HandleFunc("GET /characters/{id}/casino", h.handleGetCasinoAccount)
	mux.HandleFunc("POST /characters/{id}/casino/exchange", h.handleCasinoExchange)
	mux.HandleFunc("POST /characters/{id}/casino/slot", h.handleCasinoSlot)
	mux.HandleFunc("GET /casino/prizes", h.handleGetCasinoPrizes)
	mux.HandleFunc("POST /characters/{id}/casino/prizes/exchange", h.handleCasinoExchangePrize)
	mux.HandleFunc("GET /casino/rooms", h.handleListCasinoRooms)
	mux.HandleFunc("POST /characters/{id}/casino/rooms", h.handleCreateCasinoRoom)
	mux.HandleFunc("GET /casino/rooms/{roomId}", h.handleGetCasinoRoom)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/join", h.handleJoinCasinoRoom)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/spectate", h.handleSpectateCasinoRoom)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/leave", h.handleLeaveCasinoRoom)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/kick", h.handleKickCasinoRoomMember)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/start", h.handleStartCasinoRoomGame)
	mux.HandleFunc("POST /characters/{id}/casino/rooms/{roomId}/action", h.handlePlayCasinoRoomAction)

	// Combat & Challenges
	mux.HandleFunc("GET /challenges/tiers", h.handleListChallengeTiers)
	mux.HandleFunc("GET /challenges/hall-of-fame", h.handleListChallengeHallOfFame)
	mux.HandleFunc("GET /challenges/hall-of-fame/{tier_id}", h.handleGetChallengeHallOfFame)
	mux.HandleFunc("GET /characters/{id}/challenges/records", h.handleGetChallengeRecords)
	mux.HandleFunc("GET /characters/{id}/challenges/active", h.handleGetActiveChallenge)
	mux.HandleFunc("GET /characters/{id}/challenges/{session_id}/commands/{revision}", h.handleGetChallengeCommand)
	mux.HandleFunc("POST /characters/{id}/challenges/start", h.handleStartChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/party-start", h.handleStartPartyChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/advance", h.handleAdvanceChallenge)
	mux.HandleFunc("GET /characters/{id}/bosses", h.handleListBosses)
	mux.HandleFunc("POST /characters/{id}/bosses/fight", h.handleChallengeBoss)
	mux.HandleFunc("GET /characters/{id}/dungeons", h.handleListDungeons)
	mux.HandleFunc("POST /characters/{id}/dungeons/start", h.handleStartDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/party-start", h.handleStartPartyDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/move", h.handleMoveDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/fight", h.handleFightDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/escape", h.handleEscapeDungeon)
	// Colosseum PvP (Real-time 8-Player Bet & Split Rooms)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms", h.handleCreatePvPRoom)
	mux.HandleFunc("GET /pvp/rooms", h.handleListPvPRooms)
	mux.HandleFunc("GET /pvp/rooms/{room_id}", h.handleGetPvPRoom)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms/{room_id}/join", h.handleJoinPvPRoom)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms/{room_id}/leave", h.handleLeavePvPRoom)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms/{room_id}/team", h.handleSelectPvPTeam)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms/{room_id}/start", h.handleStartPvPMatch)
	mux.HandleFunc("POST /characters/{id}/pvp/rooms/{room_id}/advance", h.handleAdvancePvPRound)

	// Guild Battles (Live Multi-Round GvG Rooms)
	mux.HandleFunc("POST /characters/{id}/gvg/rooms", h.handleCreateGvGRoom)
	mux.HandleFunc("GET /gvg/rooms", h.handleListGvGRooms)
	mux.HandleFunc("GET /gvg/rooms/{room_id}", h.handleGetGvGRoom)
	mux.HandleFunc("POST /characters/{id}/gvg/rooms/{room_id}/join", h.handleJoinGvGRoom)
	mux.HandleFunc("POST /characters/{id}/gvg/rooms/{room_id}/leave", h.handleLeaveGvGRoom)
	mux.HandleFunc("POST /characters/{id}/gvg/rooms/{room_id}/start", h.handleStartGvGMatch)
	mux.HandleFunc("POST /characters/{id}/gvg/rooms/{room_id}/advance", h.handleAdvanceGvGRound)
	mux.HandleFunc("GET /gvg/standings/{guild_id}", h.handleGetGvGStanding)
	mux.HandleFunc("GET /gvg/leaderboard", h.handleGetGvGLeaderboard)

	// Battle Replays
	mux.HandleFunc("GET /replays/{id}", h.handleGetReplay)
	mux.HandleFunc("GET /replays/recent", h.handleGetRecentReplays)
	mux.HandleFunc("GET /characters/{id}/replays", h.handleGetCharacterReplays)

	// Guilds
	mux.HandleFunc("GET /guilds", h.handleListGuilds)
	mux.HandleFunc("POST /guilds", h.handleCreateGuild)
	mux.HandleFunc("GET /guilds/{id}", h.handleGetGuild)
	mux.HandleFunc("DELETE /guilds/{id}", h.handleDisbandGuild)
	mux.HandleFunc("POST /guilds/{id}/apply", h.handleApplyGuild)
	mux.HandleFunc("POST /guilds/{id}/applications/{applicant_id}/approve", h.handleApproveGuildApplication)
	mux.HandleFunc("POST /guilds/{id}/applications/{applicant_id}/reject", h.handleRejectGuildApplication)
	mux.HandleFunc("POST /guilds/{id}/callout", h.handleBroadcastGuildCallout)
	mux.HandleFunc("PUT /guilds/{id}/members/{char_id}/title", h.handleAssignGuildRoleTitle)
	mux.HandleFunc("PUT /guilds/{id}/customization", h.handleCustomizeGuild)
	mux.HandleFunc("POST /guilds/{id}/leave", h.handleLeaveGuild)
	mux.HandleFunc("DELETE /guilds/{id}/members/{char_id}", h.handleKickGuildMember)
	mux.HandleFunc("GET /characters/{id}/guild", h.handleGetCharacterGuild)

	// Oracle Shop (Town Boutique & Costumes)
	mux.HandleFunc("GET /characters/{id}/oracle", h.handleGetOracleStatus)
	mux.HandleFunc("POST /characters/{id}/oracle/talk", h.handleOracleTalk)
	mux.HandleFunc("POST /characters/{id}/oracle/inspect", h.handleOracleInspect)
	mux.HandleFunc("POST /characters/{id}/oracle/buy", h.handleOracleBuyCostume)
	mux.HandleFunc("POST /characters/{id}/oracle/wallpaper", h.handleOracleBuyWallpaper)
	mux.HandleFunc("POST /characters/{id}/oracle/blackmarket", h.handleOracleBlackMarket)

	// God (Endgame Wishes & Limit Breaks)
	mux.HandleFunc("GET /god/dialogue", h.handleGetGodDialogue)
	mux.HandleFunc("GET /characters/{id}/god/wishes", h.handleGetGodWishes)
	mux.HandleFunc("POST /characters/{id}/god/wish", h.handleGrantGodWish)

	// Monster Grandpa & Pet Companions
	mux.HandleFunc("GET /monster/dialogue", h.handleGetMonsterDialogue)
	mux.HandleFunc("GET /characters/{id}/monsters", h.handleGetCharacterMonsters)
	mux.HandleFunc("POST /characters/{id}/monsters/{instance_id}/bring-home", h.handleBringMonsterToHome)
	mux.HandleFunc("POST /characters/{id}/monsters/{instance_id}/deposit", h.handleDepositMonsterToBox)
	mux.HandleFunc("POST /characters/{id}/monsters/{instance_id}/rename", h.handleRenameMonster)
	mux.HandleFunc("POST /characters/{id}/monsters/{instance_id}/send", h.handleSendMonster)
	mux.HandleFunc("POST /characters/{id}/monsters/{instance_id}/release", h.handleReleaseMonster)

	// Photo Contest & Gallery
	mux.HandleFunc("GET /contest/venue", h.handleGetContestVenue)
	mux.HandleFunc("GET /contest/current", h.handleGetContestCurrent)
	mux.HandleFunc("GET /contest/past", h.handleGetContestPast)
	mux.HandleFunc("GET /contest/legends", h.handleGetContestLegends)
	mux.HandleFunc("POST /contest/settle", h.handleSettleContest)
	mux.HandleFunc("GET /characters/{id}/photos", h.handleGetCharacterPhotos)
	mux.HandleFunc("POST /characters/{id}/photos", h.handleSaveCharacterPhoto)
	mux.HandleFunc("DELETE /characters/{id}/photos/{photoId}", h.handleDeleteCharacterPhoto)
	mux.HandleFunc("POST /characters/{id}/contest/enter", h.handleEnterContest)
	mux.HandleFunc("POST /characters/{id}/contest/vote", h.handleVoteContest)

	// Multiplayer Party System (quest.cgi, party.cgi)
	mux.HandleFunc("GET /parties", h.handleListParties)
	mux.HandleFunc("POST /parties", h.handleCreateParty)
	mux.HandleFunc("GET /parties/{id}", h.handleGetParty)
	mux.HandleFunc("POST /parties/{id}/join", h.handleJoinParty)
	mux.HandleFunc("POST /parties/{id}/leave", h.handleLeaveParty)
	mux.HandleFunc("POST /parties/{id}/kick", h.handleKickPartyMember)
	mux.HandleFunc("DELETE /parties/{id}", h.handleDisbandParty)
	mux.HandleFunc("POST /parties/{id}/ready", h.handleSetPartyReady)
	mux.HandleFunc("POST /parties/{id}/start", h.handleStartPartyAdventure)
	mux.HandleFunc("POST /parties/{id}/sealing-battle", h.handleStartSealingBattle)

	return securityHeadersMiddleware(h.corsMiddleware(h.rateLimitMiddleware(h.maintenanceMiddleware(mux))))
}

// Health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Player / Session

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
		if errors.Is(err, coreplayer.ErrRegistrationLimitReached) {
			writeAppError(w, http.StatusConflict, "registration_limit_reached", "現在定員のため、新規登録は受け付けておりません")
			return
		}
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
	clientIP := ExtractClientIP(r, h.trustedProxies...)
	session, err := h.players.Login(r.Context(), req.Username, req.Password, clientIP)
	if err != nil {
		if errors.Is(err, coreplayer.ErrAuthentication) {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		if errors.Is(err, coreplayer.ErrPlayerBanned) {
			writeError(w, http.StatusForbidden, err)
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

type deletePlayerRequest struct {
	Password string `json:"password"`
}

func (h *Handler) handleDeletePlayerMe(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	var req deletePlayerRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}

	if err := h.players.DeleteAccount(r.Context(), player.ID, req.Password); err != nil {
		if errors.Is(err, coreplayer.ErrAuthentication) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid password"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   true,
		"player_id": player.ID,
	})
}

func (h *Handler) handleDeletePlayerByID(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	targetID := r.PathValue("id")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing player id"))
		return
	}

	if player.ID != targetID && !h.isAdminRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("forbidden: cannot delete another player"))
		return
	}

	var req deletePlayerRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}

	if err := h.players.DeleteAccount(r.Context(), targetID, req.Password); err != nil {
		if errors.Is(err, coreplayer.ErrAuthentication) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid password"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   true,
		"player_id": targetID,
	})
}

// Character

type createCharacterRequest struct {
	ReferrerCharacterID string `json:"referrer_character_id"`
	Name                string `json:"name"`
	JobID               string `json:"job_id"`
	Job                 string `json:"job"`
	Gender              string `json:"gender"`
	Sex                 string `json:"sex"`
}

type characterResponse struct {
	ID         string        `json:"id"`
	PlayerID   string        `json:"player_id"`
	Name       string        `json:"name"`
	JobID      string        `json:"job_id"`
	Gender     string        `json:"gender"`
	Level      int           `json:"level"`
	Experience int           `json:"experience"`
	Money      int           `json:"money"`
	SP         int           `json:"sp"`
	Stats      statsResponse `json:"stats"`
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
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}
	var req createCharacterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	char, err := h.characters.CreateWithOptions(r.Context(), player.ID, req.Name, character.CreationOptions{
		JobID:               cmp.Or(req.JobID, req.Job),
		Gender:              cmp.Or(req.Gender, req.Sex),
		ReferrerCharacterID: req.ReferrerCharacterID,
	})
	if err != nil {
		if errors.Is(err, character.ErrNameAlreadyTaken) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCharacterResponse(char))
}

func (h *Handler) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(_ coreplayer.Player, char corecharacter.Character) {
		writeJSON(w, http.StatusOK, toCharacterResponse(char))
	})
}

func (h *Handler) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(player coreplayer.Player, char corecharacter.Character) {
		if err := h.characters.Delete(r.Context(), player.ID, char.ID); err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"deleted":      true,
			"character_id": char.ID,
		})
	})
}

func toCharacterResponse(char corecharacter.Character) characterResponse {
	return characterResponse{
		ID:         char.ID,
		PlayerID:   char.PlayerID,
		Name:       char.Name,
		JobID:      char.JobID,
		Gender:     char.Gender,
		Level:      char.Level,
		Experience: char.Experience,
		Money:      char.Money,
		SP:         char.SP,
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
	PostAdventureMessages map[string]string `json:"post_adventure_messages,omitempty"`
	ID                    string            `json:"id"`
	CharacterID           string            `json:"character_id"`
	StageID               string            `json:"stage_id"`
	StartedAt             time.Time         `json:"started_at"`
	FloorsCleared         int               `json:"floors_cleared"`
	IsCleared             bool              `json:"is_cleared"`
	PartySize             int               `json:"party_size"`
	Resolved              bool              `json:"resolved"`
	ExperienceReward      int               `json:"experience_reward"`
}

func (h *Handler) handleStartAdventure(w http.ResponseWriter, r *http.Request) {
	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *startAdventureRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req startAdventureRequest) {
		adv, err := h.adventures.StartStage(r.Context(), char.ID, req.StageID)
		if err != nil {
			if errors.Is(err, adventure.ErrLevelRequirementNotMet) || errors.Is(err, adventure.ErrJobLevelRequirementNotMet) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusCreated, toAdventureResponse(adv))
	})
}

func toAdventureResponse(adv adventure.Adventure) adventureResponse {
	return adventureResponse{
		PostAdventureMessages: adv.PostAdventureMessages,
		ID:                    adv.ID,
		CharacterID:           adv.CharacterID,
		StageID:               adv.StageID,
		StartedAt:             adv.StartedAt,
		FloorsCleared:         adv.FloorsCleared,
		IsCleared:             adv.IsCleared,
		PartySize:             adv.PartySize,
		Resolved:              adv.Resolved,
		ExperienceReward:      adv.ExperienceReward,
	}
}

// -------------------------------------------------------------------
// Shop
// -------------------------------------------------------------------

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

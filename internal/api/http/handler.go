// Package http exposes game application services as HTTP JSON API endpoints.
// Handlers contain no domain business logic; they delegate strictly to
// application services injected at construction time.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helper"
	"github.com/witchcraze/party2re/internal/pagination"
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
}

// DefaultRateLimitConfig returns standard default rate limits.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  60,
		GeneralWindow: time.Minute,
	}
}

// PlayerService defines the player account operations exposed over HTTP.
type PlayerService interface {
	Register(ctx context.Context, username, password string) (coreplayer.Player, error)
	Login(ctx context.Context, username, password string) (coreplayer.Session, error)
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
	Get(ctx context.Context, id string) (corecharacter.Character, error)
	ChangeName(ctx context.Context, characterID, newName string) (corecharacter.Character, error)
	ChangeGender(ctx context.Context, characterID, newGender string) (corecharacter.Character, error)
	GetProfile(ctx context.Context, characterID string) (character.ProfileView, error)
	UpdateProfile(ctx context.Context, characterID string, req character.UpdateProfileRequest) (character.Profile, error)
	UploadAvatar(ctx context.Context, characterID string, filename string, contentType string, data []byte) (string, error)
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
	Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (shop.PurchaseResult, error)
	Sell(ctx context.Context, characterID string, itemInstanceID string, quantity int) (shop.SaleResult, error)
	GetCatalog(ctx context.Context, shopType shop.ShopType, characterID string) (shop.ShopCatalog, error)
	BatchPurchase(ctx context.Context, characterID string, shopType shop.ShopType, items []shop.BatchPurchaseItemRequest) (shop.BatchPurchaseResult, error)
	InspectNPC(ctx context.Context, shopType shop.ShopType, characterID string) (shop.NPCInspectResult, error)
	TalkNPC(ctx context.Context, shopType shop.ShopType) (string, error)
	DiscoverSecretShop(ctx context.Context, characterID string) (bool, string, error)
}

// HelperService defines the helper quest operations exposed over HTTP.
type HelperService interface {
	ListQuests(ctx context.Context, now time.Time) ([]helper.Quest, error)
	CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (helper.CompletionResult, error)
}

// RescueService defines the emergency rescue operations exposed over HTTP.
type RescueService interface {
	EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error)
	IsUnderPenalty(ctx context.Context, characterID string, now time.Time) (bool, time.Duration, error)
}

// Handler holds all HTTP handlers for the game API.
type Handler struct {
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
	collections    CollectionService
	depot          DepotService
	lottery        LotteryService
	casino         CasinoService
	challenges     ChallengeService
	bosses         BossService
	dungeons       DungeonService
	pvp            PvPService
	gvg            GvGService
	auctions       AuctionService
	eventplaza     EventPlazaService
	secretshop     SecretShopService
	tavern         TavernService
	alchemy        AlchemyService
	plantation     PlantationService
	blackmarket    BlackMarketService
	fleamarket     FleaMarketService
	gemstore       GemStoreService
	stores         StoreService
	god            GodService
	monster        MonsterService
	contest        ContestService
	parties        PartyService
	altar          AltarService
	wishingWell    WishingWellService
	bank           BankService
	maintenance    MaintenanceService
	limiter        RateLimiter
	rateLimitCfg   RateLimitConfig
	allowedOrigins map[string]struct{}
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

// WithRescue configures the emergency rescue service for the Handler.
func WithRescue(rescues RescueService) Option {
	return func(h *Handler) {
		h.rescues = rescues
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
	for _, opt := range opts {
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
	mux.HandleFunc("DELETE /characters/{id}", h.handleDeleteCharacter)
	mux.HandleFunc("GET /characters/{id}/profile", h.handleGetCharacterProfile)
	mux.HandleFunc("POST /characters/{id}/profile", h.handleUpdateCharacterProfile)
	mux.HandleFunc("PUT /characters/{id}/profile", h.handleUpdateCharacterProfile)
	mux.HandleFunc("POST /characters/{id}/name", h.handleChangeCharacterName)
	mux.HandleFunc("POST /characters/{id}/gender", h.handleChangeCharacterGender)
	mux.HandleFunc("POST /characters/{id}/avatar", h.handleUploadCharacterAvatar)
	mux.HandleFunc("GET /naming-hall/dialogue", h.handleNamingHallDialogue)

	mux.HandleFunc("POST /adventures", h.handleStartAdventure)
	mux.HandleFunc("GET /characters/{id}/adventures", h.handleListCharacterAdventures)
	mux.HandleFunc("GET /characters/{id}/adventure-chronicle", h.handleGetAdventureChronicle)

	mux.HandleFunc("POST /shop/purchase", h.handlePurchase)
	mux.HandleFunc("POST /shop/sell", h.handleSell)
	mux.HandleFunc("GET /characters/{id}/shop/{type}", h.handleGetShopCatalog)
	mux.HandleFunc("POST /characters/{id}/shop/batch-purchase", h.handleShopBatchPurchase)
	mux.HandleFunc("POST /characters/{id}/shop/{type}/inspect", h.handleShopInspectNPC)
	mux.HandleFunc("POST /characters/{id}/shop/{type}/talk", h.handleShopTalkNPC)
	mux.HandleFunc("POST /characters/{id}/shop/discover-secret", h.handleShopDiscoverSecret)

	mux.HandleFunc("GET /characters/{id}/depot", h.handleGetDepot)
	mux.HandleFunc("POST /characters/{id}/depot/deposit", h.handleDepositDepotItem)
	mux.HandleFunc("POST /characters/{id}/depot/withdraw", h.handleWithdrawDepotItem)
	mux.HandleFunc("POST /characters/{id}/depot/sell", h.handleSellDepotItem)
	mux.HandleFunc("POST /characters/{id}/depot/sell-batch", h.handleSellDepotBatch)
	mux.HandleFunc("POST /characters/{id}/depot/sort", h.handleSortDepot)
	mux.HandleFunc("POST /characters/{id}/depot/expand", h.handleExpandDepot)
	mux.HandleFunc("POST /characters/{id}/depot/send-money", h.handleDepotSendMoney)
	mux.HandleFunc("POST /characters/{id}/depot/send-item", h.handleDepotSendItem)

	if h.bank != nil {
		mux.HandleFunc("GET /characters/{id}/bank", h.handleGetBankState)
		mux.HandleFunc("POST /characters/{id}/bank/deposit", h.handleBankDeposit)
		mux.HandleFunc("POST /characters/{id}/bank/withdraw", h.handleBankWithdraw)
		mux.HandleFunc("POST /characters/{id}/bank/inspect", h.handleBankInspectNPC)
		mux.HandleFunc("POST /characters/{id}/bank/talk", h.handleBankTalkNPC)
	}

	mux.HandleFunc("GET /park/posts", h.handleGetParkPosts)
	mux.HandleFunc("POST /park/posts", h.handlePostParkMessage)
	mux.HandleFunc("POST /park/npc/talk", h.handleParkNPCTalk)
	mux.HandleFunc("POST /park/npc/divinate", h.handleParkNPCDivinate)
	mux.HandleFunc("GET /park/npc/inspect", h.handleParkNPCInspect)

	mux.HandleFunc("GET /eventplaza", h.handleGetEventPlaza)
	mux.HandleFunc("GET /eventplaza/merchant/items", h.handleGetEventPlazaMerchantItems)
	mux.HandleFunc("POST /eventplaza/merchant/purchase", h.handlePostEventPlazaMerchantPurchase)
	mux.HandleFunc("GET /eventplaza/banquets", h.handleGetEventPlazaBanquets)
	mux.HandleFunc("POST /eventplaza/banquets/{id}/toast", h.handlePostEventPlazaBanquetToast)

	mux.HandleFunc("GET /medals/rewards", h.handleGetMedalRewards)
	mux.HandleFunc("POST /medals/claim", h.handleClaimMedalReward)

	mux.HandleFunc("GET /helpers/quests", h.handleListHelperQuests)
	mux.HandleFunc("POST /helpers/complete", h.handleCompleteHelperQuest)

	mux.HandleFunc("GET /rescues/penalty", h.handleGetRescuePenalty)
	mux.HandleFunc("POST /rescues/request", h.handleRequestRescue)

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
	mux.HandleFunc("GET /rankings/battles", h.handleGetBattleRanking)
	mux.HandleFunc("GET /rankings/job-mastery", h.handleGetJobMasteryRanking)
	mux.HandleFunc("GET /rankings/job-popularity", h.handleGetJobPopularityRanking)
	mux.HandleFunc("GET /rankings/helpers", h.handleGetHelperRanking)
	mux.HandleFunc("GET /rankings/medals", h.handleGetSmallMedalRanking)
	mux.HandleFunc("GET /rankings/{type}", h.handleGetRankingByType)
	mux.HandleFunc("POST /rankings/refresh", h.handleRefreshRankings)

	// Jobs
	mux.HandleFunc("GET /jobs", h.handleListJobs)
	mux.HandleFunc("POST /characters/{id}/change-job", h.handleChangeJob)
	mux.HandleFunc("POST /characters/{id}/exchange-job", h.handleExchangeJob)
	mux.HandleFunc("GET /characters/{id}/future-memories", h.handleListFutureMemories)
	mux.HandleFunc("POST /characters/{id}/future-memories", h.handleSaveFutureMemory)
	mux.HandleFunc("POST /characters/{id}/recall-future", h.handleRecallFutureMemory)

	// Custom Skills
	mux.HandleFunc("GET /characters/{id}/custom-skills", h.handleGetCustomSkills)
	mux.HandleFunc("POST /characters/{id}/custom-skills", h.handleSetCustomSkill)

	// Chapel
	mux.HandleFunc("GET /characters/{id}/chapel", h.handleGetChapel)
	mux.HandleFunc("POST /characters/{id}/chapel/pray", h.handleChapelPray)

	// Altar of Rebirth
	mux.HandleFunc("GET /characters/{id}/altar", h.handleGetAltar)
	mux.HandleFunc("POST /characters/{id}/altar/pray", h.handleAltarPray)
	mux.HandleFunc("POST /characters/{id}/altar/wish", h.handleAltarWish)
	mux.HandleFunc("POST /characters/{id}/altar/offer", h.handleAltarOffer)

	// Wishing Well
	mux.HandleFunc("GET /characters/{id}/wishing-well", h.handleGetWishingWell)
	mux.HandleFunc("POST /characters/{id}/wishing-well/exchange", h.handleWishingWellExchange)

	// Secret Shop
	mux.HandleFunc("GET /characters/{id}/secretshop", h.handleGetSecretShop)
	mux.HandleFunc("POST /characters/{id}/secretshop/talk", h.handleSecretShopTalk)
	mux.HandleFunc("POST /characters/{id}/secretshop/inspect", h.handleSecretShopInspect)
	mux.HandleFunc("POST /characters/{id}/secretshop/puffpuff", h.handleSecretShopPuffPuff)
	mux.HandleFunc("POST /characters/{id}/secretshop/purchase", h.handleSecretShopPurchase)

	// Tavern
	mux.HandleFunc("GET /tavern/menu", h.handleGetTavernMenu)
	mux.HandleFunc("GET /characters/{id}/tavern", h.handleGetCharacterTavernStatus)
	mux.HandleFunc("POST /characters/{id}/tavern/order", h.handleTavernOrder)
	mux.HandleFunc("POST /characters/{id}/tavern/delivery", h.handleTavernReserveDelivery)
	mux.HandleFunc("GET /characters/{id}/tavern/delivery", h.handleGetTavernDelivery)
	mux.HandleFunc("DELETE /characters/{id}/tavern/delivery", h.handleTavernCancelDelivery)
	mux.HandleFunc("POST /characters/{id}/tavern/delivery/claim", h.handleTavernClaimDelivery)
	mux.HandleFunc("POST /characters/{id}/tavern/talk", h.handleTavernTalk)

	// Alchemy
	mux.HandleFunc("GET /characters/{id}/alchemy", h.handleGetCharacterAlchemy)
	mux.HandleFunc("POST /characters/{id}/alchemy/synthesize", h.handleAlchemySynthesize)
	mux.HandleFunc("POST /characters/{id}/alchemy/claim", h.handleAlchemyClaim)
	mux.HandleFunc("POST /characters/{id}/alchemy/learn", h.handleAlchemyLearn)

	// Plantation
	mux.HandleFunc("GET /characters/{id}/plantation", h.handleGetCharacterPlantation)
	mux.HandleFunc("POST /characters/{id}/plantation/sow", h.handlePlantationSow)
	mux.HandleFunc("POST /characters/{id}/plantation/fertilize", h.handlePlantationFertilize)
	mux.HandleFunc("POST /characters/{id}/plantation/harvest", h.handlePlantationHarvest)

	// Black Market
	mux.HandleFunc("GET /characters/{id}/blackmarket", h.handleGetBlackMarketStatus)
	mux.HandleFunc("POST /characters/{id}/blackmarket/talk", h.handleBlackMarketTalk)
	mux.HandleFunc("POST /characters/{id}/blackmarket/inspect", h.handleBlackMarketInspect)
	mux.HandleFunc("GET /characters/{id}/blackmarket/points", h.handleGetBlackMarketPoints)
	mux.HandleFunc("POST /characters/{id}/blackmarket/sacrifice", h.handleBlackMarketSacrifice)
	mux.HandleFunc("POST /characters/{id}/blackmarket/trade", h.handleBlackMarketTrade)

	// Collections
	mux.HandleFunc("GET /characters/{id}/collections/monsters", h.handleGetMonsterBook)
	mux.HandleFunc("GET /characters/{id}/collections/items", h.handleGetItemCollection)

	// Achievements & Commemorative Medals
	mux.HandleFunc("GET /characters/{id}/achievements", h.handleGetCharacterAchievements)
	mux.HandleFunc("POST /characters/{id}/achievements/{achievement_id}/claim", h.handleClaimAchievement)
	mux.HandleFunc("GET /characters/{id}/medals", h.handleGetCharacterMedals)

	// Lottery & Raffle
	mux.HandleFunc("GET /characters/{id}/lottery/tickets", h.handleGetLotteryTickets)
	mux.HandleFunc("POST /characters/{id}/lottery/buy-raffle", h.handleBuyRaffleTickets)
	mux.HandleFunc("POST /characters/{id}/lottery/raffle", h.handlePlayRaffle)
	mux.HandleFunc("POST /characters/{id}/lottery/buy-ticket", h.handleBuyLotteryTicket)
	mux.HandleFunc("POST /characters/{id}/lottery/claim", h.handleClaimLotteryTicket)

	// Casino
	mux.HandleFunc("GET /characters/{id}/casino", h.handleGetCasinoAccount)
	mux.HandleFunc("POST /characters/{id}/casino/exchange", h.handleCasinoExchange)
	mux.HandleFunc("POST /characters/{id}/casino/slot", h.handleCasinoSlot)
	mux.HandleFunc("POST /characters/{id}/casino/highlow", h.handleCasinoHighLow)
	mux.HandleFunc("POST /characters/{id}/casino/doppel", h.handleCasinoDoppel)
	mux.HandleFunc("POST /characters/{id}/casino/poker", h.handleCasinoPokerStart)
	mux.HandleFunc("GET /characters/{id}/casino/poker", h.handleGetCasinoPoker)
	mux.HandleFunc("POST /characters/{id}/casino/poker/action", h.handleCasinoPokerAction)

	// Combat & Challenges
	mux.HandleFunc("GET /challenges/tiers", h.handleListChallengeTiers)
	mux.HandleFunc("GET /challenges/hall-of-fame", h.handleListChallengeHallOfFame)
	mux.HandleFunc("GET /challenges/hall-of-fame/{tier_id}", h.handleGetChallengeHallOfFame)
	mux.HandleFunc("GET /characters/{id}/challenges/records", h.handleGetChallengeRecords)
	mux.HandleFunc("POST /characters/{id}/challenges/start", h.handleStartChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/party-start", h.handleStartPartyChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/advance", h.handleAdvanceChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/retire", h.handleRetireChallenge)
	mux.HandleFunc("GET /characters/{id}/bosses", h.handleListBosses)
	mux.HandleFunc("POST /characters/{id}/bosses/fight", h.handleChallengeBoss)
	mux.HandleFunc("GET /characters/{id}/dungeons", h.handleListDungeons)
	mux.HandleFunc("GET /characters/{id}/dungeons/map", h.handleGetDungeonMap)
	mux.HandleFunc("POST /characters/{id}/dungeons/start", h.handleStartDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/party-start", h.handleStartPartyDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/move", h.handleMoveDungeon)
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

	// Auction Hall (Live P2P Trading, Direct Send & Inspect)
	mux.HandleFunc("GET /auction/hall", h.handleAuctionVenueInfo)
	mux.HandleFunc("POST /characters/{id}/auction/send", h.handleAuctionSend)
	mux.HandleFunc("POST /auction/send", h.handleAuctionSendLegacy)
	mux.HandleFunc("GET /characters/{id}/auction/inspect", h.handleAuctionInspect)

	// Flea Market
	mux.HandleFunc("GET /fleamarket/listings", h.handleListFleaMarketListings)
	mux.HandleFunc("GET /fleamarket/listings/{listing_id}", h.handleGetFleaMarketListing)
	mux.HandleFunc("GET /characters/{id}/fleamarket/listings", h.handleGetCharacterFleaMarketListings)
	mux.HandleFunc("POST /characters/{id}/fleamarket/listings", h.handleCreateFleaMarketListing)
	mux.HandleFunc("POST /characters/{id}/fleamarket/listings/{listing_id}/purchase", h.handlePurchaseFleaMarketListing)
	mux.HandleFunc("DELETE /characters/{id}/fleamarket/listings/{listing_id}", h.handleCancelFleaMarketListing)

	// Gem Store
	mux.HandleFunc("GET /gemstore/catalog", h.handleGetGemStoreCatalog)
	mux.HandleFunc("GET /gemstore/recipes", h.handleGetGemStoreRecipes)
	mux.HandleFunc("GET /gemstore/dialogue", h.handleGetGemStoreDialogue)
	mux.HandleFunc("GET /characters/{id}/gembox", h.handleGetGemBox)
	mux.HandleFunc("POST /characters/{id}/gembox/sort", h.handleSortGemBox)
	mux.HandleFunc("POST /characters/{id}/gemstore/buy", h.handleGemStoreBuy)
	mux.HandleFunc("POST /characters/{id}/gemstore/sell", h.handleGemStoreSell)
	mux.HandleFunc("POST /characters/{id}/gemstore/send", h.handleGemStoreSend)
	mux.HandleFunc("POST /characters/{id}/gemstore/synthesize", h.handleGemStoreSynthesize)
	mux.HandleFunc("POST /characters/{id}/gemstore/appraise", h.handleGemStoreAppraise)

	// Player Store (Bazaar & Boutique)
	mux.HandleFunc("POST /towns/{town_id}/stores", h.handleBuildStore)
	mux.HandleFunc("GET /towns/{town_id}/stores", h.handleListTownStores)
	mux.HandleFunc("GET /stores/{store_id}", h.handleGetStore)
	mux.HandleFunc("GET /characters/{id}/store", h.handleCheckStore)
	mux.HandleFunc("POST /characters/{id}/store/listings/gold", h.handleListGoldItem)
	mux.HandleFunc("POST /characters/{id}/store/listings/barter", h.handleListBarterItem)
	mux.HandleFunc("DELETE /characters/{id}/store/listings/{sale_id}", h.handleWithdrawListing)
	mux.HandleFunc("POST /characters/{id}/store/sales/{sale_id}/buy", h.handleBuyStoreItem)
	mux.HandleFunc("POST /characters/{id}/store/sales/{sale_id}/trade", h.handleTradeStoreItem)
	mux.HandleFunc("POST /characters/{id}/store/name", h.handleChangeStoreName)
	mux.HandleFunc("POST /characters/{id}/store/wallpaper", h.handleChangeWallpaper)
	mux.HandleFunc("POST /characters/{id}/store/interiors", h.handleAddInterior)
	mux.HandleFunc("PUT /characters/{id}/store/interiors/{interior_id}/name", h.handleRenameInterior)
	mux.HandleFunc("DELETE /characters/{id}/store/interiors", h.handleCleanInteriors)

	// God (Endgame Wishes & Limit Breaks)
	mux.HandleFunc("GET /god/dialogue", h.handleGetGodDialogue)
	mux.HandleFunc("GET /characters/{id}/god/wishes", h.handleGetGodWishes)
	mux.HandleFunc("POST /characters/{id}/god/wish", h.handleGrantGodWish)

	// Monster Grandpa & Pet Companions
	mux.HandleFunc("GET /monster/dialogue", h.handleGetMonsterDialogue)
	mux.HandleFunc("GET /characters/{id}/monsters", h.handleGetCharacterMonsters)
	mux.HandleFunc("POST /characters/{id}/monsters/tame", h.handleTameMonster)
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

type deletePlayerRequest struct {
	Password string `json:"password"`
}

func (h *Handler) handleDeletePlayerMe(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	var req deletePlayerRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = decodeJSON(w, r, &req)
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
	if r.Body != nil && r.ContentLength > 0 {
		_ = decodeJSON(w, r, &req)
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

// -------------------------------------------------------------------
// Character
// -------------------------------------------------------------------

type createCharacterRequest struct {
	Name string `json:"name"`
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
	char, err := h.characters.Create(r.Context(), player.ID, req.Name)
	if err != nil {
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
	ID               string    `json:"id"`
	CharacterID      string    `json:"character_id"`
	StageID          string    `json:"stage_id"`
	StartedAt        time.Time `json:"started_at"`
	FloorsCleared    int       `json:"floors_cleared"`
	IsCleared        bool      `json:"is_cleared"`
	PartySize        int       `json:"party_size"`
	Resolved         bool      `json:"resolved"`
	ExperienceReward int       `json:"experience_reward"`
}

func (h *Handler) handleStartAdventure(w http.ResponseWriter, r *http.Request) {
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *startAdventureRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req startAdventureRequest) {
		if !h.ensureNotSleeping(w, r, char.ID) {
			return
		}
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
		ID:               adv.ID,
		CharacterID:      adv.CharacterID,
		StageID:          adv.StageID,
		StartedAt:        adv.StartedAt,
		FloorsCleared:    adv.FloorsCleared,
		IsCleared:        adv.IsCleared,
		PartySize:        adv.PartySize,
		Resolved:         adv.Resolved,
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
	CharacterID        string `json:"character_id"`
	ItemDefinitionID   string `json:"item_definition_id"`
	Quantity           int    `json:"quantity"`
	TotalCost          int    `json:"total_cost"`
	TransferredToDepot bool   `json:"transferred_to_depot"`
	NPCMessage         string `json:"npc_message,omitempty"`
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
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *purchaseRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req purchaseRequest) {
		result, err := h.shops.Purchase(r.Context(), char.ID, req.ItemDefinitionID, req.Quantity)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, purchaseResponse{
			CharacterID:        result.Character.ID,
			ItemDefinitionID:   result.ItemInstance.DefinitionID,
			Quantity:           result.ItemInstance.Quantity,
			TotalCost:          result.TotalPrice,
			TransferredToDepot: result.TransferredToDepot,
			NPCMessage:         result.NPCMessage,
		})
	})
}

func (h *Handler) handleSell(w http.ResponseWriter, r *http.Request) {
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *sellRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req sellRequest) {
		result, err := h.shops.Sell(r.Context(), char.ID, req.ItemInstanceID, req.Quantity)
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

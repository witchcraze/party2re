// Package http exposes game application services as HTTP JSON API endpoints.
// Handlers contain no domain business logic; they delegate strictly to
// application services injected at construction time.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
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
	Rebirth(ctx context.Context, id string) (corecharacter.Character, error)
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
	Claim(ctx context.Context, id string) (adventure.Adventure, error)
	Get(ctx context.Context, id string) (adventure.Adventure, error)
	ListHistory(ctx context.Context, characterID string, limit, offset int) (adventure.PaginatedAdventures, error)
	ListHistoryByCursor(ctx context.Context, characterID string, limit int, cursor string) (pagination.CursorPage[adventure.AdventureHistoryEntry], error)
	GetChronicle(ctx context.Context, characterID string) (adventure.AdventureChronicle, error)
}

// ShopService defines the shop operations exposed over HTTP.
type ShopService interface {
	Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (shop.PurchaseResult, error)
	Sell(ctx context.Context, characterID string, itemInstanceID string, quantity int) (shop.SaleResult, error)
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
	inn            InnService
	customSkills   CustomSkillService
	chapel         ChapelService
	farm           FarmService
	collections    CollectionService
	lottery        LotteryService
	casino         CasinoService
	challenges     ChallengeService
	bosses         BossService
	dungeons       DungeonService
	pvp            PvPService
	auctions       AuctionService
	eventplaza     EventPlazaService
	secretshop     SecretShopService
	tavern         TavernService
	blackmarket    BlackMarketService
	delivery       DeliveryService
	fleamarket     FleaMarketService
	gemstore       GemStoreService
	god            GodService
	monster        MonsterService
	contest        ContestService
	parties        PartyService
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
	adminKey := os.Getenv("PARTY2_ADMIN_API_KEY")
	if adminKey == "" {
		adminKey = os.Getenv("ADMIN_API_KEY")
	}
	h := &Handler{
		players:        players,
		characters:     characters,
		adventures:     adventures,
		shops:          shops,
		allowedOrigins: make(map[string]struct{}),
		adminAPIKey:    strings.TrimSpace(adminKey),
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
	mux.HandleFunc("POST /adventures/{id}/claim", h.handleClaimAdventure)
	mux.HandleFunc("GET /characters/{id}/adventures", h.handleListCharacterAdventures)
	mux.HandleFunc("GET /characters/{id}/adventure-chronicle", h.handleGetAdventureChronicle)

	mux.HandleFunc("POST /shop/purchase", h.handlePurchase)
	mux.HandleFunc("POST /shop/sell", h.handleSell)

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
	mux.HandleFunc("GET /rankings/rebirths", h.handleGetRebirthRanking)
	mux.HandleFunc("GET /rankings/medals", h.handleGetSmallMedalRanking)
	mux.HandleFunc("GET /rankings/{type}", h.handleGetRankingByType)
	mux.HandleFunc("POST /rankings/refresh", h.handleRefreshRankings)

	// Jobs, Rebirth & Inn
	mux.HandleFunc("GET /jobs", h.handleListJobs)
	mux.HandleFunc("POST /characters/{id}/change-job", h.handleChangeJob)
	mux.HandleFunc("POST /characters/{id}/rebirth", h.handleRebirth)
	mux.HandleFunc("POST /characters/{id}/inn", h.handleInnRest)

	// Custom Skills
	mux.HandleFunc("GET /characters/{id}/custom-skills", h.handleGetCustomSkills)
	mux.HandleFunc("POST /characters/{id}/custom-skills", h.handleEquipCustomSkill)
	mux.HandleFunc("DELETE /characters/{id}/custom-skills/{slot}", h.handleUnequipCustomSkill)

	// Chapel
	mux.HandleFunc("GET /characters/{id}/chapel", h.handleGetChapel)
	mux.HandleFunc("POST /characters/{id}/chapel/pray", h.handleChapelPray)
	mux.HandleFunc("POST /characters/{id}/chapel/donate", h.handleChapelDonate)

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

	// Black Market
	mux.HandleFunc("GET /characters/{id}/blackmarket", h.handleGetBlackMarketStatus)
	mux.HandleFunc("POST /characters/{id}/blackmarket/purchase", h.handleBlackMarketPurchase)
	mux.HandleFunc("POST /characters/{id}/blackmarket/sell", h.handleBlackMarketSell)
	mux.HandleFunc("POST /characters/{id}/blackmarket/talk", h.handleBlackMarketTalk)
	mux.HandleFunc("POST /characters/{id}/blackmarket/rumors", h.handleBlackMarketRumors)
	mux.HandleFunc("GET /characters/{id}/blackmarket/points", h.handleGetBlackMarketPoints)
	mux.HandleFunc("POST /characters/{id}/blackmarket/sacrifice", h.handleBlackMarketSacrifice)
	mux.HandleFunc("POST /characters/{id}/blackmarket/trade", h.handleBlackMarketTrade)

	// Delivery Quests & Courier Service
	mux.HandleFunc("GET /characters/{id}/delivery/quests", h.handleGetDeliveryQuests)
	mux.HandleFunc("GET /characters/{id}/delivery/active", h.handleGetActiveDeliveries)
	mux.HandleFunc("POST /characters/{id}/delivery/accept", h.handleAcceptDeliveryQuest)
	mux.HandleFunc("POST /characters/{id}/delivery/complete", h.handleCompleteDelivery)
	mux.HandleFunc("POST /characters/{id}/delivery/cancel", h.handleCancelDelivery)
	mux.HandleFunc("POST /characters/{id}/delivery/parcels/send", h.handleSendParcel)
	mux.HandleFunc("GET /characters/{id}/delivery/parcels/incoming", h.handleGetIncomingParcels)
	mux.HandleFunc("POST /characters/{id}/delivery/parcels/claim", h.handleClaimParcel)
	mux.HandleFunc("POST /characters/{id}/delivery/parcels/cancel", h.handleCancelParcel)

	// Farm
	mux.HandleFunc("GET /characters/{id}/farm", h.handleGetFarm)
	mux.HandleFunc("POST /characters/{id}/farm/plant", h.handleFarmPlant)
	mux.HandleFunc("POST /characters/{id}/farm/water", h.handleFarmWater)
	mux.HandleFunc("POST /characters/{id}/farm/fertilize", h.handleFarmFertilize)
	mux.HandleFunc("POST /characters/{id}/farm/harvest", h.handleFarmHarvest)
	mux.HandleFunc("POST /characters/{id}/farm/clear", h.handleFarmClear)

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
	mux.HandleFunc("GET /characters/{id}/challenges/records", h.handleGetChallengeRecords)
	mux.HandleFunc("POST /characters/{id}/challenges/start", h.handleStartChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/advance", h.handleAdvanceChallenge)
	mux.HandleFunc("POST /characters/{id}/challenges/retire", h.handleRetireChallenge)
	mux.HandleFunc("GET /characters/{id}/bosses", h.handleListBosses)
	mux.HandleFunc("POST /characters/{id}/bosses/fight", h.handleChallengeBoss)
	mux.HandleFunc("GET /characters/{id}/dungeons", h.handleListDungeons)
	mux.HandleFunc("POST /characters/{id}/dungeons/start", h.handleStartDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/move", h.handleMoveDungeon)
	mux.HandleFunc("POST /characters/{id}/dungeons/escape", h.handleEscapeDungeon)
	mux.HandleFunc("GET /characters/{id}/pvp", h.handleGetPvPRating)
	mux.HandleFunc("GET /characters/{id}/pvp/opponents", h.handleFindPvPOpponents)
	mux.HandleFunc("POST /characters/{id}/pvp/fight", h.handlePvPFight)

	// Auction House
	mux.HandleFunc("GET /auctions", h.handleListAuctions)
	mux.HandleFunc("GET /auctions/{id}", h.handleGetAuction)
	mux.HandleFunc("POST /auctions", h.handleCreateAuction)
	mux.HandleFunc("POST /auctions/{id}/bid", h.handleAuctionBid)
	mux.HandleFunc("POST /auctions/{id}/buyout", h.handleAuctionBuyout)
	mux.HandleFunc("POST /auctions/{id}/cancel", h.handleAuctionCancel)

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
	mux.HandleFunc("POST /characters/{id}/gemstore/buy", h.handleGemStoreBuy)
	mux.HandleFunc("POST /characters/{id}/gemstore/sell", h.handleGemStoreSell)
	mux.HandleFunc("POST /characters/{id}/gemstore/send", h.handleGemStoreSend)
	mux.HandleFunc("POST /characters/{id}/gemstore/synthesize", h.handleGemStoreSynthesize)
	mux.HandleFunc("POST /characters/{id}/gemstore/appraise", h.handleGemStoreAppraise)

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

	return securityHeadersMiddleware(h.corsMiddleware(h.rateLimitMiddleware(h.maintenanceMiddleware(mux))))
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware applies distributed / in-memory rate limiting to incoming HTTP requests.
func (h *Handler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractClientIP(r)
		var key string
		var limit int64
		var window time.Duration

		// Public registration/login endpoints
		if r.Method == http.MethodPost && (r.URL.Path == "/players" || r.URL.Path == "/sessions") {
			key = "http:public:" + ip + ":" + r.URL.Path
			limit = h.rateLimitCfg.PublicLimit
			window = h.rateLimitCfg.PublicWindow
		} else {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				sessionID := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				key = "http:auth:" + sessionID
			} else {
				key = "http:general:" + ip
			}
			limit = h.rateLimitCfg.GeneralLimit
			window = h.rateLimitCfg.GeneralWindow
		}

		if limit <= 0 || window <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		res, err := h.limiter.Allow(r.Context(), key, limit, window)
		if err != nil {
			// Fail-open gracefully on limiter error
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(res.ResetAfter.Seconds()), 10))

		if !res.Allowed {
			retrySec := int64(res.ResetAfter.Seconds())
			if retrySec < 1 {
				retrySec = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retrySec, 10))
			writeError(w, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers and preflight OPTIONS requests based on allowed origins.
func (h *Handler) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "*" {
			next.ServeHTTP(w, r)
			return
		}

		if _, allowed := h.allowedOrigins[origin]; allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware injects standard security response headers on every response.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
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
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *startAdventureRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req startAdventureRequest) {
		adv, err := h.adventures.StartStage(r.Context(), char.ID, req.StageID)
		if err != nil {
			if errors.Is(err, adventure.ErrLevelRequirementNotMet) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusCreated, toAdventureResponse(adv))
	})
}

func (h *Handler) handleClaimAdventure(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	advInfo, err := h.adventures.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, adventure.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if _, ok := h.authorizeCharacter(w, r, player.ID, advInfo.CharacterID); !ok {
		return
	}

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
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *purchaseRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req purchaseRequest) {
		result, err := h.shops.Purchase(r.Context(), char.ID, req.ItemDefinitionID, req.Quantity)
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

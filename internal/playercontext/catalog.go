package playercontext

// GateFlags represents bitmask flags for availability condition checks.
type GateFlags uint32

const (
	// GateDeadCheck requires the character to be alive (HP > 0).
	// Dead characters are blocked from combat, commerce, and general actions.
	GateDeadCheck GateFlags = 1 << iota

	// GateFatigueCheck requires the character to not be exhausted (Tired < 100).
	// Heavy physical actions such as combat and casino gambling are blocked at 100% fatigue.
	GateFatigueCheck

	// GateSleepCheck requires the character to be awake (not sleeping).
	// In legacy Party2 (party.cgi:14), characters with sleep > 0 are routed to sleep.cgi
	// and cannot execute town/facility actions until they wake up.
	GateSleepCheck

	// GateCooldownCheck requires the character to have no pending scheduled actions
	// (e.g. active adventure or timed training).
	GateCooldownCheck

	// GateCurrencyCheck requires the character to have sufficient currency/gold to participate.
	GateCurrencyCheck

	// GateLocationCheck requires the character to have access to the relevant facility/location.
	GateLocationCheck
)

// Has returns true if the specified flag is set on f.
func (f GateFlags) Has(flag GateFlags) bool {
	return (f & flag) == flag
}

// ActionDefinition represents the canonical static metadata for a top-level playable action.
type ActionDefinition struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Category       string    `json:"category"`
	OperationID    string    `json:"operation_id"`
	RequiredParams []string  `json:"required_params"`
	RequiredGates  GateFlags `json:"required_gates"`
}

// DefaultCatalog defines the canonical list of top-level game actions and their legacy preconditions.
// Granular item or skill selections are handled within individual endpoints/sub-commands.
var DefaultCatalog = []ActionDefinition{
	// =========================================================================
	// Category: adventure (Combat & Expeditions)
	// =========================================================================
	{
		ID:             "adventure_start",
		Label:          "冒険に出る",
		Category:       "adventure",
		OperationID:    "startAdventure",
		RequiredParams: []string{"stage_id"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "challenge_start",
		Label:          "試練の道（連戦チャレンジ）",
		Category:       "adventure",
		OperationID:    "startChallengeSession",
		RequiredParams: []string{"tier_id"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "dungeon_start",
		Label:          "ダンジョン探索",
		Category:       "adventure",
		OperationID:    "startDungeonExpedition",
		RequiredParams: []string{"dungeon_id"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "boss_fight",
		Label:          "ボス討伐",
		Category:       "adventure",
		OperationID:    "fightBoss",
		RequiredParams: []string{"boss_id"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "pvp_room_create",
		Label:          "対戦部屋作成",
		Category:       "adventure",
		OperationID:    "postCharactersIdPvpRooms",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: home (Rest & Recovery)
	// =========================================================================
	{
		ID:             "home_sleep",
		Label:          "自宅・宿屋で休む",
		Category:       "home",
		OperationID:    "homeSleep",
		RequiredParams: []string{},
		// Note: Sleeping is permitted when dead (sleep.cgi revives to full HP) and when fatigued.
		RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "home_wake",
		Label:          "目を覚ます",
		Category:       "home",
		OperationID:    "homeWake",
		RequiredParams: []string{},
		// Note: Wake requires an expired sleep timer and pending recovery; rescue is also exempt.
		RequiredGates: GateLocationCheck,
	},
	{
		ID:             "chapel_pray",
		Label:          "礼拝堂で祈る（祈願）",
		Category:       "home",
		OperationID:    "prayAtChapel",
		RequiredParams: []string{"blessing"},
		// Note: Daily blessing prayer is free and does not require wallet gold.
		RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: economy (Banking, Storage & Market)
	// =========================================================================
	{
		ID:             "bank_deposit",
		Label:          "銀行に預金する",
		Category:       "economy",
		OperationID:    "postCharactersIdBankDeposit",
		RequiredParams: []string{"amount"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "bank_withdraw",
		Label:          "銀行から引き出す",
		Category:       "economy",
		OperationID:    "postCharactersIdBankWithdraw",
		RequiredParams: []string{"amount"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "depot_deposit",
		Label:          "預かり所にアイテムを預ける",
		Category:       "economy",
		OperationID:    "postCharactersIdDepotDeposit",
		RequiredParams: []string{"item_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "depot_withdraw",
		Label:          "預かり所からアイテムを引き出す",
		Category:       "economy",
		OperationID:    "postCharactersIdDepotWithdraw",
		RequiredParams: []string{"item_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "auction_send",
		Label:          "仕送り・送金",
		Category:       "economy",
		OperationID:    "postCharactersIdAuctionSend",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "fleamarket_list",
		Label:          "フリーマーケット出品",
		Category:       "economy",
		OperationID:    "createFleaMarketListing",
		RequiredParams: []string{"item_id", "price"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "fleamarket_purchase",
		Label:          "フリーマーケット購入",
		Category:       "economy",
		OperationID:    "purchaseFleaMarketListing",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: shop (Commerce & Specialty Stores)
	// =========================================================================
	{
		ID:             "shop_purchase",
		Label:          "店でアイテム購入",
		Category:       "shop",
		OperationID:    "shopPurchase",
		RequiredParams: []string{"item_definition_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "shop_sell",
		Label:          "店にアイテム売却",
		Category:       "shop",
		OperationID:    "shopSell",
		RequiredParams: []string{"item_instance_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "shop_accessory_buy",
		Label:          "装飾品購入",
		Category:       "shop",
		OperationID:    "shopAccessoryBuy",
		RequiredParams: []string{"item_definition_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "gemstore_buy",
		Label:          "宝石購入",
		Category:       "shop",
		OperationID:    "buyGem",
		RequiredParams: []string{"gem_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "secretshop_purchase",
		Label:          "ヒミツの店で購入",
		Category:       "shop",
		OperationID:    "purchaseSecretShopItem",
		RequiredParams: []string{"item_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "blackmarket_trade",
		Label:          "闇市景品交換",
		Category:       "shop",
		OperationID:    "tradeBlackMarketPrize",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: crafting (Crafting, Synthesis & Cultivation)
	// =========================================================================
	{
		ID:             "blacksmith_seal",
		Label:          "鍛冶屋で刻印強化",
		Category:       "crafting",
		OperationID:    "postCharactersIdBlacksmithSeal",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "alchemy_synthesize",
		Label:          "錬金調合",
		Category:       "crafting",
		OperationID:    "postCharactersIdAlchemySynthesize",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "custom_skill_set",
		Label:          "カスタムスキル作成",
		Category:       "crafting",
		OperationID:    "setCustomSkill",
		RequiredParams: []string{"name"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "plantation_sow",
		Label:          "種菜園に種まき",
		Category:       "crafting",
		OperationID:    "postCharactersIdPlantationSow",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "plantation_harvest",
		Label:          "種菜園から収穫",
		Category:       "crafting",
		OperationID:    "postCharactersIdPlantationHarvest",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: entertainment (Casino, Lottery & Tavern)
	// =========================================================================
	{
		ID:             "casino_slot",
		Label:          "カジノスロット",
		Category:       "entertainment",
		OperationID:    "playCasinoSlot",
		RequiredParams: []string{"bet"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "casino_highlow",
		Label:          "ハイ＆ロー",
		Category:       "entertainment",
		OperationID:    "playCasinoHighLow",
		RequiredParams: []string{"bet", "guess"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "casino_doppel",
		Label:          "ドッペルゲンガー",
		Category:       "entertainment",
		OperationID:    "playCasinoDoppel",
		RequiredParams: []string{"bet", "pool_size", "player_mark"},
		RequiredGates:  GateDeadCheck | GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "lottery_raffle",
		Label:          "福引を引く",
		Category:       "entertainment",
		OperationID:    "playRaffle",
		RequiredParams: []string{"raffle_type"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "lottery_takarakuji_buy",
		Label:          "宝くじ購入",
		Category:       "entertainment",
		OperationID:    "buyTakarakujiTicket",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "tavern_order",
		Label:          "酒場で食事注文",
		Category:       "entertainment",
		OperationID:    "orderTavernMeal",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateCurrencyCheck | GateLocationCheck,
	},
	{
		ID:             "contest_enter",
		Label:          "フォトコンテスト応募",
		Category:       "entertainment",
		OperationID:    "enterContest",
		RequiredParams: []string{"photo_id", "title"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: growth (Sanctuary, Progression & Limit Break)
	// =========================================================================
	{
		ID:             "wishingwell_exchange",
		Label:          "願いの泉でSP交換",
		Category:       "growth",
		OperationID:    "exchangeWishingWellSP",
		RequiredParams: []string{"stat", "sp"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "altar_pray",
		Label:          "復活の祭壇（祈り）",
		Category:       "growth",
		OperationID:    "prayAltarRamia",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "god_wish",
		Label:          "神の願い・限界突破",
		Category:       "growth",
		OperationID:    "grantGodWish",
		RequiredParams: []string{"wish_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "job_change",
		Label:          "ダーマ神殿で転職",
		Category:       "growth",
		OperationID:    "changeCharacterJob",
		RequiredParams: []string{},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "medal_claim",
		Label:          "小さなメダル景品交換",
		Category:       "growth",
		OperationID:    "claimMedalReward",
		RequiredParams: []string{"reward_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},

	// =========================================================================
	// Category: social (Companions, Quests & Public Facilities)
	// =========================================================================
	{
		ID:             "monster_tame",
		Label:          "モンスター捕獲・預託",
		Category:       "social",
		OperationID:    "tameMonster",
		RequiredParams: []string{"monster_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "helper_complete",
		Label:          "何でも屋クエスト報告",
		Category:       "social",
		OperationID:    "completeHelperQuest",
		RequiredParams: []string{"quest_id"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "park_post",
		Label:          "交流広場に伝言投稿",
		Category:       "social",
		OperationID:    "postParkMessage",
		RequiredParams: []string{"message"},
		RequiredGates:  GateDeadCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck,
	},
	{
		ID:             "rescue_request",
		Label:          "緊急救出要請",
		Category:       "social",
		OperationID:    "requestEmergencyRescue",
		RequiredParams: []string{},
		// Note: Emergency rescue can be invoked while dead or asleep to unstick state.
		RequiredGates: 0,
	},
}

// AllActions returns a slice copy of the canonical default action catalog.
func AllActions() []ActionDefinition {
	actions := make([]ActionDefinition, len(DefaultCatalog))
	copy(actions, DefaultCatalog)
	return actions
}

// GetAction retrieves an ActionDefinition by its unique ID.
func GetAction(id string) (ActionDefinition, bool) {
	for _, act := range DefaultCatalog {
		if act.ID == id {
			return act, true
		}
	}
	return ActionDefinition{}, false
}

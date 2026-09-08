package god

import (
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// buildHeavenWishes constructs the list of heaven wishes based on character state, guild membership, and home members.
func buildHeavenWishes(char corecharacter.Character, inGuild bool, hasMaid bool) []Wish {
	wishes := []Wish{
		{
			ID:          WishStats,
			Name:        "強くなりたい",
			Realm:       RealmHeaven,
			Description: "全ステータス 40 アップ",
			Available:   !char.OverLevel,
		},
		{
			ID:          WishSP,
			Name:        "スキルを覚えたい",
			Realm:       RealmHeaven,
			Description: "Sp 50 アップ",
			Available:   true,
		},
		{
			ID:          WishMoney,
			Name:        "お金がほしい",
			Realm:       RealmHeaven,
			Description: "10 万G",
			Available:   true,
		},
		{
			ID:          WishCasinoCoins,
			Name:        "カジノコインがほしい",
			Realm:       RealmHeaven,
			Description: "5 万枚",
			Available:   true,
		},
		{
			ID:          WishSmallMedals,
			Name:        "小さなメダルがほしい",
			Realm:       RealmHeaven,
			Description: "20 枚",
			Available:   true,
		},
		{
			ID:          WishLotteryTickets,
			Name:        "福引券がほしい",
			Realm:       RealmHeaven,
			Description: "1000 枚",
			Available:   true,
		},
		{
			ID:          WishGuildRank,
			Name:        "ギルドランクをあげたい",
			Realm:       RealmHeaven,
			Description: "1000 ポイント",
			Available:   inGuild,
		},
		{
			ID:          WishGuildGorgeous,
			Name:        "ギルドをゴージャスにしたい",
			Realm:       RealmHeaven,
			Description: "ギルドが…",
			Available:   inGuild,
		},
		{
			ID:          WishRefresh,
			Name:        "元気いっぱいになりたい",
			Realm:       RealmHeaven,
			Description: "疲労度 -150 % (HP・MP完全回復)",
			Available:   true,
		},
		{
			ID:          WishAllOrbs,
			Name:        "新しい冒険場所に行きたい",
			Realm:       RealmHeaven,
			Description: "全オーブ",
			Available:   true,
		},
		{
			ID:          WishCelestialDragon,
			Name:        "天竜人になりたい",
			Realm:       RealmHeaven,
			Description: "転職 (空竜の民)",
			Available:   char.JobID != "job-70" && char.OldJobID != "job-70" && char.JobID != "70" && char.OldJobID != "70",
		},
		{
			ID:          WishGodOfNewWorld,
			Name:        "新世界の神になりたい",
			Realm:       RealmHeaven,
			Description: "自分の家が…",
			Available:   true,
		},
		{
			ID:          WishOrtega,
			Name:        "オルテガを生き返らして",
			Realm:       RealmHeaven,
			Description: "自分の家に…",
			Available:   true,
		},
		{
			ID:          WishCat,
			Name:        "猫を飼いたい",
			Realm:       RealmHeaven,
			Description: "自分の家に…",
			Available:   true,
		},
		{
			ID:          WishEroticBook,
			Name:        "エッチな本がほしい",
			Realm:       RealmHeaven,
			Description: "アイテム (預かり所へ送致)",
			Available:   true,
		},
		{
			ID:          WishAlchemyRecipe,
			Name:        "錬金レシピがほしい",
			Realm:       RealmHeaven,
			Description: "アイテム (預かり所へ送致)",
			Available:   true,
		},
		{
			ID:          WishLover,
			Name:        "素敵な恋人がほしい",
			Realm:       RealmHeaven,
			Description: "恋人が…？",
			Available:   true,
		},
		{
			ID:          WishSecretMaid,
			Name:        "メイドを雇いたい",
			Realm:       RealmHeaven,
			Description: "お世話係",
			Available:   !hasMaid,
		},
	}

	if char.Level >= 99 && !char.OverLevel {
		wishes = append(wishes, Wish{
			ID:          WishLimitBreakLevel,
			Name:        "もっと強くなりたい",
			Realm:       RealmHeaven,
			Description: "Lv上限を上げる",
			Available:   true,
		})
	} else if char.OverLevel {
		wishes = append(wishes, Wish{
			ID:          WishRestoreLevelLimit,
			Name:        "もとの強さに戻りたい",
			Realm:       RealmHeaven,
			Description: "Lv上限を元に戻す",
			Available:   true,
		})
	}

	return wishes
}

// buildUnderworldWishes constructs the list of underworld wishes.
func buildUnderworldWishes(char corecharacter.Character) []Wish {
	return []Wish{
		{
			ID:          WishExpandDepot,
			Name:        "もっとアイテムを預けたい",
			Realm:       RealmUnderworld,
			Description: "預かり所の上限アップ (+50枠)",
			Available:   char.OverDepot < MaxLimitBreakTier,
			CurrentTier: char.OverDepot,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          WishExpandMonster,
			Name:        "もっとモンスターを預けたい",
			Realm:       RealmUnderworld,
			Description: "モンスター預入上限アップ (+50枠)",
			Available:   char.OverMonster < MaxLimitBreakTier,
			CurrentTier: char.OverMonster,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          WishExpandJobMemory,
			Name:        "もっと職業を覚えたい",
			Realm:       RealmUnderworld,
			Description: "未来のカケラの記憶上限アップ (+1枠)",
			Available:   char.OverFuture < MaxLimitBreakTier,
			CurrentTier: char.OverFuture,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          WishExpandFleaMarket,
			Name:        "もっとフリーマーケットで出品したい",
			Realm:       RealmUnderworld,
			Description: "フリーマーケット出品数上限アップ (+1枠)",
			Available:   char.OverFlea < MaxLimitBreakTier,
			CurrentTier: char.OverFlea,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          WishExpandShopStore,
			Name:        "もっとお店で出品したい",
			Realm:       RealmUnderworld,
			Description: "お店の出品数上限アップ (+1枠)",
			Available:   char.OverStore < MaxLimitBreakTier,
			CurrentTier: char.OverStore,
			MaxTier:     MaxLimitBreakTier,
		},
	}
}

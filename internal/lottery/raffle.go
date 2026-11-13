package lottery

const (
	StandardRaffleCost = 3
	SpecialRaffleCost  = 300

	PrizeTierGrand = "GRAND_PRIZE"
	PrizeTier1st   = "1ST_PRIZE"
	PrizeTier2nd   = "2ND_PRIZE"
	PrizeTier3rd   = "3RD_PRIZE"
	PrizeTier4th   = "4TH_PRIZE"
	PrizeTier5th   = "5TH_PRIZE"
	PrizeTier6th   = "6TH_PRIZE"
	PrizeTierMiss  = "MISS"
)

type RaffleType string

const (
	RaffleStandard RaffleType = "STANDARD"
	RaffleSpecial  RaffleType = "SPECIAL"
)

type RafflePrize struct {
	Tier             string `json:"tier"`
	Name             string `json:"name"`
	Color            string `json:"color"`
	ColorName        string `json:"color_name"`
	ItemDefinitionID string `json:"item_definition_id,omitempty"`
}

type RaffleResult struct {
	RaffleType         RaffleType  `json:"raffle_type"`
	TicketsUsed        int         `json:"tickets_used"`
	Roll               int         `json:"roll"`
	Prize              RafflePrize `json:"prize"`
	TransferredToDepot bool        `json:"transferred_to_depot"`
}

type GrandPrizeItem struct {
	ItemDefinitionID string
	Name             string
}

// DayOfWeekGrandPrizes maps JST weekday (0=Sunday .. 6=Saturday) to the legacy grand prize item ($g_prizes[$wday]).
var DayOfWeekGrandPrizes = [7]GrandPrizeItem{
	0: {ItemDefinitionID: "item-027", Name: "賢者の悟り"},
	1: {ItemDefinitionID: "item-035", Name: "ドラゴンの心"},
	2: {ItemDefinitionID: "item-036", Name: "闇のロザリオ"},
	3: {ItemDefinitionID: "item-088", Name: "魔銃"},
	4: {ItemDefinitionID: "item-037", Name: "走鳥の野菜"},
	5: {ItemDefinitionID: "item-038", Name: "幻獣の実"},
	6: {ItemDefinitionID: "item-039", Name: "ギャンブルハート"},
}

// SpecialGrandPrizeItems contains the legacy special raffle materials (90..100, 142).
var SpecialGrandPrizeItems = []GrandPrizeItem{
	{ItemDefinitionID: "item-090", Name: "スライムピアス"},
	{ItemDefinitionID: "item-091", Name: "飛竜のヒゲ"},
	{ItemDefinitionID: "item-092", Name: "禁断の書"},
	{ItemDefinitionID: "item-093", Name: "コウモリの羽"},
	{ItemDefinitionID: "item-094", Name: "マジックマッシュルーム"},
	{ItemDefinitionID: "item-095", Name: "透明マント"},
	{ItemDefinitionID: "item-096", Name: "獣の血"},
	{ItemDefinitionID: "item-097", Name: "死者の骨"},
	{ItemDefinitionID: "item-098", Name: "謎の液体"},
	{ItemDefinitionID: "item-099", Name: "ヒーローソード"},
	{ItemDefinitionID: "item-100", Name: "ヒーローソード2"},
	{ItemDefinitionID: "item-142", Name: "蝶の翅"},
}

// EvaluateRaffleRoll deterministically returns the prize for a roll based on legacy lib/lot.cgi rules.
func EvaluateRaffleRoll(raffleType RaffleType, roll int, wday int, specialSubRoll ...int) RafflePrize {
	if raffleType == RaffleSpecial {
		switch {
		case roll < 3:
			idx := 0
			if len(specialSubRoll) > 0 && specialSubRoll[0] >= 0 {
				idx = specialSubRoll[0] % len(SpecialGrandPrizeItems)
			}
			item := SpecialGrandPrizeItems[idx]
			return RafflePrize{
				Tier:             PrizeTierGrand,
				Name:             item.Name,
				Color:            "gold",
				ColorName:        "ゴールド",
				ItemDefinitionID: item.ItemDefinitionID,
			}
		case roll < 15:
			return RafflePrize{
				Tier:             PrizeTier1st,
				Name:             "シルバーオーブ",
				Color:            "silver",
				ColorName:        "シルバー",
				ItemDefinitionID: "item-060",
			}
		case roll < 30:
			return RafflePrize{
				Tier:             PrizeTier2nd,
				Name:             "レッドオーブ",
				Color:            "red",
				ColorName:        "レッド",
				ItemDefinitionID: "item-061",
			}
		case roll < 40:
			return RafflePrize{
				Tier:             PrizeTier3rd,
				Name:             "ブルーオーブ",
				Color:            "blue",
				ColorName:        "ブルー",
				ItemDefinitionID: "item-062",
			}
		case roll < 50:
			return RafflePrize{
				Tier:             PrizeTier4th,
				Name:             "グリーンオーブ",
				Color:            "green",
				ColorName:        "グリーン",
				ItemDefinitionID: "item-063",
			}
		case roll < 60:
			return RafflePrize{
				Tier:             PrizeTier5th,
				Name:             "イエローオーブ",
				Color:            "yellow",
				ColorName:        "イエロー",
				ItemDefinitionID: "item-064",
			}
		case roll < 70:
			return RafflePrize{
				Tier:             PrizeTier6th,
				Name:             "パープルオーブ",
				Color:            "purple",
				ColorName:        "パープル",
				ItemDefinitionID: "item-065",
			}
		default:
			return RafflePrize{
				Tier:      PrizeTierMiss,
				Name:      "なし",
				Color:     "white",
				ColorName: "ホワイト",
			}
		}
	}

	// Standard Raffle (out of 1000)
	normalizedWday := wday % 7
	if normalizedWday < 0 {
		normalizedWday += 7
	}
	gPrize := DayOfWeekGrandPrizes[normalizedWday]

	switch {
	case roll < 1:
		return RafflePrize{
			Tier:             PrizeTierGrand,
			Name:             gPrize.Name,
			Color:            "#FFCC33",
			ColorName:        "金",
			ItemDefinitionID: gPrize.ItemDefinitionID,
		}
	case roll < 4:
		return RafflePrize{
			Tier:             PrizeTier1st,
			Name:             "精霊の守り",
			Color:            "#FF3333",
			ColorName:        "赤",
			ItemDefinitionID: "item-030",
		}
	case roll < 8:
		return RafflePrize{
			Tier:             PrizeTier2nd,
			Name:             "スライムの心",
			Color:            "#CC66FF",
			ColorName:        "紫",
			ItemDefinitionID: "item-033",
		}
	case roll < 14:
		return RafflePrize{
			Tier:             PrizeTier3rd,
			Name:             "小さなメダル",
			Color:            "#FFFF00",
			ColorName:        "黄",
			ItemDefinitionID: "item-023",
		}
	case roll < 20:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "命の木の実",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-016",
		}
	case roll < 25:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "不思議な木の実",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-017",
		}
	case roll < 30:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "力の種",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-018",
		}
	case roll < 35:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "守りの種",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-019",
		}
	case roll < 40:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "素早さの種",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-020",
		}
	case roll < 45:
		return RafflePrize{
			Tier:             PrizeTier4th,
			Name:             "スキルの種",
			Color:            "#FF33FF",
			ColorName:        "桃",
			ItemDefinitionID: "item-021",
		}
	case roll < 55:
		return RafflePrize{
			Tier:             PrizeTier5th,
			Name:             "祈りの指輪",
			Color:            "#6666FF",
			ColorName:        "青",
			ItemDefinitionID: "item-012",
		}
	case roll < 75:
		return RafflePrize{
			Tier:             PrizeTier6th,
			Name:             "福袋",
			Color:            "#33FF33",
			ColorName:        "緑",
			ItemDefinitionID: "item-125",
		}
	default:
		return RafflePrize{
			Tier:      PrizeTierMiss,
			Name:      "なし",
			Color:     "#FFFFFF",
			ColorName: "白",
		}
	}
}

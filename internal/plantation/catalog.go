package plantation

import (
	"strings"
)

var defaultSeeds = []Seed{
	{
		ID:       "red",
		Name:     "赤の種",
		Price:    50,
		HighBase: 16,
		HighRand: 5,
		LowBase:  2,
		LowRand:  2,
		HighRate: 25,
	},
	{
		ID:       "blue",
		Name:     "青の種",
		Price:    50,
		HighBase: 18,
		HighRand: 5,
		LowBase:  2,
		LowRand:  2,
		HighRate: 15,
	},
	{
		ID:       "yellow",
		Name:     "黄の種",
		Price:    80,
		HighBase: 16,
		HighRand: 2,
		LowBase:  1,
		LowRand:  3,
		HighRate: 33,
	},
	{
		ID:       "green",
		Name:     "緑の種",
		Price:    80,
		HighBase: 18,
		HighRand: 3,
		LowBase:  1,
		LowRand:  3,
		HighRate: 33,
	},
	{
		ID:       "silver",
		Name:     "銀の種",
		Price:    500,
		HighBase: 18,
		HighRand: 5,
		LowBase:  1,
		LowRand:  0,
		HighRate: 20,
	},
	{
		ID:       "gold",
		Name:     "金の種",
		Price:    500,
		HighBase: 21,
		HighRand: 2,
		LowBase:  1,
		LowRand:  0,
		HighRate: 18,
	},
}

var defaultFertilizers = []Fertilizer{
	{
		ID:         "none",
		Name:       "なし",
		Price:      0,
		ProbBonus:  0,
		WitherRate: 10,
		YieldBonus: 0,
		IsItem:     false,
	},
	{
		ID:         "rice_bran",
		Name:       "こめぬか",
		Price:      150,
		ProbBonus:  5,
		WitherRate: 8,
		YieldBonus: 0,
		IsItem:     false,
	},
	{
		ID:         "oil_cake",
		Name:       "あぶらかす",
		Price:      150,
		ProbBonus:  3,
		WitherRate: 2,
		YieldBonus: 0,
		IsItem:     false,
	},
	{
		ID:         "bone_meal",
		Name:       "骨粉",
		Price:      200,
		ProbBonus:  10,
		WitherRate: 10,
		YieldBonus: 1,
		IsItem:     false,
	},
	{
		ID:         "lime",
		Name:       "石灰",
		Price:      200,
		ProbBonus:  20,
		WitherRate: 25,
		YieldBonus: 1,
		IsItem:     false,
	},
	{
		ID:         "chemical",
		Name:       "化学肥料",
		Price:      500,
		ProbBonus:  30,
		WitherRate: 35,
		YieldBonus: 2,
		IsItem:     false,
	},
	{
		ID:         "yggdrasil_dew",
		Name:       "世界樹のしずく",
		Price:      0,
		ItemID:     "item-005",
		ProbBonus:  30,
		WitherRate: 10,
		YieldBonus: 5,
		IsItem:     true,
	},
	{
		ID:         "gysahl_greens",
		Name:       "ギザールの野菜",
		Price:      0,
		ItemID:     "item-037",
		ProbBonus:  35,
		WitherRate: 20,
		YieldBonus: 7,
		IsItem:     true,
	},
	{
		ID:         "kupo_nut",
		Name:       "クポの実",
		Price:      0,
		ItemID:     "item-038",
		ProbBonus:  50,
		WitherRate: 8,
		YieldBonus: 3,
		IsItem:     true,
	},
	{
		ID:         "gambler_heart",
		Name:       "ギャンブルハート",
		Price:      0,
		ItemID:     "item-039",
		ProbBonus:  50,
		WitherRate: 40,
		YieldBonus: 10,
		IsItem:     true,
	},
	{
		ID:         "magic_powder",
		Name:       "魔法の粉",
		Price:      0,
		ItemID:     "item-081",
		ProbBonus:  28,
		WitherRate: 0,
		YieldBonus: 2,
		IsItem:     true,
	},
	{
		ID:         "padekia_root",
		Name:       "パデキアの根っこ",
		Price:      0,
		ItemID:     "item-010",
		ProbBonus:  10,
		WitherRate: 25,
		YieldBonus: 5,
		IsItem:     true,
	},
	{
		ID:         "full_moon_herb",
		Name:       "満月草",
		Price:      0,
		ItemID:     "item-008",
		ProbBonus:  15,
		WitherRate: 8,
		YieldBonus: 2,
		IsItem:     true,
	},
	{
		ID:         "horse_dung",
		Name:       "馬のフン",
		Price:      0,
		ItemID:     "item-130",
		ProbBonus:  25,
		WitherRate: 5,
		YieldBonus: 3,
		IsItem:     true,
	},
	{
		ID:         "superb",
		Name:       "極上肥料",
		Price:      0,
		ItemID:     "item-182",
		ProbBonus:  60,
		WitherRate: 3,
		YieldBonus: 4,
		IsItem:     true,
	},
}

var DefaultLotusDialogues = []string{
	"種をまいて何ができるかはお楽しみ〜♪",
	"一日経つと収穫できるんだ",
	"肥料をまくと良いものが収穫できたり、たくさん芽が出るよ！",
	"肥料によっては枯れやすくなるかも…",
	"肥料に使えるアイテムもあるみたいだよ",
	"どの種をまこうか？",
	"種によって採れるものが変わるよ！",
	"今日は赤の種がおすすめだよ！",
	"肥料はこめぬかをまくのはどうかな？",
}

// AllSeeds returns a copy of all available seed definitions.
func AllSeeds() []Seed {
	res := make([]Seed, len(defaultSeeds))
	copy(res, defaultSeeds)
	return res
}

// AllFertilizers returns a copy of all available fertilizer definitions.
// Note: "none" is omitted from selectable fertilizers list for sowing/fertilizing.
func AllFertilizers() []Fertilizer {
	res := make([]Fertilizer, 0, len(defaultFertilizers)-1)
	for _, f := range defaultFertilizers {
		if f.ID == "none" {
			continue
		}
		res = append(res, f)
	}
	return res
}

// FindSeed finds a seed by ID or Name.
func FindSeed(idOrName string) (Seed, bool) {
	target := strings.TrimSpace(idOrName)
	for _, s := range defaultSeeds {
		if s.ID == target || s.Name == target {
			return s, true
		}
	}
	return Seed{}, false
}

// FindFertilizer finds a fertilizer by ID or Name.
func FindFertilizer(idOrName string) (Fertilizer, bool) {
	target := strings.TrimSpace(idOrName)
	for _, f := range defaultFertilizers {
		if f.ID == target || f.Name == target {
			return f, true
		}
	}
	return Fertilizer{}, false
}

// GetDefaultFertilizer returns the unfertilized default parameters ("none").
func GetDefaultFertilizer() Fertilizer {
	return defaultFertilizers[0]
}

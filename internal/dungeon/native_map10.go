package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap10ID is the three-floor sealed tower (legacy map/10).
const NativeMap10ID = "native-map-10"

// nativeMap10Floors transcribe map/10/1.cgi, _1.cgi and __1.cgi; F climbs to
// the next floor at the same coordinate.
var nativeMap10Floors = [][]string{
	{"0F0", "000", "0A0", "101", "101", "101", "1S1"},
	{"101", "101", "101", "101", "0B0", "000", "0F0"},
	{"234", "000", "0C0", "101", "101", "101", "101"},
}

// nm10Pool is _data.cgi's enemies with its appearance weights. Distinctive
// names are replaced (rule 00 §3), reusing the shared catalog's replacements.
var nm10Pool = EncounterPool{
	Monsters: []DungeonMonster{
		{ID: "nm10-purgatory-bird", Name: "業火の怪鳥", HP: 470, Attack: 320, Defense: 150, Agility: 600, ExpReward: 177, GoldReward: 72},
		{ID: "nm10-blizzer", Name: "ブリザー", HP: 400, Attack: 300, Defense: 210, Agility: 500, ExpReward: 146, GoldReward: 92},
		{ID: "nm10-death-envoy", Name: "死のつかい", HP: 380, Attack: 280, Defense: 180, Agility: 150, ExpReward: 126, GoldReward: 62},
		{ID: "nm10-kuro", Name: "クロ", HP: 396, Attack: 296, Defense: 196, Agility: 196, ExpReward: 196, GoldReward: 96},
		{ID: "nm10-flame-fiend", Name: "炎魔", HP: 400, Attack: 270, Defense: 150, Agility: 300, ExpReward: 126, GoldReward: 62},
		{ID: "nm10-vampire", Name: "バンパイア", HP: 499, Attack: 380, Defense: 160, Agility: 250, ExpReward: 146, GoldReward: 70},
		{ID: "nm10-noroi", Name: "ノロイ", HP: 410, Attack: 271, Defense: 160, Agility: 280, ExpReward: 162, GoldReward: 65},
		{ID: "nm10-death-scythe", Name: "デスサイズ", HP: 420, Attack: 440, Defense: 100, Agility: 400, ExpReward: 66, GoldReward: 242},
		{ID: "nm10-cycle", Name: "サイクル", HP: 400, Attack: 424, Defense: 150, Agility: 666, ExpReward: 164, GoldReward: 155},
		{ID: "nm10-abyss-colossus", Name: "深淵の巨魁", HP: 430, Attack: 280, Defense: 520, Agility: 180, ExpReward: 120, GoldReward: 500},
		{ID: "nm10-black-dragon", Name: "黒竜", HP: 500, Attack: 360, Defense: 240, Agility: 300, ExpReward: 175, GoldReward: 125},
		{ID: "nm10-death-eye", Name: "デスアイ", HP: 444, Attack: 300, Defense: 200, Agility: 350, ExpReward: 160, GoldReward: 90},
		{ID: "nm10-quicksilver-king", Name: "流銀スライムキング", HP: 22, Attack: 180, Defense: 5000, Agility: 2500, ExpReward: 3500, GoldReward: 100},
		{ID: "nm10-king-quicksilver", Name: "王様流銀スライム", HP: 25, Attack: 200, Defense: 6000, Agility: 3000, ExpReward: 4000, GoldReward: 100},
	},
	Appears: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
}

// Each floor file defines its own boss group; the strongest stands in for it.
var nm10Bosses = [3]DungeonMonster{
	strongest([]DungeonMonster{
		{ID: "nm10-dark-sorcerer", Name: "闇の魔術士", HP: 8000, Attack: 300, Defense: 100, Agility: 400, ExpReward: 750, GoldReward: 250},
		{ID: "nm10-mage", Name: "魔法使い", HP: 1200, Attack: 250, Defense: 80, Agility: 210, ExpReward: 150, GoldReward: 30},
		{ID: "nm10-mage-slime", Name: "メイジスライム", HP: 1300, Attack: 220, Defense: 50, Agility: 300, ExpReward: 160, GoldReward: 25},
	}),
	strongest([]DungeonMonster{
		{ID: "nm10-big-bomb-a", Name: "ビッグボムA", HP: 1200, Attack: 450, Defense: 120, Agility: 150, ExpReward: 255, GoldReward: 50},
		{ID: "nm10-fire-bird", Name: "火炎鳥", HP: 9800, Attack: 660, Defense: 210, Agility: 310, ExpReward: 1200, GoldReward: 300},
		{ID: "nm10-big-bomb-b", Name: "ビッグボムB", HP: 1200, Attack: 450, Defense: 120, Agility: 150, ExpReward: 255, GoldReward: 50},
	}),
	strongest([]DungeonMonster{
		{ID: "nm10-spirit", Name: "精霊", HP: 5000, Attack: 300, Defense: 200, Agility: 800, ExpReward: 500, GoldReward: 700},
		{ID: "nm10-end-beast", Name: "終焉の魔獣", HP: 8000, Attack: 400, Defense: 400, Agility: 999, ExpReward: 3000, GoldReward: 2500},
		{ID: "nm10-magic-orb", Name: "魔玉", HP: 5000, Attack: 300, Defense: 600, Agility: 900, ExpReward: 666, GoldReward: 666},
	}),
}

// Every event seals the party's attacks before its once guard, so a sealed
// party meets every encounter here. F also seals; native has no encounter to
// carry that to.
func nativeMap10Events() map[byte]mapEvent {
	seal := corebattle.StatusPhysicalSeal
	return map[byte]mapEvent{
		'0': {action: spawnEvent{status: seal}},
		'A': {action: bossEvent{status: seal}, perSymbol: true},
		'B': {action: bossEvent{status: seal}, perSymbol: true},
		'C': {action: bossEvent{status: seal}, perSymbol: true},
		'2': {action: treasureEvent{}, perSymbol: true},
		'3': {action: treasureEvent{}, perSymbol: true},
		'4': {action: treasureEvent{}, perSymbol: true},
	}
}

// nativeMap10 has round limit 30 on every floor and the menu threshold for
// dungeon 10 (job level 21). Only the first floor is an initial variant.
func nativeMap10() Dungeon {
	events := nativeMap10Events()
	pools := []EncounterPool{nm10Pool}
	floors := [3]Floor{
		{FloorNumber: 1, Marks: map[byte]string{'F': "凸", 'A': "◎"},
			Exits: map[byte]FloorExit{'F': {Variants: []int{2}}}, Bosses: map[byte]DungeonMonster{'A': nm10Bosses[0]}},
		{FloorNumber: 2, Marks: map[byte]string{'F': "凸", 'B': "◎"},
			Exits: map[byte]FloorExit{'F': {Variants: []int{3}}}, Bosses: map[byte]DungeonMonster{'B': nm10Bosses[1]}},
		{FloorNumber: 3, Marks: map[byte]string{'2': "宝", '3': "宝", '4': "宝", 'C': "◎"},
			Bosses: map[byte]DungeonMonster{'C': nm10Bosses[2]}},
	}
	variants := make([]Floor, len(floors))
	for i, f := range floors {
		f.Pools, f.events = pools, events
		variants[i] = gridFloor(f, nativeMap10Floors[i])
	}
	return Dungeon{
		ID:             NativeMap10ID,
		Name:           "封刃の魔塔",
		Description:    "刃を封じる呪いに満ちた三層の塔。頼れるのは魔法だけだ。",
		RoundLimit:     30,
		LeaderJobLevel: 21,
		Variants:       variants,
		Initial:        1,
	}
}

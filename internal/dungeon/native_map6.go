package dungeon

// nativeMap6ID is the choice corridor (legacy map/6, five initial variants).
// It stays out of DefaultDungeonCatalog until its gold outcomes land (#2417).
const nativeMap6ID = "native-map-6"

// nativeMap6Grids transcribe map/6/1.cgi–5.cgi with their legacy symbols.
var nativeMap6Grids = [5][]string{
	{"020", "1X1", "101", "000", "A1B", "050", "101", "101", "000", "C1D", "060", "101", "101", "000", "E1F", "070", "101", "1S1"},
	{"203", "1X1", "101", "000", "A1B", "050", "101", "101", "000", "C1D", "060", "101", "101", "000", "E1F", "070", "101", "1S1"},
	{"203", "1X1", "101", "000", "A1B", "050", "101", "101", "000", "11D", "060", "101", "101", "000", "E1F", "070", "101", "1S1"},
	{"203", "1X1", "101", "000", "A1B", "050", "101", "101", "000", "C11", "060", "101", "101", "000", "E1F", "070", "101", "1S1"},
	{"203", "1X1", "101", "000", "A1B", "050", "101", "101", "000", "C1D", "060", "101", "101", "000", "E1F", "070", "101", "1S1"},
}

// Map-local enemies (map/6/*.cgi get_monster_data*, get_boss_data*, _data.cgi
// event_X), with franchise names replaced as in internal/stagedata.
var (
	nm6SilverDrop = DungeonMonster{ID: "nm6-silver-drop", Name: "銀滴スライム", HP: 8, Attack: 70, Defense: 2500, Agility: 1500, ExpReward: 250, GoldReward: 10}
	nm6Slime      = DungeonMonster{ID: "nm6-slime", Name: "スライム", HP: 500, Attack: 200, Defense: 100, Agility: 500, ExpReward: 50, GoldReward: 30}
	nm6SilverKing = DungeonMonster{ID: "nm6-silver-king", Name: "王様流銀スライム", HP: 25, Attack: 200, Defense: 6000, Agility: 2000, ExpReward: 4000, GoldReward: 100}
	nm6KingSlime  = DungeonMonster{ID: "nm6-king-slime", Name: "キングスライム", HP: 2000, Attack: 250, Defense: 150, Agility: 120, ExpReward: 200, GoldReward: 200}
	nm6Chests     = strongest([]DungeonMonster{
		{ID: "nm6-man-eater", Name: "人食い箱", HP: 500, Attack: 250, Defense: 35, Agility: 400, ExpReward: 50, GoldReward: 100},
		{ID: "nm6-mimic", Name: "ミミック", HP: 700, Attack: 280, Defense: 65, Agility: 600, ExpReward: 60, GoldReward: 150},
		{ID: "nm6-pandora", Name: "パンドラの魔箱", HP: 900, Attack: 300, Defense: 95, Agility: 800, ExpReward: 100, GoldReward: 500},
	})
	nm6Hounds = strongest([]DungeonMonster{
		{ID: "nm6-pup-a", Name: "チビベロスA", HP: 160, Attack: 160, Defense: 90, Agility: 260, ExpReward: 32, GoldReward: 15},
		{ID: "nm6-cerberus", Name: "ケルベロス", HP: 300, Attack: 300, Defense: 100, Agility: 100, ExpReward: 48, GoldReward: 22},
		{ID: "nm6-pup-b", Name: "チビベロスB", HP: 160, Attack: 160, Defense: 90, Agility: 260, ExpReward: 32, GoldReward: 15},
	})
	nm6Panthers = strongest([]DungeonMonster{
		{ID: "nm6-cub-a", Name: "ベビーレオA", HP: 150, Attack: 180, Defense: 50, Agility: 300, ExpReward: 30, GoldReward: 18},
		{ID: "nm6-shadow-panther", Name: "シャドウパンサー", HP: 340, Attack: 300, Defense: 30, Agility: 120, ExpReward: 50, GoldReward: 20},
		{ID: "nm6-cub-b", Name: "ベビーレオB", HP: 150, Attack: 180, Defense: 50, Agility: 300, ExpReward: 30, GoldReward: 18},
	})
	// _data.cgi redefines every variant's event_X after it is compiled.
	nm6Sages = strongest([]DungeonMonster{
		{ID: "nm6-sage-1", Name: "一賢者", HP: 2000, Attack: 200, Defense: 60, Agility: 200, ExpReward: 600, GoldReward: 200},
		{ID: "nm6-sage-2", Name: "二賢者", HP: 2000, Attack: 200, Defense: 60, Agility: 200, ExpReward: 600, GoldReward: 200},
		{ID: "nm6-sage-3", Name: "三賢者", HP: 2000, Attack: 200, Defense: 60, Agility: 200, ExpReward: 600, GoldReward: 200},
	})
)

func fixedMonster(m DungeonMonster) monsterOutcome { return monsterOutcome{monster: &m} }

func fixedBoss(m DungeonMonster) bossOutcome { return bossOutcome{monster: &m} }

var (
	stageBoss    = bossOutcome{}
	stageMonster = monsterOutcome{}
	treasure     = treasureEvent{}
	rock         = trapEvent{base: 120}
	trap200      = trapEvent{base: 200, random: true}
	heal100      = healOutcome{base: 100}
)

const (
	goldGain = goldOutcome(1)
	goldLoss = goldOutcome(-1)
)

// pair binds one side of an exclusive pair; odds 0 always takes first.
func pair(flag string, odds int, first, second nativeEvent) mapEvent {
	return mapEvent{flag: flag, action: choiceEvent{odds: odds, outcomes: [2]nativeEvent{first, second}}}
}

// nativeMap6Choices are each variant's A–F events (map/6/<n>.cgi:35–45).
var nativeMap6Choices = [5]map[byte]mapEvent{
	{
		'A': pair("AB", 3, fixedMonster(nm6SilverDrop), fixedMonster(nm6Slime)),
		'B': pair("AB", 7, fixedMonster(nm6SilverKing), fixedMonster(nm6KingSlime)),
		'C': pair("CD", 2, trap200, goldGain),
		'D': pair("CD", 2, heal100, goldLoss),
		'E': pair("EF", 3, stageBoss, stageMonster),
		'F': pair("EF", 3, rock, nil),
	},
	{
		'A': pair("AB", 5, treasure, fixedBoss(nm6Chests)),
		'B': pair("AB", 3, stageBoss, stageMonster),
		'C': pair("CD", 0, stageBoss, nil),
		'D': pair("CD", 3, stageMonster, rock),
		'E': pair("EF", 0, fixedBoss(nm6Hounds), nil),
		'F': pair("EF", 0, fixedBoss(nm6Panthers), nil),
	},
	{
		'A': pair("AB", 0, stageBoss, nil),
		'B': pair("AB", 6, treasure, fixedBoss(nm6Chests)),
		'D': pair("CD", 0, nil, nil),
		'E': pair("EF", 2, stageBoss, stageMonster),
		'F': pair("EF", 2, rock, nil),
	},
	{
		'A': pair("AB", 0, stageBoss, nil),
		'B': pair("AB", 6, treasure, fixedBoss(nm6Chests)),
		'C': pair("CD", 0, nil, nil),
		'E': pair("EF", 2, stageBoss, stageMonster),
		'F': pair("EF", 2, rock, nil),
	},
	{
		'A': pair("AB", 3, fixedMonster(nm6SilverDrop), fixedMonster(nm6Slime)),
		'B': pair("AB", 7, fixedMonster(nm6SilverKing), fixedMonster(nm6KingSlime)),
		'C': pair("CD", 3, heal100, rock),
		'D': pair("CD", 3, trap200, heal100),
		'E': pair("EF", 5, treasure, fixedBoss(nm6Chests)),
		'F': pair("EF", 3, goldGain, goldLoss),
	},
}

// nativeMap6 has round limit 30 and the menu threshold for dungeon 6 (job
// level 13). Each event loads one of stages 7–11 (_data.cgi: int(rand(5) + 7)).
func nativeMap6() Dungeon {
	pools := stagePools(7, 11)
	variants := make([]Floor, len(nativeMap6Grids))
	for i, rows := range nativeMap6Grids {
		events := map[byte]mapEvent{
			'X': {action: bossEvent{}, perSymbol: true},
			'2': {action: treasureEvent{}, perSymbol: true},
			'3': {action: treasureEvent{}, perSymbol: true},
			'5': {action: messageEvent{}, perSymbol: true},
			'6': {action: messageEvent{}, perSymbol: true},
			'7': {action: messageEvent{}, perSymbol: true},
		}
		for sym, ev := range nativeMap6Choices[i] {
			events[sym] = ev
		}
		marks := map[byte]string{'X': "◎", '2': "宝", '3': "宝"}
		switch i {
		case 2:
			marks['D'] = "■" // the only route, shown as a wall until entered
		case 3:
			marks['C'] = "■"
		}
		variants[i] = gridFloor(Floor{FloorNumber: 1, Pools: pools, Marks: marks, Bosses: map[byte]DungeonMonster{'X': nm6Sages}, events: events}, rows)
	}
	return Dungeon{
		ID:             nativeMap6ID,
		Name:           "選択の回廊",
		Description:    "立て札の問いに答えて左右を選ぶ回廊。選ばなかった道は二度と開かない。",
		RoundLimit:     30,
		LeaderJobLevel: 13,
		Variants:       variants,
	}
}

package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap11ID is the boss corridor (legacy map/11, one layout).
const NativeMap11ID = "native-map-11"

// nativeMap11Grid transcribes map/11/1.cgi: six bosses guard the corridor,
// each flanked by H tables.
var nativeMap11Grid = []string{
	"234", "101", "101", "0A0", "101", "101", "HBH", "101", "101", "HCH", "101",
	"101", "HDH", "101", "101", "HEH", "101", "101", "HFH", "101", "101", "1S1",
}

// nm11Monsters are the map file's regular enemies, drawn uniformly by the base
// passage event. トンベリ is replaced (rule 00 §3).
var nm11Monsters = []DungeonMonster{
	{ID: "nm11-bomb-king", Name: "爆弾王", HP: 300, Attack: 500, Defense: 400, Agility: 50, ExpReward: 150, GoldReward: 30,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        corebattle.BattleJobs{JobID: "job-94", SP: 20},
		}),
	},
	{ID: "nm11-big-bomb", Name: "ビックボム", HP: 600, Attack: 400, Defense: 200, Agility: 200, ExpReward: 120, GoldReward: 50,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        corebattle.BattleJobs{JobID: "job-31", SP: 20},
		}),
	},
	{ID: "nm11-killer-bomb", Name: "キラーボム", HP: 250, Attack: 500, Defense: 250, Agility: 100, ExpReward: 110, GoldReward: 50,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        corebattle.BattleJobs{JobID: "job-94", SP: 10},
		}),
	},
	{ID: "nm11-tiny-bomb", Name: "チビボム", HP: 100, Attack: 50, Defense: 600, Agility: 900, ExpReward: 50, GoldReward: 1,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        corebattle.BattleJobs{JobID: "job-94", SP: 10},
			Temporary:   corebattle.TemporaryNullMagic,
		}),
	},
	{ID: "nm11-lantern-imp", Name: "ランタンの小鬼", HP: 300, Attack: 500, Defense: 200, Agility: 200, ExpReward: 180, GoldReward: 99,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 161},
			Jobs:        corebattle.BattleJobs{JobID: "job-100", SP: 999},
		}),
	},
}

// nm11BossA is get_boss_data's group.
var nm11BossA = []DungeonMonster{
	{ID: "nm11-death-flame-a", Name: "死の炎A", HP: 6666, Attack: 666, Defense: 222, Agility: 999, ExpReward: 333, GoldReward: 222,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 999},
			Jobs:          corebattle.BattleJobs{JobID: "job-93", SP: 999, OldJobID: "job-41", OldSP: 999},
			CrystalReward: 10,
			Temporary:     corebattle.TemporaryAbsorbMagic,
		}),
	},
	{ID: "nm11-lucifer", Name: "ルシファー", HP: 15000, Attack: 650, Defense: 200, Agility: 900, ExpReward: 2000, GoldReward: 500,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 99999, MaxMP: 9999999},
			Jobs:          corebattle.BattleJobs{JobID: "job-98", SP: 999},
			Hit:           400,
			CrystalReward: 40,
			Tension:       8,
		}),
	},
	{ID: "nm11-death-flame-b", Name: "死の炎B", HP: 6666, Attack: 666, Defense: 222, Agility: 999, ExpReward: 333, GoldReward: 222,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 999},
			Jobs:          corebattle.BattleJobs{JobID: "job-35", SP: 999, OldJobID: "job-51", OldSP: 999},
			CrystalReward: 10,
			Temporary:     corebattle.TemporaryAbsorbMagic,
		}),
	},
}

// H is map5's remedy table: only an acting job 43 spends the flag.
var nativeMap11Events = map[byte]mapEvent{
	'A': {action: bossEvent{}, perSymbol: true},
	'B': {action: bossEvent{}, perSymbol: true},
	'C': {action: bossEvent{}, perSymbol: true},
	'D': {action: bossEvent{}, perSymbol: true},
	'E': {action: bossEvent{}, perSymbol: true},
	'F': {action: bossEvent{}, perSymbol: true},
	'H': {action: remedyEvent{}, perSymbol: true},
	'2': {action: treasureEvent{}, perSymbol: true},
	'3': {action: treasureEvent{}, perSymbol: true},
	'4': {action: treasureEvent{}, perSymbol: true},
}

// nativeMap11 has round limit 30 and the menu threshold for dungeon 11 (job
// level 23). B–F load the boss of stages 20, 17, 11, 9 and 8.
func nativeMap11() Dungeon {
	bosses := map[byte][]DungeonMonster{'A': nm11BossA}
	for sym, stage := range map[byte]int{'B': 20, 'C': 17, 'D': 11, 'E': 9, 'F': 8} {
		bosses[sym] = stagePool(stage).Bosses
	}
	marks := map[byte]string{'2': "宝", '3': "宝", '4': "宝", 'A': "◎", 'B': "◎", 'C': "◎", 'D': "◎", 'E': "◎", 'F': "◎"}
	floor := gridFloor(Floor{Pools: []EncounterPool{{Monsters: nm11Monsters}}, Bosses: bosses, Marks: marks, events: nativeMap11Events}, nativeMap11Grid)
	return Dungeon{
		ID:             NativeMap11ID,
		Name:           "試練の回廊",
		Description:    "主たちが待ち構える長い回廊。脇の部屋には怪しげな薬と医学書が散らばっている。",
		RoundLimit:     30,
		LeaderJobLevel: 23,
		Variants:       []Floor{floor},
	}
}

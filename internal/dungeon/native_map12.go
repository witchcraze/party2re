package dungeon

import (
	"maps"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// NativeMap12ID is the second boss corridor (legacy map/12, one layout).
const NativeMap12ID = "native-map-12"

// map/12/1.cgi repeats map11's layout, regular enemies and H tables; only the
// bosses and treasure stages differ. 片翼の天使 is replaced (rule 00 §3).
var nm12Bosses = map[byte][]DungeonMonster{
	'A': { // get_boss_data2
		{ID: "nm12-fallen-angel", Name: "隻翼の堕天使", HP: 12000, Attack: 500, Defense: 300, Agility: 300, ExpReward: 3000, GoldReward: 2000,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 8000, MaxMP: 30000},
				Jobs:          corebattle.BattleJobs{JobID: "job-98", SP: 999, OldJobID: "job-48", OldSP: 999},
				Hit:           500,
				CrystalReward: 20,
				Temporary:     corebattle.TemporaryNullMagic,
			}),
		},
		{ID: "nm12-diabolos-a", Name: "ディアボロス", HP: 15000, Attack: 750, Defense: 300, Agility: 700, ExpReward: 5000, GoldReward: 1000,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 8000, MaxMP: 30000},
				Jobs:          corebattle.BattleJobs{JobID: "job-97", SP: 999, OldJobID: "job-38", OldSP: 999},
				Hit:           500,
				CrystalReward: 40,
				Tension:       8,
			}),
		},
		{ID: "nm12-bomber", Name: "ボマー", HP: 9000, Attack: 600, Defense: 500, Agility: 200, ExpReward: 1000, GoldReward: 1,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 999, Status: "大爆発"},
				Jobs:          corebattle.BattleJobs{JobID: "job-95", SP: 999, OldJobID: "job-08", OldSP: 999},
				Hit:           500,
				CrystalReward: 20,
				Temporary:     "するぞ",
			}),
		},
	},
	'B': { // get_boss_data
		{ID: "nm12-dragon-right-eye", Name: "竜の右目", HP: 6000, Attack: 500, Defense: 400, Agility: 900, ExpReward: 300, GoldReward: 300,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 999},
				Jobs:          corebattle.BattleJobs{JobID: "job-41", SP: 999, OldJobID: "job-90", OldSP: 999},
				CrystalReward: 10,
				Temporary:     corebattle.TemporaryDeflect,
			}),
		},
		{ID: "nm12-diabolos-b", Name: "ディアボロス", HP: 14000, Attack: 600, Defense: 200, Agility: 200, ExpReward: 2000, GoldReward: 500,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 8000, MaxMP: 30000},
				Jobs:          corebattle.BattleJobs{JobID: "job-97", SP: 999},
				Hit:           400,
				CrystalReward: 30,
				Temporary:     corebattle.TemporaryNullMagic,
			}),
		},
		{ID: "nm12-dragon-left-eye", Name: "竜の左目", HP: 6000, Attack: 500, Defense: 400, Agility: 900, ExpReward: 300, GoldReward: 300,
			combat: enemyCombat(corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 999},
				Jobs:          corebattle.BattleJobs{JobID: "job-41", SP: 999, OldJobID: "job-91", OldSP: 999},
				CrystalReward: 5,
				Temporary:     corebattle.TemporaryDeflect,
			}),
		},
	},
}

// nativeMap12 has round limit 30 and the menu threshold for dungeon 12 (job
// level 25). C–F load the boss of stages 13, 18, 12 and 10.
func nativeMap12() Dungeon {
	bosses := maps.Clone(nm12Bosses)
	for sym, stage := range map[byte]int{'C': 13, 'D': 18, 'E': 12, 'F': 10} {
		bosses[sym] = stagePool(stage).Bosses
	}
	marks := map[byte]string{'2': "宝", '3': "宝", '4': "宝", 'A': "◎", 'B': "◎", 'C': "◎", 'D': "◎", 'E': "◎", 'F': "◎"}
	floor := gridFloor(Floor{Pools: []EncounterPool{{Monsters: nm11Monsters}}, Bosses: bosses, Marks: marks, events: nativeMap11Events}, nativeMap11Grid)
	return Dungeon{
		ID:             NativeMap12ID,
		Name:           "深淵の回廊",
		Description:    "さらに強い主たちが待つ回廊。脇の部屋の薬と医学書は、ここでも読める者を待っている。",
		RoundLimit:     30,
		LeaderJobLevel: 25,
		Variants:       []Floor{floor},
	}
}

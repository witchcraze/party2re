package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap9ID is the sealed cave (legacy map/9, four initial variants).
const NativeMap9ID = "native-map-9"

// nativeMap9Grids transcribe map/9/1.cgi–4.cgi with their legacy symbols.
var nativeMap9Grids = [][]string{
	{"0100014", "0102010", "0100010", "011A110", "0000000", "1111101", "0301000", "0001000", "0B010C0", "1011101", "000S000"},
	{"0100014", "0102010", "0100010", "011A110", "0000000", "1011111", "0001030", "0001000", "0B010C0", "1011101", "000S000"},
	{"4100010", "0102010", "0100010", "011A110", "0000000", "1011111", "0001030", "0001000", "0B010C0", "1011101", "000S000"},
	{"4100010", "0102010", "0100010", "011A110", "0000000", "1111101", "0301000", "0001000", "0B010C0", "1011101", "000S000"},
}

// nm9Monsters are _data.cgi's regular enemies, drawn uniformly.
var nm9Monsters = []DungeonMonster{
	{ID: "nm9-behemoth-lord", Name: "ベヒーモスロード", HP: 600, Attack: 350, Defense: 200, Agility: 160, ExpReward: 150, GoldReward: 100,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 149, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-21", SP: 999, OldSP: 20},
		}),
	},
	{ID: "nm9-iclops", Name: "イクロプス", HP: 700, Attack: 500, Defense: 50, Agility: 100, ExpReward: 170, GoldReward: 10,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 999, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-01", SP: 999, OldJobID: "job-21", OldSP: 20},
		}),
	},
	{ID: "nm9-stone-golem", Name: "ストーンゴーレム", HP: 500, Attack: 350, Defense: 500, Agility: 100, ExpReward: 150, GoldReward: 250,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 155, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-03", SP: 999, OldSP: 30},
		}),
	},
	{ID: "nm9-mirror-knight", Name: "ミラーナイト", HP: 300, Attack: 300, Defense: 300, Agility: 300, ExpReward: 100, GoldReward: 100,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 94, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-36", SP: 999},
		}),
	},
	{ID: "nm9-battle-automaton", Name: "自動戦闘機械", HP: 444, Attack: 333, Defense: 66, Agility: 66, ExpReward: 96, GoldReward: 96,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 333, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-24", SP: 999, OldJobID: "job-11", OldSP: 999},
		}),
	},
	{ID: "nm9-dragon-fiend", Name: "竜魔人", HP: 480, Attack: 370, Defense: 80, Agility: 50, ExpReward: 145, GoldReward: 50,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 285, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-23", SP: 999, OldSP: 20},
		}),
	},
	{ID: "nm9-reaper-automaton", Name: "死神の魔導機械", HP: 555, Attack: 333, Defense: 111, Agility: 111, ExpReward: 111, GoldReward: 111,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 111, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-24", SP: 999, OldJobID: "job-11", OldSP: 999},
		}),
	},
	{ID: "nm9-white-knight", Name: "白騎士", HP: 400, Attack: 350, Defense: 300, Agility: 250, ExpReward: 150, GoldReward: 100,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 200, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-54", SP: 999, OldJobID: "job-17", OldSP: 999},
		}),
	},
	{ID: "nm9-phantom-knight", Name: "幻騎士", HP: 380, Attack: 370, Defense: 360, Agility: 350, ExpReward: 140, GoldReward: 130,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 320, Status: corebattle.StatusMagicSeal},
			Jobs:        corebattle.BattleJobs{JobID: "job-53", SP: 999, OldJobID: "job-49", OldSP: 999},
		}),
	},
}

// nm9Boss is _data.cgi's boss group.
var nm9Boss = []DungeonMonster{
	{ID: "nm9-cyclops-a", Name: "サイクロプスA", HP: 5000, Attack: 600, Defense: 50, Agility: 100, ExpReward: 500, GoldReward: 10,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 999},
			Jobs:          corebattle.BattleJobs{JobID: "job-01", SP: 999, OldJobID: "job-52", OldSP: 20},
			CrystalReward: 5,
			Temporary:     corebattle.TemporaryReflectAttack,
		}),
	},
	{ID: "nm9-knight-golem", Name: "ナイトゴーレム", HP: 8000, Attack: 560, Defense: 400, Agility: 200, ExpReward: 1600, GoldReward: 200,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 4999, MaxMP: 99999},
			Jobs:          corebattle.BattleJobs{JobID: "job-97", SP: 999, OldSP: 20},
			Hit:           500,
			CrystalReward: 13,
			Temporary:     corebattle.TemporaryNullAttack,
		}),
	},
	{ID: "nm9-cyclops-b", Name: "サイクロプスB", HP: 5000, Attack: 600, Defense: 50, Agility: 100, ExpReward: 500, GoldReward: 10,
		combat: enemyCombat(corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 999},
			Jobs:          corebattle.BattleJobs{JobID: "job-24", SP: 999, OldJobID: "job-21", OldSP: 20},
			CrystalReward: 5,
			Temporary:     corebattle.TemporaryReflectAttack,
		}),
	},
}

// Every event seals the party's magic before its once guard, so a sealed
// party meets every encounter here. B and C load one of stages 8–12
// (int(rand(5) + 8)) for their boss; A uses _data.cgi's.
func nativeMap9Events() map[byte]mapEvent {
	seal := corebattle.StatusMagicSeal
	stageBosses := stagePools(8, 12)
	return map[byte]mapEvent{
		'0': {action: spawnEvent{status: seal}},
		'A': {action: bossEvent{status: seal}, perSymbol: true},
		'B': {action: bossEvent{status: seal, pools: stageBosses}, perSymbol: true},
		'C': {action: bossEvent{status: seal, pools: stageBosses}, perSymbol: true},
		'2': {action: treasureEvent{}, perSymbol: true},
		'3': {action: treasureEvent{}, perSymbol: true},
		'4': {action: treasureEvent{}, perSymbol: true},
	}
}

// nativeMap9 has round limit 60 and the menu threshold for dungeon 9 (job level 19).
func nativeMap9() Dungeon {
	pools := []EncounterPool{{Monsters: nm9Monsters, Bosses: nm9Boss}}
	marks := map[byte]string{'2': "宝", '3': "宝", '4': "宝", 'A': "◎", 'B': "◎", 'C': "◎"}
	variants := variantFloors(Floor{Pools: pools, Marks: marks, events: nativeMap9Events()}, nativeMap9Grids)
	return Dungeon{
		ID:             NativeMap9ID,
		Name:           "封魔の石窟",
		Description:    "踏み入る者の魔力を封じる石窟。頼れるのは己の腕力だけだ。",
		RoundLimit:     60,
		LeaderJobLevel: 19,
		Variants:       variants,
	}
}

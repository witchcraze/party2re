package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

func nm16Enemy(id, name string, hp, at, df, ag, exp, gold int, a corebattle.LifetimeActor) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold, combat: enemyCombat(a)}
}

func nm16Jobs(job string, sp int, old string, oldSP int) corebattle.BattleJobs {
	return corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: old, OldSP: oldSP}
}

// nm16Crystal is the crystal rock that X and mining spawn through get_monster_data,
// worth one crystal on the main floor (1.cgi:85–104) and three on _1 (_1.cgi:126–145).
func nm16Crystal(crystals int) DungeonMonster {
	return nm16Enemy("nm16-crystal-rock", "クリスタルロック", 140, 50, 80, 70, 50, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 288},
		Jobs:          nm16Jobs("job-27", 40, "", 0),
		CrystalReward: crystals,
	})
}

var (
	nm16MainCrystal   = nm16Crystal(1)
	nm16SpringCrystal = nm16Crystal(3)
)

// nm16MainPool is _data.cgi's @monsters with @appears. Distinctive names are
// replaced with the shared catalog's replacements (rule 00 §3); its boss is
// never used.
var nm16MainPool = EncounterPool{
	Monsters: []DungeonMonster{
		nm16Enemy("nm16-night-bat", "ナイトバット", 15, 20, 0, 35, 5, 3, corebattle.LifetimeActor{}),
		nm16Enemy("nm16-mage-bat", "メイジバット", 54, 40, 20, 75, 14, 6, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 59}, Jobs: nm16Jobs("job-39", 4, "", 0)}),
		nm16Enemy("nm16-giant-bat", "ジャイアントバット", 180, 98, 10, 180, 40, 8, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 24}, Jobs: nm16Jobs("job-37", 30, "", 0)}),
		nm16Enemy("nm16-bomb-rock", "爆弾岩", 100, 86, 40, 10, 40, 5, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42}, Jobs: nm16Jobs("job-94", 20, "", 0)}),
		nm16Enemy("nm16-guardian-rock", "守護の岩石", 110, 75, 50, 44, 36, 10, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 62}, Jobs: nm16Jobs("job-03", 80, "", 30)}),
		nm16Enemy("nm16-ghost", "ゴースト", 20, 36, 20, 30, 7, 4, corebattle.LifetimeActor{}),
		nm16Enemy("nm16-mage-ghost", "メイジゴースト", 26, 32, 26, 36, 12, 6, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 35}, Jobs: nm16Jobs("job-06", 8, "", 0)}),
		nm16Enemy("nm16-night-wisp", "ナイトウィスプ", 25, 30, 25, 35, 9, 5, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 45}, Jobs: nm16Jobs("job-19", 4, "", 0)}),
		nm16MainCrystal,
	},
	Appears: []int{0, 0, 0, 0, 1, 1, 2, 3, 3, 4, 4, 5, 5, 5, 6, 6, 7, 7, 8},
}

// nm16Loadout is an NPC custom command from its source block's gems.
func nm16Loadout(source string, gems [3]string, costs [3]int) *corebattle.NPCLoadout {
	name := "custom"
	for _, g := range gems {
		if g != "" {
			name += "-" + g
		}
	}
	return &corebattle.NPCLoadout{Source: "ff97fc5:map/16/" + source, CustomName: name, Gems: gems, GemCosts: costs}
}

// nm16SpringPool is __data.cgi's @monsters with @appears, used on _1.
var nm16SpringPool = EncounterPool{
	Monsters: []DungeonMonster{
		nm16Enemy("nm16-black-dragon", "黒竜", 500, 360, 240, 300, 175, 125, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 149, CMP: 100},
			Jobs:        nm16Jobs("job-41", 999, "", 20),
			NPCLoadout: nm16Loadout("__data.cgi:87", [3]string{"gem_all_breath_1", "gem_all_breath_1", "gem_inflict_paralysis"},
				[3]int{28, 28, 6}),
		}),
		nm16Enemy("nm16-demon-shadow", "魔王の影", 200, 220, 300, 150, 80, 75, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 120, CMP: 50},
			Jobs:        nm16Jobs("job-35", 50, "", 0),
			NPCLoadout:  nm16Loadout("__data.cgi:108", [3]string{"gem_summon_sky", "gem_nullify_magic_2"}, [3]int{18, 19}),
		}),
		nm16Enemy("nm16-death-envoy", "死のつかい", 380, 280, 180, 150, 126, 62, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 144, CMP: 50},
			Jobs:        nm16Jobs("job-42", 999, "job-93", 999),
			NPCLoadout:  nm16Loadout("__data.cgi:127", [3]string{"gem_inflict_sleep", "gem_instant_death_sky"}, [3]int{6, 26}),
		}),
		nm16Enemy("nm16-dragon-zombie", "ドラゴンゾンビ", 650, 350, 260, 100, 155, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 92, CMP: 50},
			Jobs:        nm16Jobs("job-58", 999, "job-52", 999),
			Temporary:   corebattle.TemporaryRevival,
			NPCLoadout:  nm16Loadout("__data.cgi:148", [3]string{"gem_drain_hp_sky", "gem_inflict_poison"}, [3]int{26, 6}),
		}),
		nm16Enemy("nm16-stone-golem", "ストーンゴーレム", 300, 280, 400, 100, 100, 200, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 155, CMP: 60},
			Jobs:        nm16Jobs("job-03", 60, "", 30),
			NPCLoadout:  nm16Loadout("__data.cgi:170", [3]string{"gem_buff_def_1", "gem_big_guard_sky"}, [3]int{12, 37}),
		}),
		nm16Enemy("nm16-iclops", "イクロプス", 700, 500, 50, 50, 150, 10, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 99, CMP: 100},
			Jobs:        nm16Jobs("job-01", 999, "job-21", 20),
			NPCLoadout:  nm16Loadout("__data.cgi:190", [3]string{"gem_buff_atk_1", "gem_divine_tension_sky"}, [3]int{16, 57}),
		}),
		nm16SpringCrystal,
	},
	Appears: []int{0, 0, 0, 0, 1, 1, 1, 2, 3, 4, 4, 5, 5, 5, 6},
}

// nm16Knights is __data.cgi's @bosses, which Y spawns on a Sunday draw of 0
// because get_boss_data leaves them in place (_1.cgi:43,72–76).
var nm16Knights = func() []DungeonMonster {
	knight := func(letter, line string) DungeonMonster {
		return nm16Enemy("nm16-white-knight-"+letter, "白騎士"+letter, 1300, 330, 280, 250, 500, 150, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 800, CMP: 100},
			Jobs:        nm16Jobs("job-54", 999, "job-17", 999),
			Hit:         150,
			Temporary:   corebattle.TemporaryReflectMagic,
			NPCLoadout:  nm16Loadout("__data.cgi:"+line, [3]string{"gem_burst_sky"}, [3]int{46}),
		})
	}
	cavalier := nm16Enemy("nm16-platinum-cavalier", "プラチナキャバリア", 5000, 350, 330, 270, 600, 300, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 800, CMP: 200, ItemDefinitionIDs: []string{"item-246"}},
		Jobs:          nm16Jobs("job-23", 140, "job-48", 300),
		Hit:           150,
		CrystalReward: 5,
		Temporary:     corebattle.TemporaryReflectAttack,
		NPCLoadout:    nm16Loadout("__data.cgi:32", [3]string{"gem_combo_1", "gem_combo_burst_sky"}, [3]int{22, 46}),
	})
	return []DungeonMonster{knight("A", "10"), cavalier, knight("B", "58")}
}()

// nm16GemBosses are get_boss_data's six single rosters, keyed by kind
// (_1.cgi:147–294): each differs only in name, old job and held item.
var nm16GemBosses = func() map[string]DungeonMonster {
	out := make(map[string]DungeonMonster, len(nm16Kinds))
	for _, b := range []struct{ kind, id, name, oldJob, item string }{
		{"i", "gibeon", "ギベオン゛デーモン", "job-29", "item-162"},
		{"j", "ruby", "ルビーデーモン", "job-84", "item-189"},
		{"k", "sapphire", "サファイアデーモン", "job-46", "item-158"},
		{"l", "emerald", "エメラルドデーモン", "job-41", "item-158"},
		{"m", "topaz", "トパーズデーモン", "job-14", "item-190"},
		{"n", "amethyst", "アメジストデーモン", "job-58", "item-158"},
	} {
		out[b.kind] = nm16Enemy("nm16-"+b.id+"-demon", b.name, 6000, 380, 380, 380, 600, 300, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 8000, ItemDefinitionIDs: []string{b.item}},
			Jobs:          nm16Jobs("job-20", 999, b.oldJob, 999),
			CrystalReward: 5,
			Temporary:     corebattle.TemporaryNullMagic,
		})
	}
	return out
}()

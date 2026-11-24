package dungeon

import (
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// nm15Enemy builds a map-owned enemy from its legacy battle fields.
func nm15Enemy(id, name string, hp, at, df, ag, exp, gold int, a corebattle.LifetimeActor) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold, combat: enemyCombat(a)}
}

func nm15Jobs(job string, sp int, old string, oldSP int) corebattle.BattleJobs {
	return corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: old, OldSP: oldSP}
}

// nm15Pool is __data.cgi's @monsters with @appears; _1 and _2 use it too.
var nm15Pool = EncounterPool{
	Monsters: []DungeonMonster{
		nm15Enemy("nm15-private", "PR-4", 200, 180, 150, 97, 54, 20, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 140, CMP: 40},
			Jobs:        nm15Jobs("job-30", 999, "", 0),
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/__data.cgi:75", CustomName: "custom-gem_summon_sky",
				Gems: [3]string{"gem_summon_sky"}, GemCosts: [3]int{18}},
		}),
		nm15Enemy("nm15-roughneck", "RA-28", 480, 330, 160, 190, 98, 120, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 680, ItemDefinitionIDs: []string{"item-147"}},
			Jobs:        nm15Jobs("job-81", 999, "job-50", 999),
			Temporary:   corebattle.TemporaryNullMagic,
		}),
		nm15Enemy("nm15-sky-slaughter", "SK-51", 400, 400, 80, 380, 122, 36, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 300},
			Jobs:        nm15Jobs("job-502", 999, "job-52", 999),
		}),
		nm15Enemy("nm15-assailant", "PO-45", 500, 320, 250, 160, 100, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 421, CMP: 70},
			Jobs:        nm15Jobs("job-24", 999, "job-01", 999),
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/__data.cgi:127", CustomName: "custom-gem_buff_atk_1-gem_atk_2-gem_atk_2",
				Gems: [3]string{"gem_buff_atk_1", "gem_atk_2", "gem_atk_2"}, GemCosts: [3]int{16, 10, 10}},
		}),
		nm15Enemy("nm15-goost", "GA-67", 370, 480, 380, 350, 128, 77, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 555, ItemDefinitionIDs: []string{"item-193"}},
			Jobs:        nm15Jobs("job-85", 999, "job-20", 999),
			Temporary:   corebattle.TemporaryReduceAttack,
		}),
	},
	Appears: []int{0, 0, 0, 0, 1, 1, 1, 2, 2, 3, 3, 3, 4},
}

// nm15Letter names one of identical roster members by its source suffix.
func nm15Letter(m DungeonMonster, letter string) DungeonMonster {
	m.ID, m.Name = m.ID+"-"+strings.ToLower(letter), m.Name+letter
	return m
}

func nm15Trio(m DungeonMonster) []DungeonMonster {
	return []DungeonMonster{nm15Letter(m, "A"), nm15Letter(m, "B"), nm15Letter(m, "C")}
}

// nm15Flanked is a leader between identical A and B members.
func nm15Flanked(flank, leader DungeonMonster) []DungeonMonster {
	return []DungeonMonster{nm15Letter(flank, "A"), leader, nm15Letter(flank, "B")}
}

// nm15Main2 are the main floor's outcome 2 rosters (1.cgi:58–192): an
// inferno leader with blaze flanks, or the reverse.
var nm15Main2 = [2][]DungeonMonster{
	nm15Flanked(
		nm15Enemy("nm15-blaze", "BZ-13", 800, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:          nm15Jobs("job-54", 999, "job-47", 999),
			CrystalReward: 1,
		}),
		nm15Enemy("nm15-inferno", "IF-31", 1500, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 800, CMP: 100, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:          nm15Jobs("job-54", 999, "job-84", 999),
			Hit:           1000,
			CrystalReward: 3,
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/1.cgi:80", CustomName: "custom-gem_buff_atk_1-gem_sky_atk_2",
				Gems: [3]string{"gem_buff_atk_1", "gem_sky_atk_2"}, GemCosts: [3]int{16, 40}},
		})),
	nm15Flanked(
		nm15Enemy("nm15-inferno", "IF-31", 800, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:          nm15Jobs("job-54", 999, "job-84", 999),
			CrystalReward: 1,
		}),
		nm15Enemy("nm15-blaze", "BZ-13", 1500, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 400, CMP: 100, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:          nm15Jobs("job-54", 999, "job-47", 999),
			Hit:           1000,
			CrystalReward: 3,
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/1.cgi:148", CustomName: "custom-gem_buff_atk_1-gem_divine_tension_sky",
				Gems: [3]string{"gem_buff_atk_1", "gem_divine_tension_sky"}, GemCosts: [3]int{16, 57}},
		})),
}

// nm15Bombs is the main floor's outcome 3 (1.cgi:194–239); _1's outcome 6
// (_1.cgi:69–116) adds magic immunity.
var (
	nm15Bombs = nm15Trio(nm15Enemy("nm15-bomb", "チビボム", 100, 50, 600, 900, 50, 1, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 42},
		Jobs:        nm15Jobs("job-94", 10, "", 0),
	}))
	nm15ShieldedBombs = nm15Trio(nm15Enemy("nm15-shielded-bomb", "チビボム", 100, 50, 600, 900, 50, 1, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 42},
		Jobs:        nm15Jobs("job-94", 10, "", 0),
		Temporary:   corebattle.TemporaryNullMagic,
	}))
)

// nm15CyberArmor is CB-40 with the given HP and stolen item.
func nm15CyberArmor(id string, hp int, item string) DungeonMonster {
	return nm15Enemy(id, "CB-40", hp, 380, 400, 130, 500, 222, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 811, ItemDefinitionIDs: []string{item}},
		Jobs:          nm15Jobs("job-501", 50, "job-04", 999),
		CrystalReward: 3,
		Temporary:     corebattle.TemporaryReflectAttack,
	})
}

// nm15Shield is TR-7 with the given job (507 at __1's X, 3 at _2's Y).
func nm15Shield(id, job string) DungeonMonster {
	return nm15Enemy(id, "TR-7", 4000, 300, 600, 40, 700, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 999, ItemDefinitionIDs: []string{"item-162"}},
		Jobs:          nm15Jobs(job, 999, "job-21", 10),
		CrystalReward: 5,
	})
}

// nm15Hazard is _1's outcome 5 (__data.cgi's @bosses); nm15Core is __1's X
// (_data.cgi's @bosses).
var (
	nm15Hazard = nm15Flanked(nm15CyberArmor("nm15-cyber-armor", 2000, "item-162"),
		nm15Enemy("nm15-grand-hazard", "GN-2000", 8000, 430, 450, 260, 1800, 1800, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 2000, ItemDefinitionIDs: []string{"item-161"}},
			Jobs:          nm15Jobs("job-501", 999, "", 0),
			CrystalReward: 10,
			Temporary:     corebattle.TemporaryReflectAttack,
		}))
	nm15Core = nm15Flanked(nm15Shield("nm15-triangle-shield", "job-507"),
		nm15Enemy("nm15-core", "XS-0", 33000, 460, 500, 180, 8000, 8000, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 12000, MaxMP: 20000, ItemDefinitionIDs: []string{"item-208"}},
			Jobs:          nm15Jobs("job-503", 999, "", 0),
			CrystalReward: 17,
			Temporary:     corebattle.TemporaryNullAttack,
		}))
)

// nm15Private is PR-4 with its source letter and job (_2.cgi:192–275).
func nm15Private(letter, job string) DungeonMonster {
	return nm15Letter(nm15Enemy("nm15-private", "PR-4", 200, 180, 150, 97, 54, 20, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
		Jobs:        nm15Jobs(job, 999, "", 0),
	}), letter)
}

// _2's J/K groups 1–4 and Y's group 5 (_2.cgi:66–363); group 4 has one member.
var (
	nm15SouthGroups = [4][]DungeonMonster{
		nm15Flanked(
			nm15Enemy("nm15-gia-hollow", "GA-10", 20, 200, 6000, 1800, 90, 400, corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 999, ItemDefinitionIDs: []string{"item-208"}},
				Jobs:          nm15Jobs("job-85", 999, "job-20", 999),
				CrystalReward: 5,
				Temporary:     corebattle.TemporaryNullMagic,
			}),
			nm15Enemy("nm15-grand-hazard-2", "GN-2000", 10000, 500, 520, 260, 1800, 1800, corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 2000, ItemDefinitionIDs: []string{"item-159"}},
				Jobs:          nm15Jobs("job-501", 999, "", 0),
				CrystalReward: 10,
				Temporary:     corebattle.TemporaryReflectAttack,
			})),
		nm15Flanked(
			nm15Enemy("nm15-inferno-2", "IF-31", 1200, 300, 280, 250, 300, 100, corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
				Jobs:          nm15Jobs("job-54", 999, "job-84", 999),
				CrystalReward: 2,
			}),
			nm15Enemy("nm15-blaze-2", "BZ-13", 4000, 300, 280, 250, 300, 100, corebattle.LifetimeActor{
				Participant:   corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
				Jobs:          nm15Jobs("job-54", 999, "job-47", 999),
				Hit:           1000,
				CrystalReward: 4,
			})),
		{nm15Private("A", "job-30"), nm15Private("B", "job-31"), nm15Letter(nm15CyberArmor("nm15-cyber-armor-2", 4000, "item-192"), "B"),
			nm15Private("C", "job-16"), nm15Private("D", "job-15")},
		{nm15Enemy("nm15-sky-slaughter-2", "SK-51", 2000, 400, 80, 400, 200, 56, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 1200, ItemDefinitionIDs: []string{"item-183"}},
			Jobs:          nm15Jobs("job-502", 999, "job-52", 999),
			CrystalReward: 2,
		})},
	}
	nm15Panzer = nm15Flanked(nm15Shield("nm15-triangle-shield-2", "job-03"),
		nm15Enemy("nm15-devil-panzer", "EV-9", 15000, 540, 460, 200, 1800, 500, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 8000, MaxMP: 10000, ItemDefinitionIDs: []string{"item-204"}},
			Jobs:          nm15Jobs("job-503", 10, "job-501", 200),
			CrystalReward: 13,
		}))
)

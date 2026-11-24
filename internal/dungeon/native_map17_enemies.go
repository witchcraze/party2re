package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

func nm17Enemy(id, name string, hp, at, df, ag, exp, gold int, a corebattle.LifetimeActor) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold, combat: enemyCombat(a)}
}

func nm17Jobs(job string, sp int, old string, oldSP int) corebattle.BattleJobs {
	return corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: old, OldSP: oldSP}
}

// nm17Outer is __data.cgi's @monsters with @appears; distinctive names are
// replaced (rule 00 §3).
var nm17Outer = EncounterPool{
	Monsters: []DungeonMonster{
		nm17Enemy("nm17-blood-hand", "血塗れの手", 160, 130, 50, 60, 41, 10, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 41},
			Jobs:        nm17Jobs("job-09", 10, "job-61", 10),
		}),
		nm17Enemy("nm17-mummy", "マミー", 170, 150, 60, 60, 43, 16, corebattle.LifetimeActor{
			Jobs: nm17Jobs("", 0, "", 20),
		}),
		// The source lists old_sp twice; Perl keeps the later 40 (__data.cgi:129–130).
		nm17Enemy("nm17-stinger", "スティンガー", 180, 180, 150, 120, 54, 28, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-90", 20, "", 40),
		}),
		nm17Enemy("nm17-wraith", "死霊", 199, 119, 199, 66, 45, 13, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        nm17Jobs("job-31", 60, "", 0),
		}),
		nm17Enemy("nm17-mimic", "ミミック", 300, 100, 30, 300, 50, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 68},
			Jobs:        nm17Jobs("job-93", 10, "", 0),
			Temporary:   corebattle.TemporaryDouble,
		}),
		nm17Enemy("nm17-ooze-pharaoh", "粘体の王", 444, 144, 144, 144, 144, 144, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 144},
			Jobs:        nm17Jobs("job-19", 60, "", 0),
		}),
	},
	Appears: []int{0, 0, 1, 1, 1, 2, 2, 3, 3, 4, 5},
}

// nm17Inner is ___data.cgi's @monsters with @appears.
var nm17Inner = EncounterPool{
	Monsters: []DungeonMonster{
		nm17Enemy("nm17-sphinx-a", "スフィンクス", 500, 400, 480, 100, 250, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-37", 999, "job-30", 999),
			Hit:         150,
		}),
		nm17Enemy("nm17-sphinx-b", "スフィンクス", 500, 400, 480, 100, 250, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-19", 999, "job-27", 999),
			Hit:         150,
		}),
		nm17Enemy("nm17-sand-golem", "サンドゴーレム", 350, 320, 450, 200, 200, 200, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 200},
			Jobs:        nm17Jobs("job-16", 999, "", 30),
			Temporary:   corebattle.TemporaryRevival,
		}),
		nm17Enemy("nm17-calamity-box", "災いの箱", 900, 300, 95, 800, 100, 500, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 69},
			Jobs:        nm17Jobs("job-93", 20, "", 0),
			Temporary:   corebattle.TemporaryDouble,
		}),
		nm17Enemy("nm17-envoy", "ファラオのつかい", 450, 300, 800, 900, 130, 800, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:        nm17Jobs("job-15", 200, "job-91", 200),
		}),
	},
	Appears: []int{0, 0, 1, 1, 2, 2, 3, 4},
}

// The underground groups (_1.cgi:108–254); the mimic and box are the pool entries.
var (
	nm17Robber = nm17Enemy("nm17-robber", "墓荒らし", 500, 40, 3, 89, 20, 7, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 200},
		Jobs:        nm17Jobs("job-09", 70, "job-52", 20),
	})
	nm17RobberChief = nm17Enemy("nm17-robber-chief", "墓荒らし", 1500, 200, 90, 400, 151, 56, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 999, ItemDefinitionIDs: []string{"item-158"}},
		Jobs:        nm17Jobs("job-09", 999, "job-52", 130),
	})
	nm17Mask = nm17Enemy("nm17-mask", "ファラオの呪面", 800, 200, 300, 200, 111, 111, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 999},
		Jobs:        nm17Jobs("job-31", 999, "job-15", 999),
	})
	nm17GreatMask = nm17Enemy("nm17-great-mask", "ファラオの呪面", 1300, 400, 600, 400, 222, 222, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 999},
		Jobs:        nm17Jobs("job-31", 999, "job-15", 999),
	})
)

// nm17Pharaohs is ___data.cgi's @bosses, fought at Z.
var nm17Pharaohs = []DungeonMonster{
	nm17Enemy("nm17-cobra-a", "ウァジェイダA", 1800, 500, 30, 500, 400, 10, nm17Cobra),
	nm17Enemy("nm17-pharaoh", "ファラオ", 6000, 380, 380, 200, 7800, 15000, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 10000, ItemDefinitionIDs: []string{"item-144"},
			Abilities: []string{"pharaoh", "poison1", "paralysis1", "sleep1", "sealed1", "confusion1"}},
		Jobs:          nm17Jobs("job-505", 40, "", 0),
		Hit:           150,
		CrystalReward: 15,
		Temporary:     corebattle.TemporaryNullMagic,
	}),
	nm17Enemy("nm17-cobra-b", "ウァジェイダB", 1800, 500, 30, 500, 400, 10, nm17Cobra),
}

var nm17Cobra = corebattle.LifetimeActor{
	Participant:   corebattle.Participant{MP: 2000, Abilities: []string{"poison1", "paralysis1", "sleep1", "sealed1", "confusion1"}},
	Jobs:          nm17Jobs("job-93", 999, "job-20", 999),
	Hit:           150,
	CrystalReward: 5,
	Temporary:     corebattle.TemporaryNullAttack,
}

// nm17Necro is w's boss (1.cgi:179–200). Its max HP 12000 is preset, so it
// starts below full; add_boss leaves a preset maximum unscaled.
var nm17Necro = nm17Enemy("nm17-necro-pharaoh", "ネクロファラオ", 6000, 780, 10, 170, 9000, 0, corebattle.LifetimeActor{
	Participant: corebattle.Participant{MaxHP: 12000, ItemDefinitionIDs: []string{"item-144"},
		Abilities: []string{"poison1", "paralysis1", "sleep1", "sealed1", "confusion1"}},
	Jobs:          nm17Jobs("job-506", 0, "", 0),
	Hit:           75,
	CrystalReward: 15,
})

// nm17Guardians is __data.cgi's @bosses, fought at B.
var nm17Guardians = []DungeonMonster{
	nm17Enemy("nm17-golem-a", "サンドゴーレムA", 1300, 220, 400, 110, 250, 100, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 700},
		Jobs:        nm17Jobs("job-16", 999, "", 0),
		Hit:         150,
		Temporary:   corebattle.TemporaryRevival,
	}),
	nm17Enemy("nm17-sphinx-guard-a", "スフィンクスA", 3200, 260, 320, 150, 500, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 999},
		Jobs:          nm17Jobs("job-37", 999, "job-30", 999),
		Hit:           150,
		CrystalReward: 4,
		Temporary:     corebattle.TemporaryReflectAttack,
	}),
	nm17Enemy("nm17-sphinx-guard-b", "スフィンクスB", 3200, 260, 320, 150, 500, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 999},
		Jobs:          nm17Jobs("job-19", 999, "job-27", 999),
		Hit:           150,
		CrystalReward: 4,
		Temporary:     corebattle.TemporaryReflectMagic,
	}),
	nm17Enemy("nm17-golem-b", "サンドゴーレムB", 1300, 220, 400, 110, 250, 100, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 700},
		Jobs:        nm17Jobs("job-16", 999, "", 0),
		Hit:         150,
		Temporary:   corebattle.TemporaryRevival,
	}),
}

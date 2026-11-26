package boss

import (
	_ "embed"
	"encoding/json"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/runhost"
)

// stagesJSON transcribes %k, @treasures and @bosses of party2/stage/king*.cgi.
//
//go:embed data/stages.json
var stagesJSON []byte

// BossStage is one king stage file: the room's name, leader, capacity,
// join condition, treasure tables, initial bosses and summon pool
// (@monsters). king99's bosses are the unsealers' clones, so its list is empty.
type BossStage struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	LeaderName string                `json:"leader_name"`
	Speed      int                   `json:"speed"`
	MaxMembers int                   `json:"max_members"`
	NeedJoin   string                `json:"need_join"`
	Treasures  runhost.TreasurePools `json:"treasures"`
	Bosses     []BossMonster         `json:"bosses"`
	Monsters   []BossMonster         `json:"monsters"`
}

// BossMonster is one @bosses row with its legacy keys.
type BossMonster struct {
	Name     string `json:"name"`
	HP       int    `json:"hp"`
	MP       int    `json:"mp"`
	MaxMP    int    `json:"mmp"`
	Attack   int    `json:"at"`
	Defense  int    `json:"df"`
	Agility  int    `json:"ag"`
	EXP      int    `json:"get_exp"`
	Gold     int    `json:"get_money"`
	Crystals int    `json:"get_crystal"`
	Icon     string `json:"icon"`
	Hit      int    `json:"hit"`
	Job      int    `json:"job"`
	SP       int    `json:"sp"`
	OldJob   int    `json:"old_job"`
	OldSP    int    `json:"old_sp"`
	TMP      string `json:"tmp"`
	State    string `json:"state"`
	// RandomCrystal is get_crystal => int(rand(2)), drawn at each stage-file load.
	RandomCrystal bool `json:"random_crystal"`
}

// DefaultBossCatalog decodes the king stages in file order (king1–king10, king99).
func DefaultBossCatalog() ([]BossStage, error) {
	var stages []BossStage
	if err := json.Unmarshal(stagesJSON, &stages); err != nil {
		return nil, fmt.Errorf("decode king stages: %w", err)
	}
	return stages, nil
}

// temporaries maps @bosses tmp names (%tmp_tip, _data.cgi:2247–2270) to
// battle effects; a name without a handler (するぞ, 100倍) stays inert.
var temporaries = map[string]corebattle.TemporaryEffect{
	"防御": corebattle.TemporaryDefense, "大防御": corebattle.TemporaryStrongDefense, "単発防御": corebattle.TemporaryOneHitDefense,
	"攻軽減": corebattle.TemporaryReduceAttack, "魔軽減": corebattle.TemporaryReduceMagic, "息軽減": corebattle.TemporaryReduceBreath,
	"攻無効": corebattle.TemporaryNullAttack, "魔無効": corebattle.TemporaryNullMagic, "息無効": corebattle.TemporaryNullBreath,
	"攻反撃": corebattle.TemporaryReflectAttack, "魔反撃": corebattle.TemporaryReflectMagic, "息反撃": corebattle.TemporaryReflectBreath,
	"踊反撃": corebattle.TemporaryReflectDance, "２倍": corebattle.TemporaryDouble, "受流し": corebattle.TemporaryDeflect,
	"魔吸収": corebattle.TemporaryAbsorbMagic, "回復": corebattle.TemporaryRegeneration, "復活": corebattle.TemporaryRevival,
	"激昂": corebattle.TemporaryRage, "朧幻": corebattle.TemporaryPhantom,
}

func temporary(tmp string) corebattle.TemporaryEffect {
	if e, ok := temporaries[tmp]; ok {
		return e
	}
	return corebattle.TemporaryEffect(tmp)
}

func legacyJobID(n int) string {
	if n == 0 {
		return "starter"
	}
	return fmt.Sprintf("job-%02d", n)
}

// payload is the row's battle data as written: a missing field is 0.
func (b BossMonster) payload(id string) corebattle.LifetimeActor {
	jobs := corebattle.BattleJobs{JobID: legacyJobID(b.Job), SP: b.SP, OldJobID: legacyJobID(b.OldJob), OldSP: b.OldSP}
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID: id, Name: b.Name, TeamID: "enemies", HP: b.HP, MP: b.MP, MaxMP: b.MaxMP,
			Attack: b.Attack, Defense: b.Defense, Agility: b.Agility,
			Gear: &corebattle.BattleGearStats{}, Status: b.State, Icon: b.Icon, ExperienceReward: b.EXP, GoldReward: b.Gold,
		},
		Jobs: jobs, OriginalJobs: jobs, NPC: true, Hit: b.Hit, Temporary: temporary(b.TMP),
		EncounterIdentity: id, CrystalReward: b.Crystals,
	}
}

// actor is make_vs_king's battle row (vs_monster.cgi:178–196): max stats
// default to the base, hit to 95 and tension to 1.
func (b BossMonster) actor(id string) corebattle.LifetimeActor {
	a := b.payload(id)
	a.MaxHP = a.HP
	if a.MaxMP == 0 {
		a.MaxMP = a.MP
	}
	a.BaseStats = corebattle.BattleBaseStats{Attack: a.Attack, Defense: a.Defense, Agility: a.Agility}
	if a.Hit == 0 {
		a.Hit = 95
	}
	a.Tension = 1
	return a
}

// summonPool is @monsters as _add_enemy loads it: each random crystal is
// drawn for this load; the summon owner scales stats and fills defaults.
func (stage BossStage) summonPool(rng random.Generator) []corebattle.LifetimeActor {
	pool := make([]corebattle.LifetimeActor, len(stage.Monsters))
	for i, m := range stage.Monsters {
		pool[i] = m.payload(fmt.Sprintf("%s-monster-%d", stage.ID, i))
		if m.RandomCrystal {
			pool[i].CrystalReward = rng.Intn(2)
		}
	}
	return pool
}

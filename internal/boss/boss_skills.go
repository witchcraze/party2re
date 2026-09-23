package boss

import (
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func parseJobID(jobIDStr string) int {
	jobIDStr = strings.TrimPrefix(jobIDStr, "job-")
	jobIDStr = strings.TrimPrefix(jobIDStr, "job_")
	n, _ := strconv.Atoi(jobIDStr)
	return n
}

func bossSkillsForJob(jobID, sp int) []corebattle.ActionSkill {
	if jobID <= 0 {
		return nil
	}
	if sp <= 0 {
		sp = 999
	}

	var skills []corebattle.ActionSkill

	switch jobID {
	case 1: // 赤魔人 / 戦士
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "kaengiri", Name: "かえんぎり", MPCost: 5, Power: 30, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 2: // 呪の剣 / 僧侶
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "fuuingiri", Name: "ふういんぎり", MPCost: 8, Power: 30,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 3: // 地獄の鎧 / 武闘家
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "iatsukan", Name: "いあつかん", MPCost: 6, Power: 20, BuffStat: "defense",
				Kind: corebattle.ActionKindBuff, TargetScope: corebattle.TargetScopeSelf,
			})
		}
	case 4: // 青魔人 / 魔法使い
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "kamaitachi", Name: "かまいたち", MPCost: 5, Power: 30, Element: "wind",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 5: // ホイミスライム
		if sp >= 1 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "hoimi", Name: "ホイミ", MPCost: 3, Power: 20,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeSingleAlly,
			})
		}
		if sp >= 50 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "behomara", Name: "ベホマラー", MPCost: 25, Power: 50,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeAllAllies,
			})
		}
	case 6: // 魔法使い
		if sp >= 1 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "mera", Name: "メラ", MPCost: 2, Power: 15, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 8 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "gira", Name: "ギラ", MPCost: 5, Power: 20, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "merami", Name: "メラミ", MPCost: 8, Power: 35, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 55 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "begirama", Name: "ベギラマ", MPCost: 11, Power: 40, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 90 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "merazoma", Name: "メラゾーマ", MPCost: 30, Power: 60, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 8: // ボマー
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "jibaku", Name: "じばく", MPCost: 11, Power: 80,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 19: // 闇魔道士
		if sp >= 60 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "begiragon", Name: "ベギラゴン", MPCost: 18, Power: 50, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 20: // 悪魔
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "amaiiki", Name: "あまいいき", MPCost: 9, Status: corebattle.StatusSleep,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 21: // キングベヒーモス
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "nagiharai", Name: "なぎはらい", MPCost: 15, Power: 35,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 22: // 暗黒の剣
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "ankokuken", Name: "あんこくけん", MPCost: 20, Power: 40, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 23: // ベヒーモス
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "sutemi", Name: "すてみ", MPCost: 10, Power: 50,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 24: // 地獄の騎士
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "renzokugiri", Name: "れんぞくぎり", MPCost: 15, Power: 40,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 25: // 白魔人
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "behomara", Name: "ベホマラー", MPCost: 25, Power: 50,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeAllAllies,
			})
		}
	case 26: // 忍者
		if sp >= 5 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "kaennonoiki", Name: "かえんのいき", MPCost: 5, Power: 20, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 15 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "yaketsukuiki", Name: "やけつくいき", MPCost: 10, Status: corebattle.StatusParalyze,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 40 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "moudokunokiri", Name: "もうどくのきり", MPCost: 7, Status: corebattle.StatusPoison,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 31: // 青魔道士
		if sp >= 11 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "jibaku", Name: "じばく", MPCost: 11, Power: 80,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 121 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "white_wind", Name: "ホワイトウィンド", MPCost: 36, Power: 60,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeAllAllies,
			})
		}
	case 33: // 賢者
		if sp >= 5 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "magic_barrier", Name: "マジックバリア", MPCost: 6, Power: 20, BuffStat: "defense",
				Kind: corebattle.ActionKindBuff, TargetScope: corebattle.TargetScopeSelf,
			})
		}
		if sp >= 15 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "io_la", Name: "イオラ", MPCost: 12, Power: 25, Element: "light",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 70 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "behomara", Name: "ベホマラー", MPCost: 25, Power: 50,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeAllAllies,
			})
		}
		if sp >= 100 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "baikiruto", Name: "バイキルト", MPCost: 16, Power: 30, BuffStat: "attack",
				Kind: corebattle.ActionKindBuff, TargetScope: corebattle.TargetScopeSelf,
			})
		}
		if sp >= 130 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "ionazun", Name: "イオナズン", MPCost: 34, Power: 55, Element: "light",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 35: // 魔王
		if sp >= 90 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "shakunetsu", Name: "しゃくねつ", MPCost: 40, Power: 65, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 120 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "meisou", Name: "めいそう", MPCost: 25, Power: 60,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeSelf,
			})
		}
		if sp >= 180 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "jigospark", Name: "ジゴスパーク", MPCost: 70, Power: 75, Element: "light",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 36: // ものまね士
		if sp >= 60 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "kougeki_monomane", Name: "こうげきものまね", MPCost: 5, Power: 30,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 37: // 結界士
		if sp >= 25 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "magic_barrier", Name: "マジックバリア", MPCost: 6, Power: 20, BuffStat: "defense",
				Kind: corebattle.ActionKindBuff, TargetScope: corebattle.TargetScopeSelf,
			})
		}
		if sp >= 80 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "meisou", Name: "めいそう", MPCost: 25, Power: 60,
				Kind: corebattle.ActionKindHeal, TargetScope: corebattle.TargetScopeSelf,
			})
		}
	case 41: // ドラゴン
		if sp >= 5 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "kaennonoiki", Name: "かえんのいき", MPCost: 5, Power: 25, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 40 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "shakunetsu", Name: "しゃくねつ", MPCost: 40, Power: 65, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 51: // 一賢者
		if sp >= 30 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "ionazun", Name: "イオナズン", MPCost: 34, Power: 55, Element: "light",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 52: // 黒魔人
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "merazoma", Name: "メラゾーマ", MPCost: 30, Power: 60, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 55, 96: // ドールマスター
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "shoukan", Name: "しょうかん", MPCost: 50, Power: 70,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "merazoma", Name: "メラゾーマ", MPCost: 30, Power: 60, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	case 58: // ドラゴンゾンビ
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "yaketsukuiki", Name: "やけつくいき", MPCost: 10, Status: corebattle.StatusParalyze,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 70: // イフリート
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "jigokunogouka", Name: "じごくのごうか", MPCost: 50, Power: 70, Element: "fire",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 90: // 毒系
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "doku_kougeki", Name: "どくこうげき", MPCost: 4, Status: corebattle.StatusPoison,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 30 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "moudokunokiri", Name: "もうどくのきり", MPCost: 8, Status: corebattle.StatusPoison,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 91: // 麻痺系
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "mahi_kougeki", Name: "まひこうげき", MPCost: 8, Status: corebattle.StatusParalyze,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "shibireuchi", Name: "しびれうち", MPCost: 11, Status: corebattle.StatusParalyze,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 30 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "yaketsukuiki", Name: "やけつくいき", MPCost: 9, Status: corebattle.StatusParalyze,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 92: // 眠り系
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "rariho", Name: "ラリホー", MPCost: 8, Status: corebattle.StatusSleep,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 20 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "nemuri_kougeki", Name: "ねむりこうげき", MPCost: 15, Status: corebattle.StatusSleep,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
		if sp >= 30 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "amaiiki", Name: "あまいいき", MPCost: 9, Status: corebattle.StatusSleep,
				Kind: corebattle.ActionKindStatus, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 94: // 爆弾岩
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "megante", Name: "メガンテ", MPCost: 1, Power: 100,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 95: // 召喚
		skills = append(skills, corebattle.ActionSkill{
			ID: "shoukan", Name: "しょうかん", MPCost: 50, Power: 70,
			Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
		})
	case 97: // 破壊神
		skills = append(skills,
			corebattle.ActionSkill{
				ID: "midareuchi", Name: "みだれうち", MPCost: 24, Power: 50,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			},
			corebattle.ActionSkill{
				ID: "bakuretsuken", Name: "ばくれつけん", MPCost: 20, Power: 45,
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			},
			corebattle.ActionSkill{
				ID: "ankokuken", Name: "あんこくけん", MPCost: 40, Power: 60, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeSingleEnemy,
			},
			corebattle.ActionSkill{
				ID: "shikkokunohonoo", Name: "しっこくのほのお", MPCost: 80, Power: 80, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			},
			corebattle.ActionSkill{
				ID: "jigospark", Name: "ジゴスパーク", MPCost: 70, Power: 75, Element: "light",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			},
		)
	case 98: // 悪魔の書
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "zaraki", Name: "ザラキ", MPCost: 32, Power: 60, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	case 101: // 暗黒竜
		if sp >= 10 {
			skills = append(skills, corebattle.ActionSkill{
				ID: "yaminootakebi", Name: "やみのおたけび", MPCost: 30, Power: 50, Element: "dark",
				Kind: corebattle.ActionKindAttack, TargetScope: corebattle.TargetScopeAllEnemies,
			})
		}
	}
	return skills
}

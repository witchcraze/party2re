package battle

import (
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

// applyMonsterKills evaluates defeated non-player enemies against player combat strength
// matching legacy Party2 (_battle.cgi:230-235). Surviving characters increment MonsterKills
// for each defeated enemy satisfying IsStrongEnemy.
func (s *Service) applyMonsterKills(
	char *corecharacter.Character,
	inv coreinventory.Inventory,
	equip coreequipment.Equipment,
	res corebattle.PartyBattleResult,
	defeatedEnemies []corebattle.Participant,
) {
	if len(defeatedEnemies) == 0 {
		return
	}
	if remHP, ok := res.RemainingHP[char.ID]; ok && remHP <= 0 {
		return
	}
	if char.Stats.HP <= 0 {
		return
	}

	eqStats := CalculateEquipmentStats(*char, inv, equip, s.rng)
	playerAtk := char.Stats.Attack + eqStats.AttackBonus
	if playerAtk < 0 {
		playerAtk = 0
	}
	playerDef := char.Stats.Defense + eqStats.DefenseBonus
	if playerDef < 0 {
		playerDef = 0
	}
	playerAgi := char.Stats.Agility + eqStats.AgilityBonus
	if playerAgi < 0 {
		playerAgi = 0
	}
	playerStrong := Strong(char.Stats.MaxHP, char.Stats.MaxMP, playerAtk, playerDef, playerAgi)

	for _, enemy := range defeatedEnemies {
		// Non-player enemies only
		if strings.HasPrefix(enemy.ID, "char-") || strings.HasPrefix(enemy.ID, "user-") {
			continue
		}
		enemyMHP := enemy.MaxHP
		if enemyMHP <= 0 {
			enemyMHP = enemy.HP
		}
		enemyMMP := enemy.MaxMP
		if enemyMMP <= 0 {
			enemyMMP = enemy.MP
		}
		enemyStrong := Strong(enemyMHP, enemyMMP, enemy.Attack, enemy.Defense, enemy.Agility)
		if IsStrongEnemy(enemyStrong, playerStrong) {
			char.MonsterKills++
		}
	}
}

// applyMaoCount increments MaoCount on defeating the demon king / Stage EX unseal event
// matching legacy Party2 (vs_monster.cgi:218).
func (s *Service) applyMaoCount(char *corecharacter.Character, req ApplyPostBattleRequest) {
	if isDemonKingUnseal(req) {
		char.MaoCount++
	}
}

// isDemonKingUnseal determines whether this combat resolution corresponds to the authentic
// demon king unsealing event (vs_monster.cgi:105, 218).
func isDemonKingUnseal(req ApplyPostBattleRequest) bool {
	if req.UnsealDemonKing {
		return true
	}
	habitat := strings.TrimSpace(req.Habitat)
	if habitat == "封印の地" || habitat == "stage-20" || habitat == "20" || habitat == "stage-19" || habitat == "19" {
		return true
	}
	for _, enemy := range req.DefeatedEnemies {
		if ParseDefeatedMonsterID(enemy.ID) == "monster-212" || CleanMonsterName(enemy.Name) == "封印のツボ" {
			return true
		}
	}
	return false
}

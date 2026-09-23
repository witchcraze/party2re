package battle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// ErrMonsterBoxFull is returned or matched when a character's monster box is at capacity.
var ErrMonsterBoxFull = errors.New("monster box full")

// MonsterTameResult contains the outcome of a monster rising / taming attempt.
type MonsterTameResult struct {
	MonsterID   string `json:"monster_id"`
	MonsterName string `json:"monster_name"`
	Success     bool   `json:"success"`
	BoxFull     bool   `json:"box_full"`
	Message     string `json:"message"`
}

// MonsterTamer handles saving a tamed monster to storage.
type MonsterTamer interface {
	TameMonster(ctx context.Context, characterID, monsterID, customName string) error
}

// BlessingProvider queries active chapel blessings for a character.
type BlessingProvider interface {
	GetActiveBlessing(ctx context.Context, characterID string) (string, error)
}

// MonsterDefeatRecorder registers defeated monsters into the monster book.
type MonsterDefeatRecorder interface {
	RecordMonsterDefeat(ctx context.Context, characterID, monsterID, monsterName, habitat string) error
}

// Strong calculates combat strength metric matching legacy Perl CGI (party2/lib/_battle.cgi:1380-1383):
// int(mhp + mmp + at + df * 0.5 + ag)
func Strong(mhp, mmp, at, df, ag int) int {
	return int(float64(mhp) + float64(mmp) + float64(at) + float64(df)*0.5 + float64(ag))
}

// IsStrongEnemy determines whether an enemy is considered "strong" relative to the player:
// strong(enemy) > strong(player) * 0.5
func IsStrongEnemy(enemyStrong, playerStrong int) bool {
	return float64(enemyStrong) > float64(playerStrong)*0.5
}

// CalculateTamePar computes the recruitment probability parameter $par (out of 200)
// matching legacy party2/lib/_battle.cgi:230-240.
func CalculateTamePar(
	isStrong bool,
	jobID string,
	hasMonsterFood bool,
	hasMonsterBlessing bool,
	hasDragonRulerSet bool,
	isCaptured bool,
) float64 {
	par := 2.0 // Base: enemy is weaker
	if isStrong {
		// Strong enemy: par drops to 1, unless player is Monster Tamer (job-12)
		if jobID == "job-12" || jobID == "12" {
			par = 2.0
		} else {
			par = 1.0
		}
	}

	if hasMonsterFood {
		par += 2.0
	}
	if hasMonsterBlessing {
		par += 0.5
	}
	if hasDragonRulerSet {
		par += 0.5
	}
	if isCaptured {
		par += 4.0
	}
	return par
}

// CleanMonsterName cleans legacy prefixes and suffixes from monster names.
// e.g. "@スライムA" -> "スライム", "スライムB" -> "スライム"
func CleanMonsterName(name string) string {
	name = strings.TrimPrefix(name, "@")
	name = strings.TrimSpace(name)
	if len(name) > 1 {
		last := name[len(name)-1]
		if last >= 'A' && last <= 'Z' {
			name = strings.TrimSpace(name[:len(name)-1])
		}
	}
	return name
}

// ParseDefeatedMonsterID extracts the canonical monster ID from participant IDs
// e.g. "monster-002-f1-1" -> "monster-002"
func ParseDefeatedMonsterID(id string) string {
	if strings.HasPrefix(id, "monster-") {
		parts := strings.Split(id, "-")
		if len(parts) >= 2 {
			return parts[0] + "-" + parts[1]
		}
	}
	return id
}

// rollTameSuccess rolls whether a monster recruitment succeeds given par (probability = par / 200).
func (s *Service) rollTameSuccess(par float64) bool {
	threshold := int(par * 10.0)
	return s.rollIntn(2000) < threshold
}

// checkDragonRulerSet checks whether the character has weapon-36 and armor-39 equipped.
func hasDragonRulerEquipment(inv coreinventory.Inventory, equip coreequipment.Equipment) bool {
	hasW36 := false
	if instID, ok := equip.Equipped(coreitem.SlotMainHand); ok {
		if inst, found := inv.Find(instID); found {
			if parseItemIndex(inst.DefinitionID, "weapon-") == 36 {
				hasW36 = true
			}
		}
	}
	hasA39 := false
	if instID, ok := equip.Equipped(coreitem.SlotBody); ok {
		if inst, found := inv.Find(instID); found {
			if parseItemIndex(inst.DefinitionID, "armor-") == 39 {
				hasA39 = true
			}
		}
	}
	return hasW36 && hasA39
}

// checkMonsterFood checks whether the character possesses item-077 (魔物のエサ) in inventory.
func hasMonsterFoodItem(inv coreinventory.Inventory) bool {
	for _, inst := range inv.Items {
		if inst.DefinitionID == "item-077" || inst.DefinitionID == "item-77" || inst.DefinitionID == "77" {
			return true
		}
	}
	return false
}

func (s *Service) queryBlessing(ctx context.Context, charID string) string {
	if s.blessingProvider == nil {
		return ""
	}
	blessing, err := s.blessingProvider.GetActiveBlessing(ctx, charID)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(blessing))
}

func isGoldBlessing(blessing string) bool {
	switch blessing {
	case "gold", "1", "gold_wish", "お金がほしい":
		return true
	default:
		return false
	}
}

func isExpBlessing(blessing string) bool {
	switch blessing {
	case "exp", "2", "exp_wish", "強くなりたい":
		return true
	default:
		return false
	}
}

func isMonsterBlessing(blessing string) bool {
	switch blessing {
	case "monster", "3", "monster_wish", "モンスターと仲良くしたい":
		return true
	default:
		return false
	}
}

func isDropBlessing(blessing string) bool {
	switch blessing {
	case "drop", "4", "treasure_wish", "宝箱がほしい":
		return true
	default:
		return false
	}
}

// checkMonsterBlessing queries whether the character has an active monster blessing (wish 3).
func (s *Service) checkMonsterBlessing(ctx context.Context, charID string) bool {
	return isMonsterBlessing(s.queryBlessing(ctx, charID))
}

// processMonsterTaming handles post-battle monster rising and taming for a recipient character.
func (s *Service) processMonsterTaming(
	ctx context.Context,
	char corecharacter.Character,
	inv coreinventory.Inventory,
	equip coreequipment.Equipment,
	defeatedEnemies []corebattle.Participant,
) []MonsterTameResult {
	if s.monsterTamer == nil || len(defeatedEnemies) == 0 {
		return nil
	}

	eqStats := CalculateEquipmentStats(char, inv, equip, s.rng)
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

	hasFood := hasMonsterFoodItem(inv)
	hasBlessing := s.checkMonsterBlessing(ctx, char.ID)
	hasDragonSet := hasDragonRulerEquipment(inv, equip)

	var results []MonsterTameResult

	for _, enemy := range defeatedEnemies {
		// Non-NPC / player characters cannot be recruited
		if strings.HasPrefix(enemy.ID, "char-") || strings.HasPrefix(enemy.ID, "user-") {
			continue
		}

		enemyStrong := Strong(enemy.MaxHP, enemy.MaxMP, enemy.Attack, enemy.Defense, enemy.Agility)
		isStrong := IsStrongEnemy(enemyStrong, playerStrong)
		isCaptured := enemy.Status == "捕縛" || enemy.Status == "captured"

		par := CalculateTamePar(isStrong, char.JobID, hasFood, hasBlessing, hasDragonSet, isCaptured)

		if !s.rollTameSuccess(par) {
			continue
		}

		// Monster rises up!
		baseName := CleanMonsterName(enemy.Name)
		monsterID := ParseDefeatedMonsterID(enemy.ID)

		err := s.monsterTamer.TameMonster(ctx, char.ID, monsterID, baseName)
		if errors.Is(err, ErrMonsterBoxFull) {
			msg := fmt.Sprintf(
				"なんと %s が起き上がりこちらを見ている。しかし、%sのモンスター預かり所はいっぱいだった。%sは悲しそうに去っていった…",
				baseName, char.Name, baseName,
			)
			results = append(results, MonsterTameResult{
				MonsterID:   monsterID,
				MonsterName: baseName,
				Success:     false,
				BoxFull:     true,
				Message:     msg,
			})
		} else if err == nil {
			msg := fmt.Sprintf(
				"なんと %s が起き上がりこちらを見ている。%sはうれしそうに%sのモンスター預かり所に向かった",
				baseName, baseName, char.Name,
			)
			results = append(results, MonsterTameResult{
				MonsterID:   monsterID,
				MonsterName: baseName,
				Success:     true,
				BoxFull:     false,
				Message:     msg,
			})

			if s.monsterRecorder != nil {
				_ = s.monsterRecorder.RecordMonsterDefeat(ctx, char.ID, monsterID, baseName, "")
			}
		}
	}

	return results
}

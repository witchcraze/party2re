package battle

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

// UnsealClockWriter persists the demon king unseal clock timestamp within the settlement transaction.
type UnsealClockWriter interface {
	SaveUnsealedAt(ctx context.Context, unsealedAt time.Time) error
}

// ParticipantStrong computes combat strength for a battle participant matching legacy strong() (_battle.cgi:1380-1383).
func ParticipantStrong(p corebattle.Participant) int {
	mhp := p.MaxHP
	if mhp <= 0 {
		mhp = p.HP
	}
	mmp := p.MaxMP
	if mmp <= 0 {
		mmp = p.MP
	}
	return Strong(mhp, mmp, p.Attack, p.Defense, p.Agility)
}

// CountStrongPvPKills returns per-actor kill_p increments for non-revived combat defeats
// where the defeated opponent satisfies IsStrongEnemy (_battle.cgi:231-232).
func CountStrongPvPKills(participants []corebattle.Participant, defeats []corebattle.PartyBattleDefeat) map[string]int {
	if len(defeats) == 0 {
		return nil
	}
	byID := make(map[string]corebattle.Participant, len(participants))
	for _, p := range participants {
		byID[p.ID] = p
	}
	var kills map[string]int
	for _, d := range defeats {
		if d.ActorID == "" || d.TargetID == "" || d.ActorID == d.TargetID {
			continue
		}
		actor, okA := byID[d.ActorID]
		target, okT := byID[d.TargetID]
		if !okA || !okT {
			continue
		}
		if IsStrongEnemy(ParticipantStrong(target), ParticipantStrong(actor)) {
			if kills == nil {
				kills = make(map[string]int)
			}
			kills[d.ActorID]++
		}
	}
	return kills
}

// MergeKillCounts returns a combined copy of base plus delta kill counts.
func MergeKillCounts(base, delta map[string]int) map[string]int {
	if len(base) == 0 && len(delta) == 0 {
		return nil
	}
	out := make(map[string]int, len(base)+len(delta))
	for k, v := range base {
		if v > 0 {
			out[k] = v
		}
	}
	for k, v := range delta {
		if v > 0 {
			out[k] += v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SortedPositiveKillIDs returns character IDs with positive kill counts in ascending order for Rank 2 locking.
func SortedPositiveKillIDs(kills map[string]int) []string {
	var ids []string
	for id, count := range kills {
		if id != "" && count > 0 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

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
		if IsStrongEnemy(ParticipantStrong(enemy), playerStrong) {
			char.MonsterKills++
		}
	}
}

// applyMaoCount increments MaoCount on an explicit demon king unseal event.
func (s *Service) applyMaoCount(char *corecharacter.Character, req ApplyPostBattleRequest) {
	if req.UnsealDemonKing {
		char.MaoCount++
	}
}

func (s *Service) recordUnsealClock(ctx context.Context, req ApplyPostBattleRequest) error {
	if !req.UnsealDemonKing || s.unsealClock == nil {
		return nil
	}
	if err := s.unsealClock.SaveUnsealedAt(ctx, s.now()); err != nil {
		return fmt.Errorf("failed to save unseal clock: %w", err)
	}
	return nil
}

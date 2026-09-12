package party

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

// StartPartyAdventure starts and resolves a multiplayer 10-floor dungeon crawl with Floor 11 treasure room (vs_monster.cgi).
//
// Concurrency & Persistence Boundary:
//  1. Transition party status in Valkey Master (GetPartyForUpdate).
//  2. Begin MariaDB transaction (runInTx) acquiring Rank 2 locks on all participating
//     character rows in ascending ID order to prevent deadlock.
//  3. Execute 10-floor crawl and Floor 11 treasure room in memory.
//  4. Atomically commit character updates, inventory items, and durable party_adventure_logs in MariaDB.
//  5. Disband and clean up the ephemeral Valkey lobby state.
func (s *Service) StartPartyAdventure(ctx context.Context, partyID, leaderCharID string) (PartyAdventureResult, error) {
	var result PartyAdventureResult
	var defeatedMonsterCount int

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock party
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		if p.Status != StatusRecruiting {
			return ErrPartyNotRecruiting
		}

		// 2. Fetch members
		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil || len(members) == 0 {
			return ErrPartyNotReady
		}

		// All members must be ready
		for _, m := range members {
			if !m.ReadyState {
				return ErrPartyNotReady
			}
		}

		// 3. Lock all character rows in ascending order to prevent deadlocks
		charIDs := make([]string, len(members))
		for i, m := range members {
			charIDs[i] = m.CharacterID
		}
		sort.Strings(charIDs)

		charMap := make(map[string]corecharacter.Character, len(charIDs))
		for _, cID := range charIDs {
			c, err := s.charRepo.FindByIDForUpdate(txCtx, cID)
			if err != nil {
				return err
			}
			if c.Stats.HP <= 0 {
				return ErrCharacterUnconscious
			}
			if c.Tired >= 100 {
				return ErrCharacterExhausted
			}
			charMap[c.ID] = c
		}

		// 4. Resolve stage
		stage, err := s.stages.FindByID(p.StageID)
		if err != nil {
			return ErrStageNotFound
		}

		// 5. Build participating characters in party member order
		participatingChars := make([]corecharacter.Character, len(members))
		for i, m := range members {
			participatingChars[i] = charMap[m.CharacterID]
		}

		// 6. Execute 10-Floor Dungeon Crawl & Floor 11 Treasure Room
		session, err := adventure.NewCrawlSession(stage, participatingChars, nil)
		if err != nil {
			return err
		}

		var lastBattleRes battle.PartyBattleResult
		for floor := 1; floor <= adventure.BossFloor; floor++ {
			floorRes, err := session.AdvanceFloor(s.stages, s.monsters, s.battleEngine)
			if err != nil {
				return err
			}
			lastBattleRes = floorRes.BattleResult
			if !floorRes.Cleared {
				break
			}
		}

		// Floor 11 (Treasure Room) if stage cleared
		if session.StageCleared {
			_, _ = session.AdvanceFloor(s.stages, s.monsters, s.battleEngine)
			for _, c := range participatingChars {
				_, _ = session.ExamineTreasure(c.ID)
			}
		}

		crawlResult := session.Result()

		// 7. Distribute Rewards and update each character
		var rewardSummaries []MemberRewardSummary
		outcome := string(crawlResult.Outcome)
		if crawlResult.Outcome == battle.OutcomeWin {
			defeatedMonsterCount = crawlResult.FloorsCleared
		}

		for _, m := range members {
			c := charMap[m.CharacterID]
			levelBefore := c.Level
			gainedEXP := crawlResult.TotalEXP
			gainedGold := crawlResult.TotalGold

			_ = c.AddMoney(gainedGold)
			if gainedEXP > 0 {
				if _, err := progression.ApplyExperience(&c, gainedEXP); err != nil {
					return err
				}
			}

			// Apply HP changes from battle result
			if remHP, ok := lastBattleRes.RemainingHP[c.ID]; ok {
				if remHP <= 0 {
					c.Stats.HP = 1 // Fallen members survive with 1 HP
				} else {
					c.Stats.HP = remHP
					if c.Stats.MaxHP > 0 && c.Stats.HP > c.Stats.MaxHP {
						c.Stats.HP = c.Stats.MaxHP
					}
				}
			} else {
				isFallen := false
				for _, fallenID := range lastBattleRes.AlliesFallen {
					if fallenID == c.ID {
						isFallen = true
						break
					}
				}
				if isFallen {
					c.Stats.HP = 1
				}
			}

			if err := s.charRepo.Update(txCtx, c); err != nil {
				return err
			}

			// Award item drops from Floor 11 treasure boxes if examined
			var drops []coreitem.Instance
			if crawlResult.Outcome == battle.OutcomeWin && s.invRepo != nil {
				for _, box := range crawlResult.TreasureBoxes {
					if box.OpenedBy == c.ID && box.ItemID != "" {
						inst, err := coreitem.NewInstance(box.ItemID, 1)
						if err == nil {
							inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, c.ID)
							if err == nil {
								_ = inv.Add(inst)
								_ = s.invRepo.Save(txCtx, inv)
								drops = append(drops, inst)
							}
						}
					}
				}
			}

			rewardSummaries = append(rewardSummaries, MemberRewardSummary{
				CharacterID: c.ID,
				Name:        c.Name,
				GainedEXP:   gainedEXP,
				GainedGold:  gainedGold,
				LevelBefore: levelBefore,
				LevelAfter:  c.Level,
				Drops:       drops,
			})
		}

		// 8. Save Adventure Log
		detailsJSON, _ := json.Marshal(lastBattleRes)
		synergyBonus := (len(members) - 1) * 10
		advLog := PartyAdventureLog{
			ID:                  id.New(),
			PartyID:             partyID,
			StageID:             p.StageID,
			Outcome:             outcome,
			Turns:               crawlResult.TotalTurns,
			TotalEXP:            crawlResult.TotalEXP,
			TotalGold:           crawlResult.TotalGold,
			SynergyBonusPercent: synergyBonus,
			DetailsJSON:         string(detailsJSON),
			CreatedAt:           time.Now().UTC(),
		}
		if err := s.repo.SaveAdventureLog(txCtx, advLog); err != nil {
			return fmt.Errorf("save party adventure log: %w", err)
		}

		// 9. Reset party status & ready states for members
		for _, m := range members {
			_ = s.repo.UpdateMemberReady(txCtx, partyID, m.CharacterID, false)
		}

		result = PartyAdventureResult{
			PartyID:             partyID,
			StageID:             p.StageID,
			Outcome:             outcome,
			FloorsCleared:       crawlResult.FloorsCleared,
			Turns:               crawlResult.TotalTurns,
			TotalEXP:            crawlResult.TotalEXP,
			TotalGold:           crawlResult.TotalGold,
			SynergyBonusPercent: synergyBonus,
			Rewards:             rewardSummaries,
			TreasureBoxes:       crawlResult.TreasureBoxes,
			BattleResult:        lastBattleRes,
		}

		return nil
	})
	if err != nil {
		return PartyAdventureResult{}, err
	}

	if result.Outcome == string(battle.OutcomeWin) && s.victoryHook != nil {
		charIDs := make([]string, len(result.Rewards))
		for i, r := range result.Rewards {
			charIDs[i] = r.CharacterID
		}
		_ = s.victoryHook(ctx, charIDs, defeatedMonsterCount, result.TotalGold)
	}

	return result, nil
}

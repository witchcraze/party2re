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
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

// StartPartyAdventure starts and resolves a multiplayer 10-floor dungeon crawl with Floor 11 treasure room (vs_monster.cgi).
//
// Concurrency & Persistence Boundary:
//  1. Acquire exclusive party adventure distributed lock in Valkey Master (WithPartyAdventureLock, Rank 0).
//  2. Begin MariaDB transaction (runInTx) acquiring Rank 2 locks on all participating
//     character rows in ascending ID order to prevent deadlock.
//  3. Validate party status (StatusRecruiting) and transition to StatusInProgress in Valkey.
//  4. Execute 10-floor crawl and Floor 11 treasure room in memory.
//  5. Atomically commit character updates, inventory items, and durable party_adventure_logs in MariaDB.
//  6. Reset party status to StatusRecruiting and member ready states to false.
func (s *Service) StartPartyAdventure(ctx context.Context, partyID, leaderCharID string) (PartyAdventureResult, error) {
	var result PartyAdventureResult
	var defeatedMonsterCount int

	err := s.repo.WithPartyAdventureLock(ctx, partyID, func(lockedCtx context.Context) error {
		return s.runInTx(lockedCtx, func(txCtx context.Context) error {
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
			if s.activityCheck != nil {
				for _, cID := range charIDs {
					if err := s.activityCheck(txCtx, cID); err != nil {
						return err
					}
				}
			}

			// 4. Resolve stage and verify stage access eligibility (including seasonal restrictions)
			stage, err := s.stages.FindByID(p.StageID)
			if err != nil {
				return ErrStageNotFound
			}
			leaderChar := charMap[leaderCharID]
			if err := s.stages.CanAccessStage(leaderChar, stage.ID); err != nil {
				return err
			}
			if stage.ID == "stage-26" && s.timer != nil {
				locked, err := s.timer.IsLocked(txCtx, timer.CategoryDungeonOnce, leaderChar.ID)
				if err != nil {
					return err
				}
				if locked {
					return fmt.Errorf("%w: %sはすでに本日の限定ダンジョン探索を終えています。自宅で睡眠をとると再度挑戦できます", ErrDailyOnceAlreadyUsed, leaderChar.Name)
				}
			}

			// Transition party status to in_progress in Valkey
			p.Status = StatusInProgress
			if err := s.repo.UpdateParty(txCtx, p); err != nil {
				return err
			}
			defer func() {
				// If transaction aborts before success, reset status back to recruiting
				if result.Outcome == "" {
					p.Status = StatusRecruiting
					//lint:ignore error-swallow compensatory rollback in defer
					_ = s.repo.UpdateParty(txCtx, p)
					if stage.ID == "stage-26" && s.timer != nil {
						//lint:ignore error-swallow compensatory unlock on aborted transaction
						_ = s.timer.ReleaseLock(txCtx, timer.CategoryDungeonOnce, leaderChar.ID)
					}
				}
			}()

			// 5. Build participating characters in party member order
			participatingChars := make([]corecharacter.Character, len(members))
			for i, m := range members {
				participatingChars[i] = charMap[m.CharacterID]
			}

			// 6. Execute 10-Floor Dungeon Crawl & Floor 11 Treasure Room
			var session *adventure.CrawlSession
			if s.participantBuilder != nil {
				participants := make([]battle.Participant, len(participatingChars))
				for i, c := range participatingChars {
					part, err := s.participantBuilder.BuildParticipant(ctx, c.ID)
					if err != nil {
						return err
					}
					participants[i] = part
				}
				session, err = adventure.NewCrawlSessionWithParticipants(stage, participatingChars, participants, s.rng)
			} else {
				session, err = adventure.NewCrawlSession(stage, participatingChars, s.rng)
			}
			if err != nil {
				return err
			}
			session.LeaderID = leaderCharID
			if p.Mode != "" {
				if err := session.SetMode(p.Mode); err != nil {
					return err
				}
			}

			if s.blessingProvider != nil {
				actorID := session.LeaderID
				if actorID == "" && len(participatingChars) > 0 {
					actorID = participatingChars[0].ID
				}
				if actorID != "" {
					b, err := s.blessingProvider.GetActiveBlessing(ctx, actorID)
					if err != nil {
						return fmt.Errorf("get active blessing: %w", err)
					}
					if adventure.IsTreasureBlessing(b) {
						session.HasTreasureBlessing = true
					}
				}
			}

			if stage.ID == "stage-26" && s.timer != nil {
				ok, err := s.timer.TryLock(txCtx, timer.CategoryDungeonOnce, leaderChar.ID, 0)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("%w: %sはすでに本日の限定ダンジョン探索を終えています。自宅で睡眠をとると再度挑戦できます", ErrDailyOnceAlreadyUsed, leaderChar.Name)
				}
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
				// Only open base allowance (1 box per surviving participant).
				// Additional chest opening using ItemTreasurePocket requires explicit player choice (#948/#1330).
				for _, c := range participatingChars {
					_, _ = session.ExamineTreasure(c.ID)
				}
			}

			crawlResult := session.Result()

			synergyBonus := (len(members) - 1) * 10
			if synergyBonus < 0 {
				synergyBonus = 0
			} else if synergyBonus > 30 {
				synergyBonus = 30
			}

			// 7. Post-Battle Settlement: rewards, surviving HP/MP, and Floor 11 treasure drops
			outcome := string(crawlResult.Outcome)
			if crawlResult.Outcome == battle.OutcomeWin {
				defeatedMonsterCount = crawlResult.FloorsCleared
			}

			rewardSummaries, lostDrops, err := s.settlePostBattle(txCtx, members, charMap, &crawlResult, lastBattleRes)
			if err != nil {
				return err
			}

			// 8. Save Adventure Log
			detailsJSON, _ := json.Marshal(lastBattleRes)
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
			p.Status = StatusRecruiting
			if err := s.repo.UpdateParty(txCtx, p); err != nil {
				return fmt.Errorf("reset party status: %w", err)
			}
			for _, m := range members {
				if err := s.repo.UpdateMemberReady(txCtx, partyID, m.CharacterID, false); err != nil {
					return fmt.Errorf("reset member ready state: %w", err)
				}
			}

			result = PartyAdventureResult{
				PartyID:             partyID,
				StageID:             p.StageID,
				Outcome:             outcome,
				Round:               crawlResult.Round,
				ExitFatigue:         crawlResult.EffectiveExitFatigue(p.Mode),
				FloorsCleared:       crawlResult.FloorsCleared,
				Turns:               crawlResult.TotalTurns,
				TotalEXP:            crawlResult.TotalEXP,
				TotalGold:           crawlResult.TotalGold,
				TotalCrystals:       crawlResult.TotalCrystals,
				SynergyBonusPercent: synergyBonus,
				Rewards:             rewardSummaries,
				TreasureBoxes:       crawlResult.TreasureBoxes,
				BattleResult:        lastBattleRes,
				LostDrops:           lostDrops,
			}

			return nil
		})
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

	if s.postAdventureHook != nil {
		for _, r := range result.Rewards {
			_ = s.postAdventureHook(ctx, r.CharacterID)
		}
	}

	return result, nil
}

// SetPostAdventureHook sets the PostAdventureHook on an existing service.
func (s *Service) SetPostAdventureHook(hook PostAdventureHook) {
	s.postAdventureHook = hook
}

// SetBlessingProvider sets the BlessingProvider on an existing service.
func (s *Service) SetBlessingProvider(provider BlessingProvider) {
	s.blessingProvider = provider
}

// SetTimerService sets the TimerService on an existing service.
func (s *Service) SetTimerService(timer TimerService) {
	s.timer = timer
}

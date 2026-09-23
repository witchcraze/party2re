package contest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// SettleContest concludes the active contest round, awards prizes to winners and voters,
// records the 1st-place entry to Hall of Fame, and rolls over to the next contest round.
func (s *Service) SettleContest(ctx context.Context, force bool) (SettlementResult, error) {
	var result SettlementResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		activeRound, err := s.contests.GetActiveRoundForUpdate(txCtx)
		if err != nil {
			return err
		}

		now := s.nowFunc()
		if !force && now.Before(activeRound.EndTime) {
			return ErrContestNotReadyToSettle
		}

		entries, err := s.contests.ListEntriesByRound(txCtx, activeRound.Round)
		if err != nil {
			return err
		}

		result.Round = activeRound.Round

		// If minimum entries not reached, postpone and extend duration
		if len(entries) < MinEntriesForContest {
			activeRound.EndTime = activeRound.EndTime.Add(ContestCycleDays * 24 * time.Hour)
			activeRound.UpdatedAt = now
			if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
				return err
			}

			result.Postponed = true
			result.ExtendedUntil = activeRound.EndTime
			result.NextRound = activeRound.Round
			result.NextRoundEndTime = activeRound.EndTime
			result.Message = "Contest postponed and extended due to insufficient entries"
			return nil
		}

		// Lock preparing round upfront (along with activeRound) to strictly adhere to global lock hierarchy
		prepRound, prepErr := s.contests.GetPreparingRoundForUpdate(txCtx)

		// Sort entries descending by Votes, then ascending by CreatedAt
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Votes != entries[j].Votes {
				return entries[i].Votes > entries[j].Votes
			}
			return entries[i].CreatedAt.Before(entries[j].CreatedAt)
		})

		// Assign ranks and distribute prizes to top 3
		prizes := []Prize{PrizeFirst, PrizeSecond, PrizeThird}
		for i := range entries {
			rank := i + 1
			entries[i].Ranking = rank
			if err := s.contests.SaveEntry(txCtx, entries[i]); err != nil {
				return err
			}

			if i < len(prizes) {
				prize := prizes[i]
				char, err := s.characters.FindByIDForUpdate(txCtx, entries[i].CharacterID)
				if err != nil {
					if errors.Is(err, corecharacter.ErrNotFound) {
						continue
					}
					return err
				}

				if err := char.AddMoney(prize.Gold); err != nil {
					return err
				}
				if err := char.AddSmallMedals(prize.SmallMedals); err != nil {
					return err
				}
				if err := s.characters.Update(txCtx, char); err != nil {
					return err
				}

				if s.guilds != nil {
					if err := s.guilds.AddGuildPoints(txCtx, entries[i].CharacterID, prize.GuildPoints); err != nil {
						return err
					}
				}

				if s.news != nil {
					newsContent := fmt.Sprintf("★第%d回フォトコンテスト%d位 %s★", activeRound.Round, rank, entries[i].CharacterName)
					_ = s.news.PublishNews(txCtx, "contest", "フォトコンテスト結果発表", newsContent, "ワコール", now)
				}
			}
		}

		// Archive 1st place winner to Hall of Fame / Legend
		if len(entries) > 0 {
			winner := entries[0]
			legend := ContestLegend{
				Round:         activeRound.Round,
				EntryID:       winner.ID,
				Title:         winner.Title,
				CharacterID:   winner.CharacterID,
				CharacterName: winner.CharacterName,
				GuildName:     winner.GuildName,
				Votes:         winner.Votes,
				ImageURL:      winner.ImageURL,
				Caption:       winner.Caption,
				SettledAt:     now,
			}
			if err := s.contests.SaveLegend(txCtx, legend); err != nil {
				return err
			}
			result.WinnerLegend = &legend

			// Distribute voter bonus medals to characters who voted for 1st place
			votes, err := s.contests.ListVotesByEntryID(txCtx, winner.ID)
			if err != nil {
				return err
			}
			voterCount := 0
			for _, v := range votes {
				voterChar, err := s.characters.FindByIDForUpdate(txCtx, v.VoterCharacterID)
				if err != nil {
					if errors.Is(err, corecharacter.ErrNotFound) {
						continue
					}
					return err
				}
				if err := voterChar.AddSmallMedals(VoterBonusSmallMedals); err != nil {
					return err
				}
				if err := s.characters.Update(txCtx, voterChar); err != nil {
					return err
				}
				voterCount++
			}
			result.VotersRewarded = voterCount
		}

		// Mark active round as settled
		activeRound.Status = StatusSettled
		activeRound.UpdatedAt = now
		if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
			return err
		}

		// Promote preparing round to active round and create next preparing round
		if prepErr == nil {
			prepRound.Status = StatusActive
			prepRound.StartTime = now
			prepRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
			prepRound.UpdatedAt = now
			if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
				return err
			}

			nextPrep := ContestRound{
				Round:     prepRound.Round + 1,
				Status:    StatusPreparing,
				StartTime: prepRound.EndTime,
				EndTime:   prepRound.EndTime.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextPrep); err != nil {
				return err
			}

			result.NextRound = prepRound.Round
			result.NextRoundEndTime = prepRound.EndTime
		} else {
			// If no preparing round existed, create next active and preparing rounds
			nextActive := ContestRound{
				Round:     activeRound.Round + 1,
				Status:    StatusActive,
				StartTime: now,
				EndTime:   now.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextActive); err != nil {
				return err
			}

			nextPrep := ContestRound{
				Round:     activeRound.Round + 2,
				Status:    StatusPreparing,
				StartTime: nextActive.EndTime,
				EndTime:   nextActive.EndTime.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextPrep); err != nil {
				return err
			}

			result.NextRound = nextActive.Round
			result.NextRoundEndTime = nextActive.EndTime
		}

		result.Rankings = entries
		result.PrizesDistributed = true
		result.Message = fmt.Sprintf("Contest round %d successfully settled with %d entries", activeRound.Round, len(entries))
		return nil
	})
	if err != nil {
		return SettlementResult{}, err
	}

	return result, nil
}

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
		now := s.nowFunc()
		activeRound, activeErr := s.contests.GetActiveRoundForUpdate(txCtx)
		if activeErr != nil && !errors.Is(activeErr, ErrContestNotFound) {
			return activeErr
		}

		// Case 1: No active round currently exists (e.g. initial round is preparing)
		if errors.Is(activeErr, ErrContestNotFound) {
			prepRound, prepErr := s.contests.GetPreparingRoundForUpdate(txCtx)
			if prepErr != nil {
				if errors.Is(prepErr, ErrContestNotFound) {
					p, err := s.ensurePreparingRoundInTx(txCtx)
					if err != nil {
						return err
					}
					prepRound = p
				} else {
					return prepErr
				}
			}

			if !force && now.Before(prepRound.EndTime) {
				return ErrContestNotReadyToSettle
			}

			entries, err := s.contests.ListEntriesByRound(txCtx, prepRound.Round)
			if err != nil {
				return err
			}

			result.Round = prepRound.Round
			if len(entries) < MinEntriesForContest {
				prepRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
				prepRound.UpdatedAt = now
				if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
					return err
				}

				result.Postponed = true
				result.ExtendedUntil = prepRound.EndTime
				result.NextRound = prepRound.Round
				result.NextRoundEndTime = prepRound.EndTime
				result.Message = "Contest postponed and extended due to insufficient entries"
				return nil
			}

			// Preparing round reached quorum (5+ entries): promote to active
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
			result.Message = fmt.Sprintf("Contest round %d successfully started with %d entries", prepRound.Round, len(entries))
			return nil
		}

		// Case 2: Active round exists
		if !force && now.Before(activeRound.EndTime) {
			return ErrContestNotReadyToSettle
		}

		entries, err := s.contests.ListEntriesByRound(txCtx, activeRound.Round)
		if err != nil {
			return err
		}

		result.Round = activeRound.Round

		// Lock preparing round upfront (along with activeRound) to strictly adhere to global lock hierarchy
		prepRound, prepErr := s.contests.GetPreparingRoundForUpdate(txCtx)
		if prepErr != nil && !errors.Is(prepErr, ErrContestNotFound) {
			return prepErr
		}
		var prepEntries []ContestEntry
		if prepErr == nil {
			var prepListErr error
			prepEntries, prepListErr = s.contests.ListEntriesByRound(txCtx, prepRound.Round)
			if prepListErr != nil {
				return prepListErr
			}
		}

		// Case 2a: Active round has 0 submissions (empty active round)
		if len(entries) == 0 {
			if prepErr == nil && len(prepEntries) >= MinEntriesForContest {
				// Recover from empty active round:
				// Settle active round without prizes.
				activeRound.Status = StatusSettled
				activeRound.UpdatedAt = now
				if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
					return err
				}

				// Promote preparing round to active
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
				result.Message = fmt.Sprintf("Contest round %d successfully started from preparing entries with %d entries", prepRound.Round, len(prepEntries))
				return nil
			}

			// Preparing round also under quorum or absent: postpone and extend duration
			activeRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
			activeRound.UpdatedAt = now
			if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
				return err
			}
			if prepErr == nil {
				prepRound.EndTime = activeRound.EndTime
				prepRound.UpdatedAt = now
				if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
					return err
				}
			}

			result.Postponed = true
			result.ExtendedUntil = activeRound.EndTime
			result.NextRound = activeRound.Round
			result.NextRoundEndTime = activeRound.EndTime
			result.Message = "Contest postponed and extended due to insufficient entries"
			return nil
		}

		// Case 2b: Active round has 1 to 4 submissions (under quorum)
		if len(entries) < MinEntriesForContest {
			// If preparing round also under quorum or absent:
			// Postpone and extend active round and preparing round, retaining submissions.
			if prepErr != nil || len(prepEntries) < MinEntriesForContest {
				activeRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
				activeRound.UpdatedAt = now
				if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
					return err
				}
				if prepErr == nil {
					prepRound.EndTime = activeRound.EndTime
					prepRound.UpdatedAt = now
					if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
						return err
					}
				}

				result.Postponed = true
				result.ExtendedUntil = activeRound.EndTime
				result.NextRound = activeRound.Round
				result.NextRoundEndTime = activeRound.EndTime
				result.Message = "Contest postponed and extended due to insufficient entries"
				return nil
			}

			// Preparing round reached quorum (5+ entries):
			// Conclude active round with prize distribution to preserve existing submissions and avoid infinite
			// postponement (submissions enter preparing, so active cannot grow). Hall of Fame archiving is omitted
			// because active entries < 5 (matching legacy contest.cgi:186), and preparing round advances to active.
		}

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

				if s.guilds != nil && entries[i].GuildID != "" {
					if err := s.guilds.AddPoints(txCtx, entries[i].GuildID, int64(prize.GuildPoints)); err != nil {
						return err
					}
				}

				if s.news != nil {
					newsContent := fmt.Sprintf("★第%d回フォトコンテスト%d位 %s★", activeRound.Round, rank, entries[i].CharacterName)
					_ = s.news.PublishNews(txCtx, "contest", "フォトコンテスト結果発表", newsContent, "ワコール", now)
				}
			}
		}

		// Archive 1st place winner to Hall of Fame / Legend (requires quorum 5+ entries, matching legacy contest.cgi:186)
		if len(entries) >= MinEntriesForContest {
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
		}

		// Distribute voter bonus medals to characters who voted for 1st place
		if len(entries) > 0 {
			winner := entries[0]
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

		// Next round handling: check if preparing round reached quorum
		if prepErr == nil {
			if len(prepEntries) >= MinEntriesForContest {
				// Promote preparing round to active round and create next preparing round
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
				// Less than quorum in preparing round: retain entries and extend deadline
				prepRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
				prepRound.UpdatedAt = now
				if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
					return err
				}

				result.NextRound = prepRound.Round
				result.NextRoundEndTime = prepRound.EndTime
			}
		} else {
			// If no preparing round existed, create next preparing round
			nextPrep := ContestRound{
				Round:     activeRound.Round + 1,
				Status:    StatusPreparing,
				StartTime: now,
				EndTime:   now.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextPrep); err != nil {
				return err
			}

			result.NextRound = nextPrep.Round
			result.NextRoundEndTime = nextPrep.EndTime
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

// EnsurePreparingRound ensures a preparing round exists and returns it.
func (s *Service) EnsurePreparingRound(ctx context.Context) (ContestRound, error) {
	var prepRound ContestRound
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.ensurePreparingRoundInTx(txCtx)
		if err != nil {
			return err
		}
		prepRound = p
		return nil
	})
	if err != nil {
		return ContestRound{}, err
	}
	return prepRound, nil
}

func (s *Service) ensurePreparingRoundInTx(txCtx context.Context) (ContestRound, error) {
	p, err := s.contests.GetPreparingRoundForUpdate(txCtx)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, ErrContestNotFound) {
		return ContestRound{}, err
	}

	now := s.nowFunc()
	nextRoundNum := 1
	startTime := now

	activeRound, activeErr := s.contests.GetActiveRoundForUpdate(txCtx)
	if activeErr == nil {
		nextRoundNum = activeRound.Round + 1
		startTime = activeRound.EndTime
	} else if !errors.Is(activeErr, ErrContestNotFound) {
		return ContestRound{}, activeErr
	} else {
		settled, settledErr := s.contests.GetLatestSettledRound(txCtx)
		if settledErr == nil {
			nextRoundNum = settled.Round + 1
		} else if !errors.Is(settledErr, ErrContestNotFound) {
			return ContestRound{}, settledErr
		}
	}

	prepRound := ContestRound{
		Round:     nextRoundNum,
		Status:    StatusPreparing,
		StartTime: startTime,
		EndTime:   startTime.Add(ContestCycleDays * 24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
		return ContestRound{}, err
	}
	return prepRound, nil
}

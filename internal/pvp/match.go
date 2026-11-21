package pvp

import (
	"context"
	"errors"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// StartMatch initiates round 1 of the Colosseum battle (@かいし).
func (s *Service) StartMatch(ctx context.Context, leaderID string, roomID string) (RoomDetail, error) {
	var result RoomDetail
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.checkInactiveLocked(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.LeaderCharacterID != leaderID {
			return ErrNotRoomLeader
		}
		if detail.Room.Status != StatusRecruiting || detail.Room.Round > 0 {
			return ErrMatchAlreadyStarted
		}
		if len(detail.Members) < MinMembers {
			return ErrNotEnoughParticipants
		}

		teams := make(map[string]bool)
		for _, m := range detail.Members {
			if m.TeamColor == "" {
				return ErrTeamsNotConfigured
			}
			teams[m.TeamColor] = true
		}
		if len(teams) < 2 {
			return ErrNeedAtLeastTwoTeams
		}

		// Recover HP of all participants to MaxHP
		for i := range detail.Members {
			detail.Members[i].HP = detail.Members[i].MaxHP
		}

		now := s.now()
		detail.Room.Status = StatusInProgress
		detail.Room.Round = 1
		detail.Room.UpdatedAt = now
		detail.Room.LastActivityAt = now

		if err := s.transitions.TouchActivity(lockedCtx, roomID, detail.Room.FinancialVersion, now); err != nil {
			if errors.Is(err, ErrRoomInactive) {
				if _, cleanErr := s.cleanupInactiveRoom(lockedCtx, lastTransitionFromContext(lockedCtx), now); cleanErr != nil {
					return cleanErr
				}
				return ErrRoomNotFound
			}
			return err
		}
		if err := s.saveLiveRoom(lockedCtx, detail); err != nil {
			return err
		}
		result = detail
		return nil
	})
	return result, err
}

// AdvanceRound executes combat for the active round and advances the match (@かいし).
func (s *Service) AdvanceRound(ctx context.Context, leaderID string, roomID string) (RoundResolution, error) {
	var result RoundResolution
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		last := lastTransitionFromContext(lockedCtx)
		if last.Result != nil && last.Result.MatchCompleted && last.Detail.Room.Status != StatusDisbanded {
			if s.isInactive(last.LastActivityAt, s.now()) {
				if _, roomErr := s.repo.GetRoom(lockedCtx, roomID); roomErr != nil && !errors.Is(roomErr, ErrRoomNotFound) {
					return roomErr
				}
				if _, cleanErr := s.cleanupInactiveRoom(lockedCtx, last, s.now()); cleanErr != nil {
					return cleanErr
				}
				return ErrRoomNotFound
			}
			if last.Detail.Room.LeaderCharacterID != leaderID {
				return ErrNotRoomLeader
			}
			result = *last.Result
			return nil
		}
		detail, err := s.checkInactiveLocked(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.LeaderCharacterID != leaderID {
			return ErrNotRoomLeader
		}
		if detail.Room.Status != StatusInProgress {
			return ErrMatchNotInProgress
		}

		// Group living members by team
		teamMembers := make(map[string][]RoomMember)
		for _, m := range detail.Members {
			teamMembers[m.TeamColor] = append(teamMembers[m.TeamColor], m)
		}

		leaderTeam := ""
		for _, m := range detail.Members {
			if m.CharacterID == leaderID {
				leaderTeam = m.TeamColor
				break
			}
		}

		var otherTeam string
		for _, m := range detail.Members {
			if m.TeamColor != leaderTeam {
				otherTeam = m.TeamColor
				break
			}
		}

		// Build battle participants for Leader's Team (Allies) vs Opposing Team (Enemies)
		var allies, enemies []corebattle.Participant
		for _, m := range detail.Members {
			var p corebattle.Participant
			if s.participantBuilder != nil {
				var err error
				p, err = s.participantBuilder.BuildParticipantWithCurrentHP(lockedCtx, m.CharacterID, m.HP)
				if err != nil {
					return err
				}
			} else {
				char, err := s.characters.FindByID(lockedCtx, m.CharacterID)
				if err != nil {
					return err
				}
				p = corebattle.MustNewParticipant(char.ID, char.Stats.HP, char.Stats.Attack, char.Stats.Defense)
				p.Name = char.Name
				p.MaxHP = char.Stats.MaxHP
				p.MP = char.Stats.MP
				p.MaxMP = char.Stats.MaxMP
				p.Agility = char.Stats.Agility
			}
			if m.MaxMP != nil {
				p.MaxHP, p.MaxMP = m.MaxHP, *m.MaxMP
			}
			if m.MP != nil {
				p.MP = max(0, *m.MP)
			}
			if m.HP > 0 {
				p.HP = m.HP
			} else if p.HP <= 0 {
				p.HP = m.MaxHP
			}
			p.TeamID = m.TeamColor
			if m.TeamColor == leaderTeam {
				allies = append(allies, p)
			} else {
				enemies = append(enemies, p)
			}
		}

		var res corebattle.PartyBattleResult
		var roundWinnerTeam string
		var roundOutcome string

		if len(enemies) == 0 {
			// Legacy vs_player.cgi awards the round win when only one team remains.
			roundWinnerTeam = leaderTeam
			roundOutcome = "round_win"
		} else {
			req := corebattle.PartyBattleRequest{
				Allies:  allies,
				Enemies: enemies,
			}

			var err error
			res, err = s.battleEngine.ResolvePartyBattle(req)
			if err != nil {
				return fmt.Errorf("resolve pvp battle round: %w", err)
			}

			if len(res.RemainingHP) > 0 {
				aliveTeams := make(map[string]bool)
				for _, m := range detail.Members {
					if hp, ok := res.RemainingHP[m.CharacterID]; ok && hp > 0 {
						aliveTeams[m.TeamColor] = true
					}
				}

				if len(aliveTeams) == 1 {
					for team := range aliveTeams {
						roundWinnerTeam = team
					}
					roundOutcome = "round_win"
				} else {
					roundOutcome = "draw"
				}
			} else {
				if res.Outcome == corebattle.OutcomeWin {
					roundWinnerTeam = leaderTeam
					roundOutcome = "round_win"
				} else if res.Outcome == corebattle.OutcomeDefeat {
					roundWinnerTeam = otherTeam
					roundOutcome = "round_win"
				} else {
					roundOutcome = "draw"
				}
			}

			for i := range detail.Members {
				if remMP, ok := res.RemainingMP[detail.Members[i].CharacterID]; ok {
					mp := max(0, remMP)
					if maxMP := detail.Members[i].MaxMP; maxMP != nil && *maxMP > 0 {
						mp = min(mp, *maxMP)
					}
					detail.Members[i].MP = &mp
				}
			}
		}

		if detail.Room.TeamScores == nil {
			detail.Room.TeamScores = make(map[string]int)
		}

		matchCompleted := false
		var overallWinner string
		var prizePerMember int
		var awardedIDs []string
		var forfeitedPool int

		if roundWinnerTeam != "" {
			detail.Room.TeamScores[roundWinnerTeam]++
			if detail.Room.TeamScores[roundWinnerTeam] >= detail.Room.TargetWins {
				matchCompleted = true
				overallWinner = roundWinnerTeam
				detail.Room.Status = StatusCompleted
				detail.Room.WinnerTeam = roundWinnerTeam

				// Calculate and distribute prize pool to winning team members
				var winIDs []string
				for _, m := range detail.Members {
					if m.TeamColor == roundWinnerTeam {
						winIDs = append(winIDs, m.CharacterID)
					}
				}

				beforePool := detail.Room.PrizePool
				if len(winIDs) > 0 {
					prizePerMember = beforePool / len(winIDs)
					awardedIDs = sortCharacterIDs(winIDs)
				}
				forfeitedPool = beforePool - len(awardedIDs)*prizePerMember
				detail.Room.PrizePerMember = prizePerMember
				detail.Room.PrizePool = 0
			}
		}

		// Forced end if round > 10 without decider (no bet refund per legacy vs_player.cgi)
		if !matchCompleted && detail.Room.Round >= MaxRounds {
			matchCompleted = true
			roundOutcome = "match_draw"
			detail.Room.Status = StatusCompleted
			forfeitedPool = detail.Room.PrizePool
			detail.Room.PrizePool = 0
		}

		// Advance to next round if match not completed
		if !matchCompleted {
			detail.Room.Round++
			for i := range detail.Members {
				detail.Members[i].HP = detail.Members[i].MaxHP
			}
		}

		now := s.now()
		detail.Room.UpdatedAt = now
		detail.Room.LastActivityAt = now
		result = RoundResolution{
			Round:               detail.Room.Round,
			Outcome:             roundOutcome,
			WinnerTeam:          roundWinnerTeam,
			WinnerTeamName:      TeamColorName(roundWinnerTeam),
			TeamScores:          detail.Room.TeamScores,
			MatchCompleted:      matchCompleted,
			OverallWinnerTeam:   overallWinner,
			PrizePerMember:      prizePerMember,
			AwardedCharacterIDs: awardedIDs,
			Turns:               res.Turns,
			BattleLog:           res.Logs,
		}
		if matchCompleted {
			if err := s.commitTransition(lockedCtx, Transition{Kind: "settle", ActorID: leaderID, Detail: detail, Result: &result, ForfeitedPool: forfeitedPool, LastActivityAt: now}, func(txCtx context.Context) error {
				if err := s.awardPrizes(txCtx, awardedIDs, detail.Room.Name, prizePerMember); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		} else {
			if err := s.transitions.TouchActivity(lockedCtx, roomID, detail.Room.FinancialVersion, now); err != nil {
				if errors.Is(err, ErrRoomInactive) {
					if _, cleanErr := s.cleanupInactiveRoom(lockedCtx, lastTransitionFromContext(lockedCtx), now); cleanErr != nil {
						return cleanErr
					}
					return ErrRoomNotFound
				}
				return err
			}
			if err := s.saveLiveRoom(lockedCtx, detail); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

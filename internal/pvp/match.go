package pvp

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// StartMatch initiates round 1 of the Colosseum battle (@かいし).
func (s *Service) StartMatch(ctx context.Context, leaderID string, roomID string) (RoomDetail, error) {
	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return RoomDetail{}, err
	}
	if detail.Room.LeaderCharacterID != leaderID {
		return RoomDetail{}, ErrNotRoomLeader
	}
	if detail.Room.Status != StatusRecruiting || detail.Room.Round > 0 {
		return RoomDetail{}, ErrMatchAlreadyStarted
	}
	if len(detail.Members) < MinMembers {
		return RoomDetail{}, ErrNotEnoughParticipants
	}

	teams := make(map[string]bool)
	for _, m := range detail.Members {
		if m.TeamColor == "" {
			return RoomDetail{}, ErrTeamsNotConfigured
		}
		teams[m.TeamColor] = true
	}
	if len(teams) < 2 {
		return RoomDetail{}, ErrNeedAtLeastTwoTeams
	}

	// Recover HP of all participants to MaxHP
	for i := range detail.Members {
		detail.Members[i].HP = detail.Members[i].MaxHP
	}

	now := time.Now().UTC()
	detail.Room.Status = StatusInProgress
	detail.Room.Round = 1
	detail.Room.UpdatedAt = now

	if err := s.repo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		return RoomDetail{}, err
	}
	return detail, nil
}

// AdvanceRound executes combat for the active round and advances the match (@かいし).
func (s *Service) AdvanceRound(ctx context.Context, leaderID string, roomID string) (RoundResolution, error) {
	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return RoundResolution{}, err
	}
	if detail.Room.LeaderCharacterID != leaderID {
		return RoundResolution{}, ErrNotRoomLeader
	}
	if detail.Room.Status != StatusInProgress {
		return RoundResolution{}, ErrMatchNotInProgress
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
		char, err := s.characters.FindByID(ctx, m.CharacterID)
		if err != nil {
			return RoundResolution{}, err
		}
		p := corebattle.NewParticipantFromCharacter(char)
		p.HP = m.HP
		if p.HP <= 0 {
			p.HP = m.MaxHP
		}
		if m.TeamColor == leaderTeam {
			allies = append(allies, p)
		} else {
			enemies = append(enemies, p)
		}
	}

	req := corebattle.PartyBattleRequest{
		Allies:  allies,
		Enemies: enemies,
	}

	res, err := s.battleEngine.ResolvePartyBattle(req)
	if err != nil {
		return RoundResolution{}, fmt.Errorf("resolve pvp battle round: %w", err)
	}

	var roundWinnerTeam string
	var roundOutcome string
	if res.Outcome == corebattle.OutcomeWin {
		roundWinnerTeam = leaderTeam
		roundOutcome = "round_win"
	} else if res.Outcome == corebattle.OutcomeDefeat {
		roundWinnerTeam = otherTeam
		roundOutcome = "round_win"
	} else {
		roundOutcome = "draw"
	}

	if detail.Room.TeamScores == nil {
		detail.Room.TeamScores = make(map[string]int)
	}

	matchCompleted := false
	var overallWinner string
	var prizePerMember int
	var awardedIDs []string

	if roundWinnerTeam != "" {
		detail.Room.TeamScores[roundWinnerTeam]++
		if detail.Room.TeamScores[roundWinnerTeam] >= detail.Room.TargetWins {
			matchCompleted = true
			overallWinner = roundWinnerTeam
			detail.Room.Status = StatusCompleted
			detail.Room.WinnerTeam = roundWinnerTeam

			// Calculate and distribute prize pool to winning team members
			var winMembers []RoomMember
			for _, m := range detail.Members {
				if m.TeamColor == roundWinnerTeam {
					winMembers = append(winMembers, m)
				}
			}

			if len(winMembers) > 0 {
				prizePerMember = detail.Room.PrizePool / len(winMembers)
				for _, wm := range winMembers {
					if wChar, err := s.characters.FindByID(ctx, wm.CharacterID); err == nil {
						_ = wChar.AddMoney(prizePerMember)
						wChar.PvPWins++
						_ = s.characters.Update(ctx, wChar)
						awardedIDs = append(awardedIDs, wChar.ID)
						if s.victoryHook != nil {
							_ = s.victoryHook(ctx, wChar.ID, "")
						}
					}
				}
			}
			detail.Room.PrizePerMember = prizePerMember
			detail.Room.PrizePool = 0
		}
	}

	// Forced end if round > 10 without decider
	if !matchCompleted && detail.Room.Round >= MaxRounds {
		matchCompleted = true
		roundOutcome = "match_draw"
		detail.Room.Status = StatusCompleted
		// Refund remaining prize pool equally among all members
		if len(detail.Members) > 0 {
			refund := detail.Room.PrizePool / len(detail.Members)
			for _, m := range detail.Members {
				if mChar, err := s.characters.FindByID(ctx, m.CharacterID); err == nil {
					_ = mChar.AddMoney(refund)
					_ = s.characters.Update(ctx, mChar)
				}
			}
			detail.Room.PrizePool = 0
		}
	}

	// Advance to next round if match not completed
	if !matchCompleted {
		detail.Room.Round++
		for i := range detail.Members {
			detail.Members[i].HP = detail.Members[i].MaxHP
		}
	}

	detail.Room.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		return RoundResolution{}, err
	}

	return RoundResolution{
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
	}, nil
}

package gvg

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// StartMatch initiates round 1 of the GvG battle (@かいし, vs_guild.cgi:44-59).
func (s *Service) StartMatch(ctx context.Context, leaderID string, roomID string) (RoomDetail, error) {
	var result RoomDetail
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
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
			teams[m.GuildColor] = true
		}
		if len(teams) < 2 {
			return ErrNeedAtLeastTwoGuilds
		}

		// Recover HP of all participants to MaxHP for round 1
		for i := range detail.Members {
			detail.Members[i].HP = detail.Members[i].MaxHP
		}

		now := time.Now().UTC()
		detail.Room.Status = StatusInProgress
		detail.Room.Round = 1
		detail.Room.UpdatedAt = now

		if err := s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members); err != nil {
			return err
		}
		result = detail
		return nil
	})
	return result, err
}

// AdvanceRound executes combat for the active round and advances the match (@かいし, vs_guild.cgi:60-154).
func (s *Service) AdvanceRound(ctx context.Context, leaderID string, roomID string) (RoundResolution, error) {
	var result RoundResolution
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.LeaderCharacterID != leaderID {
			return ErrNotRoomLeader
		}
		if detail.Room.Status != StatusInProgress {
			return ErrMatchNotInProgress
		}

		leaderGuildColor := ""
		leaderGuildID := ""
		leaderGuildName := ""
		for _, m := range detail.Members {
			if m.CharacterID == leaderID {
				leaderGuildColor = m.GuildColor
				leaderGuildID = m.GuildID
				leaderGuildName = m.GuildName
				break
			}
		}

		var otherGuildID, otherGuildName, otherGuildColor string
		for _, m := range detail.Members {
			if m.GuildColor != leaderGuildColor {
				otherGuildID = m.GuildID
				otherGuildName = m.GuildName
				otherGuildColor = m.GuildColor
				break
			}
		}

		// Build battle participants: Leader's Guild (Allies) vs Opposing Guild(s) (Enemies)
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
			if m.HP > 0 {
				p.HP = m.HP
			} else if p.HP <= 0 {
				p.HP = m.MaxHP
			}
			p.TeamID = m.GuildID
			if m.GuildColor == leaderGuildColor {
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
			return fmt.Errorf("resolve gvg battle round: %w", err)
		}

		var roundWinnerGuildID string
		var roundWinnerGuildName string
		var roundWinnerGuildColor string
		var roundOutcome string

		if len(res.RemainingHP) > 0 {
			type guildInfo struct {
				id    string
				name  string
				color string
			}
			aliveGuilds := make(map[string]guildInfo)
			for _, m := range detail.Members {
				if hp, ok := res.RemainingHP[m.CharacterID]; ok && hp > 0 {
					aliveGuilds[m.GuildID] = guildInfo{
						id:    m.GuildID,
						name:  m.GuildName,
						color: m.GuildColor,
					}
				}
			}

			if len(aliveGuilds) == 1 {
				for _, g := range aliveGuilds {
					roundWinnerGuildID = g.id
					roundWinnerGuildName = g.name
					roundWinnerGuildColor = g.color
				}
				roundOutcome = "round_win"
			} else {
				roundOutcome = "draw"
			}
		} else {
			if res.Outcome == corebattle.OutcomeWin {
				roundWinnerGuildID = leaderGuildID
				roundWinnerGuildName = leaderGuildName
				roundWinnerGuildColor = leaderGuildColor
				roundOutcome = "round_win"
			} else if res.Outcome == corebattle.OutcomeDefeat {
				roundWinnerGuildID = otherGuildID
				roundWinnerGuildName = otherGuildName
				roundWinnerGuildColor = otherGuildColor
				roundOutcome = "round_win"
			} else {
				roundOutcome = "draw"
			}
		}

		if detail.Room.GuildScores == nil {
			detail.Room.GuildScores = make(map[string]int)
		}

		matchCompleted := false
		var overallWinnerGuildID string
		var overallWinnerGuildName string

		// Handle round win: +3 GP to the round winning guild (vs_guild.cgi:121)
		if roundWinnerGuildID != "" {
			if err := s.standings.AddRoundWinGP(lockedCtx, roundWinnerGuildID, RoundWinGP); err != nil {
				return err
			}
			detail.Room.GuildScores[roundWinnerGuildID]++

			if detail.Room.GuildScores[roundWinnerGuildID] >= detail.Room.TargetWins {
				matchCompleted = true
				overallWinnerGuildID = roundWinnerGuildID
				overallWinnerGuildName = roundWinnerGuildName
				roundOutcome = "match_won"
			}
		}

		// 10-round draw limit without winner (vs_guild.cgi:67, 114)
		if !matchCompleted && detail.Room.Round >= MaxRounds {
			matchCompleted = true
			roundOutcome = "match_draw"
		}

		if matchCompleted {
			// Settle match standings
			participantGP := make(map[string]int)
			guildIDsMap := make(map[string]bool)
			for _, m := range detail.Members {
				guildIDsMap[m.GuildID] = true
				participantGP[m.GuildID] += MatchParticipantGP // 4 GP per participant (vs_guild.cgi:187)
			}
			var guildIDs []string
			for gid := range guildIDsMap {
				guildIDs = append(guildIDs, gid)
			}

			settlement := MatchSettlement{
				WinnerGuildID: overallWinnerGuildID,
				WinnerPrizeGP: detail.Room.PrizePool,
				GuildIDs:      guildIDs,
				IsDraw:        overallWinnerGuildID == "",
				ParticipantGP: participantGP,
			}
			if err := s.standings.RecordMatchSettlement(lockedCtx, settlement); err != nil {
				return err
			}

			detail.Room.Status = StatusCompleted
			detail.Room.WinnerGuildID = overallWinnerGuildID
		} else {
			// Advance to next round and restore HP to MaxHP for all participants (vs_guild.cgi:142)
			detail.Room.Round++
			for i := range detail.Members {
				detail.Members[i].HP = detail.Members[i].MaxHP
			}
		}

		detail.Room.UpdatedAt = time.Now().UTC()
		if err := s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members); err != nil {
			return err
		}

		result = RoundResolution{
			Round:                  detail.Room.Round,
			Outcome:                roundOutcome,
			WinnerGuildID:          roundWinnerGuildID,
			WinnerGuildName:        roundWinnerGuildName,
			WinnerGuildColor:       roundWinnerGuildColor,
			GuildScores:            detail.Room.GuildScores,
			MatchCompleted:         matchCompleted,
			OverallWinnerGuildID:   overallWinnerGuildID,
			OverallWinnerGuildName: overallWinnerGuildName,
			PrizeGP:                detail.Room.PrizePool,
			Turns:                  res.Turns,
			BattleLog:              res.Logs,
		}
		return nil
	})
	return result, err
}

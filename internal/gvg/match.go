package gvg

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// StartMatch initiates round 1 of the GvG battle (@かいし, vs_guild.cgi:44-59).
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
		teams[m.GuildColor] = true
	}
	if len(teams) < 2 {
		return RoomDetail{}, ErrNeedAtLeastTwoGuilds
	}

	// Recover HP of all participants to MaxHP for round 1
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

// AdvanceRound executes combat for the active round and advances the match (@かいし, vs_guild.cgi:60-154).
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
		char, err := s.characters.FindByID(ctx, m.CharacterID)
		if err != nil {
			return RoundResolution{}, err
		}
		p := corebattle.NewParticipantFromCharacter(char)
		p.HP = m.HP
		if p.HP <= 0 {
			p.HP = m.MaxHP
		}
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
		return RoundResolution{}, fmt.Errorf("resolve gvg battle round: %w", err)
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
		detail.Room.GuildScores[roundWinnerGuildID]++
		_ = s.standings.AddRoundWinGP(ctx, roundWinnerGuildID, RoundWinGP)

		if detail.Room.GuildScores[roundWinnerGuildID] >= detail.Room.TargetWins {
			matchCompleted = true
			overallWinnerGuildID = roundWinnerGuildID
			overallWinnerGuildName = roundWinnerGuildName
			roundOutcome = "match_won"
			detail.Room.Status = StatusCompleted
			detail.Room.WinnerGuildID = roundWinnerGuildID
		}
	}

	// 10-round draw limit without winner (vs_guild.cgi:67, 114)
	if !matchCompleted && detail.Room.Round >= MaxRounds {
		matchCompleted = true
		roundOutcome = "match_draw"
		detail.Room.Status = StatusCompleted
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
		_ = s.standings.RecordMatchSettlement(ctx, settlement)
	} else {
		// Advance to next round and restore HP to MaxHP for all participants (vs_guild.cgi:142)
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
	}, nil
}

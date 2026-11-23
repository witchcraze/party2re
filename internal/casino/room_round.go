package casino

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type creditWinsFunc func(ctx context.Context, winnerIDs []string) error

func (s *Service) incrementCasinoWins(txCtx context.Context, winnerIDs []string) error {
	if s.charRepo == nil || len(winnerIDs) == 0 {
		return nil
	}
	ids := slices.Clone(winnerIDs)
	slices.Sort(ids)
	for _, winnerID := range slices.Compact(ids) {
		wChar, err := s.charRepo.FindByIDForUpdate(txCtx, winnerID)
		if err != nil {
			return err
		}
		wChar.CasinoWins++
		if err := s.charRepo.Update(txCtx, wChar); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) finalizeShowdown(txCtx context.Context, room *Room, members *[]RoomMember, winnerIDs []string, now time.Time, creditWins creditWinsFunc) error {
	if creditWins != nil && len(winnerIDs) > 0 {
		if err := creditWins(txCtx, winnerIDs); err != nil {
			return err
		}
	}

	room.Round = 0
	room.Status = RoomStatusWaiting
	room.CurrentBet = room.Rate
	room.Pot = 0
	if len(winnerIDs) > 0 {
		joined := strings.Join(winnerIDs, ",")
		room.WinnerCharacterID = &joined
	} else {
		room.WinnerCharacterID = nil
	}

	primaryWinner := ""
	if len(winnerIDs) > 0 {
		primaryWinner = winnerIDs[0]
	}

	var eliminated []string
	for i := range *members {
		m := &(*members)[i]
		if m.IsSpectator {
			continue
		}
		pAcc, err := s.repo.GetAccount(txCtx, m.CharacterID)
		if err != nil {
			return fmt.Errorf("getting casino account for member %s: %w", m.CharacterID, err)
		}
		if pAcc.Coins <= 0 {
			eliminated = append(eliminated, m.CharacterID)
			if room.LeaderCharacterID == m.CharacterID && primaryWinner != "" {
				room.LeaderCharacterID = primaryWinner
			}
		} else {
			m.Action = "待機中"
			m.UpdatedAt = now
		}
	}

	*members = slices.DeleteFunc(*members, func(m RoomMember) bool {
		return slices.Contains(eliminated, m.CharacterID)
	})
	return nil
}

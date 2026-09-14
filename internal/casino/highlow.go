package casino

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"
)

type HighLowAction string

const (
	HighLowActionCall HighLowAction = "call" // つづける: match current bet and continue
	HighLowActionHigh HighLowAction = "high" // ハイ: bet that own card is highest
	HighLowActionLow  HighLowAction = "low"  // ロウ: bet that own card is lowest (requires > 2 participants)
	HighLowActionFold HighLowAction = "fold" // おりる: forfeit hand
)

func (a HighLowAction) Valid(participantCount int) bool {
	switch a {
	case HighLowActionCall, HighLowActionHigh, HighLowActionFold:
		return true
	case HighLowActionLow:
		return participantCount > 2
	default:
		return false
	}
}

func ParseHighLowAction(act string) (HighLowAction, error) {
	s := strings.ToLower(strings.TrimSpace(act))
	switch s {
	case "call", "tsuzukeru", "つづける":
		return HighLowActionCall, nil
	case "high", "ハイ":
		return HighLowActionHigh, nil
	case "low", "ロウ":
		return HighLowActionLow, nil
	case "fold", "oriru", "おりる":
		return HighLowActionFold, nil
	default:
		return "", ErrInvalidHighLowAction
	}
}

var (
	ErrInvalidHighLowAction = errors.New("invalid high & low action: must be call, high, low (3+ players), or fold")
	ErrLowNotAllowedTwo     = errors.New("low action is only available with more than 2 participants")
)

// StartHighLow deals 1 card to each participant and starts Round 1 (party2/lib/casino_highlow.cgi:53-77).
func (s *Service) StartHighLow(ctx context.Context, roomID string, leaderID string) (*RoomDetail, error) {
	if leaderID == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}
		if room.LeaderCharacterID != leaderID {
			return ErrNotLeader
		}
		if room.Round > 0 {
			return ErrGameInProgress
		}

		members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}

		var participants []RoomMember
		for _, m := range members {
			if !m.IsSpectator {
				participants = append(participants, m)
			}
		}

		if len(participants) < MinPartyCount {
			return ErrNotEnoughPlayers
		}

		// Prepare 13 card deck (0..12) and deal 1 unique card per participant
		cardDeck := make([]int, 13)
		for i := 0; i < 13; i++ {
			cardDeck[i] = i
		}

		for _, m := range participants {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(cardDeck))))
			if err != nil {
				return err
			}
			idx := int(n.Int64())
			drawnCard := cardDeck[idx]
			cardDeck = append(cardDeck[:idx], cardDeck[idx+1:]...)

			m.Card = drawnCard
			m.Action = ""
			m.UpdatedAt = time.Now().UTC()
			if err := s.roomRepo.UpdateMember(txCtx, m); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		room.Round = 1
		room.Status = RoomStatusInProgress
		room.CurrentBet = room.Rate
		room.MaxBet = room.Rate * 5
		room.Pot = 0
		room.WinnerCharacterID = nil
		room.UpdatedAt = now

		return s.roomRepo.UpdateRoom(txCtx, *room)
	})
	if err != nil {
		return nil, err
	}

	return s.GetRoomDetail(ctx, roomID, leaderID)
}

// PlayHighLowAction processes a player's round action (call, high, low, fold)
// and handles pot accumulation, round progression, and showdown settlement (party2/lib/casino_highlow.cgi:80-224).
func (s *Service) PlayHighLowAction(ctx context.Context, roomID string, characterID string, action HighLowAction) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var showdownWinners []string

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}
		if room.Round <= 0 || room.Status != RoomStatusInProgress {
			return ErrGameNotInRound
		}

		acc, err := s.repo.GetAccountForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		member, err := s.roomRepo.GetMemberForUpdate(txCtx, roomID, characterID)
		if err != nil {
			return ErrMemberNotFound
		}
		if member.IsSpectator {
			return ErrSpectatorCannot
		}
		if member.Action != "" && member.Action != "待機中" {
			return ErrAlreadyActed
		}

		// Re-fetch all members to validate participant count and rules
		members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}

		var participants []RoomMember
		for _, m := range members {
			if !m.IsSpectator {
				participants = append(participants, m)
			}
		}

		if !action.Valid(len(participants)) {
			if action == HighLowActionLow && len(participants) <= 2 {
				return ErrLowNotAllowedTwo
			}
			return ErrInvalidHighLowAction
		}

		betToDeduct := room.CurrentBet
		if acc.Coins < betToDeduct {
			betToDeduct = acc.Coins // all-in remaining coins
		}

		if betToDeduct > 0 {
			_, err = s.repo.DeductBetAndCreditPayout(txCtx, characterID, betToDeduct, 0)
			if err != nil {
				return err
			}
			room.Pot += betToDeduct
		}

		member.Action = string(action)
		member.UpdatedAt = time.Now().UTC()
		if err := s.roomRepo.UpdateMember(txCtx, *member); err != nil {
			return err
		}

		// Re-count actions of all participants
		foldedCount := 0
		highCount := 0
		lowCount := 0
		callCount := 0
		actedCount := 0

		for i, m := range participants {
			if m.CharacterID == characterID {
				m = *member
				participants[i] = m
			}
			switch m.Action {
			case string(HighLowActionFold):
				foldedCount++
				actedCount++
			case string(HighLowActionHigh):
				highCount++
				actedCount++
			case string(HighLowActionLow):
				lowCount++
				actedCount++
			case string(HighLowActionCall):
				callCount++
				actedCount++
			}
		}

		totalParticipants := len(participants)
		activeNonFolded := totalParticipants - foldedCount

		shouldShowdown := false
		if actedCount == totalParticipants {
			// All players have acted in this round (party2/lib/casino_highlow.cgi:146)
			hasCoinlessPlayer := false
			for _, m := range participants {
				if m.Action != string(HighLowActionFold) {
					pAcc, err := s.repo.GetAccount(txCtx, m.CharacterID)
					if err == nil && pAcc.Coins <= 0 {
						hasCoinlessPlayer = true
						break
					}
				}
			}

			if hasCoinlessPlayer || room.CurrentBet >= room.MaxBet || float64(highCount+lowCount) >= float64(activeNonFolded)*0.5 || activeNonFolded <= 1 {
				shouldShowdown = true
			} else {
				// Next Round!
				room.Round++
				room.CurrentBet += room.Rate
				for _, m := range participants {
					if m.Action != string(HighLowActionFold) {
						m.Action = ""
						m.UpdatedAt = time.Now().UTC()
						if err := s.roomRepo.UpdateMember(txCtx, m); err != nil {
							return err
						}
					}
				}
			}
		}

		if shouldShowdown {
			// Showdown resolution (party2/lib/casino_highlow.cgi:167-224)
			higher := ""
			lower := ""
			maxCard := -1
			minCard := 99

			for _, m := range participants {
				if m.Action == string(HighLowActionHigh) {
					if m.Card > maxCard {
						higher = m.CharacterID
						maxCard = m.Card
					}
				} else if m.Action == string(HighLowActionLow) {
					if m.Card < minCard {
						lower = m.CharacterID
						minCard = m.Card
					}
				}
			}

			// If all active players folded except 1 and no one declared High/Low, surviving active player wins
			if higher == "" && lower == "" && activeNonFolded == 1 {
				for _, m := range participants {
					if m.Action != string(HighLowActionFold) {
						higher = m.CharacterID
						break
					}
				}
			}

			if totalParticipants > 2 && higher != "" && lower != "" {
				// Split prize 50/50 between highest and lowest
				halfPot := room.Pot / 2
				if halfPot > 0 {
					_, _ = s.repo.DeductBetAndCreditPayout(txCtx, higher, 0, halfPot)
					_, _ = s.repo.DeductBetAndCreditPayout(txCtx, lower, 0, halfPot)
				}
				showdownWinners = []string{higher, lower}
				winnerCombined := higher + "," + lower
				room.WinnerCharacterID = &winnerCombined
			} else if higher != "" {
				if room.Pot > 0 {
					_, _ = s.repo.DeductBetAndCreditPayout(txCtx, higher, 0, room.Pot)
				}
				showdownWinners = []string{higher}
				room.WinnerCharacterID = &higher
			} else if lower != "" {
				if room.Pot > 0 {
					_, _ = s.repo.DeductBetAndCreditPayout(txCtx, lower, 0, room.Pot)
				}
				showdownWinners = []string{lower}
				room.WinnerCharacterID = &lower
			} else {
				// Everyone folded or no high/low winners
				room.WinnerCharacterID = nil
			}

			room.Round = 0
			room.Status = RoomStatusWaiting
			room.CurrentBet = room.Rate
			room.Pot = 0

			// Eject members with 0 coins and set surviving to "待機中"
			primaryWinner := ""
			if len(showdownWinners) > 0 {
				primaryWinner = showdownWinners[0]
			}

			for _, m := range participants {
				pAcc, err := s.repo.GetAccount(txCtx, m.CharacterID)
				if err == nil && pAcc.Coins <= 0 {
					_ = s.roomRepo.RemoveMember(txCtx, roomID, m.CharacterID)
					if room.LeaderCharacterID == m.CharacterID && primaryWinner != "" {
						room.LeaderCharacterID = primaryWinner
					}
				} else {
					m.Action = "待機中"
					m.UpdatedAt = time.Now().UTC()
					_ = s.roomRepo.UpdateMember(txCtx, m)
				}
			}
		}

		room.UpdatedAt = time.Now().UTC()
		return s.roomRepo.UpdateRoom(txCtx, *room)
	})
	if err != nil {
		return nil, err
	}

	if len(showdownWinners) > 0 && s.gamePlayedHook != nil {
		for _, w := range showdownWinners {
			_ = s.gamePlayedHook(ctx, w, "highlow")
		}
	}

	return s.GetRoomDetail(ctx, roomID, characterID)
}

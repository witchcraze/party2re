package casino

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

type Action string

const (
	ActionCall     Action = "call"     // つづける: match current bet and continue
	ActionShowdown Action = "showdown" // しょうぶ: match current bet and force showdown
	ActionFold     Action = "fold"     // おりる: forfeit hand
)

func (a Action) Valid() bool {
	switch a {
	case ActionCall, ActionShowdown, ActionFold:
		return true
	default:
		return false
	}
}

var (
	ErrInvalidAction   = errors.New("invalid casino action: must be call, showdown, or fold")
	ErrAlreadyActed    = errors.New("character has already acted in the current round")
	ErrGameNotInRound  = errors.New("game is not currently in progress")
	ErrSpectatorCannot = errors.New("spectators cannot play actions")
)

// StartIndianPoker deals 1 card to each participant and starts Round 1 (party2/lib/casino_indian.cgi:39-61).
func (s *Service) StartIndianPoker(ctx context.Context, roomID string, leaderID string) (*RoomDetail, error) {
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

// PlayIndianPokerAction processes a player's round action (call, showdown, fold)
// and handles pot accumulation, round progression, and showdown settlement (party2/lib/casino_indian.cgi:64-176).
func (s *Service) PlayIndianPokerAction(ctx context.Context, roomID string, characterID string, action Action) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if !action.Valid() {
		return nil, ErrInvalidAction
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var showdownWinner string

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

		effectiveAction := action
		if action == ActionCall && acc.Coins < room.CurrentBet {
			// Forced showdown if insufficient coins to call
			effectiveAction = ActionShowdown
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

		member.Action = string(effectiveAction)
		member.UpdatedAt = time.Now().UTC()
		if err := s.roomRepo.UpdateMember(txCtx, *member); err != nil {
			return err
		}

		// Re-fetch all members to evaluate round completion
		members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}

		var participants []RoomMember
		foldedCount := 0
		showdownCount := 0
		actedCount := 0

		for _, m := range members {
			if m.IsSpectator {
				continue
			}
			participants = append(participants, m)
			if m.Action == string(ActionFold) {
				foldedCount++
				actedCount++
			} else if m.Action == string(ActionShowdown) {
				showdownCount++
				actedCount++
			} else if m.Action == string(ActionCall) {
				actedCount++
			}
		}

		totalParticipants := len(participants)
		activeNonFolded := totalParticipants - foldedCount

		shouldShowdown := false
		if activeNonFolded <= 1 {
			// All players except 1 folded
			shouldShowdown = true
		} else if actedCount == totalParticipants {
			// All players have acted in this round
			hasCoinlessPlayer := false
			for _, m := range participants {
				if m.Action != string(ActionFold) {
					pAcc, err := s.repo.GetAccount(txCtx, m.CharacterID)
					if err == nil && pAcc.Coins <= 0 {
						hasCoinlessPlayer = true
						break
					}
				}
			}

			if hasCoinlessPlayer || room.CurrentBet >= room.MaxBet || float64(showdownCount) >= float64(activeNonFolded)*0.5 {
				shouldShowdown = true
			} else {
				// Next Round!
				room.Round++
				room.CurrentBet += room.Rate
				for _, m := range participants {
					if m.Action != string(ActionFold) {
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
			// Showdown resolution (party2/lib/casino_indian.cgi:147-176)
			maxCard := -1
			winnerID := ""

			for _, m := range participants {
				if m.Action == string(ActionFold) {
					continue
				}
				if m.Card > maxCard {
					maxCard = m.Card
					winnerID = m.CharacterID
				}
			}

			if winnerID != "" && room.Pot > 0 {
				_, err = s.repo.DeductBetAndCreditPayout(txCtx, winnerID, 0, room.Pot)
				if err != nil {
					return err
				}
				showdownWinner = winnerID
			}

			room.Round = 0
			room.Status = RoomStatusWaiting
			room.CurrentBet = room.Rate
			room.Pot = 0
			if winnerID != "" {
				room.WinnerCharacterID = &winnerID
			}

			// Eject members with 0 coins and set surviving to "待機中"
			for _, m := range participants {
				pAcc, err := s.repo.GetAccount(txCtx, m.CharacterID)
				if err == nil && pAcc.Coins <= 0 {
					_ = s.roomRepo.RemoveMember(txCtx, roomID, m.CharacterID)
					if room.LeaderCharacterID == m.CharacterID && winnerID != "" {
						room.LeaderCharacterID = winnerID
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

	if showdownWinner != "" && s.gamePlayedHook != nil {
		_ = s.gamePlayedHook(ctx, showdownWinner, "indian_poker")
	}

	return s.GetRoomDetail(ctx, roomID, characterID)
}

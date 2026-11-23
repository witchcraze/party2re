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
	// ErrHighLowCallUnavailable: つづける is unregistered at the max bet or without enough coins.
	ErrHighLowCallUnavailable = errors.New("call is only available below the max bet with enough coins")
)

// highLowCanCall mirrors the つづける registration: now_bet < max_bet && coin >= now_bet
// (party2/lib/casino_highlow.cgi:13-16).
func highLowCanCall(currentBet, maxBet, coins int64) bool {
	return currentBet < maxBet && coins >= currentBet
}

// StartHighLow deals 1 card to each participant and starts Round 1 (party2/lib/casino_highlow.cgi:53-77).
func (s *Service) StartHighLow(ctx context.Context, roomID string, leaderID string) (*RoomDetail, error) {
	if leaderID == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var detail *RoomDetail
	err := s.withRoomMutationLock(ctx, roomID, leaderID, func(lockedCtx context.Context) error {
		txErr := s.runInTx(lockedCtx, func(txCtx context.Context) error {
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

			now := time.Now().UTC()
			for i := range members {
				if members[i].IsSpectator {
					continue
				}
				n, err := rand.Int(rand.Reader, big.NewInt(int64(len(cardDeck))))
				if err != nil {
					return err
				}
				idx := int(n.Int64())
				drawnCard := cardDeck[idx]
				cardDeck = append(cardDeck[:idx], cardDeck[idx+1:]...)

				members[i].Card = drawnCard
				members[i].Action = ""
				members[i].UpdatedAt = now
			}

			room.Round = 1
			room.Status = RoomStatusInProgress
			room.CurrentBet = room.Rate
			room.MaxBet = room.Rate * 5
			room.Pot = 0
			room.WinnerCharacterID = nil
			room.UpdatedAt = now

			return s.roomRepo.UpdateRoomDetail(txCtx, RoomDetail{Room: *room, Members: members})
		})
		if txErr != nil {
			return txErr
		}
		var err error
		detail, err = s.getRoomDetail(lockedCtx, roomID, leaderID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return detail, nil
}

// PlayHighLowAction processes a player's round action (call, high, low, fold)
// and handles pot accumulation, round progression, and showdown settlement (party2/lib/casino_highlow.cgi:80-224).
// Transaction: RunInTx for SQL, with a separate staged Valkey publication boundary.
// Locks: room coordination, atomic account mutations, then winner characters (rank 2).
func (s *Service) PlayHighLowAction(ctx context.Context, roomID string, characterID string, action HighLowAction) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var detail *RoomDetail
	var readErr error
	var secondarySyncErr error

	err := s.withRoomMutationLock(ctx, roomID, characterID, func(lockedCtx context.Context) error {
		txErr := s.runInTx(lockedCtx, func(txCtx context.Context) error {
			room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			if room.Round <= 0 || room.Status != RoomStatusInProgress {
				return ErrGameNotInRound
			}

			member, err := s.roomRepo.GetMemberForUpdate(txCtx, roomID, characterID)
			if err != nil {
				return err
			}
			if member.IsSpectator {
				return ErrSpectatorCannot
			}
			// 待機中 (joined mid-game) is a declared action too (casino_highlow.cgi:83-86).
			if member.Action != "" {
				return ErrAlreadyActed
			}

			acc, err := s.repo.GetAccount(txCtx, characterID)
			if err != nil {
				return err
			}
			if action == HighLowActionCall && !highLowCanCall(room.CurrentBet, room.MaxBet, acc.Coins) {
				return ErrHighLowCallUnavailable
			}

			// Re-fetch all members to validate participant count and rules
			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}

			var participantCount int
			for i := range members {
				m := &members[i]
				if m.CharacterID == characterID {
					*m = *member
				}
				if !m.IsSpectator {
					participantCount++
				}
			}

			if !action.Valid(participantCount) {
				if action == HighLowActionLow && participantCount <= 2 {
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

			now := time.Now().UTC()
			member.Action = string(action)
			member.UpdatedAt = now
			for i := range members {
				if members[i].CharacterID == characterID {
					members[i] = *member
					break
				}
			}

			if err := s.evaluateHighLowRound(txCtx, room, &members, acc.Coins-betToDeduct, now, s.incrementCasinoWins); err != nil {
				return err
			}

			room.UpdatedAt = now
			if err := s.roomRepo.UpdateRoomDetail(txCtx, RoomDetail{Room: *room, Members: members}); err != nil {
				if errors.Is(err, ErrSecondarySync) {
					secondarySyncErr = err
				} else {
					return err
				}
			}
			return nil
		})
		if txErr != nil {
			return txErr
		}
		if secondarySyncErr != nil {
			return secondarySyncErr
		}
		detail, readErr = s.getRoomDetail(lockedCtx, roomID, characterID)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if readErr != nil {
		return nil, readErr
	}

	return detail, nil
}

// evaluateHighLowRound mirrors next_round and battle (party2/lib/casino_highlow.cgi:130-224).
func (s *Service) evaluateHighLowRound(txCtx context.Context, room *Room, members *[]RoomMember, actorCoins int64, now time.Time, creditWins creditWinsFunc) error {
	if room.Round <= 0 || room.Status != RoomStatusInProgress {
		return nil
	}

	var participants []*RoomMember
	foldedCount := 0
	highCount := 0
	lowCount := 0
	actedCount := 0

	for i := range *members {
		m := &(*members)[i]
		if m.IsSpectator {
			continue
		}
		participants = append(participants, m)
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
		case string(HighLowActionCall), "待機中": // legacy counts every non-empty action, including 待機中
			actedCount++
		}
	}

	totalParticipants := len(participants)
	if totalParticipants == 0 {
		return nil
	}
	activeNonFolded := totalParticipants - foldedCount

	shouldShowdown := false
	if actedCount == totalParticipants {
		// All players have acted in this round; only the acting/leaving player's
		// balance counts ($m{coin}, party2/lib/casino_highlow.cgi:146).
		if actorCoins <= 0 || room.CurrentBet >= room.MaxBet || float64(highCount+lowCount) >= float64(activeNonFolded)*0.5 {
			shouldShowdown = true
		} else {
			room.Round++
			room.CurrentBet += room.Rate
			for _, m := range participants {
				if m.Action != string(HighLowActionFold) && m.Action != "待機中" {
					m.Action = ""
					m.UpdatedAt = now
				}
			}
		}
	}

	if !shouldShowdown || room.Pot <= 0 {
		return nil
	}

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

	var showdownWinners []string
	if totalParticipants > 2 && higher != "" && lower != "" {
		halfPot := room.Pot / 2
		if halfPot > 0 {
			if _, err := s.repo.DeductBetAndCreditPayout(txCtx, higher, 0, halfPot); err != nil {
				return err
			}
			if _, err := s.repo.DeductBetAndCreditPayout(txCtx, lower, 0, halfPot); err != nil {
				return err
			}
		}
		showdownWinners = []string{higher, lower}
	} else if higher != "" {
		if _, err := s.repo.DeductBetAndCreditPayout(txCtx, higher, 0, room.Pot); err != nil {
			return err
		}
		showdownWinners = []string{higher}
	} else if lower != "" {
		if _, err := s.repo.DeductBetAndCreditPayout(txCtx, lower, 0, room.Pot); err != nil {
			return err
		}
		showdownWinners = []string{lower}
	}

	return s.finalizeShowdown(txCtx, room, members, showdownWinners, now, creditWins)
}

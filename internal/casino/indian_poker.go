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

// PlayIndianPokerAction processes a player's round action (call, showdown, fold)
// and handles pot accumulation, round progression, and showdown settlement (party2/lib/casino_indian.cgi:64-176).
// Transaction: RunInTx for SQL, with a separate staged Valkey publication boundary.
// Locks: room coordination, atomic account mutations, then winner characters (rank 2).
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
			// 待機中 (joined mid-game) is a declared action too (casino_indian.cgi:66-70).
			if member.Action != "" {
				return ErrAlreadyActed
			}

			acc, err := s.repo.GetAccount(txCtx, characterID)
			if err != nil {
				return err
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

			// Stage all member changes until the complete action can be published.
			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			for i := range members {
				if members[i].CharacterID == characterID {
					members[i] = *member
					break
				}
			}

			now := time.Now().UTC()
			if err := s.evaluateIndianPokerRound(txCtx, room, &members, acc.Coins-betToDeduct, now, s.incrementCasinoWins); err != nil {
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

// evaluateIndianPokerRound mirrors next_round and battle (party2/lib/casino_indian.cgi:109-176).
func (s *Service) evaluateIndianPokerRound(txCtx context.Context, room *Room, members *[]RoomMember, actorCoins int64, now time.Time, creditWins creditWinsFunc) error {
	if room.Round <= 0 || room.Status != RoomStatusInProgress {
		return nil
	}

	var participants []*RoomMember
	foldedCount := 0
	showdownCount := 0
	actedCount := 0

	for i := range *members {
		m := &(*members)[i]
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
		} else if m.Action == string(ActionCall) || m.Action == "待機中" {
			actedCount++ // legacy counts every non-empty action, including 待機中
		}
	}

	totalParticipants := len(participants)
	if totalParticipants == 0 {
		return nil
	}
	activeNonFolded := totalParticipants - foldedCount

	shouldShowdown := false
	if activeNonFolded <= 1 {
		// All players except 1 folded
		shouldShowdown = true
	} else if actedCount == totalParticipants {
		// All players have acted in this round; only the acting/leaving player's
		// balance counts ($m{coin}, casino_indian.cgi:128).
		if actorCoins <= 0 || room.CurrentBet >= room.MaxBet || float64(showdownCount) >= float64(activeNonFolded)*0.5 {
			shouldShowdown = true
		} else {
			room.Round++
			room.CurrentBet += room.Rate
			for _, m := range participants {
				if m.Action != string(ActionFold) && m.Action != "待機中" {
					m.Action = ""
					m.UpdatedAt = now
				}
			}
		}
	}

	if !shouldShowdown || room.Pot <= 0 {
		return nil
	}

	maxCard := -1
	winnerID := ""
	for _, m := range participants {
		if m.Action == string(ActionFold) || m.Action == "待機中" {
			continue
		}
		if m.Card > maxCard {
			maxCard = m.Card
			winnerID = m.CharacterID
		}
	}

	var winnerIDs []string
	if winnerID != "" {
		if _, err := s.repo.DeductBetAndCreditPayout(txCtx, winnerID, 0, room.Pot); err != nil {
			return err
		}
		winnerIDs = []string{winnerID}
	}
	return s.finalizeShowdown(txCtx, room, members, winnerIDs, now, creditWins)
}

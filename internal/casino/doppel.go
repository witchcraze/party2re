package casino

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type DoppelMark string

const (
	MarkStar             DoppelMark = "★"
	MarkCircle           DoppelMark = "●"
	MarkDiamond          DoppelMark = "◆"
	MarkNote             DoppelMark = "♪"
	MarkSquare           DoppelMark = "■"
	MarkTriangle         DoppelMark = "▲"
	MarkDagger           DoppelMark = "†"
	MarkInvertedTriangle DoppelMark = "▼"
)

var AuthenticDoppelMarks = [8]DoppelMark{
	MarkStar,
	MarkCircle,
	MarkDiamond,
	MarkNote,
	MarkSquare,
	MarkTriangle,
	MarkDagger,
	MarkInvertedTriangle,
}

var (
	ErrInvalidDoppelMarkIndex = errors.New("invalid doppel mark index for party size")
	ErrInvalidDoppelMark      = errors.New("invalid doppel mark: must be mark symbol or valid index")
)

// ParseDoppelMark converts a mark symbol (★..▼) or integer string ("0".."7") to its index 0..7.
func ParseDoppelMark(s string) (int, error) {
	s = strings.TrimSpace(s)
	for i, m := range AuthenticDoppelMarks {
		if string(m) == s {
			return i, nil
		}
	}
	idx, err := strconv.Atoi(s)
	if err == nil && idx >= 0 && idx < len(AuthenticDoppelMarks) {
		return idx, nil
	}
	return -1, ErrInvalidDoppelMark
}

// StartDoppel initializes a Doppelganger round (party2/lib/casino_doppel.cgi:58-80).
func (s *Service) StartDoppel(ctx context.Context, roomID string, leaderID string) (*RoomDetail, error) {
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

			now := time.Now().UTC()
			for i := range members {
				if members[i].IsSpectator {
					continue
				}
				members[i].Card = -1
				members[i].Action = ""
				members[i].UpdatedAt = now
			}

			room.Round = 1
			room.Status = RoomStatusInProgress
			// Legacy stores the starting participant count in $now_bet for mark bounds (casino_doppel.cgi:76).
			room.CurrentBet = int64(len(participants))
			room.MaxBet = room.Rate
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

// PlayDoppelAction handles a participant's mark selection, coin deduction, and showdown evaluation (party2/lib/casino_doppel.cgi:41-146).
// Transaction: RunInTx for SQL, with a separate staged Valkey publication boundary.
// Locks: room coordination, atomic account mutations, then winner characters (rank 2).
func (s *Service) PlayDoppelAction(ctx context.Context, roomID string, characterID string, markIndex int) (*RoomDetail, error) {
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
			// A mid-game entrant (待機中) gets no marks (casino_doppel.cgi:11-12,41-42).
			if member.Action == "待機中" {
				return ErrAlreadyActed
			}

			acc, err := s.repo.GetAccount(txCtx, characterID)
			if err != nil {
				return err
			}

			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}

			// Available marks: 0..min(now_bet, 7); now_bet is fixed in CurrentBet at kaishi
			// (party2/lib/casino_doppel.cgi:13-17, 76), so mid-game joins/leaves keep it unchanged.
			maxAllowed := int(room.CurrentBet)
			if maxAllowed > len(AuthenticDoppelMarks)-1 {
				maxAllowed = len(AuthenticDoppelMarks) - 1
			}
			if markIndex < 0 || markIndex > maxAllowed {
				return ErrInvalidDoppelMarkIndex
			}

			// Deduct bet if this is the first selection for this player in this round
			if member.Card < 0 {
				betToDeduct := room.Rate
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
			}

			now := time.Now().UTC()
			member.Card = markIndex
			member.Action = string(AuthenticDoppelMarks[markIndex])
			member.UpdatedAt = now
			leaderCard := -1
			for i := range members {
				if members[i].CharacterID == characterID {
					members[i] = *member
				}
				if members[i].CharacterID == room.LeaderCharacterID {
					leaderCard = members[i].Card
				}
			}

			if err := s.evaluateDoppelRound(txCtx, room, &members, leaderCard, now, s.incrementCasinoWins); err != nil {
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

// evaluateDoppelRound mirrors next_round, is_all_select, and battle (party2/lib/casino_doppel.cgi:85-146).
func (s *Service) evaluateDoppelRound(txCtx context.Context, room *Room, members *[]RoomMember, leaderCard int, now time.Time, creditWins creditWinsFunc) error {
	if room.Round <= 0 || room.Status != RoomStatusInProgress {
		return nil
	}

	var participants []*RoomMember
	for i := range *members {
		m := &(*members)[i]
		if !m.IsSpectator {
			participants = append(participants, m)
		}
	}
	if len(participants) == 0 {
		return nil
	}

	for _, m := range participants {
		if m.Card < 0 && m.Action != "待機中" { // is_all_select skips 待機中
			return nil
		}
	}

	if room.Pot <= 0 {
		return nil
	}

	var matchedChildren []string
	for _, m := range participants {
		if m.CharacterID == room.LeaderCharacterID || m.Action == "待機中" {
			continue
		}
		if m.Card == leaderCard {
			matchedChildren = append(matchedChildren, m.CharacterID)
		}
	}

	var showdownWinners []string
	if len(matchedChildren) >= 1 {
		payout := room.Pot / int64(len(matchedChildren))
		if payout > 0 {
			for _, childID := range matchedChildren {
				if _, err := s.repo.DeductBetAndCreditPayout(txCtx, childID, 0, payout); err != nil {
					return err
				}
			}
		}
		showdownWinners = matchedChildren

		// Leadership changes to one of the winners chosen at random (party2/lib/casino_doppel.cgi:133)
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(matchedChildren))))
		if err == nil {
			room.LeaderCharacterID = matchedChildren[n.Int64()]
		} else {
			room.LeaderCharacterID = matchedChildren[0]
		}
	} else {
		if _, err := s.repo.DeductBetAndCreditPayout(txCtx, room.LeaderCharacterID, 0, room.Pot); err != nil {
			return err
		}
		showdownWinners = []string{room.LeaderCharacterID}
	}

	return s.finalizeShowdown(txCtx, room, members, showdownWinners, now, creditWins)
}

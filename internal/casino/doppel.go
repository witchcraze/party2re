package casino

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
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

	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		return s.runInTx(lockedCtx, func(txCtx context.Context) error {
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
			for _, m := range participants {
				m.Card = -1
				m.Action = ""
				m.UpdatedAt = now
				if err := s.roomRepo.UpdateMember(txCtx, m); err != nil {
					return err
				}
			}

			room.Round = 1
			room.Status = RoomStatusInProgress
			room.CurrentBet = room.Rate
			room.MaxBet = room.Rate
			room.Pot = 0
			room.WinnerCharacterID = nil
			room.UpdatedAt = now

			return s.roomRepo.UpdateRoom(txCtx, *room)
		})
	})
	if err != nil {
		return nil, err
	}

	return s.getRoomDetail(ctx, roomID, leaderID)
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

	var showdownWinners []string
	var secondarySyncErr error

	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
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

			acc, err := s.repo.GetAccount(txCtx, characterID)
			if err != nil {
				return err
			}

			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}

			var participants []*RoomMember
			var eliminated []string
			for i := range members {
				m := &members[i]
				if !m.IsSpectator {
					participants = append(participants, m)
				}
			}

			// Available marks: 0..min(len(participants), 7) (party2/lib/casino_doppel.cgi:13-17, 76)
			maxAllowed := len(participants)
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

			member.Card = markIndex
			member.Action = string(AuthenticDoppelMarks[markIndex])
			member.UpdatedAt = time.Now().UTC()
			for _, m := range participants {
				if m.CharacterID == characterID {
					*m = *member
				}
			}

			// Check if all participants have selected a mark
			allSelected := true
			for _, m := range participants {
				if m.Card < 0 {
					allSelected = false
					break
				}
			}

			if allSelected {
				// Showdown (party2/lib/casino_doppel.cgi:104-146)
				// Leader is the "親" (dealer / target)
				var leaderCard int
				for _, m := range participants {
					if m.CharacterID == room.LeaderCharacterID {
						leaderCard = m.Card
					}
				}

				var matchedChildren []string
				for _, m := range participants {
					if m.CharacterID == room.LeaderCharacterID {
						continue
					}
					if m.Card == leaderCard {
						matchedChildren = append(matchedChildren, m.CharacterID)
					}
				}

				if len(matchedChildren) >= 1 {
					// Children win! Prize is split equally among matching children
					payout := room.Pot / int64(len(matchedChildren))
					if payout > 0 {
						for _, childID := range matchedChildren {
							if _, err := s.repo.DeductBetAndCreditPayout(txCtx, childID, 0, payout); err != nil {
								return err
							}
						}
					}
					showdownWinners = matchedChildren
					winnerStr := strings.Join(matchedChildren, ",")
					room.WinnerCharacterID = &winnerStr

					// Leadership changes to one of the winners chosen at random (party2/lib/casino_doppel.cgi:133)
					n, err := rand.Int(rand.Reader, big.NewInt(int64(len(matchedChildren))))
					if err == nil {
						room.LeaderCharacterID = matchedChildren[n.Int64()]
					} else {
						room.LeaderCharacterID = matchedChildren[0]
					}
				} else {
					// Parent (Leader) wins! Leader takes the full pot
					if room.Pot > 0 {
						if _, err := s.repo.DeductBetAndCreditPayout(txCtx, room.LeaderCharacterID, 0, room.Pot); err != nil {
							return err
						}
					}
					showdownWinners = []string{room.LeaderCharacterID}
					room.WinnerCharacterID = &room.LeaderCharacterID
				}

				for _, winnerID := range showdownWinners {
					if s.charRepo != nil {
						wChar, err := s.charRepo.FindByIDForUpdate(txCtx, winnerID)
						if err != nil {
							return err
						}
						wChar.CasinoWins++
						if err := s.charRepo.Update(txCtx, wChar); err != nil {
							return err
						}
					}
				}

				room.Round = 0
				room.Status = RoomStatusWaiting
				room.Pot = 0

				primaryWinner := ""
				if len(showdownWinners) > 0 {
					primaryWinner = showdownWinners[0]
				}

				for _, m := range participants {
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
						m.UpdatedAt = time.Now().UTC()
					}
				}
			}

			room.UpdatedAt = time.Now().UTC()
			members = slices.DeleteFunc(members, func(m RoomMember) bool { return slices.Contains(eliminated, m.CharacterID) })
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
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(showdownWinners) > 0 && s.gamePlayedHook != nil {
		for _, w := range showdownWinners {
			_ = s.gamePlayedHook(ctx, w, "doppel")
		}
	}

	return s.getRoomDetail(ctx, roomID, characterID)
}

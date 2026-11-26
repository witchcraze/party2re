package casino

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

const KickActionWait = 60 * time.Second

var ErrDepartureRecovery = errors.New("casino departure requires recovery")

// ActionWaitWriter extends a kicked member's action wait until an absolute deadline.
type ActionWaitWriter interface {
	ImposeActionWaitUntil(ctx context.Context, characterID string, until time.Time) error
}

// Departure retains the latest admission outcome, independently of live room TTL.
type Departure struct {
	Version      int
	Member       RoomMember
	Detail       RoomDetail
	PasswordHash string
	ClaimedIDs   []string
	Pending      bool
	Kicked       bool
	LeaderID     string
	WaitUntil    time.Time
}

type DepartureRepository interface {
	FindDeparture(context.Context, string) (Departure, error)
	FindPendingDeparture(context.Context, string) (string, error)
	CommitDeparture(context.Context, *Departure, func(context.Context) error) error
	AcknowledgeDeparture(context.Context, string, int) error
}

type DeparturePublisher interface {
	PublishDeparture(context.Context, Departure) error
}

func WithDepartureRepository(repo DepartureRepository) Option {
	return func(s *Service) { s.departures = repo }
}

func WithActionWait(writer ActionWaitWriter) Option {
	return func(s *Service) { s.actionWait = writer }
}

// SetActionWait wires the kick wait writer after service construction.
func (s *Service) SetActionWait(writer ActionWaitWriter) {
	s.actionWait = writer
}

func (s *Service) publishDeparture(ctx context.Context, d Departure) error {
	if err := s.roomRepo.PublishDeparture(ctx, d); err != nil {
		return errors.Join(ErrDepartureRecovery, err)
	}
	if d.Kicked && !d.WaitUntil.IsZero() && s.actionWait != nil {
		if err := s.actionWait.ImposeActionWaitUntil(ctx, d.Member.CharacterID, d.WaitUntil); err != nil {
			return errors.Join(ErrDepartureRecovery, err)
		}
	}
	if err := s.departures.AcknowledgeDeparture(ctx, d.Detail.Room.ID, d.Version); err != nil {
		return errors.Join(ErrDepartureRecovery, err)
	}
	return nil
}

// RecoverCharacterDeparture finishes any pending room departure and kick wait
// owned by a prior committed mutation for characterID.
func (s *Service) RecoverCharacterDeparture(ctx context.Context, characterID string) error {
	return s.recoverCharacterDeparture(ctx, characterID)
}

// Recover through commands only; owned observations must never publish or acknowledge.
func (s *Service) recoverCharacterDeparture(ctx context.Context, characterID string) error {
	if s.departures == nil || s.roomRepo == nil {
		return nil
	}
	roomID, err := s.departures.FindPendingDeparture(ctx, characterID)
	if err != nil || roomID == "" {
		return err
	}
	return s.roomRepo.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		return s.recoverRoomDeparture(lockedCtx, roomID)
	})
}

func (s *Service) recoverRoomDeparture(ctx context.Context, roomID string) error {
	if s.departures == nil {
		return nil
	}
	d, err := s.departures.FindDeparture(ctx, roomID)
	if err != nil {
		return err
	}
	if d.Pending {
		return s.publishDeparture(ctx, d)
	}
	return nil
}

func (s *Service) withRoomMutationLock(ctx context.Context, roomID, characterID string, fn func(context.Context) error) error {
	if err := s.recoverCharacterDeparture(ctx, characterID); err != nil {
		return err
	}
	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		if err := s.recoverRoomDeparture(lockedCtx, roomID); err != nil {
			return err
		}
		return fn(lockedCtx)
	})
}

func (s *Service) checkPendingDeparture(ctx context.Context, characterID string) error {
	if s.departures == nil {
		return nil
	}
	roomID, err := s.departures.FindPendingDeparture(ctx, characterID)
	if err != nil {
		return err
	}
	if roomID != "" {
		return ErrDepartureRecovery
	}
	return nil
}

// The caller's transaction holds this lock through admission, excluding new departure claims.
func (s *Service) checkAdmissionDeparture(ctx context.Context, characterID string) error {
	if s.departures == nil {
		return nil
	}
	if s.charRepo != nil {
		if _, err := s.charRepo.FindByIDForUpdate(ctx, characterID); err != nil {
			return err
		}
	}
	return s.checkPendingDeparture(ctx, characterID)
}

// LeaveRoom commits fatigue and the complete departure before any room publication.
func (s *Service) LeaveRoom(ctx context.Context, roomID, characterID string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if s.roomRepo == nil || s.departures == nil {
		return errors.New("room and departure repositories are required")
	}
	return s.withRoomMutationLock(ctx, roomID, characterID, func(lockedCtx context.Context) error {
		previous, err := s.departures.FindDeparture(lockedCtx, roomID)
		if err != nil {
			return err
		}
		room, err := s.roomRepo.GetRoomForUpdate(lockedCtx, roomID)
		if errors.Is(err, ErrRoomNotFound) && !previous.Kicked && previous.Member.CharacterID == characterID {
			return nil
		}
		if err != nil {
			return err
		}
		members, err := s.roomRepo.ListMembersForUpdate(lockedCtx, roomID)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(members, func(m RoomMember) bool { return m.CharacterID == characterID })
		if index < 0 {
			if !previous.Kicked && previous.Member.CharacterID == characterID {
				return nil
			}
			return ErrMemberNotFound
		}
		if room.DepartureVersion != previous.Version {
			return ErrDepartureRecovery
		}
		member := members[index]
		if previous.Member.CharacterID == characterID && previous.Member.JoinedAt.Equal(member.JoinedAt) {
			return ErrDepartureRecovery
		}
		claimed := departureClaimedIDs(members)
		leaderCard := -1
		for _, m := range members {
			if m.CharacterID == room.LeaderCharacterID {
				leaderCard = m.Card
				break
			}
		}
		remaining := slices.Delete(members, index, index+1)
		evalRound := !member.IsSpectator && room.Round > 0 && room.Status == RoomStatusInProgress &&
			slices.ContainsFunc(remaining, func(m RoomMember) bool { return !m.IsSpectator })
		if !member.IsSpectator && !evalRound {
			firstActive := slices.IndexFunc(remaining, func(m RoomMember) bool { return !m.IsSpectator })
			if firstActive < 0 {
				room.Status = RoomStatusDisbanded
			} else if room.LeaderCharacterID == characterID {
				room.LeaderCharacterID = remaining[firstActive].CharacterID
			}
		}
		room.UpdatedAt = time.Now().UTC()
		room.DepartureVersion = previous.Version + 1
		d := Departure{Version: room.DepartureVersion, Member: member, Detail: RoomDetail{Room: *room, Members: remaining},
			PasswordHash: room.PasswordHash, ClaimedIDs: claimed, Pending: true}
		if err := s.commitDeparture(lockedCtx, &d, !member.IsSpectator, evalRound, leaderCard); err != nil {
			return err
		}
		return s.publishDeparture(lockedCtx, d)
	})
}

func departureClaimedIDs(members []RoomMember) []string {
	claimed := make([]string, 0, len(members))
	for _, m := range members {
		claimed = append(claimed, m.CharacterID)
	}
	return claimed
}

func (s *Service) commitDeparture(ctx context.Context, d *Departure, addFatigue bool, evalRound bool, leaderCard int) error {
	roomID := d.Detail.Room.ID
	characterID := d.Member.CharacterID
	if err := s.departures.CommitDeparture(ctx, d, func(txCtx context.Context) error {
		return s.runInTx(txCtx, func(txCtx context.Context) error {
			lockedChars := make(map[string]corecharacter.Character)
			if s.charRepo != nil {
				if err := economy.LockInOrder(txCtx, d.ClaimedIDs, func(txCtx context.Context, actorID string) error {
					locked, err := s.charRepo.FindByIDForUpdate(txCtx, actorID)
					if err != nil {
						return err
					}
					lockedChars[actorID] = locked
					currentRoom, err := s.roomRepo.GetCharacterRoom(txCtx, actorID)
					if errors.Is(err, ErrRoomNotFound) {
						return nil
					}
					if err != nil {
						return err
					}
					if currentRoom != roomID {
						return ErrDepartureRecovery
					}
					return nil
				}); err != nil {
					return err
				}
			}

			dirtyChars := make(map[string]bool)
			creditWins := func(_ context.Context, winnerIDs []string) error {
				if s.charRepo == nil {
					return nil
				}
				for _, winnerID := range winnerIDs {
					ch, ok := lockedChars[winnerID]
					if !ok {
						return fmt.Errorf("winner %s missing from locked departure claims", winnerID)
					}
					ch.CasinoWins++
					lockedChars[winnerID] = ch
					dirtyChars[winnerID] = true
				}
				return nil
			}

			if evalRound {
				if err := s.evaluateDepartureRound(txCtx, &d.Detail.Room, &d.Detail.Members, characterID, leaderCard, creditWins); err != nil {
					return err
				}
				firstActive := slices.IndexFunc(d.Detail.Members, func(m RoomMember) bool { return !m.IsSpectator })
				if firstActive < 0 {
					d.Detail.Room.Status = RoomStatusDisbanded
				} else if d.Detail.Room.LeaderCharacterID == characterID {
					d.Detail.Room.LeaderCharacterID = d.Detail.Members[firstActive].CharacterID
				}
			}

			if s.charRepo == nil {
				return nil
			}
			if addFatigue {
				ch := lockedChars[characterID]
				ch.AddTired(1)
				lockedChars[characterID] = ch
				dirtyChars[characterID] = true
			}
			dirtyIDs := make([]string, 0, len(dirtyChars))
			for id := range dirtyChars {
				dirtyIDs = append(dirtyIDs, id)
			}
			slices.Sort(dirtyIDs)
			for _, id := range dirtyIDs {
				if err := s.charRepo.Update(txCtx, lockedChars[id]); err != nil {
					return err
				}
			}
			return nil
		})
	}); err != nil {
		return errors.Join(ErrDepartureRecovery, err)
	}
	return nil
}

func (s *Service) evaluateDepartureRound(txCtx context.Context, room *Room, members *[]RoomMember, leaverID string, leaderCard int, creditWins creditWinsFunc) error {
	now := time.Now().UTC()
	switch room.GameType {
	case GameTypeIndian:
		leaverAcc, err := s.repo.GetAccount(txCtx, leaverID)
		if err != nil {
			return err
		}
		return s.evaluateIndianPokerRound(txCtx, room, members, leaverAcc.Coins, now, creditWins)
	case GameTypeHighLow:
		leaverAcc, err := s.repo.GetAccount(txCtx, leaverID)
		if err != nil {
			return err
		}
		return s.evaluateHighLowRound(txCtx, room, members, leaverAcc.Coins, now, creditWins)
	case GameTypeDoppel:
		return s.evaluateDoppelRound(txCtx, room, members, leaderCard, now, creditWins)
	default:
		return nil
	}
}

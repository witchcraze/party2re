package casino

import (
	"context"
	"errors"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

var ErrDepartureRecovery = errors.New("casino departure requires recovery")

// Departure retains the latest admission outcome, independently of live room TTL.
type Departure struct {
	Version      int
	Member       RoomMember
	Detail       RoomDetail
	PasswordHash string
	ClaimedIDs   []string
	Pending      bool
}

type DepartureRepository interface {
	FindDeparture(context.Context, string) (Departure, error)
	FindPendingDeparture(context.Context, string) (string, error)
	CommitDeparture(context.Context, Departure, func(context.Context) error) error
	AcknowledgeDeparture(context.Context, string, int) error
}

type DeparturePublisher interface {
	PublishDeparture(context.Context, Departure) error
}

func WithDepartureRepository(repo DepartureRepository) Option {
	return func(s *Service) { s.departures = repo }
}

func (s *Service) publishDeparture(ctx context.Context, d Departure) error {
	if err := s.roomRepo.PublishDeparture(ctx, d); err != nil {
		return errors.Join(ErrDepartureRecovery, err)
	}
	if err := s.departures.AcknowledgeDeparture(ctx, d.Detail.Room.ID, d.Version); err != nil {
		return errors.Join(ErrDepartureRecovery, err)
	}
	return nil
}

// Recover through commands only; owned observations must never publish or acknowledge.
func (s *Service) recoverCharacterDeparture(ctx context.Context, characterID string) error {
	if s.departures == nil {
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
		if errors.Is(err, ErrRoomNotFound) && previous.Member.CharacterID == characterID {
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
			if previous.Member.CharacterID == characterID {
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
		claimed := make([]string, 0, len(members))
		for _, m := range members {
			claimed = append(claimed, m.CharacterID)
		}
		remaining := slices.Delete(members, index, index+1)
		if !member.IsSpectator {
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
		if err := s.departures.CommitDeparture(lockedCtx, d, func(txCtx context.Context) error {
			return s.runInTx(txCtx, func(txCtx context.Context) error {
				if s.charRepo == nil {
					return nil
				}
				var char corecharacter.Character
				if err := economy.LockInOrder(txCtx, claimed, func(txCtx context.Context, actorID string) error {
					locked, err := s.charRepo.FindByIDForUpdate(txCtx, actorID)
					if actorID == characterID {
						char = locked
					}
					if err != nil {
						return err
					}
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
				if member.IsSpectator {
					return nil
				}
				char.AddTired(1)
				return s.charRepo.Update(txCtx, char)
			})
		}); err != nil {
			return errors.Join(ErrDepartureRecovery, err)
		}
		return s.publishDeparture(lockedCtx, d)
	})
}

package casino

import (
	"context"
	"errors"
	"slices"
	"time"
)

// JoinRoom adds a character as an active participant to a room (party2/lib/casino.cgi:410-470).
func (s *Service) JoinRoom(ctx context.Context, roomID string, characterID string, password string, fatigue int) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if fatigue >= 100 {
		return nil, ErrCharacterExhausted
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var detail *RoomDetail
	err := s.withRoomMutationLock(ctx, roomID, characterID, func(lockedCtx context.Context) error {
		txErr := s.runInTx(lockedCtx, func(txCtx context.Context) error {
			room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			if room.Status == RoomStatusDisbanded {
				return ErrRoomNotFound
			}
			if room.HasPassword && !verifyPassword(password, room.PasswordHash) {
				return ErrInvalidPassword
			}

			acc, err := s.repo.GetAccount(txCtx, characterID)
			if err != nil {
				return err
			}
			if acc.Coins < room.Rate {
				return ErrInsufficientCoins
			}

			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}

			activeCount := 0
			for _, m := range members {
				if m.CharacterID == characterID {
					return ErrAlreadyInRoom
				}
				if !m.IsSpectator {
					activeCount++
				}
			}

			if activeCount >= room.MaxPlayers {
				return ErrRoomFull
			}

			now := time.Now().UTC()
			newMember := RoomMember{
				RoomID:      roomID,
				CharacterID: characterID,
				IsSpectator: false,
				Action:      "待機中",
				Card:        -1,
				JoinedAt:    now,
				UpdatedAt:   now,
			}

			if err := s.checkAdmissionDeparture(txCtx, characterID); err != nil {
				return err
			}
			return s.roomRepo.AddMember(txCtx, newMember)
		})
		if txErr != nil {
			return txErr
		}
		var err error
		detail, err = s.getRoomDetail(lockedCtx, roomID, characterID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return detail, nil
}

// SpectateRoom adds a character as a spectator to a room (party2/lib/casino.cgi:475-518).
func (s *Service) SpectateRoom(ctx context.Context, roomID string, characterID string, password string) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	var detail *RoomDetail
	err := s.withRoomMutationLock(ctx, roomID, characterID, func(lockedCtx context.Context) error {
		txErr := s.runInTx(lockedCtx, func(txCtx context.Context) error {
			room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			if room.Status == RoomStatusDisbanded {
				return ErrRoomNotFound
			}
			if !room.AllowSpectators {
				return ErrSpectatorsNotAllowed
			}
			if room.HasPassword && !verifyPassword(password, room.PasswordHash) {
				return ErrInvalidPassword
			}

			existing, err := s.roomRepo.GetMember(txCtx, roomID, characterID)
			if err == nil && existing != nil {
				return ErrAlreadyInRoom
			}

			now := time.Now().UTC()
			spectator := RoomMember{
				RoomID:      roomID,
				CharacterID: characterID,
				IsSpectator: true,
				Action:      "",
				Card:        -1,
				JoinedAt:    now,
				UpdatedAt:   now,
			}
			if err := s.checkAdmissionDeparture(txCtx, characterID); err != nil {
				return err
			}
			return s.roomRepo.AddMember(txCtx, spectator)
		})
		if txErr != nil {
			return txErr
		}
		var err error
		detail, err = s.getRoomDetail(lockedCtx, roomID, characterID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return detail, nil
}

// KickMember commits a kick departure and the target's 60 s wait deadline before publishing room state (party2/lib/_casino.cgi:148-187).
func (s *Service) KickMember(ctx context.Context, roomID string, leaderID string, targetID string) error {
	if leaderID == "" || targetID == "" {
		return ErrInvalidCharacterID
	}
	if leaderID == targetID {
		return ErrCannotKickSelf
	}
	if s.roomRepo == nil || s.departures == nil {
		return errors.New("room and departure repositories are required")
	}

	return s.withRoomMutationLock(ctx, roomID, leaderID, func(lockedCtx context.Context) error {
		previous, err := s.departures.FindDeparture(lockedCtx, roomID)
		if err != nil {
			return err
		}
		isReplay := previous.Kicked && previous.Member.CharacterID == targetID && previous.LeaderID == leaderID

		room, err := s.roomRepo.GetRoomForUpdate(lockedCtx, roomID)
		if errors.Is(err, ErrRoomNotFound) && isReplay {
			return nil
		}
		if err != nil {
			return err
		}
		if room.LeaderCharacterID != leaderID {
			return ErrNotLeader
		}
		if room.Round > 0 {
			return ErrGameInProgress
		}

		members, err := s.roomRepo.ListMembersForUpdate(lockedCtx, roomID)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(members, func(m RoomMember) bool { return m.CharacterID == targetID })
		if index < 0 {
			if isReplay {
				return nil
			}
			return ErrMemberNotFound
		}
		if room.DepartureVersion != previous.Version {
			return ErrDepartureRecovery
		}
		member := members[index]
		if previous.Member.CharacterID == targetID && previous.Member.JoinedAt.Equal(member.JoinedAt) {
			return ErrDepartureRecovery
		}

		claimed := departureClaimedIDs(members)
		remaining := slices.Delete(members, index, index+1)
		now := time.Now().UTC()
		room.UpdatedAt = now
		room.DepartureVersion = previous.Version + 1
		d := Departure{
			Version:      room.DepartureVersion,
			Member:       member,
			Detail:       RoomDetail{Room: *room, Members: remaining},
			PasswordHash: room.PasswordHash,
			ClaimedIDs:   claimed,
			Pending:      true,
			Kicked:       true,
			LeaderID:     leaderID,
			WaitUntil:    now.Add(KickActionWait),
		}
		if err := s.commitDeparture(lockedCtx, d, false); err != nil {
			return err
		}
		return s.publishDeparture(lockedCtx, d)
	})
}

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
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
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
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
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

// LeaveRoom removes a character from a room, transferring leader or disbanding if empty (party2/lib/_casino.cgi:193-239).
func (s *Service) LeaveRoom(ctx context.Context, roomID string, characterID string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if s.roomRepo == nil {
		return errors.New("room repository is required")
	}

	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		var secondarySyncErr error
		txErr := s.runInTx(lockedCtx, func(txCtx context.Context) error {
			room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			members, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			index := slices.IndexFunc(members, func(m RoomMember) bool { return m.CharacterID == characterID })
			if index < 0 {
				return ErrMemberNotFound
			}
			member := members[index]
			remaining := slices.Delete(members, index, index+1)

			// Participating player leaving room incurs fatigue penalty (party2/lib/_casino.cgi:216-218)
			if !member.IsSpectator && s.charRepo != nil {
				char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
				if err != nil {
					return err
				}
				char.AddTired(1)
				if err := s.charRepo.Update(txCtx, char); err != nil {
					return err
				}
			}

			if !member.IsSpectator {
				firstActive := slices.IndexFunc(remaining, func(m RoomMember) bool { return !m.IsSpectator })
				if firstActive < 0 {
					room.Status = RoomStatusDisbanded
				} else if room.LeaderCharacterID == characterID {
					room.LeaderCharacterID = remaining[firstActive].CharacterID
				}
			}
			room.UpdatedAt = time.Now().UTC()
			if err := s.roomRepo.UpdateRoomDetail(txCtx, RoomDetail{Room: *room, Members: remaining}); err != nil {
				if errors.Is(err, ErrSecondarySync) {
					secondarySyncErr = err
				} else {
					return err
				}
			}
			return nil
		})
		return errors.Join(txErr, secondarySyncErr)
	})
}

// KickMember allows leader to kick a non-leader member while round is 0 (party2/lib/_casino.cgi:148-187).
func (s *Service) KickMember(ctx context.Context, roomID string, leaderID string, targetID string) error {
	if leaderID == "" || targetID == "" {
		return ErrInvalidCharacterID
	}
	if leaderID == targetID {
		return ErrCannotKickSelf
	}
	if s.roomRepo == nil {
		return errors.New("room repository is required")
	}

	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
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

			_, err = s.roomRepo.GetMember(txCtx, roomID, targetID)
			if err != nil {
				return ErrMemberNotFound
			}

			return s.roomRepo.RemoveMember(txCtx, roomID, targetID)
		})
	})
}

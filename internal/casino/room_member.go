package casino

import (
	"context"
	"errors"
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

	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		return s.runInTx(lockedCtx, func(txCtx context.Context) error {
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
	})
	if err != nil {
		return nil, err
	}

	return s.GetRoomDetail(ctx, roomID, characterID)
}

// SpectateRoom adds a character as a spectator to a room (party2/lib/casino.cgi:475-518).
func (s *Service) SpectateRoom(ctx context.Context, roomID string, characterID string, password string) (*RoomDetail, error) {
	if characterID == "" {
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
	})
	if err != nil {
		return nil, err
	}

	return s.GetRoomDetail(ctx, roomID, characterID)
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
		return s.runInTx(lockedCtx, func(txCtx context.Context) error {
			room, err := s.roomRepo.GetRoomForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}
			member, err := s.roomRepo.GetMemberForUpdate(txCtx, roomID, characterID)
			if err != nil {
				return ErrMemberNotFound
			}

			if err := s.roomRepo.RemoveMember(txCtx, roomID, characterID); err != nil {
				return err
			}

			// If leaving member was spectator, no leader transfer or disband needed
			if member.IsSpectator {
				return nil
			}

			remaining, err := s.roomRepo.ListMembersForUpdate(txCtx, roomID)
			if err != nil {
				return err
			}

			// Count remaining active non-spectators
			var activeMembers []RoomMember
			for _, m := range remaining {
				if !m.IsSpectator {
					activeMembers = append(activeMembers, m)
				}
			}

			if len(activeMembers) == 0 {
				room.Status = RoomStatusDisbanded
				return s.roomRepo.UpdateRoom(txCtx, *room)
			}

			if room.LeaderCharacterID == characterID {
				room.LeaderCharacterID = activeMembers[0].CharacterID
				if err := s.roomRepo.UpdateRoom(txCtx, *room); err != nil {
					return err
				}
			}

			return nil
		})
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

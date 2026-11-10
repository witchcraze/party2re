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

	return s.withRoomMutationLock(ctx, roomID, leaderID, func(lockedCtx context.Context) error {
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

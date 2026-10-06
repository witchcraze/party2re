package casino

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// ListActiveRooms lists non-disbanded rooms from the active sorted set with lazy pruning.
func (v *ValkeyRoomRepository) ListActiveRooms(ctx context.Context) ([]RoomDetail, error) {
	now := time.Now().UTC()
	cutoff := float64(now.Add(-DefaultLobbyTTL).Unix())
	remCmd := v.client.B().Zremrangebyscore().Key(DefaultRoomsActiveIndexKey).Min("-inf").Max(strconv.FormatFloat(cutoff, 'f', 0, 64)).Build()
	if err := v.client.Do(ctx, remCmd).Error(); err != nil {
		return nil, err
	}

	cmd := v.client.B().Zrevrange().Key(DefaultRoomsActiveIndexKey).Start(0).Stop(99).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		return nil, err
	}

	roomIDs, err := res.AsStrSlice()
	if err != nil {
		return nil, err
	}

	var list []RoomDetail
	for _, id := range roomIDs {
		dto, err := v.getRoomDetail(ctx, id)
		if err != nil {
			if errors.Is(err, ErrRoomNotFound) {
				if err := v.client.Do(ctx, v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(id).Build()).Error(); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
			continue
		}
		if dto.Room.Status != RoomStatusDisbanded {
			list = append(list, RoomDetail{
				Room:    dto.Room,
				Members: dto.Members,
			})
		}
	}
	return list, nil
}

// PurgeIdleRooms explicitly deletes rooms whose last activity is before cutoff.
func (v *ValkeyRoomRepository) PurgeIdleRooms(ctx context.Context, cutoff time.Time) (int, error) {
	cmd := v.client.B().Zrangebyscore().Key(DefaultRoomsActiveIndexKey).
		Min("-inf").Max(strconv.FormatFloat(float64(cutoff.Unix()), 'f', 0, 64)).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		return 0, err
	}
	expiredIDs, err := res.AsStrSlice()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range expiredIDs {
		err := v.WithRoomLock(ctx, id, func(lockedCtx context.Context) error {
			room, err := v.GetRoom(lockedCtx, id)
			if err != nil && !errors.Is(err, ErrRoomNotFound) {
				return err
			}
			if room != nil && room.UpdatedAt.After(cutoff) {
				return nil
			}
			if err := v.DeleteRoom(lockedCtx, id); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func (v *ValkeyRoomRepository) populateMemberName(ctx context.Context, m *RoomMember) error {
	if m.CharacterName != "" || v.charRepo == nil {
		return nil
	}
	ch, err := v.charRepo.FindByID(ctx, m.CharacterID)
	if err != nil {
		return err
	}
	m.CharacterName = ch.Name
	return nil
}

func (v *ValkeyRoomRepository) populateMemberNames(ctx context.Context, members []RoomMember) error {
	for i := range members {
		if err := v.populateMemberName(ctx, &members[i]); err != nil {
			return err
		}
	}
	return nil
}

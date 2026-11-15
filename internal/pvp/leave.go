package pvp

import (
	"context"
	"errors"
	"slices"
	"time"
)

// LeaveRoom departs a live PvP room (@にげる) across recruiting, in-progress, and completed phases.
func (s *Service) LeaveRoom(ctx context.Context, characterID string, roomID string) error {
	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		last, err := s.transitions.FindTransition(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if (last.Kind == "disband" || last.Kind == "forfeit") && last.ActorID == characterID {
			return nil
		}
		if last.Kind == "abandon" && slices.Contains(last.RemovedIDs, characterID) {
			return nil
		}
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if errors.Is(err, ErrRoomNotFound) {
			if last.Detail.Room.Status == StatusCompleted && hasRoomMember(last.Detail.Members, characterID) {
				return s.repo.DeleteCharacterRoom(lockedCtx, characterID, roomID)
			}
			return s.abandonLostRoom(lockedCtx, characterID, last)
		}
		if err != nil {
			return err
		}

		char, err := s.characters.FindByID(lockedCtx, characterID)
		if err != nil {
			return err
		}

		found := false
		isLeader := false
		for _, m := range detail.Members {
			if m.CharacterID == characterID {
				found = true
				isLeader = m.IsLeader
				break
			}
		}
		if !found {
			if last.Kind == "leave" && last.ActorID == characterID {
				return nil
			}
			if detail.Room.Status == StatusCompleted && hasRoomMember(last.Detail.Members, characterID) {
				return s.repo.DeleteCharacterRoom(lockedCtx, char.ID, roomID)
			}
			return ErrCharacterNotInRoom
		}

		now := time.Now().UTC()
		hasRemaining := removeMemberAndTransferLeader(&detail, characterID, isLeader)

		switch detail.Room.Status {
		case StatusRecruiting:
			refundAmount := 0
			if detail.Room.Round == 0 {
				refundAmount = detail.Room.Bet
			}
			kind := "disband"
			if hasRemaining {
				kind = "leave"
				detail.Room.PrizePool -= refundAmount
				if detail.Room.PrizePool < 0 {
					detail.Room.PrizePool = 0
				}
			} else {
				detail.Room.Status = StatusDisbanded
				detail.Room.PrizePool = 0
			}
			detail.Room.UpdatedAt = now
			return s.commitTransition(lockedCtx, Transition{
				Kind:       kind,
				ActorID:    characterID,
				Detail:     detail,
				RemovedIDs: []string{char.ID},
			}, func(txCtx context.Context) error {
				return s.refundMembers(txCtx, []string{char.ID}, refundAmount)
			})

		case StatusInProgress:
			kind := "forfeit"
			forfeitedPool := 0
			if hasRemaining {
				kind = "leave"
			} else {
				forfeitedPool = detail.Room.PrizePool
				detail.Room.Status = StatusDisbanded
				detail.Room.PrizePool = 0
			}
			detail.Room.UpdatedAt = now
			return s.commitTransition(lockedCtx, Transition{
				Kind:          kind,
				ActorID:       characterID,
				Detail:        detail,
				RemovedIDs:    []string{char.ID},
				ForfeitedPool: forfeitedPool,
			}, func(context.Context) error { return nil })

		case StatusCompleted:
			if err := s.repo.DeleteCharacterRoom(lockedCtx, char.ID, roomID); err != nil {
				return err
			}
			if hasRemaining {
				detail.Room.UpdatedAt = now
				return s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members)
			}
			return s.repo.DeleteRoom(lockedCtx, roomID)

		default:
			return s.repo.DeleteCharacterRoom(lockedCtx, char.ID, roomID)
		}
	})
}

func removeMemberAndTransferLeader(detail *RoomDetail, characterID string, isLeader bool) bool {
	updated := make([]RoomMember, 0, len(detail.Members))
	for _, m := range detail.Members {
		if m.CharacterID != characterID {
			updated = append(updated, m)
		}
	}
	if len(updated) == 0 {
		return false
	}
	if isLeader {
		for i := range updated {
			updated[i].IsLeader = i == 0
		}
		detail.Room.LeaderCharacterID = updated[0].CharacterID
	}
	detail.Members = updated
	return true
}

func hasRoomMember(members []RoomMember, characterID string) bool {
	for _, m := range members {
		if m.CharacterID == characterID {
			return true
		}
	}
	return false
}

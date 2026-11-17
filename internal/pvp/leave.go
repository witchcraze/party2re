package pvp

import (
	"context"
	"errors"
	"slices"
)

// LeaveRoom departs a live PvP room (@にげる) across recruiting, in-progress, and completed phases.
func (s *Service) LeaveRoom(ctx context.Context, characterID string, roomID string) error {
	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		last := lastTransitionFromContext(lockedCtx)
		if (last.Kind == "disband" || last.Kind == "forfeit") && last.ActorID == characterID {
			return nil
		}
		if (last.Kind == "abandon" || last.Kind == "timeout") && slices.Contains(last.RemovedIDs, characterID) {
			return nil
		}
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if errors.Is(err, ErrRoomNotFound) {
			if last.Version == 0 || last.Detail.Room.Status == StatusDisbanded {
				return ErrRoomNotFound
			}
			if !hasRoomMember(last.Detail.Members, characterID) {
				return ErrCharacterNotInRoom
			}
			claim, claimErr := s.transitions.FindCharacterClaim(lockedCtx, characterID)
			if claimErr != nil {
				return claimErr
			}
			if claim != last.Detail.Room.ID {
				return ErrCharacterNotInRoom
			}
			if s.isInactive(last.LastActivityAt, s.now()) {
				_, cleanErr := s.cleanupInactiveRoom(lockedCtx, last, s.now())
				return cleanErr
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
			return ErrCharacterNotInRoom
		}

		now := s.now()
		if last.Version > 0 && last.Detail.Room.Status != StatusDisbanded && s.isInactive(last.LastActivityAt, now) {
			_, cleanErr := s.cleanupInactiveRoom(lockedCtx, last, now)
			return cleanErr
		}
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
			detail.Room.LastActivityAt = now
			return s.commitTransition(lockedCtx, Transition{
				Kind:           kind,
				ActorID:        characterID,
				Detail:         detail,
				RemovedIDs:     []string{char.ID},
				LastActivityAt: now,
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
			detail.Room.LastActivityAt = now
			return s.commitTransition(lockedCtx, Transition{
				Kind:           kind,
				ActorID:        characterID,
				Detail:         detail,
				RemovedIDs:     []string{char.ID},
				ForfeitedPool:  forfeitedPool,
				LastActivityAt: now,
			}, func(context.Context) error { return nil })

		case StatusCompleted:
			kind := "disband"
			if hasRemaining {
				kind = "leave"
			} else {
				detail.Room.Status = StatusDisbanded
			}
			detail.Room.UpdatedAt = now
			detail.Room.LastActivityAt = now
			return s.commitTransition(lockedCtx, Transition{
				Kind:           kind,
				ActorID:        characterID,
				Detail:         detail,
				RemovedIDs:     []string{char.ID},
				LastActivityAt: now,
			}, func(context.Context) error { return nil })

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
		detail.Room.LeaderName = updated[0].CharacterName
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

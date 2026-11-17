package pvp

import (
	"context"
)

// abandonLostRoom follows legacy missing-file recovery: release membership without a refund.
func (s *Service) abandonLostRoom(ctx context.Context, characterID string, last Transition) error {
	if last.Version == 0 || (last.Detail.Room.Status != StatusRecruiting && last.Detail.Room.Status != StatusInProgress && last.Detail.Room.Status != StatusCompleted) {
		return ErrRoomNotFound
	}
	found := false
	memberIDs := make([]string, 0, len(last.Detail.Members))
	for _, member := range last.Detail.Members {
		memberIDs = append(memberIDs, member.CharacterID)
		if member.CharacterID == characterID {
			found = true
		}
	}
	if !found {
		return ErrCharacterNotInRoom
	}
	claim, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil {
		return err
	}
	if claim != last.Detail.Room.ID {
		return ErrCharacterNotInRoom
	}
	now := s.now()
	beforePool := last.Detail.Room.PrizePool
	detail := last.Detail
	detail.Room.Status = StatusDisbanded
	detail.Room.PrizePool = 0
	detail.Room.UpdatedAt = now
	detail.Room.LastActivityAt = now
	return s.commitTransition(ctx, Transition{Kind: "abandon", ActorID: characterID, Detail: detail, RemovedIDs: memberIDs, ForfeitedPool: last.ForfeitedPool + beforePool, LastActivityAt: now}, func(context.Context) error { return nil })
}

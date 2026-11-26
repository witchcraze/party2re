package pvp

import (
	"context"
	"errors"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

		departing, found := findMember(detail.Members, characterID)
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
		hasRemaining := removeMemberAndTransferLeader(&detail, characterID, departing.IsLeader)

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
				if err := s.refundMembers(txCtx, []string{char.ID}, refundAmount); err != nil {
					return err
				}
				return s.applyExitSideEffects(txCtx, char.ID, detail.Room.Round, departing)
			})

		case StatusInProgress:
			kind := "forfeit"
			forfeitedPool := 0
			var pendingKills map[string]int
			if hasRemaining {
				kind = "leave"
			} else {
				forfeitedPool = detail.Room.PrizePool
				detail.Room.Status = StatusDisbanded
				detail.Room.PrizePool = 0
				pendingKills = cloneIntMap(detail.Room.PendingPvPKills)
				detail.Room.PendingPvPKills = nil
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
			}, func(txCtx context.Context) error {
				if err := s.lockExitCharacters(txCtx, char.ID, pendingKills); err != nil {
					return err
				}
				if err := s.settleRewards(txCtx, nil, "", 0, pendingKills); err != nil {
					return err
				}
				return s.applyExitSideEffects(txCtx, char.ID, detail.Room.Round, departing)
			})

		case StatusCompleted:
			kind := "disband"
			var preservedResult *RoundResolution
			if hasRemaining {
				kind = "leave"
				preservedResult = last.Result
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
				Result:         preservedResult,
				ForfeitedPool:  last.ForfeitedPool,
				LastActivityAt: now,
			}, func(txCtx context.Context) error {
				return s.applyExitSideEffects(txCtx, char.ID, detail.Room.Round, departing)
			})

		default:
			return s.repo.DeleteCharacterRoom(lockedCtx, char.ID, roomID)
		}
	})
}

// KickMember removes a participant before match start (@きっく, _skill.cgi:796-837),
// applying the 60s ActionWait without bet refund or @にげる exit side-effects.
func (s *Service) KickMember(ctx context.Context, roomID, leaderID, targetID string) (KickResult, error) {
	if roomID == "" {
		return KickResult{}, ErrRoomNotFound
	}
	if leaderID == "" || targetID == "" {
		return KickResult{}, ErrCharacterNotInRoom
	}
	if leaderID == targetID {
		return KickResult{}, ErrCannotKickSelf
	}
	var result KickResult
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		last := lastTransitionFromContext(lockedCtx)
		if recoveredPendingFromContext(lockedCtx) && last.Kind == "kick" && last.ActorID == leaderID && slices.Contains(last.RemovedIDs, targetID) {
			result = kickResultFromDetail(last.Detail, leaderID)
			return nil
		}
		detail, err := s.checkInactiveLocked(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.LeaderCharacterID != leaderID {
			return ErrNotRoomLeader
		}
		if detail.Room.Status != StatusRecruiting || detail.Room.Round > 0 {
			return ErrMatchAlreadyStarted
		}
		if !hasRoomMember(detail.Members, targetID) {
			return ErrCharacterNotInRoom
		}

		res := kickResultFromDetail(detail, leaderID)
		removeMemberAndTransferLeader(&detail, targetID, false)
		now := s.now()
		detail.Room.UpdatedAt = now
		detail.Room.LastActivityAt = now
		if err := s.commitActivityTransition(lockedCtx, last, Transition{
			Kind:           "kick",
			ActorID:        leaderID,
			Detail:         detail,
			RemovedIDs:     []string{targetID},
			LastActivityAt: now,
			PenaltyUntil:   now.Add(KickActionWait),
		}); err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return KickResult{}, err
	}
	return result, nil
}

func kickResultFromDetail(detail RoomDetail, leaderID string) KickResult {
	leaderName := detail.Room.LeaderName
	if leaderName == "" {
		if m, ok := findMember(detail.Members, leaderID); ok {
			leaderName = m.CharacterName
		}
	}
	return KickResult{RoomName: detail.Room.Name, LeaderName: leaderName}
}

func (s *Service) lockExitCharacters(ctx context.Context, charID string, pendingKills map[string]int) error {
	if len(pendingKills) == 0 {
		return nil
	}
	ids := []string{charID}
	for id, count := range pendingKills {
		if id != "" && count > 0 {
			ids = append(ids, id)
		}
	}
	sortedIDs := sortCharacterIDs(ids)
	if len(sortedIDs) <= 1 {
		return nil
	}
	for _, id := range sortedIDs {
		if _, err := s.characters.FindByIDForUpdate(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applyExitSideEffects(ctx context.Context, charID string, round int, departing RoomMember) error {
	exitFatigue := round*3 + 1
	if s.settler == nil {
		char, err := s.characters.FindByIDForUpdate(ctx, charID)
		if err != nil {
			return err
		}
		remMP := -1
		if departing.MP != nil {
			remMP = *departing.MP
		}
		char.ApplyCombatSurvival(departing.HP, remMP, departing.HP <= 0)
		char.AddTired(exitFatigue)
		if err := s.characters.Update(ctx, char); err != nil {
			return err
		}
	} else {
		var remMP map[string]int
		if departing.MP != nil {
			remMP = map[string]int{charID: *departing.MP}
		}
		var consumed map[string][]corebattle.ConsumedItem
		if len(departing.ConsumedItems) > 0 {
			consumed = map[string][]corebattle.ConsumedItem{charID: departing.ConsumedItems}
		}
		req := battle.ApplyPostBattleRequest{
			CharacterIDs:          []string{charID},
			PreserveEarnedRewards: true,
			RecipientRewards:      map[string]corebattle.Reward{charID: {}},
			ExitFatigue:           exitFatigue,
			ExitBonusRounds:       round,
			BattleResult: corebattle.PartyBattleResult{
				RemainingHP:   map[string]int{charID: departing.HP},
				RemainingMP:   remMP,
				ConsumedItems: consumed,
			},
		}
		if _, err := s.settler.ApplyPostBattleResult(ctx, req); err != nil {
			return err
		}
	}
	if s.postExitHook != nil {
		if _, err := s.postExitHook(ctx, charID); err != nil {
			return err
		}
	}
	return nil
}

func findMember(members []RoomMember, characterID string) (RoomMember, bool) {
	for _, m := range members {
		if m.CharacterID == characterID {
			return m, true
		}
	}
	return RoomMember{}, false
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

package gvg

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// LeaveRoom exits a GvG room (@にげる, _battle.cgi:1184-1247,1330-1375) across all phases,
// transferring leadership to the first remaining member and disbanding only when empty.
// SQL exit side-effects commit in applyExitSideEffects independently of Valkey room publication and mapping cleanup.
// The result carries the standing Tavern delivery settled by this call; recovery replays carry none.
func (s *Service) LeaveRoom(ctx context.Context, charID string, roomID string) (LeaveResult, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return LeaveResult{}, ErrRoomNotFound
	}
	var result LeaveResult
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if err != nil {
			if errors.Is(err, ErrRoomNotFound) {
				return s.cleanupPendingCharacterRoom(lockedCtx, charID, roomID, ErrRoomNotFound)
			}
			return err
		}
		detail, err = s.recoverPendingRoundTransition(lockedCtx, roomID, detail)
		if err != nil {
			return err
		}

		departing, found := findMember(detail.Members, charID)
		if !found {
			return s.cleanupPendingCharacterRoom(lockedCtx, charID, roomID, ErrCharacterNotInRoom)
		}

		round := detail.Room.Round
		isPreStart := detail.Room.Status == StatusRecruiting && round == 0
		isLeader := departing.IsLeader || detail.Room.LeaderCharacterID == charID
		hasRemaining := removeMemberAndTransferLeader(&detail, charID, isLeader)
		if hasRemaining && isPreStart {
			// Legacy _battle.cgi:1238 & system.cgi:1357 deduct 2 GP with no floor.
			detail.Room.PrizePool -= LeavePrizeGP
		}
		detail.Room.UpdatedAt = time.Now().UTC()

		delivery, err := s.applyExitSideEffects(lockedCtx, roomID, charID, round, departing)
		if err != nil {
			return err
		}
		result.Delivery = delivery

		if !hasRemaining {
			if err := s.repo.DeleteRoom(lockedCtx, roomID); err != nil {
				return err
			}
		} else {
			if err := s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members); err != nil {
				return err
			}
		}
		return s.repo.DeleteCharacterRoom(lockedCtx, charID, roomID)
	})
	if err != nil {
		return LeaveResult{}, err
	}
	return result, nil
}

func (s *Service) cleanupPendingCharacterRoom(ctx context.Context, charID, roomID string, notPendingErr error) error {
	mappedRoomID, err := s.repo.GetCharacterRoom(ctx, charID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return notPendingErr
		}
		return err
	}
	if mappedRoomID != roomID {
		return notPendingErr
	}
	return s.repo.DeleteCharacterRoom(ctx, charID, roomID)
}

func (s *Service) recoverPendingRoundTransition(ctx context.Context, roomID string, detail RoomDetail) (RoomDetail, error) {
	committed, found, err := s.standings.FindRoundTransition(ctx, roomID)
	if err != nil {
		return RoomDetail{}, err
	}
	if !found || !hasPendingRoundPublication(detail, committed) {
		return detail, nil
	}
	if err := s.repo.SaveRoom(ctx, committed.Detail.Room, committed.Detail.Members); err != nil {
		return RoomDetail{}, err
	}
	return committed.Detail, nil
}

func (s *Service) applyExitSideEffects(ctx context.Context, roomID, charID string, round int, departing GvGMember) (*DeliveryOutcome, error) {
	exitFatigue := round*3 + 1
	if s.settler == nil {
		char, err := s.characters.FindByID(ctx, charID)
		if err != nil {
			return nil, err
		}
		remMP := -1
		if departing.MP != nil {
			remMP = *departing.MP
		}
		char.ApplyCombatSurvival(departing.HP, remMP, departing.HP <= 0)
		char.AddTired(exitFatigue)
		if err := s.characters.Update(ctx, char); err != nil {
			return nil, err
		}
		if s.postExitHook == nil {
			return nil, nil
		}
		return s.postExitHook(ctx, charID)
	}

	joinID := memberJoinKey(departing)
	var delivery *DeliveryOutcome
	if err := s.runInTx(ctx, func(txCtx context.Context) error {
		var claim ExitClaimState
		if s.exitClaims != nil {
			var err error
			claim, err = s.exitClaims.ClaimRoomExitForUpdate(txCtx, roomID, charID, joinID)
			if err != nil {
				return err
			}
		}
		if !claim.AlreadyClaimed {
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
			if _, err := s.settler.ApplyPostBattleResult(txCtx, req); err != nil {
				return err
			}
		}
		if s.postExitHook != nil && !claim.DeliveryDone {
			// Acquire the Rank 2 character lock before ResetFullness writes Rank 8 tavern_character_status
			// when post-battle settlement was already claimed in an earlier transaction.
			if claim.AlreadyClaimed {
				if _, err := s.characters.FindByIDForUpdate(txCtx, charID); err != nil {
					return err
				}
			}
			var err error
			if delivery, err = s.postExitHook(txCtx, charID); err != nil {
				return err
			}
			if s.exitClaims != nil {
				if err := s.exitClaims.MarkExitDeliveryDone(txCtx, roomID, charID, joinID); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return delivery, nil
}

func memberJoinKey(m GvGMember) string {
	if m.JoinID != "" {
		return m.JoinID
	}
	if !m.JoinedAt.IsZero() {
		return m.JoinedAt.UTC().Format(time.RFC3339Nano)
	}
	return "legacy"
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func findMember(members []GvGMember, charID string) (GvGMember, bool) {
	for _, m := range members {
		if m.CharacterID == charID {
			return m, true
		}
	}
	return GvGMember{}, false
}

func removeMemberAndTransferLeader(detail *RoomDetail, charID string, isLeader bool) bool {
	updated := make([]GvGMember, 0, len(detail.Members))
	for _, m := range detail.Members {
		if m.CharacterID != charID {
			updated = append(updated, m)
		}
	}
	if len(updated) == 0 {
		detail.Members = nil
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

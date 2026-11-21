package gvg

import (
	"context"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// LeaveRoom exits a GvG room (@にげる, _battle.cgi:1184-1247,1330-1375) across all phases,
// transferring leadership to the first remaining member and disbanding only when empty.
// SQL exit side-effects commit in applyExitSideEffects independently of Valkey cleanup and postExitHook.
func (s *Service) LeaveRoom(ctx context.Context, charID string, roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return ErrRoomNotFound
	}
	return s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if err != nil {
			return err
		}

		departing, found := findMember(detail.Members, charID)
		if !found {
			return ErrCharacterNotInRoom
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

		if err := s.applyExitSideEffects(lockedCtx, roomID, charID, round, departing); err != nil {
			return err
		}

		if err := s.repo.DeleteCharacterRoom(lockedCtx, charID); err != nil {
			return err
		}
		if !hasRemaining {
			return s.repo.DeleteRoom(lockedCtx, roomID)
		}
		return s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members)
	})
}

func (s *Service) applyExitSideEffects(ctx context.Context, roomID, charID string, round int, departing GvGMember) error {
	exitFatigue := round*3 + 1
	if s.settler == nil {
		char, err := s.characters.FindByID(ctx, charID)
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
		if s.postExitHook != nil {
			if _, err := s.postExitHook(ctx, charID); err != nil {
				return err
			}
		}
		return nil
	}

	joinID := memberJoinKey(departing)
	var claim ExitClaimState
	if err := s.runInTx(ctx, func(txCtx context.Context) error {
		if s.exitClaims != nil {
			var err error
			claim, err = s.exitClaims.ClaimRoomExitForUpdate(txCtx, roomID, charID, joinID)
			if err != nil {
				return err
			}
		}
		if claim.AlreadyClaimed {
			return nil
		}
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
		_, err := s.settler.ApplyPostBattleResult(txCtx, req)
		return err
	}); err != nil {
		return err
	}

	if s.postExitHook != nil && !claim.DeliveryDone {
		if _, err := s.postExitHook(ctx, charID); err != nil {
			return err
		}
		if s.exitClaims != nil {
			if err := s.exitClaims.MarkExitDeliveryDone(ctx, roomID, charID, joinID); err != nil {
				return err
			}
		}
	}
	return nil
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

package boss

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/party"
)

// SealingSettlement is committed with all participant assets and retained for cleanup retries.
type SealingSettlement struct {
	Lobby  party.LobbyState
	Result SealingBattleResult
}

type SealingSettlementRepository interface {
	// LockSealingSettlement serializes a room before character locks; nil means no committed battle.
	LockSealingSettlement(ctx context.Context, roomID string) (*SealingSettlement, error)
	SaveSealingSettlement(ctx context.Context, settlement SealingSettlement) error
}

func (s *Service) completeSealingBattle(ctx context.Context, partyID, leaderID string) (SealingBattleResult, error) {
	if s.settlementRepo == nil {
		return SealingBattleResult{}, errors.New("sealing settlement repository not configured")
	}
	var receipt *SealingSettlement
	var original *party.Party
	var slayerName string
	known := false
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		var err error
		receipt, err = s.settlementRepo.LockSealingSettlement(txCtx, partyID)
		known = err == nil && receipt != nil
		if err != nil || receipt != nil {
			return err
		}
		receipt, slayerName, err = s.settleSealingBattle(txCtx, partyID, leaderID, &original)
		return err
	})
	if err != nil && !known {
		if original == nil {
			return SealingBattleResult{}, err
		}
		// A locking read waits for the original transaction's outcome, including a lost COMMIT response.
		recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		receipt = nil
		probeErr := s.runInTx(recoveryCtx, func(txCtx context.Context) error {
			var probeErr error
			receipt, probeErr = s.settlementRepo.LockSealingSettlement(txCtx, partyID)
			known = probeErr == nil && receipt != nil
			if probeErr != nil || receipt != nil {
				return probeErr
			}
			return s.partyRepo.UpdateParty(txCtx, *original)
		})
		if probeErr != nil && !known {
			return SealingBattleResult{}, errors.Join(err, fmt.Errorf("sealing outcome/release unresolved: %w", probeErr))
		}
		if receipt == nil {
			return SealingBattleResult{}, err
		}
	}
	if receipt.Lobby.Party.LeaderCharacterID != leaderID {
		return SealingBattleResult{}, ErrNotPartyLeader
	}
	result := receipt.Result
	result.BanquetHeld = false
	if !known {
		s.holdVictoryBanquet(ctx, &result, leaderID, slayerName)
	}
	if err := s.partyRepo.CleanupParty(ctx, receipt.Lobby); err != nil {
		return result, fmt.Errorf("sealing committed; cleanup pending: %w", err)
	}
	return result, nil
}

func (s *Service) holdVictoryBanquet(ctx context.Context, result *SealingBattleResult, slayerID, slayerName string) {
	if result.Outcome != corebattle.OutcomeWin || s.victoryBanquetHook == nil {
		return
	}
	//lint:ignore error-swallow best-effort ephemeral crowd after SQL commit; never replay an uncertain append
	if err := s.victoryBanquetHook(ctx, result.StageID, result.StageName, slayerID, slayerName, stageTierFromID(result.StageID)); err != nil {
		slog.WarnContext(ctx, "boss victory banquet failed", "error_type", reflect.TypeOf(err).String())
		return
	}
	result.BanquetHeld = true
}

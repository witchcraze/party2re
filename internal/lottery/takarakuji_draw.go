package lottery

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// DrawTakarakuji executes the scheduled lottery draw for the currently active round.
// It runs atomically within an ambient transaction boundary, validates the draw date,
// picks winners, delivers prizes to winner depots, settles the round, and initializes the next round.
func (s *Service) DrawTakarakuji(ctx context.Context, now time.Time) (TakarakujiDrawResult, error) {
	var result TakarakujiDrawResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		round, err := s.repo.GetActiveTakarakujiRoundForUpdate(txCtx)
		if err != nil {
			return err
		}
		if round.IsDrawn {
			return ErrAlreadyDrawn
		}
		if now.Before(round.DrawDate) {
			return ErrNotReadyToDraw
		}

		tickets, err := s.repo.ListRoundTakarakujiTickets(txCtx, round.RoundID)
		if err != nil {
			return err
		}

		pool := make([]string, 0, TakarakujiMaxTickets)
		for _, t := range tickets {
			pool = append(pool, t.CharacterID)
		}
		for len(pool) < TakarakujiMaxTickets {
			pool = append(pool, fmt.Sprintf("<dummy_%d>", len(pool)))
		}

		type prizeSpec struct {
			rank   int
			itemID string
			amount int
		}
		prizes := []prizeSpec{
			{rank: 1, itemID: round.Prize1ItemID, amount: round.Prize1Amount},
			{rank: 2, itemID: round.Prize2ItemID, amount: round.Prize2Amount},
			{rank: 3, itemID: round.Prize3ItemID, amount: round.Prize3Amount},
		}

		var winners []TakarakujiWinner
		winningTicketsMap := make(map[string]TakarakujiTicket)

		type pendingDelivery struct {
			winnerID string
			itemID   string
		}
		var deliveries []pendingDelivery

		for _, p := range prizes {
			for i := 0; i < p.amount && len(pool) > 0; i++ {
				idxBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(pool))))
				if err != nil {
					return err
				}
				idx := int(idxBig.Int64())
				winnerID := pool[idx]

				pool = append(pool[:idx], pool[idx+1:]...)

				isDummy := strings.HasPrefix(winnerID, "<dummy_")
				winner := TakarakujiWinner{
					Rank:        p.rank,
					CharacterID: winnerID,
					ItemID:      p.itemID,
					ItemName:    s.resolveItemName(p.itemID),
					IsDummy:     isDummy,
				}
				winners = append(winners, winner)

				if !isDummy {
					for _, t := range tickets {
						if t.CharacterID == winnerID {
							itemCopy := p.itemID
							t.WonRank = p.rank
							t.WonItemID = &itemCopy
							winningTicketsMap[t.ID] = t
							break
						}
					}

					deliveries = append(deliveries, pendingDelivery{
						winnerID: winnerID,
						itemID:   p.itemID,
					})
				}
			}
		}

		// Sort deliveries by winnerID ascending to enforce deterministic Rank 5 lock ordering
		slices.SortFunc(deliveries, func(a, b pendingDelivery) int {
			return cmp.Compare(a.winnerID, b.winnerID)
		})

		for _, d := range deliveries {
			if err := s.deliverPrizeToDepot(txCtx, d.winnerID, d.itemID); err != nil {
				return fmt.Errorf("failed delivering prize to depot for winner %s: %w", d.winnerID, err)
			}
		}

		var winningTickets []TakarakujiTicket
		for _, t := range winningTicketsMap {
			winningTickets = append(winningTickets, t)
		}

		if err := s.repo.SettleTakarakujiRound(txCtx, round.RoundID, now, winningTickets); err != nil {
			return err
		}

		p1, a1, p2, a2, p3, a3, err := RollNewRoundPrizes()
		if err != nil {
			return err
		}
		nextDrawDate := NextDrawDateJST(now)
		nextRound, err := s.repo.CreateTakarakujiRound(txCtx, TakarakujiRound{
			DrawDate:     nextDrawDate,
			IsDrawn:      false,
			Prize1ItemID: p1,
			Prize1Amount: a1,
			Prize2ItemID: p2,
			Prize2Amount: a2,
			Prize3ItemID: p3,
			Prize3Amount: a3,
			CreatedAt:    now,
		})
		if err != nil {
			return err
		}

		result = TakarakujiDrawResult{
			RoundID:   round.RoundID,
			DrawnAt:   now,
			Winners:   winners,
			NextRound: nextRound,
		}
		return nil
	})
	if err != nil {
		return TakarakujiDrawResult{}, err
	}
	return result, nil
}

func (s *Service) deliverPrizeToDepot(ctx context.Context, characterID, itemID string) error {
	if s.depotRepo == nil {
		return nil
	}

	var char corecharacter.Character
	if s.charRepo != nil {
		c, err := s.charRepo.FindByID(ctx, characterID)
		if err == nil {
			char = c
		}
	}

	dep, err := s.depotRepo.FindByCharacterIDForUpdate(ctx, characterID)
	if errors.Is(err, depot.ErrNotFound) {
		dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	dep.RefreshCapacity(char.JobLevel, char.OverDepot)

	inst, err := coreitem.NewInstance(itemID, 1)
	if err != nil {
		return err
	}

	if err := dep.AddItem(inst); err != nil {
		return err
	}

	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return err
	}

	if s.collectionRecorder != nil {
		itemName := s.resolveItemName(itemID)
		_ = s.collectionRecorder.RecordItemDiscovered(ctx, characterID, itemID, itemName, "takarakuji")
	}

	return nil
}

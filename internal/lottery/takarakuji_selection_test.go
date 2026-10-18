package lottery_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/lottery"
)

func TestDrawTakarakuji_AllPoolSlots(t *testing.T) {
	originalReader := rand.Reader
	t.Cleanup(func() { rand.Reader = originalReader })
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)

	for _, sold := range []int{19, 20} {
		for first := 0; first < 20; first++ {
			t.Run(fmt.Sprintf("sold_%d/slot_%d", sold, first), func(t *testing.T) {
				tickets := make([]lottery.TakarakujiTicket, sold)
				for i := range tickets {
					tickets[i] = lottery.TakarakujiTicket{ID: fmt.Sprintf("ticket-%d", i), CharacterID: fmt.Sprintf("char-%d", i)}
				}
				repo := &mockLotteryRepo{
					getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) {
						return lottery.TakarakujiRound{
							RoundID: 1, DrawDate: now,
							Prize1ItemID: "item-129", Prize1Amount: 1,
							Prize2ItemID: "item-168", Prize2Amount: 2,
							Prize3ItemID: "item-126", Prize3Amount: 4,
						}, nil
					},
					listRoundTakarakujiTicketsFn: func(context.Context, int) ([]lottery.TakarakujiTicket, error) {
						return tickets, nil
					},
				}
				svc, err := lottery.NewService(repo)
				if err != nil {
					t.Fatal(err)
				}
				// Cover every initial slot, then the upper boundary of each shrinking pool.
				// The final five bytes generate the next round's prizes and quotas.
				rand.Reader = bytes.NewReader([]byte{byte(first), 18, 17, 16, 15, 14, 13, 0, 0, 0, 0, 0})
				result, err := svc.DrawTakarakuji(context.Background(), now)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Winners) != 7 {
					t.Fatalf("winners = %d, want 7", len(result.Winners))
				}
				wantSlots := []int{first}
				for slot := 19; len(wantSlots) < 7; slot-- {
					if slot != first {
						wantSlots = append(wantSlots, slot)
					}
				}
				for i, winner := range result.Winners {
					slot := wantSlots[i]
					wantID := fmt.Sprintf("char-%d", slot)
					isDummy := slot >= sold
					if isDummy {
						wantID = "<dummy_19>"
					}
					wantRank := 3
					if i == 0 {
						wantRank = 1
					} else if i < 3 {
						wantRank = 2
					}
					if winner.CharacterID != wantID || winner.IsDummy != isDummy || winner.Rank != wantRank {
						t.Errorf("winner %d = %+v, want %s/dummy=%t/rank=%d", i, winner, wantID, isDummy, wantRank)
					}
				}
			})
		}
	}
}

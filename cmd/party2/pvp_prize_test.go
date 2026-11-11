package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/pvp"
)

type prizeBattle struct{}

func (prizeBattle) ResolvePartyBattle(corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}, nil
}

type failSecondPrizeWrite struct {
	depot.RemittanceRepository
	calls int
}

func (f *failSecondPrizeWrite) AddRemittance(ctx context.Context, r depot.Remittance) error {
	if err := f.RemittanceRepository.AddRemittance(ctx, r); err != nil {
		return err
	}
	f.calls++
	if f.calls == 2 {
		return errNoticeAfterWrite
	}
	return nil
}

type failSecondPrizeNotice struct {
	*home.Service
	calls int
}

func (f *failSecondPrizeNotice) AddDeliveryFlag(ctx context.Context, actor, kind, message string) error {
	if err := f.Service.AddDeliveryFlag(ctx, actor, kind, message); err != nil {
		return err
	}
	f.calls++
	if f.calls == 2 {
		return errNoticeAfterWrite
	}
	return nil
}

func TestPvPPrizeRollbackAndRetryIntegration(t *testing.T) {
	for _, fault := range []string{"remittance", "notice"} {
		t.Run(fault, func(t *testing.T) {
			f := newDepotEquipmentFixture(t, item.SlotNone, false)
			ctx := context.Background()
			soc, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop())
			if err != nil {
				t.Fatal(err)
			}
			other, err := database.CreateTestCharacterWithFunds(ctx, f.db, "Prize teammate", 1000)
			if err != nil {
				t.Fatal(err)
			}
			store, err := database.NewPvPTransitionRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			remitter := pvpPrizeRemitter{remittances: f.econ.depotRepo, notifier: soc.home}
			if fault == "remittance" {
				remitter.remittances = &failSecondPrizeWrite{RemittanceRepository: f.econ.depotRepo}
			} else {
				remitter.notifier = &failSecondPrizeNotice{Service: soc.home}
			}
			svc, err := pvp.NewService(pvp.NewMemoryRoomRepository(), f.core.charRepo, prizeBattle{},
				pvp.WithTransactionProvider(f.core.txProvider), pvp.WithTransitionRepository(store), pvp.WithPrizeRemitter(remitter))
			if err != nil {
				t.Fatal(err)
			}
			arena := strings.Repeat("賞", 50)
			room := preparePrizeMatch(t, svc, []string{f.sender.ID, other.ID, f.recipient.ID}, arena)
			// Entry fees are already escrowed; winners can earn money elsewhere before settlement.
			if _, err := f.db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id IN (?, ?)", corecharacter.MaxMoney, f.sender.ID, other.ID); err != nil {
				t.Fatal(err)
			}
			assertWinner := func(actor string, wins, amount int) {
				t.Helper()
				char, err := f.core.charRepo.FindByID(ctx, actor)
				if err != nil || char.Money != corecharacter.MaxMoney || char.PvPWins != wins {
					t.Fatalf("wallet=%d wins=%d: %v", char.Money, char.PvPWins, err)
				}
				held, err := f.econ.depotRepo.ListRemittances(ctx, actor)
				if err != nil {
					t.Fatal(err)
				}
				notices, err := soc.home.ListDeliveryNotices(ctx, actor, true)
				if err != nil {
					t.Fatal(err)
				}
				if len(held) != wins || len(notices) != wins {
					t.Fatalf("held=%+v notices=%+v; want %d each", held, notices, wins)
				}
				if wins == 1 && (held[0].Amount != amount || held[0].Source != depot.RemittancePvPPrize || held[0].SenderName != arena || notices[0].NoticeType != depot.NoticeMoneyTransfer || notices[0].Message != "お金が預かり所に届いています") {
					t.Fatalf("prize=%+v notice=%+v", held, notices)
				}
			}
			if _, err := svc.AdvanceRound(ctx, f.sender.ID, room); !errors.Is(err, errNoticeAfterWrite) {
				t.Fatalf("injected failure: %v", err)
			}
			for _, actor := range []string{f.sender.ID, other.ID} {
				assertWinner(actor, 0, 0)
			}
			detail, err := svc.GetRoom(ctx, room)
			if err != nil || detail.Room.Status != pvp.StatusInProgress || detail.Room.PrizePool != 300 {
				t.Fatalf("failed settlement changed escrow: %+v, %v", detail.Room, err)
			}
			// Retry succeeds with the same adapter, then replay cannot create another prize.
			for range 2 {
				if _, err := svc.AdvanceRound(ctx, f.sender.ID, room); err != nil {
					t.Fatal(err)
				}
			}
			for _, actor := range []string{f.sender.ID, other.ID} {
				assertWinner(actor, 1, 150)
			}
			loser, err := f.core.charRepo.FindByID(ctx, f.recipient.ID)
			if err != nil || loser.Money != 1900 || loser.PvPWins != 0 {
				t.Fatalf("loser changed: %+v, %v", loser, err)
			}
			t.Cleanup(func() {
				if _, err := f.db.ExecContext(ctx, "DELETE FROM pvp_room_transitions WHERE room_id = ?", room); err != nil {
					t.Error(err)
				}
			})
		})
	}
}

// All but the last participant join the red team.
func preparePrizeMatch(t *testing.T, svc *pvp.Service, actors []string, arena string) string {
	t.Helper()
	ctx := context.Background()
	detail, err := svc.CreateRoom(ctx, actors[0], pvp.CreateRoomRequest{Name: arena, Bet: 100, TargetWins: 1, MaxMembers: len(actors)})
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range actors[1:] {
		if _, err := svc.JoinRoom(ctx, actor, detail.Room.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	for i, actor := range actors {
		color := pvp.ColorRed
		if i == len(actors)-1 {
			color = pvp.ColorBlue
		}
		if _, err := svc.SelectTeam(ctx, actor, detail.Room.ID, color); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.StartMatch(ctx, actors[0], detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	return detail.Room.ID
}

func TestPvPPrizeProductionWiringIntegration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotNone, false)
	ctx := context.Background()
	soc, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(f.db, f.core, soc, f.econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, "UPDATE characters SET hp = 100000, max_hp = 100000, attack = 100000, defense = 100000, agility = 100000 WHERE id = ?", f.sender.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, "UPDATE characters SET hp = 1, max_hp = 1, attack = 0, defense = 0, agility = 0 WHERE id = ?", f.recipient.ID); err != nil {
		t.Fatal(err)
	}
	room := preparePrizeMatch(t, cmbt.pvp, []string{f.sender.ID, f.recipient.ID}, "Wired arena")
	// Existing uncleared flags must not multiply when an arena prize arrives.
	if err := soc.home.AddDeliveryFlag(ctx, f.sender.ID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています"); err != nil {
		t.Fatal(err)
	}
	result, err := cmbt.pvp.AdvanceRound(ctx, f.sender.ID, room)
	if err != nil || !result.MatchCompleted || result.OverallWinnerTeam != pvp.ColorRed {
		t.Fatalf("production settlement: %+v, %v", result, err)
	}
	held, err := f.econ.depot.PendingRemittances(ctx, f.sender.ID)
	if err != nil || len(held) != 1 || held[0].Amount != 200 || held[0].Source != depot.RemittancePvPPrize || held[0].SenderName != "Wired arena" {
		t.Fatalf("production held prize: %+v, %v", held, err)
	}
	notices, err := soc.home.ListDeliveryNotices(ctx, f.sender.ID, true)
	if err != nil || len(notices) != 1 {
		t.Fatalf("production notices: %+v, %v", notices, err)
	}
	// Depot collection uses the existing port and returns the new receipt source.
	_, collected, err := f.econ.depot.WithdrawItem(ctx, f.sender.ID, "")
	if err != nil || len(collected) != 1 || collected[0].Amount != 200 || collected[0].Source != depot.RemittancePvPPrize {
		t.Fatalf("collected prize: %+v, %v", collected, err)
	}
	char, err := f.core.charRepo.FindByID(ctx, f.sender.ID)
	if err != nil || char.Money != 1100 || char.PvPWins != 1 {
		t.Fatalf("collected wallet=%d wins=%d: %v", char.Money, char.PvPWins, err)
	}
}

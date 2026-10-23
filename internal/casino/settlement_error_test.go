package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

type trackingTxProvider struct{ commits, rollbacks int }

func (p *trackingTxProvider) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		p.rollbacks++
		return err
	}
	p.commits++
	return nil
}

type publicationErrorRooms struct {
	casino.RoomRepository
	failure error
}

func (r *publicationErrorRooms) UpdateRoomDetail(ctx context.Context, detail casino.RoomDetail) error {
	if r.failure != nil && !errors.Is(r.failure, casino.ErrSecondarySync) {
		return r.failure
	}
	if err := r.RoomRepository.UpdateRoomDetail(ctx, detail); err != nil {
		return err
	}
	return r.failure
}

// SQL must roll back a rejected payload and commit an applied payload even if secondary sync fails.
func TestActionPublication_TransactionOutcome(t *testing.T) {
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow, casino.GameTypeDoppel} {
		for _, secondary := range []bool{false, true} {
			name := string(game) + "/primary"
			if secondary {
				name = string(game) + "/secondary"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				accounts := newMockPrizeCasinoRepo()
				accounts.accounts["a"] = casino.Account{CharacterID: "a", Coins: 500}
				accounts.accounts["b"] = casino.Account{CharacterID: "b", Coins: 500}
				rooms := &publicationErrorRooms{RoomRepository: casino.NewMemoryRoomRepository()}
				tx := &trackingTxProvider{}
				svc, err := casino.NewService(accounts, casino.WithRoomRepository(rooms), casino.WithTransactionProvider(tx))
				if err != nil {
					t.Fatal(err)
				}
				detail, err := svc.CreateRoom(ctx, "a", casino.CreateRoomRequest{Name: "Publication", GameType: game, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10})
				if err != nil {
					t.Fatal(err)
				}
				roomID := detail.Room.ID
				if _, err := svc.JoinRoom(ctx, roomID, "b", "", 0); err != nil {
					t.Fatal(err)
				}
				switch game {
				case casino.GameTypeIndian:
					_, err = svc.StartIndianPoker(ctx, roomID, "a")
				case casino.GameTypeHighLow:
					_, err = svc.StartHighLow(ctx, roomID, "a")
				case casino.GameTypeDoppel:
					_, err = svc.StartDoppel(ctx, roomID, "a")
				}
				if err != nil {
					t.Fatal(err)
				}
				failure := errors.New("publication failed")
				rooms.failure = failure
				if secondary {
					rooms.failure = &casino.SecondarySyncError{Err: failure}
				}
				commits, rollbacks := tx.commits, tx.rollbacks
				switch game {
				case casino.GameTypeIndian:
					_, err = svc.PlayIndianPokerAction(ctx, roomID, "a", casino.ActionShowdown)
				case casino.GameTypeHighLow:
					_, err = svc.PlayHighLowAction(ctx, roomID, "a", casino.HighLowActionHigh)
				case casino.GameTypeDoppel:
					_, err = svc.PlayDoppelAction(ctx, roomID, "a", 0)
				}
				if !errors.Is(err, failure) {
					t.Fatalf("lost publication error: %v", err)
				}
				if secondary {
					if tx.commits != commits+1 || tx.rollbacks != rollbacks {
						t.Fatal("applied payload rolled SQL back")
					}
				} else if tx.commits != commits || tx.rollbacks != rollbacks+1 {
					t.Fatal("rejected payload committed SQL")
				}
				room, err := rooms.GetRoom(ctx, roomID)
				if err != nil {
					t.Fatal(err)
				}
				member, err := rooms.GetMember(ctx, roomID, "a")
				if err != nil {
					t.Fatal(err)
				}
				if secondary {
					if room.Pot != 10 || member.Action == "" {
						t.Fatalf("applied snapshot missing: %+v %+v", room, member)
					}
				} else if room.Pot != 0 || member.Action != "" {
					t.Fatalf("rejected snapshot leaked: %+v %+v", room, member)
				}
			})
		}
	}
}

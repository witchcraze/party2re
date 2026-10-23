package casino_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

var errPublicationFault = errors.New("injected action publication failure")

// Records actual required store calls; failAt is one-based and zero disables faults.
type publicationFault struct {
	mu     sync.Mutex
	calls  []string
	failAt int
}

func (f *publicationFault) hit(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if len(f.calls) == f.failAt {
		return errPublicationFault
	}
	return nil
}

func (f *publicationFault) resetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *publicationFault) getCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type publicationValkey struct {
	valkeygo.Client
	fault  *publicationFault
	roomID string
}

func (c *publicationValkey) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	args := cmd.Commands()
	if len(args) > 1 && (args[1] == casino.DefaultRoomKeyPrefix+c.roomID || args[0] == "ZADD" || args[0] == "DEL") {
		if err := c.fault.hit("valkey/" + args[0]); err != nil {
			return valkeygo.NewErrorResult(err)
		}
	}
	return c.Client.Do(ctx, cmd)
}

type publicationAccounts struct {
	casino.Repository
	fault *publicationFault
}

func (r publicationAccounts) GetAccount(ctx context.Context, charID string) (casino.Account, error) {
	if err := r.fault.hit("sql/account-read"); err != nil {
		return casino.Account{}, err
	}
	return r.Repository.GetAccount(ctx, charID)
}

func (r publicationAccounts) DeductBetAndCreditPayout(ctx context.Context, charID string, bet, payout int64) (casino.Account, error) {
	if err := r.fault.hit("sql/wager-payout"); err != nil {
		return casino.Account{}, err
	}
	return r.Repository.DeductBetAndCreditPayout(ctx, charID, bet, payout)
}

type publicationCharacters struct {
	casino.CharacterRepository
	fault *publicationFault
}

func (r publicationCharacters) FindByIDForUpdate(ctx context.Context, charID string) (corecharacter.Character, error) {
	if err := r.fault.hit("sql/winner-read"); err != nil {
		return corecharacter.Character{}, err
	}
	return r.CharacterRepository.FindByIDForUpdate(ctx, charID)
}

func (r publicationCharacters) Update(ctx context.Context, ch corecharacter.Character) error {
	if err := r.fault.hit("sql/wins-write"); err != nil {
		return err
	}
	return r.CharacterRepository.Update(ctx, ch)
}

type publicationFixture struct {
	svc        *casino.Service
	rooms      *casino.ValkeyRoomRepository
	accounts   *database.CasinoRepository
	characters *database.CharacterRepository
	fault      *publicationFault
	roomID     string
	actors     [2]string
	game       casino.GameType
}

func newPublicationFixture(t *testing.T, ctx context.Context, db *sql.DB, raw valkeygo.Client, game casino.GameType, coinsB int64) *publicationFixture {
	t.Helper()
	f := &publicationFixture{game: game, fault: &publicationFault{}}
	var err error
	f.accounts, err = database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.characters, err = database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	client := &publicationValkey{Client: raw, fault: f.fault}
	f.rooms, err = casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(f.characters))
	if err != nil {
		t.Fatal(err)
	}
	f.svc, err = casino.NewService(publicationAccounts{Repository: f.accounts, fault: f.fault},
		casino.WithCharacterRepository(publicationCharacters{CharacterRepository: f.characters, fault: f.fault}),
		casino.WithTransactionProvider(database.NewTransactionProvider(db)), casino.WithRoomRepository(f.rooms))
	if err != nil {
		t.Fatal(err)
	}
	for i, coins := range []int64{500, coinsB} {
		ch, err := database.CreateTestCharacterWithFunds(ctx, db, "Wager", int(coins*20))
		if err != nil {
			t.Fatal(err)
		}
		f.actors[i] = ch.ID
		t.Cleanup(func() {
			for _, query := range []string{"DELETE FROM casino_accounts WHERE character_id = ?", "DELETE FROM character_depots WHERE character_id = ?", "DELETE FROM characters WHERE id = ?"} {
				if _, err := db.ExecContext(context.Background(), query, ch.ID); err != nil {
					t.Error(err)
				}
			}
			if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", ch.PlayerID); err != nil {
				t.Error(err)
			}
		})
		if _, _, err := f.svc.ExchangeGoldToCoins(ctx, ch.ID, coins); err != nil {
			t.Fatal(err)
		}
	}
	detail, err := f.svc.CreateRoom(ctx, f.actors[0], casino.CreateRoomRequest{
		Name: "Publication-" + id.New()[:8], GameType: game, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.roomID = detail.Room.ID
	client.roomID = f.roomID
	t.Cleanup(func() {
		f.fault.failAt = 0
		if err := f.rooms.DeleteRoom(context.Background(), f.roomID); err != nil {
			t.Error(err)
		}
		// A failed secondary DEL can leave an already-ejected actor's mapping.
		for _, actor := range f.actors {
			if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultCharacterKeyPrefix+actor).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err := f.svc.JoinRoom(ctx, f.roomID, f.actors[1], "", 0); err != nil {
		t.Fatal(err)
	}
	switch game {
	case casino.GameTypeIndian:
		_, err = f.svc.StartIndianPoker(ctx, f.roomID, f.actors[0])
	case casino.GameTypeHighLow:
		_, err = f.svc.StartHighLow(ctx, f.roomID, f.actors[0])
	case casino.GameTypeDoppel:
		_, err = f.svc.StartDoppel(ctx, f.roomID, f.actors[0])
	}
	if err != nil {
		t.Fatal(err)
	}
	if game != casino.GameTypeDoppel {
		for i, card := range []int{12, 0} {
			member, err := f.rooms.GetMember(ctx, f.roomID, f.actors[i])
			if err != nil {
				t.Fatal(err)
			}
			member.Card = card
			if err := f.rooms.UpdateMember(ctx, *member); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f
}

func (f *publicationFixture) play(ctx context.Context, actor int, call bool) error {
	var err error
	switch f.game {
	case casino.GameTypeIndian:
		action := casino.ActionShowdown
		if call {
			action = casino.ActionCall
		}
		_, err = f.svc.PlayIndianPokerAction(ctx, f.roomID, f.actors[actor], action)
	case casino.GameTypeHighLow:
		action := casino.HighLowActionHigh
		if call {
			action = casino.HighLowActionCall
		}
		_, err = f.svc.PlayHighLowAction(ctx, f.roomID, f.actors[actor], action)
	case casino.GameTypeDoppel:
		_, err = f.svc.PlayDoppelAction(ctx, f.roomID, f.actors[actor], actor)
	}
	return err
}

type publicationState struct {
	room    casino.Room
	members []casino.RoomMember
	coins   [2]int64
	wins    [2]int
}

func (f *publicationFixture) state(t *testing.T, ctx context.Context) publicationState {
	t.Helper()
	room, err := f.rooms.GetRoom(ctx, f.roomID)
	if err != nil {
		t.Fatal(err)
	}
	members, err := f.rooms.ListMembers(ctx, f.roomID)
	if err != nil {
		t.Fatal(err)
	}
	s := publicationState{room: *room, members: members}
	for i, actor := range f.actors {
		acc, err := f.accounts.GetAccount(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := f.characters.FindByID(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		s.coins[i], s.wins[i] = acc.Coins, ch.CasinoWins
	}
	return s
}

func (f *publicationFixture) assertUnchanged(t *testing.T, ctx context.Context, before publicationState) {
	t.Helper()
	if after := f.state(t, ctx); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected action changed persisted state:\nbefore=%+v\nafter=%+v", before, after)
	}
}

// Restore only this fixture's assets and room between sequential fault cases.
func (f *publicationFixture) restore(t *testing.T, ctx context.Context, before publicationState) {
	t.Helper()
	f.fault.failAt = 0
	for i, actor := range f.actors {
		acc, err := f.accounts.GetAccount(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.accounts.AdjustCoins(ctx, actor, before.coins[i]-acc.Coins); err != nil {
			t.Fatal(err)
		}
		ch, err := f.characters.FindByID(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		ch.CasinoWins = before.wins[i]
		if err := f.characters.Update(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	for _, member := range before.members {
		if err := f.rooms.AddMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: before.room, Members: before.members}); err != nil {
		t.Fatal(err)
	}
}

func (f *publicationFixture) assertPendingOpponent(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := f.play(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	s := f.state(t, ctx)
	if s.coins != [2]int64{500, 490} || s.wins != [2]int{} || s.room.Pot != 10 || s.room.Round != 1 || s.room.WinnerCharacterID != nil {
		t.Fatalf("opponent settled against an unpaid action: %+v", s)
	}
	for _, m := range s.members {
		if m.CharacterID == f.actors[0] && (m.Action != "" || (f.game == casino.GameTypeDoppel && m.Card != -1)) {
			t.Fatalf("unpaid actor accepted: %+v", m)
		}
	}
}

func publicationFailureName(index int, call string) string {
	return fmt.Sprintf("%02d-%s", index+1, call)
}

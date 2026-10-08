package casino_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
)

// hlFaultValkeyClient intercepts Valkey commands and selectively injects
// pre-payload (SET) or post-payload (ZADD) failures on final showdown reset.
type hlFaultValkeyClient struct {
	valkeygo.Client
	armed               atomic.Bool
	failNextWaitingSet  atomic.Bool
	failNextWaitingZadd atomic.Bool
	zaddFailed          atomic.Bool
	setFailed           atomic.Bool
}

func (c *hlFaultValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if c.armed.Load() {
		cmds := cmd.Commands()
		if len(cmds) > 0 {
			if cmds[0] == "SET" && len(cmds) >= 3 {
				val := cmds[2]
				if strings.Contains(val, `"status":"waiting"`) && strings.Contains(val, `"round":0`) {
					if c.failNextWaitingSet.Load() {
						c.failNextWaitingSet.Store(false)
						c.setFailed.Store(true)
						return valkeygo.NewErrorResult(errors.New("injected HighLow room payload SET failure"))
					}
					c.failNextWaitingZadd.Store(true)
				}
			} else if cmds[0] == "ZADD" && c.failNextWaitingZadd.Load() {
				c.failNextWaitingZadd.Store(false)
				c.zaddFailed.Store(true)
				return valkeygo.NewErrorResult(errors.New("injected HighLow active-index ZADD failure"))
			}
		}
	}
	return c.Client.Do(ctx, cmd)
}

func (c *hlFaultValkeyClient) DoMulti(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
	results := make([]valkeygo.ValkeyResult, len(multi))
	for i, cmd := range multi {
		results[i] = c.Do(ctx, cmd)
	}
	return results
}

func TestSettlementBoundary_HighLow_PostPayloadSecondarySyncFailure(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("Integration environment not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rawValkey := openIsolatedTestValkey(t, 13)
	defer rawValkey.Close()

	ctx := context.Background()
	_ = rawValkey.Do(ctx, rawValkey.B().Flushdb().Build())

	faultClient := &hlFaultValkeyClient{Client: rawValkey}
	roomRepo, err := casino.NewValkeyRoomRepository(faultClient)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithCharacterRepository(charRepo),
		casino.WithRoomRepository(roomRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "HL_A_Post", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "HL_B_Post", 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM casino_accounts WHERE character_id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID)
	}()

	_, _, _ = svc.ExchangeGoldToCoins(ctx, charA.ID, 500)
	_, _, _ = svc.ExchangeGoldToCoins(ctx, charB.ID, 500)

	roomName := fmt.Sprintf("HLPost-%d", time.Now().UnixNano()%1000000000)
	createdRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeHighLow,
		Name:       roomName,
		Speed:      12,
		Rate:       10,
		MaxPlayers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := createdRoom.Room.ID
	defer func() { _ = roomRepo.DeleteRoom(ctx, roomID) }()

	_, _ = svc.JoinRoom(ctx, roomID, charB.ID, "", 0)
	_, _ = svc.StartHighLow(ctx, roomID, charA.ID)

	mA, _ := roomRepo.GetMember(ctx, roomID, charA.ID)
	mA.Card = 12 // K (high card, winner)
	_ = roomRepo.UpdateMember(ctx, *mA)
	mB, _ := roomRepo.GetMember(ctx, roomID, charB.ID)
	mB.Card = 0 // A (low card, loser)
	_ = roomRepo.UpdateMember(ctx, *mB)

	// A plays High: bet 10 deducted, balances 490/500, pot=10
	_, err = svc.PlayHighLowAction(ctx, roomID, charA.ID, casino.HighLowActionHigh)
	if err != nil {
		t.Fatalf("charA High action failed: %v", err)
	}

	// Arm fault client to fail ZADD after final SET on B's showdown
	faultClient.armed.Store(true)

	_, bErr := svc.PlayHighLowAction(ctx, roomID, charB.ID, casino.HighLowActionHigh)
	if bErr == nil {
		t.Fatal("expected error on secondary ZADD failure, got nil")
	}
	if !errors.Is(bErr, casino.ErrSecondarySync) {
		t.Fatalf("expected ErrSecondarySync, got: %v", bErr)
	}
	if !faultClient.zaddFailed.Load() {
		t.Fatal("expected fault client to have intercepted ZADD")
	}

	// Verify durable coins are conserved:
	// A won showdown and received pot (10 + 10 = 20 coins payout) -> 510 coins
	// B lost bet of 10 coins -> 490 coins
	accA, _ := casinoRepo.GetAccount(ctx, charA.ID)
	accB, _ := casinoRepo.GetAccount(ctx, charB.ID)
	if accA.Coins != 510 || accB.Coins != 490 {
		t.Fatalf("expected accounts 510/490, got %d/%d", accA.Coins, accB.Coins)
	}
	if accA.Coins+accB.Coins != 1000 {
		t.Fatalf("coins not conserved: total=%d", accA.Coins+accB.Coins)
	}

	// Valkey room is reset: waiting, round=0, pot=0
	snapRoom, _ := roomRepo.GetRoom(ctx, roomID)
	if snapRoom.Status != casino.RoomStatusWaiting || snapRoom.Round != 0 || snapRoom.Pot != 0 {
		t.Fatalf("unexpected room: status=%s round=%d pot=%d", snapRoom.Status, snapRoom.Round, snapRoom.Pot)
	}

	// Winner's CasinoWins is 1
	cA, _ := charRepo.FindByID(ctx, charA.ID)
	if cA.CasinoWins != 1 {
		t.Fatalf("expected 1 casino win, got %d", cA.CasinoWins)
	}

	// Blind replay returns ErrGameNotInRound
	_, replayErr := svc.PlayHighLowAction(ctx, roomID, charB.ID, casino.HighLowActionHigh)
	if !errors.Is(replayErr, casino.ErrGameNotInRound) {
		t.Fatalf("expected ErrGameNotInRound on replay, got %v", replayErr)
	}
}

func TestSettlementBoundary_HighLow_PrePayloadFailureRollback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("Integration environment not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rawValkey := openIsolatedTestValkey(t, 13)
	defer rawValkey.Close()

	ctx := context.Background()
	_ = rawValkey.Do(ctx, rawValkey.B().Flushdb().Build())

	faultClient := &hlFaultValkeyClient{Client: rawValkey}
	roomRepo, err := casino.NewValkeyRoomRepository(faultClient)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithCharacterRepository(charRepo),
		casino.WithRoomRepository(roomRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "HL_A_Pre", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "HL_B_Pre", 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM casino_accounts WHERE character_id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID)
	}()

	_, _, _ = svc.ExchangeGoldToCoins(ctx, charA.ID, 500)
	_, _, _ = svc.ExchangeGoldToCoins(ctx, charB.ID, 500)

	roomName := fmt.Sprintf("HLPre-%d", time.Now().UnixNano()%1000000000)
	createdRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeHighLow,
		Name:       roomName,
		Speed:      12,
		Rate:       10,
		MaxPlayers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := createdRoom.Room.ID
	defer func() { _ = roomRepo.DeleteRoom(ctx, roomID) }()

	_, _ = svc.JoinRoom(ctx, roomID, charB.ID, "", 0)
	_, _ = svc.StartHighLow(ctx, roomID, charA.ID)

	mA, _ := roomRepo.GetMember(ctx, roomID, charA.ID)
	mA.Card = 12
	_ = roomRepo.UpdateMember(ctx, *mA)
	mB, _ := roomRepo.GetMember(ctx, roomID, charB.ID)
	mB.Card = 0
	_ = roomRepo.UpdateMember(ctx, *mB)

	// A plays High: bet 10 deducted, pot=10, A=490, B=500
	_, _ = svc.PlayHighLowAction(ctx, roomID, charA.ID, casino.HighLowActionHigh)

	// Arm fault client to fail the SET payload write when B triggers showdown
	faultClient.failNextWaitingSet.Store(true)
	faultClient.armed.Store(true)

	_, bErr := svc.PlayHighLowAction(ctx, roomID, charB.ID, casino.HighLowActionHigh)
	if bErr == nil {
		t.Fatal("expected error on pre-payload SET failure, got nil")
	}
	if !faultClient.setFailed.Load() {
		t.Fatal("expected SET to fail")
	}

	// Verify rollback in MariaDB: B not charged, A not credited
	accA, _ := casinoRepo.GetAccount(ctx, charA.ID)
	accB, _ := casinoRepo.GetAccount(ctx, charB.ID)
	if accA.Coins != 490 || accB.Coins != 500 {
		t.Fatalf("expected rollback accounts 490/500, got %d/%d", accA.Coins, accB.Coins)
	}

	snapRoom, _ := roomRepo.GetRoom(ctx, roomID)
	if snapRoom.Round != 1 || snapRoom.Pot != 10 {
		t.Fatalf("expected room round=1 pot=10, got round=%d pot=%d", snapRoom.Round, snapRoom.Pot)
	}

	if accA.Coins+accB.Coins+snapRoom.Pot != 1000 {
		t.Fatalf("coins not conserved")
	}

	cA, _ := charRepo.FindByID(ctx, charA.ID)
	if cA.CasinoWins != 0 {
		t.Fatalf("expected 0 casino wins on rollback, got %d", cA.CasinoWins)
	}
}

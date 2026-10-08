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

// showdownFaultValkeyClient intercepts Valkey commands and selectively injects an error into
// the active-index ZADD immediately following the final SET that transitions the room to waiting/round=0.
type showdownFaultValkeyClient struct {
	valkeygo.Client
	armed               atomic.Bool
	failNextWaitingSet  atomic.Bool
	failNextWaitingZadd atomic.Bool
	zaddFailed          atomic.Bool
	setFailed           atomic.Bool
}

func (c *showdownFaultValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if c.armed.Load() {
		cmds := cmd.Commands()
		if len(cmds) > 0 {
			if cmds[0] == "SET" && len(cmds) >= 3 {
				val := cmds[2]
				if strings.Contains(val, `"status":"waiting"`) && strings.Contains(val, `"round":0`) {
					if c.failNextWaitingSet.Load() {
						c.failNextWaitingSet.Store(false)
						c.setFailed.Store(true)
						return valkeygo.NewErrorResult(errors.New("injected room payload SET failure"))
					}
					c.failNextWaitingZadd.Store(true)
				}
			} else if cmds[0] == "ZADD" && c.failNextWaitingZadd.Load() {
				c.failNextWaitingZadd.Store(false)
				c.zaddFailed.Store(true)
				return valkeygo.NewErrorResult(errors.New("injected active-index ZADD failure on final showdown reset"))
			}
		}
	}
	return c.Client.Do(ctx, cmd)
}

func (c *showdownFaultValkeyClient) DoMulti(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
	results := make([]valkeygo.ValkeyResult, len(multi))
	for i, cmd := range multi {
		results[i] = c.Do(ctx, cmd)
	}
	return results
}

// TestSettlementBoundary_ReproductionIssue1080 reproduces the exact scenario in Issue #1080:
//  1. A and B exchange 500 coins each. Room rate=10.
//  2. A.Card=12 (winner), B.Card=0 (loser). A plays ActionShowdown: pot=10, balances 490/500.
//  3. B plays ActionShowdown. Final UpdateRoom payload SET (waiting, round=0, pot=0) succeeds.
//     Subsequent ZADD fails.
//  4. In buggy code: SQL rolls back, leaving accounts at 490/500, Valkey pot=0. 10 coins lost.
func TestSettlementBoundary_ReproductionIssue1080(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
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

	faultClient := &showdownFaultValkeyClient{Client: rawValkey}
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

	// 1. Create A and B with 10000 gold; exchange to 500 coins each.
	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerA_Repro", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerB_Repro", 10000)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		// Step 5: Cleanup only test-owned characters/players
		_, _ = db.ExecContext(ctx, "DELETE FROM casino_accounts WHERE character_id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id IN (?, ?)", charA.ID, charB.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID)
	}()

	if _, _, err := svc.ExchangeGoldToCoins(ctx, charA.ID, 500); err != nil {
		t.Fatalf("ExchangeGoldToCoins charA failed: %v", err)
	}
	if _, _, err := svc.ExchangeGoldToCoins(ctx, charB.ID, 500); err != nil {
		t.Fatalf("ExchangeGoldToCoins charB failed: %v", err)
	}

	roomName := fmt.Sprintf("Repro1080-%d", time.Now().UnixNano()%1000000000)
	createdRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeIndian,
		Name:       roomName,
		Speed:      12,
		Rate:       10,
		MaxPlayers: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := createdRoom.Room.ID
	defer func() {
		_ = roomRepo.DeleteRoom(ctx, roomID)
	}()

	if _, err := svc.JoinRoom(ctx, roomID, charB.ID, "", 0); err != nil {
		t.Fatalf("JoinRoom charB failed: %v", err)
	}

	if _, err := svc.StartIndianPoker(ctx, roomID, charA.ID); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	// 2. Set A.Card=12 (K, high card, winner) and B.Card=0 (A, low card, loser).
	mA, err := roomRepo.GetMember(ctx, roomID, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	mA.Card = 12
	if err := roomRepo.UpdateMember(ctx, *mA); err != nil {
		t.Fatal(err)
	}

	mB, err := roomRepo.GetMember(ctx, roomID, charB.ID)
	if err != nil {
		t.Fatal(err)
	}
	mB.Card = 0
	if err := roomRepo.UpdateMember(ctx, *mB); err != nil {
		t.Fatal(err)
	}

	// A plays ActionShowdown successfully.
	if _, err := svc.PlayIndianPokerAction(ctx, roomID, charA.ID, casino.ActionShowdown); err != nil {
		t.Fatalf("charA showdown failed: %v", err)
	}

	// Assert healthy prerequisite/contrast: durable balances 490/500 and live pot=10
	accA, err := casinoRepo.GetAccount(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	accB, err := casinoRepo.GetAccount(ctx, charB.ID)
	if err != nil {
		t.Fatal(err)
	}
	liveRoom, err := roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}

	if accA.Coins != 490 || accB.Coins != 500 {
		t.Fatalf("expected prerequisite accounts 490/500, got %d/%d", accA.Coins, accB.Coins)
	}
	if liveRoom.Pot != 10 {
		t.Fatalf("expected prerequisite pot=10, got %d", liveRoom.Pot)
	}

	// 3. B plays ActionShowdown.
	// Allow SQL and payload SET. Inject failure on ZADD immediately after final SET (status=waiting, round=0).
	faultClient.armed.Store(true)
	_, bErr := svc.PlayIndianPokerAction(ctx, roomID, charB.ID, casino.ActionShowdown)

	// In both buggy and fixed versions, B receives an error (failure returned to caller).
	if bErr == nil {
		t.Fatal("expected error on B's showdown when ZADD fails, got nil")
	}
	if !faultClient.zaddFailed.Load() {
		t.Fatal("expected fault client to have intercepted and failed the target ZADD")
	}

	// 4. Read both accounts and room from real repositories.
	finalAccA, err := casinoRepo.GetAccount(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalAccB, err := casinoRepo.GetAccount(ctx, charB.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalRoom, err := roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Post-failure state: A.Coins=%d, B.Coins=%d, Room.Status=%s, Room.Round=%d, Room.Pot=%d, zaddFailed=%v, error=%v",
		finalAccA.Coins, finalAccB.Coins, finalRoom.Status, finalRoom.Round, finalRoom.Pot, faultClient.zaddFailed.Load(), bErr)

	// If the bug is present, total coins = 990 (A=490, B=500, pot=0).
	// If the bug is fixed, total coins = 1000 (A=500, B=500, pot=0).
	totalCoins := finalAccA.Coins + finalAccB.Coins + finalRoom.Pot
	if totalCoins != 1000 {
		t.Errorf("BUG REPRODUCED: Coins not conserved! Expected 1000 total coins, got %d (A=%d, B=%d, pot=%d)",
			totalCoins, finalAccA.Coins, finalAccB.Coins, finalRoom.Pot)
	}

	// 5. Verify healthy replay attempt is rejected with ErrGameNotInRound (no blind replay allowed).
	_, replayErr := svc.PlayIndianPokerAction(ctx, roomID, charB.ID, casino.ActionShowdown)
	if !errors.Is(replayErr, casino.ErrGameNotInRound) {
		t.Fatalf("expected ErrGameNotInRound on blind replay, got %v", replayErr)
	}

	// Verify winner's CasinoWins was durably incremented
	charAUpdated, err := charRepo.FindByID(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if charAUpdated.CasinoWins != 1 {
		t.Fatalf("expected charA CasinoWins=1, got %d", charAUpdated.CasinoWins)
	}
}

// TestSettlementBoundary_IndianPoker_PrePayloadFailureRollback tests that when the authoritative
// room payload SET fails (pre-payload failure), the SQL transaction rolls back completely:
// balances are untouched, win counters are not incremented, and room state in Valkey is preserved.
func TestSettlementBoundary_IndianPoker_PrePayloadFailureRollback(t *testing.T) {
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

	faultClient := &showdownFaultValkeyClient{Client: rawValkey}
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

	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerA_PreFail", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerB_PreFail", 10000)
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

	roomName := fmt.Sprintf("PreFail-%d", time.Now().UnixNano()%1000000000)
	createdRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeIndian,
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
	_, _ = svc.StartIndianPoker(ctx, roomID, charA.ID)

	mA, _ := roomRepo.GetMember(ctx, roomID, charA.ID)
	mA.Card = 12
	_ = roomRepo.UpdateMember(ctx, *mA)
	mB, _ := roomRepo.GetMember(ctx, roomID, charB.ID)
	mB.Card = 0
	_ = roomRepo.UpdateMember(ctx, *mB)

	// A plays Showdown: pot=10, A=490, B=500
	_, _ = svc.PlayIndianPokerAction(ctx, roomID, charA.ID, casino.ActionShowdown)

	// Arm fault client to fail the SET payload write when B triggers showdown
	faultClient.failNextWaitingSet.Store(true)
	faultClient.armed.Store(true)

	_, bErr := svc.PlayIndianPokerAction(ctx, roomID, charB.ID, casino.ActionShowdown)
	if bErr == nil {
		t.Fatal("expected error on pre-payload SET failure, got nil")
	}
	if !faultClient.setFailed.Load() {
		t.Fatal("expected SET to fail")
	}

	// Verify rollback:
	// In MariaDB, B was NOT deducted 10 coins, and A was NOT paid pot (transaction rolled back)
	accA, _ := casinoRepo.GetAccount(ctx, charA.ID)
	accB, _ := casinoRepo.GetAccount(ctx, charB.ID)
	if accA.Coins != 490 || accB.Coins != 500 {
		t.Fatalf("expected rollback accounts 490/500, got %d/%d", accA.Coins, accB.Coins)
	}

	// In Valkey, room is still in round 1 with pot=10
	snapRoom, _ := roomRepo.GetRoom(ctx, roomID)
	if snapRoom.Round != 1 || snapRoom.Pot != 10 {
		t.Fatalf("expected room round=1 pot=10, got round=%d pot=%d", snapRoom.Round, snapRoom.Pot)
	}

	// Total coins conserved
	if accA.Coins+accB.Coins+snapRoom.Pot != 1000 {
		t.Fatalf("expected total 1000 coins conserved on pre-payload rollback")
	}

	// Win counter untouched
	cA, _ := charRepo.FindByID(ctx, charA.ID)
	if cA.CasinoWins != 0 {
		t.Fatalf("expected 0 casino wins, got %d", cA.CasinoWins)
	}
}

// TestSettlementBoundary_IndianPoker_HealthySettlementContrast tests normal settlement without faults.
func TestSettlementBoundary_IndianPoker_HealthySettlementContrast(t *testing.T) {
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

	roomRepo, err := casino.NewValkeyRoomRepository(rawValkey)
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

	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerA_Healthy", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerB_Healthy", 10000)
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

	roomName := fmt.Sprintf("Healthy-%d", time.Now().UnixNano()%1000000000)
	createdRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeIndian,
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
	_, _ = svc.StartIndianPoker(ctx, roomID, charA.ID)

	mA, _ := roomRepo.GetMember(ctx, roomID, charA.ID)
	mA.Card = 12
	_ = roomRepo.UpdateMember(ctx, *mA)
	mB, _ := roomRepo.GetMember(ctx, roomID, charB.ID)
	mB.Card = 0
	_ = roomRepo.UpdateMember(ctx, *mB)

	// A plays Showdown: pot=10, A=490, B=500
	_, err = svc.PlayIndianPokerAction(ctx, roomID, charA.ID, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("charA showdown failed: %v", err)
	}

	// B plays Showdown: healthy resolution, winner A gets pot of 20
	res, err := svc.PlayIndianPokerAction(ctx, roomID, charB.ID, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("charB showdown failed: %v", err)
	}

	if res.Room.Status != casino.RoomStatusWaiting || res.Room.Round != 0 || res.Room.Pot != 0 {
		t.Fatalf("unexpected room state: status=%s round=%d pot=%d", res.Room.Status, res.Room.Round, res.Room.Pot)
	}

	accA, _ := casinoRepo.GetAccount(ctx, charA.ID)
	accB, _ := casinoRepo.GetAccount(ctx, charB.ID)
	if accA.Coins != 510 || accB.Coins != 490 {
		t.Fatalf("expected final accounts 510/490, got %d/%d", accA.Coins, accB.Coins)
	}
	if accA.Coins+accB.Coins != 1000 {
		t.Fatalf("coins not conserved")
	}

	cA, _ := charRepo.FindByID(ctx, charA.ID)
	if cA.CasinoWins != 1 {
		t.Fatalf("expected 1 casino win, got %d", cA.CasinoWins)
	}
}

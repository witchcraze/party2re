package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
)

var errCasinoResponseReadInjected = errors.New("injected casino response snapshot read failure")

// casinoResponseReadFaultClient fails the first room GET that occurs after a room
// settlement payload SET (status=waiting, round=0) and active-index ZADD succeed.
type casinoResponseReadFaultClient struct {
	valkeygo.Client
	roomID            string
	armed             atomic.Bool
	failAfterSettled  atomic.Bool
	failedResponseGet atomic.Bool
}

func (c *casinoResponseReadFaultClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if c.armed.Load() {
		args := cmd.Commands()
		if len(args) >= 3 && args[0] == "SET" && args[1] == casino.DefaultRoomKeyPrefix+c.roomID {
			if strings.Contains(args[2], `"status":"waiting"`) && strings.Contains(args[2], `"round":0`) {
				c.failAfterSettled.Store(true)
			}
		} else if len(args) >= 2 && args[0] == "GET" && args[1] == casino.DefaultRoomKeyPrefix+c.roomID && c.failAfterSettled.Load() {
			c.failAfterSettled.Store(false)
			c.failedResponseGet.Store(true)
			return valkeygo.NewErrorResult(errCasinoResponseReadInjected)
		}
	}
	return c.Client.Do(ctx, cmd)
}

func TestWireCasinoMedalProgressPreservedOnResponseReadFailure(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("Integration environment not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	rawValkey, err := valkeygo.NewClient(valkeygo.ClientOption{
		InitAddress: []string{valkeyAddr},
		SelectDB:    13,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rawValkey.Close)

	ctx := context.Background()
	faultClient := &casinoResponseReadFaultClient{Client: rawValkey}

	core, err := newCoreServices(db, faultClient)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, faultClient, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, faultClient)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, faultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}

	achievementRepo, err := database.NewAchievementRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "CasWireA_"+id.New()[:6], 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "CasWireB_"+id.New()[:6], 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, cID := range []string{charA.ID, charB.ID} {
			for _, query := range []string{
				"DELETE FROM character_achievements WHERE character_id = ?",
				"DELETE FROM casino_accounts WHERE character_id = ?",
				"DELETE FROM character_depots WHERE character_id = ?",
				"DELETE FROM characters WHERE id = ?",
			} {
				if _, err := db.ExecContext(context.Background(), query, cID); err != nil {
					t.Error(err)
				}
			}
		}
		if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID); err != nil {
			t.Error(err)
		}
	})

	if _, _, err := misc.casino.ExchangeGoldToCoins(ctx, charA.ID, 500); err != nil {
		t.Fatal(err)
	}
	if _, _, err := misc.casino.ExchangeGoldToCoins(ctx, charB.ID, 500); err != nil {
		t.Fatal(err)
	}

	roomDetail, err := misc.casino.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		Name:       "WireCas-" + id.New()[:8],
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := roomDetail.Room.ID
	faultClient.roomID = roomID
	t.Cleanup(func() {
		faultClient.armed.Store(false)
		for _, key := range []string{
			casino.DefaultRoomKeyPrefix + roomID,
			casino.DefaultCharacterKeyPrefix + charA.ID,
			casino.DefaultCharacterKeyPrefix + charB.ID,
		} {
			if err := rawValkey.Do(context.Background(), rawValkey.B().Del().Key(key).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
		if err := rawValkey.Do(context.Background(), rawValkey.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build()).Error(); err != nil {
			t.Error(err)
		}
	})

	if _, err := misc.casino.JoinRoom(ctx, roomID, charB.ID, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := misc.casino.StartDoppel(ctx, roomID, charA.ID); err != nil {
		t.Fatal(err)
	}

	// Leader charA chooses mark 0; child charB chooses mark 1 (mismatch -> leader charA wins).
	if _, err := misc.casino.PlayDoppelAction(ctx, roomID, charA.ID, 0); err != nil {
		t.Fatal(err)
	}

	faultClient.armed.Store(true)
	_, err = misc.casino.PlayDoppelAction(ctx, roomID, charB.ID, 1)
	faultClient.armed.Store(false)

	if !errors.Is(err, errCasinoResponseReadInjected) {
		t.Fatalf("expected injected response-read error, got %v", err)
	}
	if !faultClient.failedResponseGet.Load() {
		t.Fatal("expected fault client to fail the post-settlement response GET")
	}

	accA, err := casinoRepo.GetAccount(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	accB, err := casinoRepo.GetAccount(ctx, charB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accA.Coins != 510 || accB.Coins != 490 {
		t.Fatalf("coins = %d/%d, want 510/490", accA.Coins, accB.Coins)
	}

	updatedA, err := core.charRepo.FindByID(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedA.CasinoWins != 1 {
		t.Fatalf("winner CasinoWins = %d, want 1", updatedA.CasinoWins)
	}

	recordsA, err := achievementRepo.GetCharacterAchievements(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recordsA) == 0 {
		t.Fatal("expected casino achievement progress records for winner charA, got none")
	}
	for _, rec := range recordsA {
		if rec.CurrentProgress != 1 {
			t.Fatalf("winner achievement %s progress = %d, want 1", rec.AchievementID, rec.CurrentProgress)
		}
	}
	recordsB, err := achievementRepo.GetCharacterAchievements(ctx, charB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recordsB) != 0 {
		t.Fatalf("loser achievement records = %+v, want none", recordsB)
	}

	// Subsequent observation succeeds and replay is rejected without repeating payouts or progression.
	view, err := misc.casino.GetRoomView(ctx, roomID, charA.PlayerID, charA.ID)
	if err != nil {
		t.Fatalf("GetRoomView after read failure: %v", err)
	}
	if view.Room.Status != casino.RoomStatusWaiting || view.Room.Round != 0 {
		t.Fatalf("room view after settlement = %+v, want waiting/round=0", view.Room)
	}
	if _, err := misc.casino.PlayDoppelAction(ctx, roomID, charB.ID, 1); !errors.Is(err, casino.ErrGameNotInRound) {
		t.Fatalf("replay error = %v, want ErrGameNotInRound", err)
	}
	recordsAfterReplay, err := achievementRepo.GetCharacterAchievements(ctx, charA.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recordsAfterReplay {
		if rec.CurrentProgress != 1 {
			t.Fatalf("winner achievement %s progress after rejected replay = %d, want 1", rec.AchievementID, rec.CurrentProgress)
		}
	}
}

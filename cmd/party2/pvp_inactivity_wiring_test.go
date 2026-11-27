package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPvPInactivityProductionWiring(t *testing.T) {
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is required")
	}
	client, err := valkeygo.NewClient(valkeygo.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	db, char := newHomePetCharacter(t)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	econ.initStore(core, soc.guildRepo, soc.timer, client, storeDelivery{econ.depotRepo, soc.home, core.itemCatalog})
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}
	f := hookWiringFixture{db, char, core, econ, soc, misc, cmbt}
	ctx := context.Background()

	// Ensure sufficient gold for bet and damaged vitality to observe Wake recovery.
	if _, err := f.db.ExecContext(ctx, `UPDATE characters SET money = 500, hp = 5, tired = 40 WHERE id = ?`, f.char.ID); err != nil {
		t.Fatal(err)
	}

	room, err := f.cmbt.pvp.CreateRoom(ctx, f.char.ID, pvp.CreateRoomRequest{Name: "ProdIdle", Bet: 100})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if err := f.soc.timer.ReleaseLock(cleanupCtx, timer.CategorySleep, f.char.ID); err != nil {
			t.Errorf("cleanup ReleaseLock sleep %s: %v", f.char.ID, err)
		}
		for _, key := range []string{pvp.DefaultRoomKeyPrefix + room.Room.ID, pvp.DefaultCharacterKeyPrefix + f.char.ID} {
			if err := client.Do(cleanupCtx, client.B().Del().Key(key).Build()).Error(); err != nil {
				t.Errorf("cleanup DEL %s: %v", key, err)
			}
		}
		if _, err := f.db.ExecContext(cleanupCtx, `DELETE FROM pvp_room_transitions WHERE room_id = ?`, room.Room.ID); err != nil {
			t.Errorf("cleanup DELETE pvp_room_transitions %s: %v", room.Room.ID, err)
		}
	})

	// Backdate SQL last_activity_at beyond 1800s and remove the expired Valkey room body.
	staleAt := time.Now().UTC().Add(-35 * time.Minute)
	if _, err := f.db.ExecContext(ctx, `UPDATE pvp_room_transitions SET last_activity_at = ? WHERE room_id = ?`, staleAt, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Del().Key(pvp.DefaultRoomKeyPrefix+room.Room.ID).Build()).Error(); err != nil {
		t.Fatal(err)
	}

	pc := newPlayerContext(f.core, f.soc, f.econ, f.cmbt, f.misc)
	before, err := pc.Query(ctx, f.char.ID, f.char.PlayerID)
	if err != nil {
		t.Fatalf("Query before leave: %v", err)
	}
	if before.Snapshot.Sleeping || len(before.Snapshot.Activities) != 1 || before.Snapshot.Activities[0].Phase != "recovery_pending" {
		t.Fatalf("expected recovery_pending without sleep on GET: %+v", before.Snapshot)
	}

	// LeaveRoom triggers genuine inactivity cleanup with production timer + Home penalty reader.
	if _, err := f.cmbt.pvp.LeaveRoom(ctx, f.char.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom timeout: %v", err)
	}

	rem, pending, err := f.soc.home.GetRemainingSleep(ctx, f.char.ID)
	if err != nil || pending || rem < 3500*time.Second || rem > 3605*time.Second {
		t.Fatalf("sleep remaining=(%s, %v, %v); want ~3600s", rem, pending, err)
	}
	if _, err := f.soc.home.Wake(ctx, f.char.ID); !errors.Is(err, home.ErrStillSleeping) {
		t.Fatalf("expected ErrStillSleeping, got %v", err)
	}

	after, err := pc.Query(ctx, f.char.ID, f.char.PlayerID)
	if err != nil {
		t.Fatalf("Query after leave: %v", err)
	}
	if !after.Snapshot.Sleeping || after.Snapshot.CanWake || len(after.Snapshot.Activities) != 0 {
		t.Fatalf("unexpected post-timeout context: %+v", after.Snapshot)
	}

	// Rescue sees no PvP activity remaining.
	rec, err := f.misc.rescue.EmergencyRescue(ctx, f.char.ID, "check", time.Now().UTC())
	if err != nil || rec.PenaltySeconds != 0 || rec.ID != "" {
		t.Fatalf("expected idle rescue after PvP timeout, got %+v err=%v", rec, err)
	}

	// Simulate an unacknowledged pending timeout transition: Home.Wake and playercontext must block wake.
	pastDeadline := time.Now().UTC().Add(-time.Minute)
	pendingTransition, err := json.Marshal(pvp.Transition{
		Version:      2,
		Kind:         "timeout",
		ActorID:      f.char.ID,
		Detail:       pvp.RoomDetail{Room: pvp.ColosseumRoom{ID: room.Room.ID, Status: pvp.StatusDisbanded}},
		RemovedIDs:   []string{f.char.ID},
		PenaltyUntil: pastDeadline,
		Pending:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE pvp_room_transitions SET version = 2, pending = TRUE, transition_json = ? WHERE room_id = ?`, pendingTransition, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `INSERT IGNORE INTO pvp_character_claims (character_id, room_id) VALUES (?, ?)`, f.char.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.soc.timer.ReleaseLock(ctx, timer.CategorySleep, f.char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.soc.home.Wake(ctx, f.char.ID); !errors.Is(err, home.ErrWakeInProgress) {
		t.Fatalf("expected ErrWakeInProgress while PvP timeout pending, got %v", err)
	}
	blocked, err := pc.Query(ctx, f.char.ID, f.char.PlayerID)
	if err != nil {
		t.Fatalf("Query with pending penalty: %v", err)
	}
	if blocked.Snapshot.CanWake {
		t.Fatal("CanWake must be false while PvP timeout publication is pending")
	}

	// Complete pending recovery, then Home.Wake succeeds and restores vitality.
	if _, err := f.cmbt.pvp.LeaveRoom(ctx, f.char.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom finish pending: %v", err)
	}
	woke, err := f.soc.home.Wake(ctx, f.char.ID)
	if err != nil || !woke.Recovered || woke.Character.PendingWake || woke.Character.Stats.HP != woke.Character.Stats.MaxHP || woke.Character.Tired != 0 {
		t.Fatalf("Wake after completed PvP timeout: %+v err=%v", woke, err)
	}
}

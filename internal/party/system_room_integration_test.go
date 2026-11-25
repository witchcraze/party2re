package party_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
)

func liveSystemRoomRepo(t *testing.T) *party.ValkeyRepository {
	t.Helper()
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is required")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ns := "party2:test:party:" + id.New() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		ids, err := client.Do(ctx, client.B().Zrange().Key(ns+"lobbies").Min("0").Max("-1").Build()).AsStrSlice()
		if err != nil {
			t.Error(err)
		}
		keys := []string{ns + "lobbies", ns + "sealing:king1", ns + "sealing:king99"}
		for _, id := range ids {
			keys = append(keys, ns+"lobby:"+id)
		}
		for _, key := range keys {
			if err := client.Do(ctx, client.B().Del().Key(key).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
	})
	return party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(ns+"lobby:"), party.WithReadyKeyPrefix(ns+"ready:"), party.WithCharacterKeyPrefix(ns+"character:"),
		party.WithLobbiesIndexKey(ns+"lobbies"), party.WithSystemRoomKeyPrefix(ns+"sealing:"))
}

func sealingRoom(stageID string, capacity int) party.Party {
	now := time.Now().UTC()
	return party.Party{
		ID: id.New(), Kind: party.KindSealing, LeaderName: "@破壊神", Name: "@" + stageID + "@", StageID: stageID, Speed: 12,
		MaxMembers: capacity, Status: party.StatusRecruiting, CreatedAt: now, UpdatedAt: now,
		Enemies: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: stageID + "-boss-0", Name: "破壊神", MaxHP: 9, Skills: []corebattle.ActionSkill{{ID: "dejon"}}}, NPC: true, CrystalReward: 40}},
	}
}

func TestSystemRoomValkey_OneRoomPerStageListedUntilGone(t *testing.T) {
	ctx := context.Background()
	repo := liveSystemRoomRepo(t)

	rooms := make([]party.Party, 8)
	opens := make([]func() error, len(rooms))
	opened := make([]bool, len(rooms))
	for i := range rooms {
		rooms[i] = sealingRoom("king1", 6)
		opens[i] = func() (err error) { opened[i], err = repo.OpenSystemRoom(ctx, rooms[i]); return err }
	}
	for _, err := range testutil.RunRace(opens...) {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner party.Party
	for i, ok := range opened {
		if ok {
			if winner.ID != "" {
				t.Fatal("concurrent unseals opened two king1 rooms")
			}
			winner = rooms[i]
		}
	}
	if winner.ID == "" {
		t.Fatal("no king1 room opened")
	}
	if ok, err := repo.OpenSystemRoom(ctx, sealingRoom("king99", 4)); err != nil || !ok {
		t.Fatalf("another stage was blocked: %v %v", ok, err)
	}

	got, err := repo.GetParty(ctx, winner.ID)
	if err != nil || got.Kind != party.KindSealing || got.LeaderCharacterID != "" || len(got.Enemies) != 1 || got.Enemies[0].CrystalReward != 40 || got.Enemies[0].Skills[0].ID != "dejon" {
		t.Fatalf("stored room lost its kind or roster: %+v %v", got, err)
	}
	list, total, err := repo.ListParties(ctx, party.StatusRecruiting, 50, 0)
	if err != nil || total != 2 {
		t.Fatalf("expected both empty rooms listed: total=%d err=%v", total, err)
	}
	for _, s := range list {
		if s.ID == winner.ID && (s.Kind != party.KindSealing || s.LeaderName != "@破壊神" || s.CurrentMembers != 0 || s.MaxMembers != 6) {
			t.Fatalf("unexpected sealing room summary: %+v", s)
		}
	}

	// The roster survives Lua lobby rewrites.
	if err := repo.AddMember(ctx, party.Member{PartyID: winner.ID, CharacterID: "joiner", JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetParty(ctx, winner.ID); err != nil || len(got.Enemies) != 1 {
		t.Fatalf("member write dropped the roster: %+v %v", got, err)
	}
	if err := repo.CleanupParty(ctx, party.LobbyState{Party: winner, Members: []party.Member{{CharacterID: "joiner"}}}); err != nil {
		t.Fatal(err)
	}
	next := sealingRoom("king1", 6)
	if ok, err := repo.OpenSystemRoom(ctx, next); err != nil || !ok {
		t.Fatalf("a stale claim blocked reopening after deletion: %v %v", ok, err)
	}
	if err := repo.DeleteParty(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSystemRoomJoin_ConcurrentJoinsFillCapacityAndChargeOnce(t *testing.T) {
	for _, capacity := range []int{6, 4} {
		ctx := context.Background()
		repo := liveSystemRoomRepo(t)
		db := database.OpenTestDB(t)
		chars, err := database.NewCharacterRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		stages, err := adventure.InitialStageCatalog()
		if err != nil {
			t.Fatal(err)
		}
		monsters, err := adventure.InitialMonsterCatalog()
		if err != nil {
			t.Fatal(err)
		}
		svc, err := party.NewService(repo, chars, nil, stages, monsters, nil, party.WithTransactionProvider(database.NewTransactionProvider(db)))
		if err != nil {
			t.Fatal(err)
		}
		room := sealingRoom("king1", capacity)
		if ok, err := repo.OpenSystemRoom(ctx, room); err != nil || !ok {
			t.Fatal(ok, err)
		}
		joiners := make([]corecharacter.Character, capacity+3)
		for i := range joiners {
			if joiners[i], err = database.CreateTestCharacter(ctx, db, "Seal"); err != nil {
				t.Fatal(err)
			}
		}
		database.RegisterCharacterCleanup(t, db, joiners...)
		state := party.LobbyState{Party: room}
		for _, c := range joiners {
			state.Members = append(state.Members, party.Member{CharacterID: c.ID})
		}
		t.Cleanup(func() {
			if err := repo.CleanupParty(context.Background(), state); err != nil {
				t.Error(err)
			}
		})

		stats := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: len(joiners), OpsPerWorker: 1}, func(worker, _ int) error {
			for {
				_, err := svc.JoinParty(ctx, room.ID, joiners[worker].ID, "")
				if !errors.Is(err, party.ErrPartyNotRecruiting) { // the lobby lock is busy; retry
					return err
				}
			}
		})
		if stats.Successes != int64(capacity) {
			t.Fatalf("capacity %d: %d joins succeeded", capacity, stats.Successes)
		}
		members, err := repo.GetMembers(ctx, room.ID)
		if err != nil || len(members) != capacity {
			t.Fatalf("capacity %d: %d members, err=%v", capacity, len(members), err)
		}
		joined := make(map[string]bool, len(members))
		for _, m := range members {
			joined[m.CharacterID] = true
		}
		for _, c := range joiners {
			got, err := chars.FindByID(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if joined[c.ID] {
				want = 20
			}
			if got.Tired != c.Tired+want {
				t.Fatalf("capacity %d: %s joined=%v Tired %d -> %d", capacity, c.ID, joined[c.ID], c.Tired, got.Tired)
			}
		}
	}
}

// The membership check must follow the character row lock: joins to different
// rooms hold different lobby locks and only meet on that row.
func TestJoin_SameCharacterRacingTwoRoomsJoinsOnce(t *testing.T) {
	ctx := context.Background()
	repo := liveSystemRoomRepo(t)
	db := database.OpenTestDB(t)
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := party.NewService(repo, chars, nil, stages, monsters, nil, party.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	racer, err := database.CreateTestCharacter(ctx, db, "Racer")
	if err != nil {
		t.Fatal(err)
	}
	leader, err := database.CreateTestCharacter(ctx, db, "Leader")
	if err != nil {
		t.Fatal(err)
	}
	database.RegisterCharacterCleanup(t, db, racer, leader)
	sealing := sealingRoom("king1", 6)
	if ok, err := repo.OpenSystemRoom(ctx, sealing); err != nil || !ok {
		t.Fatal(ok, err)
	}
	lobby, err := svc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Name: "race", StageID: adventure.StarterAdventure})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, p := range []party.Party{sealing, lobby.Party} {
			state := party.LobbyState{Party: p, Members: []party.Member{{CharacterID: racer.ID}, {CharacterID: leader.ID}}}
			if err := repo.CleanupParty(context.Background(), state); err != nil {
				t.Error(err)
			}
		}
	})

	join := func(id string) func() error {
		return func() error {
			for {
				_, err := svc.JoinParty(ctx, id, racer.ID, "")
				if !errors.Is(err, party.ErrPartyNotRecruiting) { // the lobby lock is busy; retry
					return err
				}
			}
		}
	}
	for round := range 10 {
		base, err := chars.FindByID(ctx, racer.ID)
		if err != nil {
			t.Fatal(err)
		}
		errs := testutil.RunRace(join(sealing.ID), join(lobby.Party.ID))
		var won []string
		for i, err := range errs {
			switch {
			case err == nil:
				won = append(won, []string{sealing.ID, lobby.Party.ID}[i])
			case !errors.Is(err, party.ErrAlreadyInParty):
				t.Fatalf("round %d: unexpected join error %v", round, err)
			}
		}
		if len(won) != 1 {
			t.Fatalf("round %d: %d joins succeeded", round, len(won))
		}
		active, _, err := repo.GetActivePartyByCharacter(ctx, racer.ID)
		if err != nil || active.ID != won[0] {
			t.Fatalf("round %d: membership index %q, want %q (%v)", round, active.ID, won[0], err)
		}
		for _, id := range []string{sealing.ID, lobby.Party.ID} {
			if _, err := repo.GetMember(ctx, id, racer.ID); (err == nil) != (id == won[0]) {
				t.Fatalf("round %d: room %s membership disagrees with the index: %v", round, id, err)
			}
		}
		got, err := chars.FindByID(ctx, racer.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := base.Tired
		if won[0] == sealing.ID {
			want += 20
		}
		if got.Tired != want {
			t.Fatalf("round %d: Tired %d -> %d, want %d", round, base.Tired, got.Tired, want)
		}
		if err := svc.LeaveParty(ctx, won[0], racer.ID); err != nil {
			t.Fatal(err)
		}
		got.Tired = 0
		if err := chars.Update(ctx, got); err != nil {
			t.Fatal(err)
		}
	}
}

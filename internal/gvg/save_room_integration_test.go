package gvg_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
)

func TestGvGPasswordVerifierRoundTripIntegration(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is required")
	}

	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Fatalf("ping Valkey at %s: %v", valkeyAddr, err)
	}

	repo1, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	repo2, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}

	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	standingRepo := newMockStandingRepo()
	battleEngine := &mockBattleEngine{outcome: corebattle.OutcomeWin}

	leaderID := "gvg-pw-leader-" + id.New()
	joinerID := "gvg-pw-joiner-" + id.New()
	thirdID := "gvg-pw-third-" + id.New()
	openLeaderID := "gvg-open-leader-" + id.New()
	openJoinerID := "gvg-open-joiner-" + id.New()
	legacyLeaderID := "gvg-legacy-leader-" + id.New()
	legacyJoinerID := "gvg-legacy-joiner-" + id.New()

	allCharIDs := []string{leaderID, joinerID, thirdID, openLeaderID, openJoinerID, legacyLeaderID, legacyJoinerID}
	for _, cid := range allCharIDs {
		charRepo.characters[cid] = createTestCharacter(cid, "Char_"+cid[:8], 100, 0)
	}
	g1ID := "g1-" + id.New()
	g2ID := "g2-" + id.New()
	guildRepo.guilds[g1ID] = guild.Guild{ID: g1ID, Name: "Crimson", Color: "#FF3333"}
	guildRepo.guilds[g2ID] = guild.Guild{ID: g2ID, Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds[leaderID] = g1ID
	guildRepo.charGuilds[openLeaderID] = g1ID
	guildRepo.charGuilds[legacyLeaderID] = g1ID
	guildRepo.charGuilds[joinerID] = g2ID
	guildRepo.charGuilds[thirdID] = g2ID
	guildRepo.charGuilds[openJoinerID] = g2ID
	guildRepo.charGuilds[legacyJoinerID] = g2ID

	var ownedRoomIDs []string
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		for _, rid := range ownedRoomIDs {
			if err := repo1.DeleteRoom(cleanCtx, rid); err != nil {
				t.Errorf("cleanup DeleteRoom(%s): %v", rid, err)
			}
		}
		for _, rid := range ownedRoomIDs {
			for _, cid := range allCharIDs {
				if err := repo1.DeleteCharacterRoom(cleanCtx, cid, rid); err != nil {
					t.Errorf("cleanup DeleteCharacterRoom(%s, %s): %v", cid, rid, err)
				}
			}
		}
	})

	svc1, err := gvg.NewService(repo1, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}
	svc2, err := gvg.NewService(repo2, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	assertNoSecretLeak := func(label string, v any, secretHash string) {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s json.Marshal: %v", label, err)
		}
		s := string(raw)
		if strings.Contains(s, "password_hash") || (secretHash != "" && strings.Contains(s, secretHash)) || strings.Contains(s, `"secret"`) {
			t.Fatalf("%s exposed secret verifier in public JSON: %s", label, s)
		}
	}

	// 1. Create password-protected room and verify across fresh repository/service reload.
	created, err := svc1.CreateRoom(ctx, leaderID, gvg.CreateRoomRequest{
		Name:       "Protected-" + id.New()[:8],
		Password:   "secret",
		MaxMembers: 4,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom(protected): %v", err)
	}
	ownedRoomIDs = append(ownedRoomIDs, created.Room.ID)
	wantHash := created.Room.PasswordHash
	if wantHash == "" {
		t.Fatal("expected non-empty PasswordHash on created protected room")
	}
	assertNoSecretLeak("CreateRoom(protected)", created, wantHash)

	reloaded, err := svc2.GetRoom(ctx, created.Room.ID)
	if err != nil {
		t.Fatalf("GetRoom(protected): %v", err)
	}
	if reloaded.Room.PasswordHash != wantHash {
		t.Fatalf("reloaded PasswordHash = %q, want %q", reloaded.Room.PasswordHash, wantHash)
	}
	assertNoSecretLeak("GetRoom(protected)", reloaded, wantHash)

	findSummary := func(rooms []gvg.RoomSummary, roomID string) (gvg.RoomSummary, bool) {
		for _, r := range rooms {
			if r.ID == roomID {
				return r, true
			}
		}
		return gvg.RoomSummary{}, false
	}

	listed, err := svc2.ListRooms(ctx)
	if err != nil {
		t.Fatalf("ListRooms(protected): %v", err)
	}
	assertNoSecretLeak("ListRooms(protected)", listed, wantHash)
	if summary, ok := findSummary(listed, created.Room.ID); !ok || !summary.HasPassword {
		t.Fatalf("expected protected room in listing with HasPassword=true, got found=%v summary=%+v", ok, summary)
	}

	// 2. Real service rejects wrong/empty passwords after reload.
	if _, err := svc2.JoinRoom(ctx, joinerID, created.Room.ID, "wrong"); !errors.Is(err, gvg.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for wrong password, got %v", err)
	}
	if _, err := svc2.JoinRoom(ctx, joinerID, created.Room.ID, ""); !errors.Is(err, gvg.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for empty password, got %v", err)
	}

	// 3. Correct password joins and preserves verifier across join and leave recruiting-room rewrites.
	joined, err := svc2.JoinRoom(ctx, joinerID, created.Room.ID, "secret")
	if err != nil {
		t.Fatalf("JoinRoom(protected): %v", err)
	}
	assertNoSecretLeak("JoinRoom(protected)", joined, wantHash)
	if afterJoin, err := repo2.GetRoom(ctx, created.Room.ID); err != nil || afterJoin.Room.PasswordHash != wantHash {
		t.Fatalf("PasswordHash after JoinRoom = %q, want %q (err=%v)", afterJoin.Room.PasswordHash, wantHash, err)
	}

	// Non-leader leave rewrites recruiting room.
	if _, err := svc2.LeaveRoom(ctx, joinerID, created.Room.ID); err != nil {
		t.Fatalf("LeaveRoom(joiner): %v", err)
	}
	if afterLeave, err := repo2.GetRoom(ctx, created.Room.ID); err != nil || afterLeave.Room.PasswordHash != wantHash {
		t.Fatalf("PasswordHash after LeaveRoom = %q, want %q (err=%v)", afterLeave.Room.PasswordHash, wantHash, err)
	}

	// Re-join and leader leave (leadership transfer rewrite) preserves verifier and policy.
	if _, err := svc2.JoinRoom(ctx, joinerID, created.Room.ID, "secret"); err != nil {
		t.Fatalf("re-JoinRoom(protected): %v", err)
	}
	if _, err := svc2.LeaveRoom(ctx, leaderID, created.Room.ID); err != nil {
		t.Fatalf("LeaveRoom(leader): %v", err)
	}
	if afterTransfer, err := repo2.GetRoom(ctx, created.Room.ID); err != nil || afterTransfer.Room.PasswordHash != wantHash {
		t.Fatalf("PasswordHash after leader transfer = %q, want %q (err=%v)", afterTransfer.Room.PasswordHash, wantHash, err)
	}
	if listedAfter, err := svc2.ListRooms(ctx); err != nil {
		t.Fatalf("ListRooms after recruiting updates: %v", err)
	} else if summary, ok := findSummary(listedAfter, created.Room.ID); !ok || !summary.HasPassword {
		t.Fatalf("expected HasPassword=true after recruiting updates, got found=%v summary=%+v", ok, summary)
	}
	if _, err := svc2.JoinRoom(ctx, thirdID, created.Room.ID, "wrong"); !errors.Is(err, gvg.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword after recruiting updates, got %v", err)
	}
	if _, err := svc2.JoinRoom(ctx, thirdID, created.Room.ID, "secret"); err != nil {
		t.Fatalf("expected valid password join after recruiting updates, got %v", err)
	}

	// 4. Unprotected room control.
	openRoom, err := svc1.CreateRoom(ctx, openLeaderID, gvg.CreateRoomRequest{
		Name:       "Open-" + id.New()[:8],
		MaxMembers: 4,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom(open): %v", err)
	}
	ownedRoomIDs = append(ownedRoomIDs, openRoom.Room.ID)
	if listedOpen, err := svc2.ListRooms(ctx); err != nil {
		t.Fatalf("ListRooms(open): %v", err)
	} else if summary, ok := findSummary(listedOpen, openRoom.Room.ID); !ok || summary.HasPassword {
		t.Fatalf("expected open room HasPassword=false, got found=%v summary=%+v", ok, summary)
	}
	if _, err := svc2.JoinRoom(ctx, openJoinerID, openRoom.Room.ID, ""); err != nil {
		t.Fatalf("JoinRoom(open): %v", err)
	}
	if _, err := svc2.LeaveRoom(ctx, openJoinerID, openRoom.Room.ID); err != nil {
		t.Fatalf("LeaveRoom(open): %v", err)
	}
	if afterOpenLeave, err := repo2.GetRoom(ctx, openRoom.Room.ID); err != nil || afterOpenLeave.Room.PasswordHash != "" {
		t.Fatalf("expected empty PasswordHash on open room after update, got %q (err=%v)", afterOpenLeave.Room.PasswordHash, err)
	}

	// 5. Pre-update recruiting room stored without password_hash remains readable and joinable.
	legacyRoomID := "legacy-" + id.New()
	ownedRoomIDs = append(ownedRoomIDs, legacyRoomID)
	now := time.Now().UTC()
	legacyDetail := gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:                legacyRoomID,
			Name:              "Legacy-" + id.New()[:8],
			LeaderCharacterID: legacyLeaderID,
			LeaderName:        "LegacyLeader",
			Speed:             gvg.DefaultSpeed,
			MaxMembers:        4,
			TargetWins:        1,
			PrizePool:         gvg.InitialPrizeGP,
			Status:            gvg.StatusRecruiting,
			GuildScores:       map[string]int{},
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		Members: []gvg.GvGMember{
			{RoomID: legacyRoomID, CharacterID: legacyLeaderID, CharacterName: "LegacyLeader", GuildID: g1ID, GuildColor: "#FF3333", IsLeader: true, JoinedAt: now},
		},
	}
	legacyBytes, err := json.Marshal(legacyDetail)
	if err != nil {
		t.Fatalf("marshal legacy detail: %v", err)
	}
	if err := client.Do(ctx, client.B().Zadd().Key(gvg.DefaultRoomsActiveIndexKey).ScoreMember().ScoreMember(float64(now.Unix()), legacyRoomID).Build()).Error(); err != nil {
		t.Fatalf("seed legacy index: %v", err)
	}
	if err := client.Do(ctx, client.B().Set().Key(gvg.DefaultRoomKeyPrefix+legacyRoomID).Value(string(legacyBytes)).Ex(gvg.DefaultLobbyTTL).Build()).Error(); err != nil {
		t.Fatalf("seed legacy room: %v", err)
	}
	if err := repo1.SetCharacterRoom(ctx, legacyLeaderID, legacyRoomID); err != nil {
		t.Fatalf("seed legacy char room: %v", err)
	}
	if gotLegacy, err := svc2.GetRoom(ctx, legacyRoomID); err != nil || gotLegacy.Room.PasswordHash != "" {
		t.Fatalf("GetRoom(legacy) got %+v, err=%v", gotLegacy, err)
	}
	if listedLegacy, err := svc2.ListRooms(ctx); err != nil {
		t.Fatalf("ListRooms(legacy): %v", err)
	} else if summary, ok := findSummary(listedLegacy, legacyRoomID); !ok || summary.HasPassword {
		t.Fatalf("expected legacy room HasPassword=false, got found=%v summary=%+v", ok, summary)
	}
	if _, err := svc2.JoinRoom(ctx, legacyJoinerID, legacyRoomID, ""); err != nil {
		t.Fatalf("JoinRoom(legacy): %v", err)
	}
}

// Reject one save command with a real server error without corrupting the shared index.
type saveFaultClient struct {
	valkey.Client
	command, key, wrongTypeKey string
	fail                       bool
	beforeSet                  func()
}

func (c *saveFaultClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	args := cmd.Commands()
	if args[0] == "SET" && args[1] == c.key && c.beforeSet != nil {
		c.beforeSet()
	}
	if c.fail && args[0] == c.command && args[1] == c.key {
		return c.Client.Do(ctx, c.B().Zadd().Key(c.wrongTypeKey).ScoreMember().ScoreMember(1, "fault").Build())
	}
	return c.Client.Do(ctx, cmd)
}

type countingGvGBattle struct {
	gvg.BattleEngine
	calls int
}

func TestGvGRoomListingDuringPublicationIntegration(t *testing.T) {
	f := newRewardFixture(t, 2, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	roomKey := gvg.DefaultRoomKeyPrefix + f.detail.Room.ID
	if err := f.client.Do(ctx, f.client.B().Del().Key(roomKey).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	detail := f.detail
	detail.Room.Status = gvg.StatusRecruiting
	// Keep this test's candidate in the bounded listing even alongside other fixtures.
	detail.Room.UpdatedAt = time.Now().UTC().Add(time.Minute)
	listed := false
	client := &saveFaultClient{Client: f.client, key: roomKey, beforeSet: func() {
		listed = true
		if _, err := f.rooms.ListRooms(ctx); err != nil {
			t.Fatal(err)
		}
	}}
	rooms, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	if !listed {
		t.Fatal("listing did not interleave with publication")
	}
	if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(detail.Room.ID).Build()).AsFloat64(); err != nil || score != float64(detail.Room.UpdatedAt.Unix()) {
		t.Fatalf("listing erased publishing room's index: %v %v", score, err)
	}
	list, err := rooms.ListRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range list {
		if room.ID == detail.Room.ID {
			return
		}
	}
	t.Fatal("successful publication is absent from room listing")
}

func (b *countingGvGBattle) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	b.calls++
	return b.BattleEngine.ResolvePartyBattle(req)
}

func TestGvGSaveRoomCommandFailureIntegration(t *testing.T) {
	for _, command := range []string{"ZADD", "SET"} {
		for _, target := range []int{1, 2} {
			name := command + map[int]string{1: "/final", 2: "/intermediate"}[target]
			t.Run(name, func(t *testing.T) {
				f := newRewardFixture(t, target, 1, corebattle.OutcomeWin)
				ctx := context.Background()
				roomKey := gvg.DefaultRoomKeyPrefix + f.detail.Room.ID
				key := roomKey
				if command == "ZADD" {
					key = gvg.DefaultRoomsActiveIndexKey
				}
				wrongTypeKey := "party2:test:gvg-save:" + id.New()
				if err := f.client.Do(ctx, f.client.B().Set().Key(wrongTypeKey).Value("wrong type").ExSeconds(60).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := f.client.Do(ctx, f.client.B().Del().Key(wrongTypeKey).Build()).Error(); err != nil {
						t.Error(err)
					}
				})
				fault := &saveFaultClient{Client: f.client, command: command, key: key, wrongTypeKey: wrongTypeKey, fail: true}
				rooms, err := gvg.NewValkeyRoomRepository(fault)
				if err != nil {
					t.Fatal(err)
				}
				chars, err := database.NewCharacterRepository(f.db)
				if err != nil {
					t.Fatal(err)
				}
				battle := &countingGvGBattle{BattleEngine: f.battle}
				service, err := gvg.NewService(rooms, f.standings, newMockGuildRepo(), chars, battle)
				if err != nil {
					t.Fatal(err)
				}
				before, err := rooms.GetRoom(ctx, f.detail.Room.ID)
				if err != nil {
					t.Fatal(err)
				}
				before.Room.UpdatedAt = before.Room.UpdatedAt.Add(-time.Minute)
				if err := f.rooms.SaveRoom(ctx, before.Room, before.Members); err != nil {
					t.Fatal(err)
				}
				if err := f.client.Do(ctx, f.client.B().Expire().Key(roomKey).Seconds(60).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				if _, err := service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 1); err == nil || !strings.Contains(err.Error(), "WRONGTYPE") {
					t.Fatalf("expected %s server error: %v", command, err)
				}
				after, err := rooms.GetRoom(ctx, before.Room.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Errorf("failed save advanced room: before=%+v after=%+v", before.Room, after.Room)
				}
				if ttl, err := f.client.Do(ctx, f.client.B().Ttl().Key(roomKey).Build()).AsInt64(); err != nil || ttl <= 0 || ttl > 60 {
					t.Errorf("failed save refreshed room TTL: %d %v", ttl, err)
				}
				gp, opponentGP, wins := int64(3), int64(0), 0
				if target == 1 {
					gp, opponentGP, wins = 7, 4, 1
				}
				f.assertRewards(t, gp, opponentGP, wins, 0)
				saved, found, err := f.standings.FindRoundTransition(ctx, before.Room.ID)
				if err != nil || !found {
					t.Fatalf("SQL commit missing: %v %v", found, err)
				}
				expectedScore := float64(before.Room.UpdatedAt.Unix())
				if command == "SET" {
					expectedScore = float64(saved.Detail.Room.UpdatedAt.Unix())
				}
				if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(before.Room.ID).Build()).AsFloat64(); err != nil || score != expectedScore {
					t.Errorf("index after failed %s: score=%v want=%v err=%v", command, score, expectedScore, err)
				}
				fault.fail = false
				result, err := service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 1)
				if err != nil {
					t.Fatal(err)
				}
				if battle.calls != 1 || !reflect.DeepEqual(result, saved.Result) {
					t.Fatalf("retry ran another battle: calls=%d result=%+v", battle.calls, result)
				}
				if ttl, err := f.client.Do(ctx, f.client.B().Ttl().Key(roomKey).Build()).AsInt64(); err != nil || ttl < 1700 || ttl > 1800 {
					t.Errorf("recovered room TTL: %d %v", ttl, err)
				}
				if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(before.Room.ID).Build()).AsFloat64(); err != nil || score != float64(saved.Detail.Room.UpdatedAt.Unix()) {
					t.Errorf("recovered index: %v %v", score, err)
				}
				f.assertRewards(t, gp, opponentGP, wins, 0)
				if target == 2 {
					result, err = service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 2)
					if err != nil {
						t.Fatal(err)
					}
					if battle.calls != 2 || !result.MatchCompleted {
						t.Fatalf("next round did not complete: calls=%d result=%+v", battle.calls, result)
					}
					f.assertRewards(t, 10, 4, 1, 0)
				}
			})
		}
	}
}

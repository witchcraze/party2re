package boss_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

// Faults affect one lower-level command; every other read/write reaches real Valkey.
type sealingFaultClient struct {
	valkey.Client
	command, key string
	after, fired bool
}

func (c *sealingFaultClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	args := cmd.Commands()
	if !c.fired && args[0] == c.command && len(args) > 1 && args[1] == c.key {
		c.fired = true
		if c.after {
			if res := c.Client.Do(ctx, cmd); res.Error() != nil {
				return res
			}
		}
		return valkeytest.MakeErrorResult(errInjected)
	}
	return c.Client.Do(ctx, cmd)
}

type sealingTxFault struct {
	boss.TransactionProvider
	commit, unknown bool
	probeLost       bool
	calls           int
	cancel          context.CancelFunc
}

func (p *sealingTxFault) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	p.calls++
	if p.calls > 1 {
		if p.unknown {
			return errInjected
		}
		err := p.TransactionProvider.RunInTx(ctx, fn)
		if err == nil && p.probeLost {
			return errInjected
		}
		return err
	}
	err := p.TransactionProvider.RunInTx(ctx, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		if p.cancel != nil {
			p.cancel()
			return ctx.Err()
		}
		if !p.commit {
			return errInjected
		}
		return nil
	})
	if err != nil {
		return err
	}
	return errInjected // SQL committed, but its response was lost.
}

type sealingStorageFixture struct {
	db       *sql.DB
	client   valkey.Client
	rooms    *party.ValkeyRepository
	repo     *database.BossRepository
	chars    *database.CharacterRepository
	state    party.LobbyState
	indexKey string
	before   map[string]corecharacter.Character
	battles  atomic.Int32
}

func newSealingStorageFixture(t *testing.T) *sealingStorageFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("MariaDB and Valkey settings required")
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
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{os.Getenv("PARTY2_VALKEY_ADDR")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	repo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := &sealingStorageFixture{db: db, client: client, repo: repo, chars: chars, before: map[string]corecharacter.Character{}, indexKey: "party2:test:boss:rooms:" + id.New()}
	f.rooms = f.roomRepository(client)
	f.state.Party = party.Party{ID: id.New(), StageID: "king1", Status: party.StatusRecruiting, MaxMembers: 6, CreatedAt: time.Now().UTC()}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, "SealRecovery", 1000)
		if err != nil {
			t.Fatal(err)
		}
		c.Level = 50
		c.Stats.HP, c.Stats.MaxHP = 600, 600
		if err := chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		c, err = chars.FindByID(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.before[c.ID] = c
		f.state.Members = append(f.state.Members, party.Member{PartyID: f.state.Party.ID, CharacterID: c.ID, CharacterName: c.Name, ReadyState: true, IsLeader: i == 0})
		if i == 0 {
			f.state.Party.LeaderCharacterID = c.ID
		}
	}
	if err := f.rooms.SaveParty(ctx, f.state.Party); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.state.Members {
		if err := f.rooms.AddMember(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := f.rooms.CleanupParty(context.Background(), f.state); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *sealingStorageFixture) roomRepository(client valkey.Client) *party.ValkeyRepository {
	return party.NewValkeyRepository(client, party.WithLobbiesIndexKey(f.indexKey))
}

func (f *sealingStorageFixture) service(t *testing.T, rooms boss.PartyRepository, tx boss.TransactionProvider, opts ...boss.ServiceOption) *boss.Service {
	t.Helper()
	engine := stubPartyBattleEngine{resolveFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		f.battles.Add(1)
		return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, Turns: 1, TotalReward: req.VictoryReward}, nil
	}}
	s, err := boss.NewService(f.repo, f.chars, engine)
	if err != nil {
		t.Fatal(err)
	}
	s.Configure(boss.WithPartyRepository(rooms), boss.WithTransactionProvider(tx))
	s.Configure(opts...)
	return s
}

func (f *sealingStorageFixture) start(s *boss.Service) (boss.SealingBattleResult, error) {
	return s.StartSealingBattle(context.Background(), f.state.Party.ID, f.state.Party.LeaderCharacterID)
}

func (f *sealingStorageFixture) assertAssets(t *testing.T, committed bool) {
	t.Helper()
	for charID, before := range f.before {
		c, err := f.chars.FindByID(context.Background(), charID)
		if err != nil {
			t.Fatal(err)
		}
		history, err := f.repo.GetHistory(context.Background(), charID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if !committed {
			if c != before || len(history) != 0 {
				t.Fatalf("rollback lost assets/history for %s: %+v histories=%d", charID, c, len(history))
			}
			continue
		}
		if len(history) != 1 || c.HeroCount != before.HeroCount+1 || c.Money != before.Money+history[0].RewardGold || c.Tired != before.Tired+20 || c.Experience != before.Experience+history[0].RewardExp || c.Stats.HP != before.Stats.HP || c.Stats.MP != before.Stats.MP {
			t.Fatalf("settlement not exactly once: %+v history=%+v", c, history)
		}
		var receipt boss.SealingSettlement
		if err := database.RunInTx(context.Background(), f.db, func(ctx context.Context) error {
			r, err := f.repo.LockSealingSettlement(ctx, f.state.Party.ID)
			if err == nil && r != nil {
				receipt = *r
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if receipt.Lobby.Party.ID != f.state.Party.ID || c.Crystal != before.Crystal+receipt.Result.RewardCrystals {
			t.Fatal("crystals/receipt lost")
		}
		var loot int
		if err := f.db.QueryRow(`SELECT COALESCE(SUM(quantity),0) FROM inventory_items WHERE character_id = ? AND definition_id = ?`, charID, history[0].RewardItemID).Scan(&loot); err != nil {
			t.Fatal(err)
		}
		if loot != 1 {
			t.Fatalf("reward loot=%d, want 1", loot)
		}
		rec, err := f.repo.GetOrCreateRecord(context.Background(), charID)
		if err != nil || rec.TotalBossDefeats != 1 {
			t.Fatalf("boss record=%+v err=%v", rec, err)
		}
	}
}

func (f *sealingStorageFixture) assertRoom(t *testing.T, present bool) {
	t.Helper()
	ctx := context.Background()
	p, err := f.rooms.GetParty(ctx, f.state.Party.ID)
	if !present {
		if !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("room remains: %+v %v", p, err)
		}
	} else {
		if err != nil || p.Status != party.StatusRecruiting {
			t.Fatalf("room not resumable: %+v %v", p, err)
		}
		members, err := f.rooms.GetMembers(ctx, p.ID)
		if err != nil || len(members) != 2 {
			t.Fatalf("roster: %+v %v", members, err)
		}
		for _, m := range members {
			if !m.ReadyState {
				t.Fatal("ready state lost")
			}
		}
	}
	for _, m := range f.state.Members {
		mapping, err := f.client.Do(ctx, f.client.B().Get().Key(party.DefaultCharacterKeyPrefix+m.CharacterID).Build()).ToString()
		if present && (err != nil || mapping != f.state.Party.ID) {
			t.Fatalf("mapping lost: %s %v", mapping, err)
		}
		if !present && !valkey.IsValkeyNil(err) {
			t.Fatalf("mapping remains: %s %v", mapping, err)
		}
		ready, err := f.client.Do(ctx, f.client.B().Exists().Key(party.DefaultReadyKeyPrefix+f.state.Party.ID+":"+m.CharacterID).Build()).AsInt64()
		if err != nil || (ready == 1) != present {
			t.Fatalf("ready key=%d err=%v", ready, err)
		}
	}
	_, err = f.client.Do(ctx, f.client.B().Zscore().Key(f.indexKey).Member(f.state.Party.ID).Build()).AsFloat64()
	if present && err != nil || !present && !valkey.IsValkeyNil(err) {
		t.Fatalf("lobby index: %v", err)
	}
}

func TestSealingCleanupRecoveryIntegration(t *testing.T) {
	for _, command := range []string{"member", "lobby", "index", "lost-delete-response", "success"} {
		t.Run(command, func(t *testing.T) {
			f := newSealingStorageFixture(t)
			fault := &sealingFaultClient{Client: f.client, command: "DEL", key: party.DefaultLobbyKeyPrefix + f.state.Party.ID}
			switch command {
			case "member":
				fault.key = party.DefaultReadyKeyPrefix + f.state.Party.ID + ":" + f.state.Members[1].CharacterID
			case "index":
				fault.command, fault.key = "ZREM", f.indexKey
			case "lost-delete-response":
				fault.after = true
			case "success":
				fault.fired = true
			}
			s := f.service(t, f.roomRepository(fault), database.NewTransactionProvider(f.db))
			result, err := f.start(s)
			if command != "success" && !errors.Is(err, errInjected) || command == "success" && err != nil {
				t.Fatalf("start: %v", err)
			}
			f.assertAssets(t, true)
			ctx := context.Background()
			_, indexErr := f.client.Do(ctx, f.client.B().Zscore().Key(f.indexKey).Member(f.state.Party.ID).Build()).AsFloat64()
			if command == "success" && !valkey.IsValkeyNil(indexErr) || command != "success" && indexErr != nil {
				t.Fatalf("partial cleanup index: %v", indexErr)
			}
			p, roomErr := f.rooms.GetParty(ctx, f.state.Party.ID)
			bodyRemains := command == "member" || command == "lobby"
			if bodyRemains && (roomErr != nil || p.Status != party.StatusInProgress) || !bodyRemains && !errors.Is(roomErr, party.ErrNotFound) {
				t.Fatalf("partial cleanup room: %+v %v", p, roomErr)
			}
			for i, m := range f.state.Members {
				_, mapErr := f.client.Do(ctx, f.client.B().Get().Key(party.DefaultCharacterKeyPrefix+m.CharacterID).Build()).ToString()
				if !valkey.IsValkeyNil(mapErr) {
					t.Fatalf("partial cleanup mapping remains: %v", mapErr)
				}
				ready, readyErr := f.client.Do(ctx, f.client.B().Exists().Key(party.DefaultReadyKeyPrefix+f.state.Party.ID+":"+m.CharacterID).Build()).AsInt64()
				wantReady := int64(0)
				if command == "member" && i == 1 {
					wantReady = 1
				}
				if readyErr != nil || ready != wantReady {
					t.Fatalf("partial cleanup ready=%d want=%d err=%v", ready, wantReady, readyErr)
				}
			}

			// A fresh service must discover the receipt even if the lobby body was already deleted.
			s = f.service(t, f.rooms, database.NewTransactionProvider(f.db))
			for i := 0; i < 2; i++ {
				replayed, err := f.start(s)
				if err != nil || !reflect.DeepEqual(result, replayed) {
					t.Fatalf("recovery changed outcome: %v", err)
				}
			}
			if f.battles.Load() != 1 {
				t.Fatal("cleanup replayed battle")
			}
			f.assertAssets(t, true)
			f.assertRoom(t, false)
		})
	}
}

func TestSealingCommitBoundaryRecoveryIntegration(t *testing.T) {
	for _, mode := range []string{"rollback", "commit-response-lost", "probe-response-lost", "unknown-rollback", "unknown-commit", "collection", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newSealingStorageFixture(t)
			committed := mode == "commit-response-lost" || mode == "unknown-commit" || mode == "probe-response-lost"
			tx := &sealingTxFault{TransactionProvider: database.NewTransactionProvider(f.db), commit: committed, unknown: mode == "unknown-rollback" || mode == "unknown-commit", probeLost: mode == "probe-response-lost"}
			s := f.service(t, f.rooms, tx)
			if mode == "collection" {
				s.Configure(boss.WithMonsterDefeatRecorder(&failingRecorder{failOn: 9}))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				tx.cancel = cancel
			}
			_, err := s.StartSealingBattle(ctx, f.state.Party.ID, f.state.Party.LeaderCharacterID)
			if mode == "commit-response-lost" || mode == "probe-response-lost" {
				if err != nil {
					t.Fatalf("known committed result lost: %v", err)
				}
			} else if mode == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			} else if !errors.Is(err, errInjected) {
				t.Fatalf("failure not propagated: %v", err)
			}
			f.assertAssets(t, committed)
			if tx.unknown {
				p, err := f.rooms.GetParty(context.Background(), f.state.Party.ID)
				if err != nil || p.Status != party.StatusInProgress {
					t.Fatalf("unknown outcome released/destroyed: %+v %v", p, err)
				}
			} else {
				f.assertRoom(t, !committed)
			}
			// Remove faults and resume from authoritative SQL, not the previous process's memory.
			s = f.service(t, f.rooms, database.NewTransactionProvider(f.db))
			if _, err := f.start(s); err != nil {
				t.Fatalf("retry: %v", err)
			}
			wantBattles := int32(2)
			if committed {
				wantBattles = 1
			}
			if f.battles.Load() != wantBattles {
				t.Fatalf("battles=%d want=%d", f.battles.Load(), wantBattles)
			}
			f.assertAssets(t, true)
			f.assertRoom(t, false)
		})
	}
}

func TestSealingCleanupPreservesNewMembershipIntegration(t *testing.T) {
	f := newSealingStorageFixture(t)
	fault := &sealingFaultClient{Client: f.client, command: "DEL", key: party.DefaultLobbyKeyPrefix + f.state.Party.ID}
	s := f.service(t, f.roomRepository(fault), database.NewTransactionProvider(f.db))
	if _, err := f.start(s); !errors.Is(err, errInjected) {
		t.Fatal(err)
	}
	newState := party.LobbyState{Party: f.state.Party, Members: f.state.Members[:1]}
	newState.Party.ID = id.New()
	newState.Members = append([]party.Member(nil), newState.Members...)
	newState.Members[0].PartyID = newState.Party.ID
	ctx := context.Background()
	if err := f.rooms.SaveParty(ctx, newState.Party); err != nil {
		t.Fatal(err)
	}
	if err := f.rooms.AddMember(ctx, newState.Members[0]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.rooms.CleanupParty(ctx, newState); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.start(s); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSealingBattle(ctx, f.state.Party.ID, f.state.Members[1].CharacterID); !errors.Is(err, boss.ErrNotPartyLeader) {
		t.Fatalf("receipt authorization: %v", err)
	}
	p, m, err := f.rooms.GetActivePartyByCharacter(ctx, newState.Party.LeaderCharacterID)
	if err != nil || p.ID != newState.Party.ID || !m.ReadyState {
		t.Fatalf("cleanup erased successor membership: %+v %+v %v", p, m, err)
	}
	f.assertAssets(t, true)
}

// Simulate a lapsed Valkey lease: correctness must come from the SQL room lock/receipt.
type lapsedSealingLock struct{ *party.ValkeyRepository }

func (r lapsedSealingLock) WithPartyAdventureLock(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestSealingConcurrentRecoveryIntegration(t *testing.T) {
	f := newSealingStorageFixture(t)
	s := f.service(t, lapsedSealingLock{f.rooms}, database.NewTransactionProvider(f.db))
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
		_, err := f.start(s)
		if errors.Is(err, boss.ErrPartyNotRecruiting) {
			return nil
		}
		return err
	})
	if res.Failures != 0 || f.battles.Load() != 1 {
		t.Fatalf("concurrent settlement: %+v battles=%d", res, f.battles.Load())
	}
	f.assertAssets(t, true)
	f.assertRoom(t, false)
}

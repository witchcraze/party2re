package boss_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

var errInjected = errors.New("injected settlement failure")

// snapshotTx emulates SQL rollback over the in-memory repositories: when the
// transaction body fails, the SQL-side state is restored to the pre-start snapshot.
type snapshotTx struct{ f *sealFixture }

func (t snapshotTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	chars := make(map[string]corecharacter.Character, len(t.f.charRepo.chars))
	for k, v := range t.f.charRepo.chars {
		chars[k] = v
	}
	records := make(map[string]boss.CharacterBossRecord, len(t.f.bossRepo.records))
	for k, v := range t.f.bossRepo.records {
		records[k] = v
	}
	histories := make(map[string][]boss.BossChallengeHistory, len(t.f.bossRepo.histories))
	for k, v := range t.f.bossRepo.histories {
		histories[k] = v
	}
	items := make(map[string][]coreitem.Instance, len(t.f.bossRepo.awardedItems))
	for k, v := range t.f.bossRepo.awardedItems {
		items[k] = v
	}
	err := fn(ctx)
	if err != nil {
		t.f.charRepo.chars = chars
		t.f.bossRepo.records = records
		t.f.bossRepo.histories = histories
		t.f.bossRepo.awardedItems = items
	}
	return err
}

// failingBossRepo fails the failOn-th RecordChallenge call.
type failingBossRepo struct {
	*mockBossRepo
	failOn int
	calls  int
}

func (r *failingBossRepo) RecordChallenge(ctx context.Context, h boss.BossChallengeHistory, rec boss.CharacterBossRecord, c corecharacter.Character, item *coreitem.Instance) error {
	r.calls++
	if r.calls == r.failOn {
		return errInjected
	}
	return r.mockBossRepo.RecordChallenge(ctx, h, rec, c, item)
}

// failingDeletePartyRepo fails room completion (DeleteParty).
type failingDeletePartyRepo struct{ *mockPartyRepo }

func (r failingDeletePartyRepo) DeleteParty(context.Context, string) error { return errInjected }

// failingRecorder fails the failOn-th mandatory Collection/Hall of Fame record.
type failingRecorder struct {
	failOn int
	calls  int
}

func (r *failingRecorder) RecordMonsterDefeat(context.Context, string, boss.DefeatedMonsterRecord) error {
	r.calls++
	if r.calls == r.failOn {
		return errInjected
	}
	return nil
}

type sealFixture struct {
	bossRepo  *mockBossRepo
	charRepo  *mockCharRepo
	partyRepo *mockPartyRepo
	pID       string
	original  map[string]corecharacter.Character
}

func newSealFixture(ids ...string) *sealFixture {
	f := &sealFixture{
		bossRepo:  newMockBossRepo(),
		charRepo:  &mockCharRepo{chars: map[string]corecharacter.Character{}},
		partyRepo: newMockPartyRepo(),
		pID:       "party-atomic",
		original:  map[string]corecharacter.Character{},
	}
	f.bossRepo.charRepo = f.charRepo
	f.partyRepo.parties[f.pID] = party.Party{
		ID: f.pID, LeaderCharacterID: ids[0], StageID: "king1",
		Status: party.StatusRecruiting, CreatedAt: time.Now().UTC(),
	}
	for i, id := range ids {
		c := createTestChar(id, 50, 600, 300, 200)
		f.charRepo.chars[id] = c
		f.original[id] = c
		f.partyRepo.members[f.pID] = append(f.partyRepo.members[f.pID],
			party.Member{PartyID: f.pID, CharacterID: id, CharacterName: c.Name, ReadyState: true, IsLeader: i == 0})
	}
	return f
}

func (f *sealFixture) service(t *testing.T, repo boss.Repository, pr boss.PartyRepository, opts ...boss.ServiceOption) *boss.Service {
	t.Helper()
	svc, err := boss.NewService(repo, f.charRepo, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Configure(append([]boss.ServiceOption{
		boss.WithPartyRepository(pr),
		boss.WithTransactionProvider(snapshotTx{f}),
	}, opts...)...)
	return svc
}

func (f *sealFixture) requireNothingSettled(t *testing.T) {
	t.Helper()
	for id, want := range f.original {
		if got := f.charRepo.chars[id]; got != want {
			t.Errorf("character %s partially settled: got %+v, want %+v", id, got, want)
		}
		if n := len(f.bossRepo.histories[id]); n != 0 {
			t.Errorf("character %s has %d committed challenge histories", id, n)
		}
		if rec := f.bossRepo.records[id]; rec.TotalBossDefeats != 0 {
			t.Errorf("character %s boss record partially committed: %+v", id, rec)
		}
	}
}

func TestStartSealingBattle_RequiredFailureRollsBackWholeSettlement(t *testing.T) {
	cases := []struct {
		name  string
		build func(f *sealFixture, t *testing.T) *boss.Service
	}{
		{"collection record failure after earlier rewards", func(f *sealFixture, t *testing.T) *boss.Service {
			return f.service(t, f.bossRepo, f.partyRepo, boss.WithMonsterDefeatRecorder(&failingRecorder{failOn: 9}))
		}},
		{"boss record failure for second member", func(f *sealFixture, t *testing.T) *boss.Service {
			return f.service(t, &failingBossRepo{mockBossRepo: f.bossRepo, failOn: 2}, f.partyRepo)
		}},
		{"room completion failure", func(f *sealFixture, t *testing.T) *boss.Service {
			return f.service(t, f.bossRepo, failingDeletePartyRepo{f.partyRepo})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSealFixture("leader", "m1")
			svc := tc.build(f, t)

			_, err := svc.StartSealingBattle(context.Background(), f.pID, "leader")
			if !errors.Is(err, errInjected) {
				t.Fatalf("expected injected failure to propagate, got %v", err)
			}
			f.requireNothingSettled(t)
			if f.partyRepo.deleted[f.pID] {
				t.Error("room must not be completed when settlement fails")
			}
			if got := f.partyRepo.parties[f.pID].Status; got != party.StatusRecruiting {
				t.Errorf("failed settlement must release the room claim, status=%q", got)
			}
		})
	}
}

func TestChallengeBoss_RequiredFailureRollsBackWholeSettlement(t *testing.T) {
	cases := []struct {
		name  string
		build func(f *sealFixture, t *testing.T) *boss.Service
	}{
		{"collection record failure", func(f *sealFixture, t *testing.T) *boss.Service {
			return f.service(t, f.bossRepo, f.partyRepo, boss.WithMonsterDefeatRecorder(&failingRecorder{failOn: 1}))
		}},
		{"boss record failure", func(f *sealFixture, t *testing.T) *boss.Service {
			return f.service(t, &failingBossRepo{mockBossRepo: f.bossRepo, failOn: 1}, f.partyRepo)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSealFixture("solo")
			svc := tc.build(f, t)

			_, err := svc.ChallengeBoss(context.Background(), "solo", "king1")
			if !errors.Is(err, errInjected) {
				t.Fatalf("expected injected failure to propagate, got %v", err)
			}
			f.requireNothingSettled(t)
		})
	}
}

// TestStartSealingBattle_ConcurrentStartSettlesOnce overlaps two starts for the same
// room using the production party repository's lock/state claim.
func TestStartSealingBattle_ConcurrentStartSettlesOnce(t *testing.T) {
	f := newSealFixture("leader", "m1")
	rooms := party.NewValkeyRepository(nil)
	ctx := context.Background()
	if err := rooms.SaveParty(ctx, f.partyRepo.parties[f.pID]); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.partyRepo.members[f.pID] {
		if err := rooms.AddMember(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	started := make(chan struct{})
	release := make(chan struct{})
	engine := stubPartyBattleEngine{resolveFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		close(started)
		<-release
		return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies", Turns: 1, TotalReward: req.VictoryReward}, nil
	}}
	svc, err := boss.NewService(f.bossRepo, f.charRepo, engine)
	if err != nil {
		t.Fatal(err)
	}
	svc.Configure(boss.WithPartyRepository(rooms), boss.WithTransactionProvider(snapshotTx{f}))

	first := make(chan error, 1)
	go func() {
		_, err := svc.StartSealingBattle(ctx, f.pID, "leader")
		first <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first start never reached battle resolution")
	}

	_, err = svc.StartSealingBattle(ctx, f.pID, "leader")
	if !errors.Is(err, boss.ErrPartyNotRecruiting) {
		t.Fatalf("overlapping start must be rejected as not recruiting, got %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	if _, err := svc.StartSealingBattle(ctx, f.pID, "leader"); !errors.Is(err, boss.ErrPartyNotFound) {
		t.Fatalf("settled room must be gone, got %v", err)
	}
	for _, id := range []string{"leader", "m1"} {
		if got := f.charRepo.chars[id].HeroCount; got != 1 {
			t.Errorf("%s HeroCount=%d, want exactly 1", id, got)
		}
		if got := len(f.bossRepo.histories[id]); got != 1 {
			t.Errorf("%s histories=%d, want exactly 1", id, got)
		}
	}
}

// ctxCheckingPartyRepo rejects UpdateParty when the caller's context is already canceled,
// matching a real Valkey client when the HTTP request context is canceled or times out.
type ctxCheckingPartyRepo struct {
	*mockPartyRepo
	hasReleaseDeadline bool
}

func (r *ctxCheckingPartyRepo) UpdateParty(ctx context.Context, p party.Party) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Status == party.StatusRecruiting {
		_, r.hasReleaseDeadline = ctx.Deadline()
	}
	return r.mockPartyRepo.UpdateParty(ctx, p)
}

func TestStartSealingBattle_RequestCancellationReleasesRoomClaim(t *testing.T) {
	f := newSealFixture("leader", "m1")
	pr := &ctxCheckingPartyRepo{mockPartyRepo: f.partyRepo}

	ctx, cancel := context.WithCancel(context.Background())
	engine := stubPartyBattleEngine{resolveFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		cancel()
		return corebattle.PartyBattleResult{}, ctx.Err()
	}}
	svc, err := boss.NewService(f.bossRepo, f.charRepo, engine)
	if err != nil {
		t.Fatal(err)
	}
	svc.Configure(
		boss.WithPartyRepository(pr),
		boss.WithTransactionProvider(snapshotTx{f}),
	)

	_, err = svc.StartSealingBattle(ctx, f.pID, "leader")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	f.requireNothingSettled(t)
	if got := f.partyRepo.parties[f.pID].Status; got != party.StatusRecruiting {
		t.Fatalf("room remained %q after request cancellation; want %q", got, party.StatusRecruiting)
	}
	if !pr.hasReleaseDeadline {
		t.Fatal("expected room release context to carry a timeout deadline")
	}
}

func TestStartSealingBattle_ValkeyDeletePartyPartialFailureRollsBackSettlement(t *testing.T) {
	for _, failCmd := range []string{"DEL", "ZREM"} {
		t.Run(failCmd, func(t *testing.T) {
			f := newSealFixture("leader", "m1")
			state := party.LobbyState{
				Party:   f.partyRepo.parties[f.pID],
				Members: f.partyRepo.members[f.pID],
			}
			lobbyBytes, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}

			delCalls := 0
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				if err := ctx.Err(); err != nil {
					return valkeytest.MakeErrorResult(err)
				}
				args := cmd.Commands()
				switch args[0] {
				case "SET":
					return valkeytest.MakeOKResult()
				case "GET":
					return valkeytest.MakeStringResult(string(lobbyBytes))
				case "EXISTS":
					return valkeytest.MakeIntResult(1)
				case "EVALSHA", "EVAL":
					if len(args) >= 6 {
						var updated party.Party
						if json.Unmarshal([]byte(args[3]), &updated) == nil {
							state.Party = updated
							lobbyBytes, _ = json.Marshal(state)
						}
					}
					return valkeytest.MakeStringResult("OK")
				case "DEL":
					delCalls++
					// Let member DELs succeed, then fail on the lobby key DEL (5th DEL: 2 members * 2 + leader).
					if failCmd == "DEL" && delCalls == 5 {
						return valkeytest.MakeErrorResult(errInjected)
					}
					return valkeytest.MakeIntResult(1)
				case "ZREM":
					if failCmd == "ZREM" {
						return valkeytest.MakeErrorResult(errInjected)
					}
					return valkeytest.MakeIntResult(1)
				default:
					return valkeytest.MakeOKResult()
				}
			}))

			valkeyRepo := party.NewValkeyRepository(client)
			svc := f.service(t, f.bossRepo, valkeyRepo)

			_, err = svc.StartSealingBattle(context.Background(), f.pID, "leader")
			if !errors.Is(err, errInjected) {
				t.Fatalf("expected Valkey %s error to propagate, got %v", failCmd, err)
			}
			f.requireNothingSettled(t)
			if state.Party.Status != party.StatusRecruiting {
				t.Fatalf("expected room status to recover to %q, got %q", party.StatusRecruiting, state.Party.Status)
			}
		})
	}
}

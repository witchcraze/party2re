package challenge_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestAdvanceRound_CompareAndSwap(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "memory"
		if live {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var store challenge.ActiveSessionStore = challenge.NewMemorySessionRepository()
			if live {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				r, err := challenge.NewValkeySessionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
				store = r
			}
			leader, follower := id.New(), id.New()
			run := sampleChallengeSession(id.New(), leader, "1")
			run.PartyID, run.PartyName, run.PartyColor = "party", "Party", "#123456"
			run.AccumulatedExp, run.AccumulatedGold = 10, 20
			run.AccumulatedItems = []string{"existing"}
			run.Members = []challenge.ChallengeMember{
				{CharacterID: leader, CharacterCurrentHP: 200, MaxHP: 200},
				{CharacterID: follower, CharacterCurrentHP: 150, MaxHP: 150},
			}
			if err := store.SaveActiveSession(ctx, run); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveSession(context.Background(), leader); err != nil {
					t.Error(err)
				}
			})
			var applied, conflicts atomic.Int64
			winner := make(chan challenge.ChallengeSession, 1)
			stress := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				members := append([]challenge.ChallengeMember(nil), run.Members...)
				members[0].CharacterCurrentHP = 100 + worker
				members[1].CharacterCurrentHP = worker
				out, err := store.AdvanceRound(ctx, leader, challenge.AdvanceRoundParams{
					ExpectedSessionID: run.ID, ExpectedRound: 1, Members: members,
					SurvivingHP: members[0].CharacterCurrentHP, ExpDelta: 30, GoldDelta: 40,
					RewardItemID: fmt.Sprintf("item-%d", worker), Now: run.UpdatedAt.Add(time.Second),
				})
				if errors.Is(err, challenge.ErrSessionRoundMismatch) {
					conflicts.Add(1)
					return nil
				}
				if err == nil && applied.Add(1) == 1 {
					winner <- out.Session
				}
				return err
			})
			if stress.Failures != 0 || applied.Load() != 1 || conflicts.Load() != stress.TotalOps-1 {
				t.Fatalf("applied=%d conflicts=%d stress=%+v", applied.Load(), conflicts.Load(), stress)
			}
			want := <-winner
			worker := want.CharacterCurrentHP - 100
			if want.CurrentRound != 2 || want.AccumulatedExp != 40 || want.AccumulatedGold != 60 ||
				len(want.AccumulatedItems) != 2 || want.AccumulatedItems[0] != "existing" ||
				want.AccumulatedItems[1] != fmt.Sprintf("item-%d", worker) ||
				want.Members[0].CharacterCurrentHP != want.CharacterCurrentHP || want.Members[1].CharacterCurrentHP != worker ||
				want.PartyID != run.PartyID || want.PartyName != run.PartyName || want.PartyColor != run.PartyColor {
				t.Fatalf("incorrect winning transition: %+v", want)
			}
			for _, actor := range []string{leader, follower} {
				got, err := store.GetActiveSession(ctx, actor)
				if err != nil || got == nil || !reflect.DeepEqual(*got, want) {
					t.Fatalf("persisted snapshot for %s: %+v, want %+v, err=%v", actor, got, want, err)
				}
			}

			// A later successful round must survive both stale and future computations.
			nextMembers := append([]challenge.ChallengeMember(nil), want.Members...)
			nextMembers[0].CharacterCurrentHP = 80
			nextMembers[1].CharacterCurrentHP = 0
			out, err := store.AdvanceRound(ctx, leader, challenge.AdvanceRoundParams{
				ExpectedSessionID: run.ID, ExpectedRound: 2, Members: nextMembers,
				SurvivingHP: 80, ExpDelta: 5, GoldDelta: 6, Now: run.UpdatedAt.Add(2 * time.Second),
			})
			if err != nil {
				t.Fatal(err)
			}
			want = out.Session
			want.Members = append([]challenge.ChallengeMember(nil), out.Session.Members...)
			if want.CurrentRound != 3 || want.AccumulatedExp != 45 || want.AccumulatedGold != 66 || len(want.AccumulatedItems) != 2 {
				t.Fatalf("incorrect second transition: %+v", want)
			}
			for _, expected := range []int{0, 1, 2, 4} {
				_, err := store.AdvanceRound(ctx, leader, challenge.AdvanceRoundParams{
					ExpectedSessionID: run.ID, ExpectedRound: expected, Members: run.Members,
					SurvivingHP: 200, ExpDelta: 999, GoldDelta: 999, RewardItemID: "duplicate", Now: time.Now(),
				})
				if !errors.Is(err, challenge.ErrSessionRoundMismatch) {
					t.Fatalf("expected round %d: %v", expected, err)
				}
			}
			// Inputs and returned snapshots must not alias the memory store.
			nextMembers[0].CharacterCurrentHP = 999
			out.Session.Members[0].CharacterCurrentHP = 999
			got, err := store.GetActiveSession(ctx, follower)
			if err != nil || got == nil || !reflect.DeepEqual(*got, want) {
				t.Fatalf("later snapshot rolled back: %+v, want %+v, err=%v", got, want, err)
			}
		})
	}
}

type synchronizedRoundStore struct {
	challenge.ActiveSessionStore
	reads atomic.Int64
	ready chan struct{}
}

func (s *synchronizedRoundStore) GetActiveSession(ctx context.Context, characterID string) (*challenge.ChallengeSession, error) {
	run, err := s.ActiveSessionStore.GetActiveSession(ctx, characterID)
	if err != nil || run == nil || run.CurrentRound != 1 {
		return run, err
	}
	if s.reads.Add(1) == 2 {
		close(s.ready)
	}
	select {
	case <-s.ready:
		return run, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *synchronizedRoundStore) SaveActiveSession(context.Context, challenge.ChallengeSession) error {
	return errors.New("round advancement must not perform a second save")
}

type roundWinEngine struct{}

func (roundWinEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	actor := req.Participants[0]
	return corebattle.Result{
		Outcome: corebattle.OutcomeWin, WinnerID: actor.ID,
		Logs: []corebattle.TurnLog{{RemainingHP: map[string]int{actor.ID: actor.HP - 10}}},
	}, nil
}

func TestServiceAdvanceRound_ConcurrentSnapshot(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "memory"
		if live {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var store challenge.ActiveSessionStore = challenge.NewMemorySessionRepository()
			if live {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				r, err := challenge.NewValkeySessionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
				store = r
			}
			leader := id.New()
			run := sampleChallengeSession(id.New(), leader, "0")
			run.Members = []challenge.ChallengeMember{{CharacterID: leader, CharacterCurrentHP: 200, MaxHP: 200}}
			if err := store.SaveActiveSession(ctx, run); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveSession(context.Background(), leader); err != nil {
					t.Error(err)
				}
			})
			repo := newMockChallengeRepo()
			// Hall-of-fame settlement is outside the active-step concurrency boundary.
			repo.hof["0"] = challenge.HallOfFameEntry{HighestRound: 100}
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{leader: createPartyTestChar(leader, 30, 200, 100, 100)}}
			barrier := &synchronizedRoundStore{ActiveSessionStore: store, ready: make(chan struct{})}
			svc, err := challenge.NewService(repo, chars, roundWinEngine{}, challenge.WithActiveSessionStore(barrier))
			if err != nil {
				t.Fatal(err)
			}
			advance := func() error {
				_, _, err := svc.AdvanceRound(ctx, leader, run.ID)
				return err
			}
			a, b := testutil.RunRace2(advance, advance)
			if !((a == nil && errors.Is(b, challenge.ErrSessionRoundMismatch)) || (b == nil && errors.Is(a, challenge.ErrSessionRoundMismatch))) {
				t.Fatalf("expected one success and one conflict: %v, %v", a, b)
			}
			got, err := store.GetActiveSession(ctx, leader)
			if err != nil || got == nil || got.CurrentRound != 2 || got.CharacterCurrentHP != 190 ||
				got.Members[0].CharacterCurrentHP != 190 || got.AccumulatedExp != 22 || got.AccumulatedGold != 6 {
				t.Fatalf("incorrect persisted combat state: %+v, err=%v", got, err)
			}
		})
	}
}

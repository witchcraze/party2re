package boss_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/boss"
)

type banquetCommitTx struct {
	snapshotTx
	committed bool
	fail      bool
}

func (tx *banquetCommitTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	err := tx.snapshotTx.RunInTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		if tx.fail {
			return errInjected
		}
		return nil
	})
	tx.committed = err == nil
	return err
}

func TestBossBanquet_CommitAndFailureBoundary(t *testing.T) {
	for _, solo := range []bool{false, true} {
		for _, mode := range []string{"success", "hook-failure", "late-settlement-failure", "commit-failure"} {
			name := "party/"
			if solo {
				name = "solo/"
			}
			t.Run(name+mode, func(t *testing.T) {
				f := newSealFixture("leader", "member")
				f.partyRepo.members[f.pID][0].CharacterName = "stale lobby name"
				tx := &banquetCommitTx{snapshotTx: snapshotTx{f}, fail: mode == "commit-failure"}
				var repo boss.Repository = f.bossRepo
				if mode == "late-settlement-failure" {
					repo = &failingBossRepo{mockBossRepo: f.bossRepo, failOn: 1}
				}
				s := f.service(t, repo, f.partyRepo, boss.WithTransactionProvider(tx))
				calls := 0
				s.SetVictoryBanquetHook(func(ctx context.Context, stageID, stageName, actorID, actorName string, tier int) error {
					calls++
					if !tx.committed || stageID != "king1" || stageName == "" || actorID != "leader" || actorName != f.original["leader"].Name || tier != 1 {
						t.Fatalf("banquet before commit or lost arguments: committed=%v stage=%s actor=%s name=%s tier=%d", tx.committed, stageID, actorID, actorName, tier)
					}
					if mode == "hook-failure" {
						return errInjected
					}
					return nil
				})
				run := func() (boss.SealingBattleResult, error) {
					if solo {
						return s.ChallengeBoss(context.Background(), "leader", "king1")
					}
					return s.StartSealingBattle(context.Background(), f.pID, "leader")
				}
				res, err := run()
				if mode == "late-settlement-failure" || mode == "commit-failure" {
					if !errors.Is(err, errInjected) || calls != 0 || res.BanquetHeld {
						t.Fatalf("rolled-back banquet: err=%v calls=%d held=%v", err, calls, res.BanquetHeld)
					}
					f.requireNothingSettled(t)
					return
				}
				if err != nil || calls != 1 || res.BanquetHeld != (mode == "success") || f.charRepo.chars["leader"].HeroCount != 1 {
					t.Fatalf("committed banquet: err=%v calls=%d held=%v", err, calls, res.BanquetHeld)
				}
				if !solo {
					res, err = run()
					if err != nil || calls != 1 || res.BanquetHeld {
						t.Fatalf("receipt replay appended crowd or claimed uncertain publication: err=%v calls=%d held=%v", err, calls, res.BanquetHeld)
					}
				}
			})
		}
	}
}

func TestUnsealDemonKing_NewsFailureIsBestEffort(t *testing.T) {
	f := newSealFixture("hero")
	s := f.service(t, f.bossRepo, f.partyRepo, boss.WithNewsPublisher(boss.NewsPublisherFunc(failingNews)))
	if err := s.UnsealDemonKing(context.Background(), []string{"hero"}); err != nil {
		t.Fatal(err)
	}
	if got := f.charRepo.chars["hero"].MaoCount; got != f.original["hero"].MaoCount+1 {
		t.Fatalf("news failure lost unseal count: %d", got)
	}
}

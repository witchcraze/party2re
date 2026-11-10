package main

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/medal"
)

func TestAdventureVictoryHook_ProgressErrors(t *testing.T) {
	wantMetrics := []medal.MetricType{medal.MetricAdventureVictories, medal.MetricMonstersSlain, medal.MetricGoldEarned}
	wantAmounts := []int{1, 3, 20}
	injected := errors.New("progress unavailable")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for failAt := 0; failAt <= len(wantMetrics); failAt++ {
		calls := 0
		hook := buildAdventureVictoryHook(func(gotCtx context.Context, actor string, metric medal.MetricType, amount int) error {
			if gotCtx != ctx || actor != "hero" || metric != wantMetrics[calls] || amount != wantAmounts[calls] {
				t.Fatalf("lost progress arguments: actor=%s metric=%s amount=%d", actor, metric, amount)
			}
			calls++
			if calls == failAt {
				return injected
			}
			return nil
		})
		err := hook(ctx, "hero", 3, 20)
		if failAt == 0 {
			if err != nil || calls != 3 {
				t.Fatalf("success: err=%v calls=%d", err, calls)
			}
		} else if !errors.Is(err, injected) || calls != 3 {
			t.Fatalf("failure at %d: err=%v calls=%d", failAt, err, calls)
		}
	}
	for _, amount := range []int{0, -1} {
		var got []medal.MetricType
		hook := buildAdventureVictoryHook(func(_ context.Context, _ string, metric medal.MetricType, _ int) error {
			got = append(got, metric)
			return nil
		})
		if err := hook(ctx, "hero", amount, amount); err != nil || !reflect.DeepEqual(got, wantMetrics[:1]) {
			t.Fatalf("nonpositive optional metrics: got=%v err=%v", got, err)
		}
	}
}

func TestPartyVictoryHook_LaterParticipantFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	injected := errors.New("second participant progress unavailable")
	var actors []string
	hook := buildPartyVictoryHook(func(gotCtx context.Context, actor string, metric medal.MetricType, amount int) error {
		if gotCtx != ctx {
			t.Fatal("lost caller context")
		}
		actors = append(actors, actor)
		if actor == "second" {
			return injected
		}
		return nil
	})
	if err := hook(ctx, []string{"first", "second", "third"}, 2, 5); !errors.Is(err, injected) {
		t.Fatalf("suppressed later participant failure: %v", err)
	}
	if !reflect.DeepEqual(actors, []string{"first", "first", "first", "second", "second", "second", "third", "third", "third"}) {
		t.Fatalf("progress actors = %v", actors)
	}
}

func TestWireBossBanquet_SkipsAmbientTransaction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plaza := &banquetPlazaSpy{}
	hook := buildVictoryBanquetHook(plaza)
	injected := errors.New("outer rollback")
	err = database.RunInTx(context.Background(), db, func(ctx context.Context) error {
		if err := hook(ctx, "king1", "King", "hero", "Hero", 1); err == nil {
			t.Fatal("ambient transaction must not publish a banquet")
		}
		return injected
	})
	if !errors.Is(err, injected) || plaza.calls != 0 {
		t.Fatalf("rolled-back transaction published a banquet: err=%v calls=%d", err, plaza.calls)
	}
}

type banquetPlazaSpy struct{ calls int }

func (p *banquetPlazaSpy) RecordVictoryCrowd(context.Context) error {
	p.calls++
	return nil
}

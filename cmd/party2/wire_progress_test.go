package main

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func TestWireBossBanquet_SkipsAmbientTransaction(t *testing.T) {
	db := database.OpenTestDB(t)
	plaza := &banquetPlazaSpy{}
	hook := buildVictoryBanquetHook(plaza)
	injected := errors.New("outer rollback")
	err := database.RunInTx(context.Background(), db, func(ctx context.Context) error {
		if err := hook(ctx); err == nil {
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

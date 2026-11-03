package main

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type contextSleepReader struct {
	playercontext.TimerReader
	home interface {
		GetRemainingSleep(context.Context, string) (time.Duration, bool, error)
	}
}

func (r contextSleepReader) GetRemainingLock(ctx context.Context, category, actor string) (time.Duration, error) {
	if category != timer.CategorySleep {
		return r.TimerReader.GetRemainingLock(ctx, category, actor)
	}
	remaining, _, err := r.home.GetRemainingSleep(ctx, actor)
	return remaining, err
}

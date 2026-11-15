package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/scheduling"
)

type wiringLotteryRepo struct {
	lottery.Repository
	round lottery.TakarakujiRound
	err   error
	read  chan struct{}
}

func (r wiringLotteryRepo) GetActiveTakarakujiRound(context.Context) (lottery.TakarakujiRound, error) {
	if r.read != nil {
		select {
		case r.read <- struct{}{}:
		default:
		}
	}
	return r.round, r.err
}

func TestWireTakarakujiDrawing(t *testing.T) {
	storageErr := errors.New("SQL unavailable")
	scheduleErr := errors.New("Valkey unavailable")
	for _, tc := range []struct {
		name                 string
		readErr, scheduleErr error
	}{
		{name: "SQL round date"}, {name: "SQL failure", readErr: storageErr}, {name: "schedule failure", scheduleErr: scheduleErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			date := time.Now().Add(-time.Hour).UTC()
			svc, err := lottery.NewService(wiringLotteryRepo{round: lottery.TakarakujiRound{RoundID: 1, DrawDate: date}, err: tc.readErr})
			if err != nil {
				t.Fatal(err)
			}
			queue := &mockSchedRepo{scheduleErr: tc.scheduleErr}
			err = wireTakarakujiDrawing(scheduling.NewService(queue), svc)
			wantErr := tc.readErr
			if wantErr == nil {
				wantErr = tc.scheduleErr
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if wantErr == nil && (len(queue.actions) != 1 || !queue.actions[0].ExecuteAt.Equal(date)) {
				t.Fatalf("actions = %+v", queue.actions)
			}
			if tc.readErr != nil && len(queue.actions) != 0 {
				t.Fatal("scheduled despite unread SQL state")
			}
		})
	}
}

func TestLotteryPeriodicReconciliationWired(t *testing.T) {
	read := make(chan struct{}, 1)
	svc, err := lottery.NewService(wiringLotteryRepo{round: lottery.TakarakujiRound{RoundID: 3, DrawDate: time.Now().Add(time.Hour)}, read: read})
	if err != nil {
		t.Fatal(err)
	}
	queue := &mockSchedRepo{}
	soc := &socServices{sched: scheduling.NewService(queue), worker: scheduling.NewWorker(queue, time.Millisecond, logging.Nop())}
	soc.registerWorkerHandlers(nil, nil, svc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); soc.worker.Run(ctx) }()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("lottery reconciliation not connected")
	}
	cancel()
	<-done
	if len(queue.actions) != 1 {
		t.Fatalf("actions = %+v", queue.actions)
	}
}

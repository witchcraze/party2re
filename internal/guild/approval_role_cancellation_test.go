package guild_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

func TestApprovalRoleWorkerCleanup(t *testing.T) {
	db, err := sql.Open("mysql", "unused:unused@tcp(127.0.0.1:3306)/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := &approvalRoleFixture{ctx: ctx, cancel: cancel, db: db}
	entered, blocked, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	t.Run("return with worker at barrier", func(t *testing.T) {
		f.startWorker(t, "cancelled gate", func() error {
			close(entered)
			err := awaitApprovalRoleSignal(ctx, blocked)
			finished <- err
			return err
		})
		if err := awaitApprovalRoleSignal(ctx, entered); err != nil {
			t.Fatalf("worker start: %v", err)
		}
		// Cleanup must cancel and join the worker even without calling its join function.
	})
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("barrier result = %v, want cancellation", err)
		}
	default:
		t.Fatal("subtest cleanup returned before worker finished")
	}
	closed := make(chan struct{})
	close(closed)
	if err := awaitApprovalRoleSignal(context.Background(), closed); err != nil {
		t.Fatalf("released barrier = %v", err)
	}
}

func TestGuildApprovalRole_Cancellation(t *testing.T) {
	for _, first := range []string{"transfer", "mutation"} {
		for _, stage := range []string{"locked", "contender started"} {
			t.Run(first+"/"+stage, func(t *testing.T) {
				f := newApprovalRoleFixture(t)
				beforeGuild, beforeMembers, err := f.repo.GetGuild(f.ctx, f.g.ID)
				if err != nil {
					t.Fatal(err)
				}
				locked, resume, started := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var once, startedOnce sync.Once
				holder := f.newService(t, &approvalRoleGateRepo{GuildRepository: f.repo, t: t, afterRead: func(ctx context.Context) {
					once.Do(func() {
						close(locked)
						select {
						case <-resume:
						case <-ctx.Done():
						}
					})
				}})
				contender := f.newService(t, &approvalRoleGateRepo{GuildRepository: f.repo, t: t, beforeLocked: func(context.Context) {
					startedOnce.Do(func() { close(started) })
				}})
				transfer := func(s *guild.Service) error {
					return s.TransferLeadership(f.ctx, f.g.ID, f.leader.ID, f.active.ID)
				}
				mutation := func(s *guild.Service) error {
					return f.executeOp(f.ctx, s, "approvePending", "CancelTitle")
				}
				firstOp, secondOp := transfer, mutation
				if first == "mutation" {
					firstOp, secondOp = mutation, transfer
				}
				joins := []func() error{f.startWorker(t, "holder", func() error { return firstOp(holder) })}
				if err := awaitApprovalRoleSignal(f.ctx, locked); err != nil {
					t.Fatalf("holder gate: %v", err)
				}
				if stage == "contender started" {
					joins = append(joins, f.startWorker(t, "contender", func() error { return secondOp(contender) }))
					if err := awaitApprovalRoleSignal(f.ctx, started); err != nil {
						t.Fatalf("contender gate: %v", err)
					}
				}
				f.cancel() // Keep resume blocked: cancellation alone must release the gate.
				for _, join := range joins {
					// MySQL may close a connection or roll back its transaction during cancellation.
					if err := join(); !errors.Is(err, context.Canceled) && !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, sql.ErrTxDone) {
						t.Errorf("worker result = %v, want cancellation/closed connection/rolled-back transaction", err)
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				afterGuild, afterMembers, err := f.repo.GetGuild(ctx, f.g.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(beforeGuild, afterGuild) || !reflect.DeepEqual(beforeMembers, afterMembers) {
					t.Fatalf("cancelled transaction changed guild or roster: %+v / %+v", afterGuild, afterMembers)
				}
				var letters int
				if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_letters WHERE recipient_character_id IN (?, ?)", f.active.ID, f.pending.ID).Scan(&letters); err != nil {
					t.Fatal(err)
				}
				if letters != 0 {
					t.Fatalf("cancelled operations sent %d letters", letters)
				}
				if err := f.service.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID); err != nil {
					t.Fatalf("transfer after cancellation: %v", err)
				}
			})
		}
	}
}

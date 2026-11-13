package guild_test

import (
	"bufio"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

func TestApprovalRoleWorkerCleanup_StopTimeout(t *testing.T) {
	t.Parallel()
	const diagnostic = "blocked worker did not stop after cancellation"
	const cleaned = "fixture cleanup after worker stopped"
	if os.Getenv("PARTY2_APPROVAL_WORKER_SUBPROCESS") == "1" {
		db, err := sql.Open("mysql", "unused:unused@tcp(127.0.0.1:3306)/unused")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		f := &approvalRoleFixture{ctx: ctx, cancel: cancel, db: db, stopTimeout: 250 * time.Millisecond}
		finished := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-finished:
				fmt.Println(cleaned)
			default:
				t.Error("fixture cleanup ran before worker stopped")
			}
		})
		f.startWorker(t, "blocked", func() error {
			defer close(finished)
			_, err := bufio.NewReader(os.Stdin).ReadString('\n')
			return err
		})
		return // Run the actual registered cancellation/join cleanup.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestApprovalRoleWorkerCleanup_StopTimeout$", "-test.v", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "PARTY2_APPROVAL_WORKER_SUBPROCESS=1")
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var transcript strings.Builder
	var reported, teardown bool
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(&transcript, line)
		if strings.Contains(line, diagnostic) && !reported {
			reported = true
			if _, err := fmt.Fprintln(input, "release"); err != nil {
				t.Errorf("release worker: %v", err)
			}
		}
		if line == cleaned {
			teardown = reported
		}
	}
	err = cmd.Wait()
	if scanner.Err() != nil || ctx.Err() != nil || !reported || !teardown || strings.Contains(transcript.String(), "panic:") {
		t.Fatalf("worker cleanup ordering failed (scan=%v, context=%v):\n%s", scanner.Err(), ctx.Err(), transcript.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected timeout diagnostic to fail child test with exit 1, got %v:\n%s", err, transcript.String())
	}
}

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

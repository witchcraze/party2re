package challenge_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
)

func TestRunProtocolSQLRollbackIntegration(t *testing.T) {
	for _, table := range []string{"characters", "tavern_character_status"} {
		t.Run(table, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			ctx := context.Background()
			run := f.advance(t, f.start(t))
			actor := f.actors[0].ID
			trigger := fmt.Sprintf("CREATE TRIGGER reject_exit BEFORE UPDATE ON %s FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='exit rollback'", table)
			if _, err := f.db.ExecContext(ctx, trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision); err == nil || !strings.Contains(err.Error(), "exit rollback") {
				t.Fatalf("SQL failure: %v", err)
			}
			got, err := f.store.ReadRun(ctx, run.ID)
			if err != nil || !reflect.DeepEqual(got, run) {
				t.Fatalf("rollback changed live state: %+v %v", got, err)
			}
			for _, original := range f.actors {
				got, err := f.chars.FindByID(ctx, original.ID)
				if err != nil || !reflect.DeepEqual(got, original) {
					t.Fatalf("rollback changed character: %+v %v", got, err)
				}
			}
			if result, err := f.repo.FindCommandResult(ctx, run.ID, actor, run.Revision); err != nil || result != nil {
				t.Fatalf("rolled-back receipt: %+v %v", result, err)
			}
			if _, err := f.db.ExecContext(ctx, "DROP TRIGGER reject_exit"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision); err != nil {
				t.Fatal(err)
			}
			f.assertSettled(t, 0, 1, 1)
		})
	}
}

func TestRunProtocolPublicationBoundariesIntegration(t *testing.T) {
	for _, boundary := range []string{"canonical", "rewards", "remaining reference", "acknowledgement"} {
		t.Run(boundary, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			ctx := context.Background()
			run := f.advance(t, f.start(t))
			actor := f.actors[0].ID
			key := ""
			switch boundary {
			case "canonical":
				key = fmt.Sprintf("party2:challenge:{run:%s}:session", run.ID)
			case "rewards":
				key = fmt.Sprintf("party2:challenge:{run:%s}:rewards", run.ID)
			case "remaining reference":
				key = fmt.Sprintf("party2:challenge:{char:%s}:member", f.actors[1].ID)
			case "acknowledgement":
				if _, err := f.db.ExecContext(ctx, `CREATE TRIGGER reject_ack BEFORE UPDATE ON challenge_run_transitions FOR EACH ROW BEGIN IF OLD.pending=TRUE AND NEW.pending=FALSE THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='ack failed'; END IF; END`); err != nil {
					t.Fatal(err)
				}
			}
			// Do not corrupt the live input before it is accepted: a wrapper injects
			// the real storage failure only when SQL already holds its publication intent.
			fault := &beforePublicationStore{ActiveSessionStore: f.store, before: func() {
				if key == "" {
					return
				}
				if err := f.client.Do(ctx, f.client.B().Del().Key(key).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				if boundary == "remaining reference" {
					if err := f.client.Do(ctx, f.client.B().Hset().Key(key).FieldValue().FieldValue("bad", "type").Build()).Error(); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := f.client.Do(ctx, f.client.B().Set().Key(key).Value("bad type").Build()).Error(); err != nil {
						t.Fatal(err)
					}
				}
			}}
			f.svc = f.service(t, fault, f.repo)
			out, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
			if out == nil || !errors.Is(err, challenge.ErrRecoveryRequired) {
				t.Fatalf("expected pending: %+v %v", out, err)
			}
			f.assertSettled(t, 0, 1, 1)
			if key != "" {
				if err := f.client.Do(ctx, f.client.B().Del().Key(key).Build()).Error(); err != nil {
					t.Fatal(err)
				}
			}
			if boundary == "acknowledgement" {
				if _, err := f.db.ExecContext(ctx, "DROP TRIGGER reject_ack"); err != nil {
					t.Fatal(err)
				}
			}
			fault.before = nil
			retry, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
			if err != nil || !reflect.DeepEqual(out, retry) {
				t.Fatalf("retry: %+v %v", retry, err)
			}
			f.assertSettled(t, 0, 1, 1)
			if _, err := f.svc.GetCommandResult(ctx, f.actors[1].ID, run.ID, run.Revision); !errors.Is(err, challenge.ErrForbidden) {
				t.Fatalf("foreign receipt exposed: %v", err)
			}
			if _, _, err := f.svc.AdvanceRound(ctx, actor, run.ID, run.Revision); !errors.Is(err, challenge.ErrRevisionConflict) {
				t.Fatalf("different input reused receipt: %v", err)
			}
		})
	}
}

type beforePublicationStore struct {
	challenge.ActiveSessionStore
	before func()
}

func (s *beforePublicationStore) PublishRun(ctx context.Context, t challenge.Transition) error {
	if s.before != nil {
		s.before()
	}
	return s.ActiveSessionStore.PublishRun(ctx, t)
}

func (f *protocolFixture) service(t *testing.T, store challenge.ActiveSessionStore, transitions challenge.TransitionRepository) *challenge.Service {
	t.Helper()
	svc, err := challenge.NewService(f.repo, f.chars, f.engine, challenge.WithActiveSessionStore(store), challenge.WithTransitionRepository(transitions), challenge.WithAdmission(approvedRoster{}), challenge.WithParticipantBuilder(f.adapter), challenge.WithFullnessResetter(f.fullness), challenge.WithRNG(zeroRNG{}), challenge.WithCustomTiers(map[string]challenge.ChallengeTier{"test": {ID: "test", MaxParticipants: 4, BaseMonster: challenge.ChallengeMonster{Name: "Test", BaseHP: 1, BaseExp: 20, BaseGold: 5}}}))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

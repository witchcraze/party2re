package challenge_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
)

func TestRunProtocolLegacyMigrationIntegration(t *testing.T) {
	for _, mode := range []string{"success", "partial publication", "missing rewards"} {
		t.Run(mode, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			ctx := context.Background()
			now := time.Now().UTC()
			run := sampleChallengeSession(id.New(), f.actors[0].ID, "test")
			run.CurrentRound = 3
			run.AccumulatedExp = 46
			run.AccumulatedGold = 12
			run.AccumulatedItems = []string{"item-001"}
			for _, c := range f.actors {
				mp := 36
				run.Members = append(run.Members, challenge.ChallengeMember{CharacterID: c.ID, CharacterName: c.Name, CharacterCurrentHP: 180, CharacterCurrentMP: &mp, MaxHP: 200, MaxMP: 40, AccumulatedExp: 46, AccumulatedGold: 12, ConsumedItems: []corebattle.ConsumedItem{{ID: "item-193", Quantity: 1}}})
			}
			if err := f.repo.SaveSession(ctx, run); err != nil {
				t.Fatal(err)
			}
			legacy, err := challenge.NewValkeySessionRepository(f.client, challenge.WithSessionTTL(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if err := legacy.SaveActiveSession(ctx, run); err != nil {
				t.Fatal(err)
			}
			oldkey := fmt.Sprintf("party2:challenge:{char:%s}:session", run.CharacterID)
			deadline, err := f.client.Do(ctx, f.client.B().Pexpiretime().Key(oldkey).Build()).AsInt64()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing rewards" {
				key := fmt.Sprintf("party2:challenge:{char:%s}:rewards", run.CharacterID)
				if err := f.client.Do(ctx, f.client.B().Del().Key(key).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				if err := f.svc.MigrateSession(ctx, run.CharacterID, run.ID); !errors.Is(err, challenge.ErrRecoveryRequired) {
					t.Fatalf("missing ledger invented: %v", err)
				}
				view, err := f.svc.GetActiveSession(ctx, f.actors[1].ID)
				if err != nil || view == nil || !view.RecoveryPending {
					t.Fatalf("old unresolved enrollment hidden: %+v %v", view, err)
				}
				return
			}
			ref := fmt.Sprintf("party2:challenge:{char:%s}:member", f.actors[1].ID)
			if mode == "partial publication" {
				fault := &beforePublicationStore{ActiveSessionStore: f.store, before: func() {
					if err := f.client.Do(ctx, f.client.B().Del().Key(ref).Build()).Error(); err != nil {
						t.Fatal(err)
					}
					if err := f.client.Do(ctx, f.client.B().Hset().Key(ref).FieldValue().FieldValue("bad", "type").Build()).Error(); err != nil {
						t.Fatal(err)
					}
				}}
				f.svc = f.service(t, fault, f.repo)
				if err := f.svc.MigrateSession(ctx, run.CharacterID, run.ID); err == nil {
					t.Fatal("expected partial publication")
				}
				if _, err := legacy.AdvanceRound(ctx, run.CharacterID, challenge.AdvanceRoundParams{ExpectedSessionID: run.ID, ExpectedRound: run.CurrentRound}); !errors.Is(err, challenge.ErrSessionNotActive) {
					t.Fatalf("old writer bypassed migration freeze: %v", err)
				}
				tr, err := f.repo.FindTransition(ctx, run.ID)
				if err != nil || !tr.Pending {
					t.Fatalf("migration intent missing: %+v %v", tr, err)
				}
				if err := f.client.Do(ctx, f.client.B().Del().Key(ref).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				fault.before = nil
			}
			if err := f.svc.MigrateSession(ctx, run.CharacterID, run.ID); err != nil {
				t.Fatal(err)
			}
			for _, c := range f.actors {
				got, err := f.svc.GetActiveSession(ctx, c.ID)
				if err != nil || got == nil || got.Revision != 1 || !reflect.DeepEqual(got.Members, run.Members) || !reflect.DeepEqual(got.AccumulatedItems, run.AccumulatedItems) {
					t.Fatalf("migration changed snapshot: %+v %v", got, err)
				}
			}
			newkey := fmt.Sprintf("party2:challenge:{run:%s}:session", run.ID)
			gotDeadline, err := f.client.Do(ctx, f.client.B().Pexpiretime().Key(newkey).Build()).AsInt64()
			if err != nil || gotDeadline != deadline {
				t.Fatalf("migration/GET renewed TTL: %d != %d (%v elapsed=%v)", gotDeadline, deadline, err, time.Since(now))
			}
			oldExists, err := f.client.Do(ctx, f.client.B().Exists().Key(oldkey).Build()).AsInt64()
			if err != nil || oldExists != 0 {
				t.Fatalf("old buffer retained: %d %v", oldExists, err)
			}
			// Apply the actual cutover triggers to this isolated DB and try old SQL.
			migration, err := os.ReadFile("../../migrations/107_challenge_run_transitions.sql")
			if err != nil {
				t.Fatal(err)
			}
			triggers := strings.Split(strings.Split(string(migration), "DELIMITER $$\n")[1], "DELIMITER ;")[0]
			for _, statement := range strings.Split(triggers, "$$") {
				if strings.TrimSpace(statement) != "" {
					if _, err := f.db.ExecContext(ctx, statement); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := f.db.ExecContext(ctx, `UPDATE challenge_sessions SET status='claimed' WHERE id=?`, run.ID); err == nil {
				t.Fatal("old finalizer bypassed run guard")
			}
			if _, err := f.db.ExecContext(ctx, `INSERT INTO challenge_sessions (id,character_id,tier_id,accumulated_items_json) VALUES (?,?,?,'[]')`, id.New(), run.CharacterID, "test"); err == nil {
				t.Fatal("old start bypassed run guard")
			}
			first, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, 1)
			if err != nil {
				t.Fatal(err)
			}
			last, err := f.svc.ExitSession(ctx, f.actors[1].ID, run.ID, first.Session.Revision)
			if err != nil || last.Session.Status != challenge.StatusClaimed {
				t.Fatalf("last exit: %+v %v", last, err)
			}
			for i, c := range f.actors {
				var count int
				if err := f.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM inventory_items WHERE character_id=? AND definition_id='item-001')+(SELECT COUNT(*) FROM depot_items WHERE character_id=? AND definition_id='item-001')`, c.ID, c.ID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				want := 0
				if i == 0 {
					want = 1
				}
				if count != want {
					t.Fatalf("treasure recipient changed or award cloned: actor=%d count=%d", i, count)
				}
			}
		})
	}
}

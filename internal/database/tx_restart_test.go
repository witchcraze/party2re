package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestDeletionRestartOwnsCompleteTransaction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := errors.New("unrelated failure")
	for _, tc := range []struct {
		name     string
		failures int
		err      error
		cancel   bool
		attempts int
		want     error
	}{
		{"restart", 2, errDeletionRowsChanged, false, 3, nil},
		{"exhausted", 3, errDeletionRowsChanged, false, 3, errDeletionRowsChanged},
		{"unrelated", 1, unrelated, false, 1, unrelated},
		{"canceled", 1, errDeletionRowsChanged, true, 1, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, err := CreateTestCharacterWithFunds(ctx, db, "Restart", 1000)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := chars.Delete(context.Background(), c.ID); err != nil {
					t.Error(err)
				}
				players, err := NewPlayerRepository(db)
				if err != nil {
					t.Error(err)
					return
				}
				if err := players.Delete(context.Background(), c.PlayerID); err != nil {
					t.Error(err)
				}
			})
			outer, inner := 0, 0
			err = RunInTx(ctx, db, func(txCtx context.Context) error {
				outer++
				// The write before the nested callback must roll back on every failed attempt.
				if _, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
					`UPDATE characters SET money = money - 1 WHERE id = ?`, c.ID); err != nil {
					return err
				}
				return RunInTx(txCtx, db, func(nested context.Context) error {
					inner++
					if TxFromContext(nested) != TxFromContext(txCtx) {
						return errors.New("nested callback lost ambient transaction")
					}
					if tc.cancel {
						cancel()
					}
					if outer <= tc.failures {
						return fmt.Errorf("deletion: %w", tc.err)
					}
					return nil
				})
			})
			if !errors.Is(err, tc.want) || outer != tc.attempts || inner != outer {
				t.Fatalf("result=%v outer=%d inner=%d; want %v, %d complete attempts", err, outer, inner, tc.want, tc.attempts)
			}
			reloaded, err := chars.FindByID(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantMoney := c.Money
			if tc.want == nil {
				wantMoney--
			}
			if reloaded.Money != wantMoney {
				t.Fatalf("wallet=%d, want %d after rollback/commit", reloaded.Money, wantMoney)
			}
		})
	}
}

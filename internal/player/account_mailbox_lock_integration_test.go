package player_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

type crossedMailboxRepo struct {
	*database.CharacterRepository
	firstA, firstB     string
	deletedA, deletedB chan struct{}
	release            chan struct{}
}

func (r *crossedMailboxRepo) Delete(ctx context.Context, characterID string) error {
	if err := r.CharacterRepository.Delete(ctx, characterID); err != nil {
		return err
	}
	if characterID == r.firstA {
		close(r.deletedA)
		return waitMarketDeletion(ctx, r.release)
	}
	if characterID == r.firstB {
		close(r.deletedB)
	}
	return nil
}

func TestAccountDeletionCrossedMailboxLocks(t *testing.T) {
	monitor := lockMonitor(t)
	for round := range testutil.GetStressConfig().OpsPerWorker {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			f := newDeletionFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			ownerA, ownerB := f.player(t), f.player(t)
			a := []corecharacter.Character{f.character(t, ownerA.ID), f.character(t, ownerA.ID)}
			b := []corecharacter.Character{f.character(t, ownerB.ID), f.character(t, ownerB.ID)}
			if a[0].ID > a[1].ID {
				a[0], a[1] = a[1], a[0]
			}
			if b[0].ID > b[1].ID {
				b[0], b[1] = b[1], b[0]
			}
			homes, err := database.NewHomeRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			letterIDs := []string{id.New(), id.New()}
			if letterIDs[0] > letterIDs[1] {
				letterIDs[0], letterIDs[1] = letterIDs[1], letterIDs[0]
			}
			for i, from := range a {
				to := b[1-i]
				if err := homes.CreateLetter(ctx, home.Letter{ID: letterIDs[i],
					SenderCharacterID: from.ID, SenderName: from.Name,
					RecipientCharacterID: to.ID, RecipientName: to.Name,
					Content: "crossed", Color: "#123456", CreatedAt: time.Now().UTC()}, 100); err != nil {
					t.Fatal(err)
				}
			}
			r := &crossedMailboxRepo{CharacterRepository: f.chars.CharacterRepository,
				firstA: a[0].ID, firstB: b[0].ID, deletedA: make(chan struct{}),
				deletedB: make(chan struct{}), release: make(chan struct{})}
			threadB := make(chan int64, 1)
			tx := database.NewTransactionProvider(f.db)
			chars, err := character.NewService(r, character.WithTransactionProvider(tx),
				character.WithCleanupHook(character.CleanupHookFunc(func(ctx context.Context, ids ...string) error {
					if err := f.guilds.RemoveFromGuilds(ctx, ids...); err != nil {
						return err
					}
					if ids[0] == b[0].ID {
						var thread int64
						if err := database.ExecutorFromContext(ctx, f.db).QueryRowContext(ctx, `SELECT CONNECTION_ID()`).Scan(&thread); err != nil {
							return err
						}
						threadB <- thread
					}
					return nil
				})))
			if err != nil {
				t.Fatal(err)
			}
			accounts, err := player.NewService(f.players, player.NewValkeySessionRepository(nil),
				player.WithCharacterService(chars), player.WithTransactionProvider(tx))
			if err != nil {
				t.Fatal(err)
			}
			// A retains its first character's locks. B either waits on the complete
			// mailbox set, or (before the fix) holds L2 and waits on L1 after B1.
			// Release A only after observing that wait, forcing the old L1/L2 cycle.
			errs := testutil.RunRace(func() error {
				return accounts.DeleteAccount(ctx, ownerA.ID, "")
			}, func() error {
				if err := waitMarketDeletion(ctx, r.deletedA); err != nil {
					return err
				}
				return accounts.DeleteAccount(ctx, ownerB.ID, "")
			}, func() error {
				defer close(r.release)
				var thread int64
				select {
				case thread = <-threadB:
				case <-ctx.Done():
					return ctx.Err()
				}
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for {
					var query sql.NullString
					if err := monitor.QueryRowContext(ctx, `SELECT INFO FROM information_schema.PROCESSLIST WHERE ID = ?`, thread).Scan(&query); err != nil {
						return err
					}
					if strings.Contains(query.String, "FROM character_letters WHERE id = ? FOR UPDATE") {
						return nil
					}
					select {
					case <-r.deletedB:
						if strings.Contains(query.String, "UPDATE character_letters") {
							return nil
						}
					default:
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
					}
				}
			})
			for i, err := range errs {
				if err != nil {
					t.Fatalf("worker %d: %v", i, err)
				}
			}
			for _, owner := range []string{ownerA.ID, ownerB.ID} {
				if f.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, owner) != 0 ||
					f.count(t, `SELECT COUNT(*) FROM characters WHERE player_id = ?`, owner) != 0 {
					t.Fatalf("account %s survived", owner)
				}
			}
			for _, letterID := range letterIDs {
				if _, err := homes.GetLetterByID(ctx, letterID); !errors.Is(err, home.ErrLetterNotFound) {
					t.Fatalf("mutually unowned letter survived: %v", err)
				}
			}
		})
	}
}

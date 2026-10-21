package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type guildDecayFixture struct {
	db       *sql.DB
	guilds   *database.GuildRepository
	gvg      *database.GvGRepository
	guildIDs []string
}

func newGuildDecayFixture(t *testing.T, points ...int64) guildDecayFixture {
	t.Helper()
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "guilds", "guild_members", "gvg_standings", "gvg_match_transitions")
	guilds, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	standings, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := guildDecayFixture{db: db, guilds: guilds, gvg: standings}
	ctx := context.Background()
	for i := range points {
		g, _, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("Decay%d_%s", i, id.New()[:8]), 10000)
		if err != nil {
			t.Fatal(err)
		}
		f.guildIDs = append(f.guildIDs, g.ID)
	}
	slices.Sort(f.guildIDs)
	for i, guildID := range f.guildIDs {
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET points = ?, updated_at = '2000-01-01' WHERE id = ?", points[i], guildID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f guildDecayFixture) points(t *testing.T) []int64 {
	t.Helper()
	points := make([]int64, len(f.guildIDs))
	for i, guildID := range f.guildIDs {
		if err := f.db.QueryRow("SELECT points FROM guilds WHERE id = ?", guildID).Scan(&points[i]); err != nil {
			t.Fatal(err)
		}
	}
	return points
}

func (f guildDecayFixture) transition(draw bool) gvg.RoundTransition {
	a, b := f.guildIDs[0], f.guildIDs[1]
	roomID := id.New()
	transition := gvg.RoundTransition{
		Detail: gvg.RoomDetail{Room: gvg.GvGRoom{ID: roomID, Round: 1, Status: gvg.StatusCompleted}},
		Result: gvg.RoundResolution{Round: 1, WinnerGuildID: a, OverallWinnerGuildID: a, MatchCompleted: true},
		Settlement: &gvg.MatchSettlement{
			WinnerGuildID: a, WinnerPrizeGP: 3,
			GuildIDs: []string{b, a}, ParticipantGP: map[string]int{a: 4, b: 4},
		},
	}
	if draw {
		transition.Detail.Room.Round, transition.Result.Round = gvg.MaxRounds, gvg.MaxRounds
		transition.Result.WinnerGuildID, transition.Result.OverallWinnerGuildID = "", ""
		transition.Settlement.WinnerGuildID = ""
		transition.Settlement.ParticipantGP = nil
		transition.Settlement.IsDraw = true
	}
	return transition
}

func (f guildDecayFixture) assertSettlement(t *testing.T, transitions ...gvg.RoundTransition) {
	t.Helper()
	for _, transition := range transitions {
		got, found, err := f.gvg.FindRoundTransition(context.Background(), transition.Detail.Room.ID, 1)
		if err != nil || !found || !reflect.DeepEqual(got, transition) {
			t.Fatalf("saved transition: got=%+v found=%v err=%v want=%+v", got, found, err, transition)
		}
	}
	var claims int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM gvg_match_transitions").Scan(&claims); err != nil || claims != len(transitions) {
		t.Fatalf("claims=%d want=%d err=%v", claims, len(transitions), err)
	}
	for i, guildID := range f.guildIDs {
		var gp int64
		var wins, losses, draws, bronze, otherMedals int
		err := f.db.QueryRow(`SELECT victory_points, wins, losses, draws, bronze_medals,
			silver_medals + gold_medals + orders + trophies + championship_cups + champion_cups
			FROM gvg_standings WHERE guild_id = ?`, guildID).Scan(&gp, &wins, &losses, &draws, &bronze, &otherMedals)
		wantGP, wantWins, wantLosses, wantBronze := int64(7), 1, 0, 1
		if i == 1 {
			wantGP, wantWins, wantLosses, wantBronze = 4, 0, 1, 0
		}
		if err != nil || gp != wantGP || wins != wantWins || losses != wantLosses || draws != len(transitions)-1 || bronze != wantBronze || otherMedals != 0 {
			t.Fatalf("guild %s: GP=%d W/L/D=%d/%d/%d bronze=%d other=%d err=%v", guildID, gp, wins, losses, draws, bronze, otherMedals, err)
		}
	}
}

func TestGuildRepository_DecayGuildPoints(t *testing.T) {
	f := newGuildDecayFixture(t, 100, 9, 1, 0)
	ctx := context.Background()
	rollback := errors.New("outer rollback")
	if err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
		if err := f.guilds.DecayGuildPoints(txCtx, .8); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := f.points(t); !slices.Equal(got, []int64{100, 9, 1, 0}) {
		t.Fatalf("decay survived outer rollback: %v", got)
	}
	for _, want := range [][]int64{{80, 7, 0, 0}, {64, 5, 0, 0}} {
		if err := f.guilds.DecayGuildPoints(ctx, .8); err != nil {
			t.Fatal(err)
		}
		if got := f.points(t); !slices.Equal(got, want) {
			t.Fatalf("decayed GP=%v want=%v", got, want)
		}
	}
	var updated time.Time
	if err := f.db.QueryRow("SELECT updated_at FROM guilds WHERE id = ?", f.guildIDs[3]).Scan(&updated); err != nil || updated.Year() != 2000 {
		t.Fatalf("zero GP guild changed: updated=%v err=%v", updated, err)
	}
}

// Observe a real InnoDB wait instead of guessing when the contender reached its lock.
func waitForGuildDecayLock(ctx context.Context, db *sql.DB, waiting, blocking int64) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.INNODB_LOCK_WAITS w
			JOIN information_schema.INNODB_TRX r ON r.trx_id = w.requesting_trx_id
			JOIN information_schema.INNODB_TRX b ON b.trx_id = w.blocking_trx_id
			WHERE r.trx_mysql_thread_id = ? AND b.trx_mysql_thread_id = ?`, waiting, blocking).Scan(&count)
		if err != nil || count > 0 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestGuildDecayGvGControlledInterleaving(t *testing.T) {
	f := newGuildDecayFixture(t, 100, 200) // Points index visits B before A.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transition := f.transition(false)
	started := make(chan int64, 1)
	done := make(chan error, 1)
	decayStarted := false
	err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
		tx := database.TxFromContext(txCtx)
		// Reproduce the existing GvG prefix: room claim, then the first guild.
		if _, err := tx.ExecContext(txCtx, "INSERT INTO gvg_match_transitions (room_id) VALUES (?)", transition.Detail.Room.ID); err != nil {
			return err
		}
		var locked string
		if err := tx.QueryRowContext(txCtx, "SELECT id FROM guilds WHERE id = ? FOR UPDATE", f.guildIDs[0]).Scan(&locked); err != nil {
			return err
		}
		var blocking int64
		if err := tx.QueryRowContext(txCtx, "SELECT CONNECTION_ID()").Scan(&blocking); err != nil {
			return err
		}
		decayStarted = true
		go func() {
			done <- database.RunInTx(ctx, f.db, func(decayCtx context.Context) error {
				var waiting int64
				err := database.TxFromContext(decayCtx).QueryRowContext(decayCtx, "SELECT CONNECTION_ID()").Scan(&waiting)
				started <- waiting
				if err != nil {
					return err
				}
				return f.guilds.DecayGuildPoints(decayCtx, .8)
			})
		}()
		select {
		case waiting := <-started:
			if err := waitForGuildDecayLock(txCtx, f.db, waiting, blocking); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		_, err := f.gvg.CommitRoundTransition(txCtx, transition.Detail.Room.ID, 1, transition)
		return err
	})
	if decayStarted {
		if decayErr := <-done; decayErr != nil {
			t.Errorf("decay failed: %v", decayErr)
		}
	}
	if err != nil {
		t.Fatalf("settlement failed: %v", err)
	}
	if got := f.points(t); !slices.Equal(got, []int64{85, 163}) {
		t.Fatalf("settlement then decay GP=%v want=[85 163]", got)
	}
	f.assertSettlement(t, transition)
}

func TestGuildDecayGvGSerialControls(t *testing.T) {
	for _, decayFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("decay_first=%v", decayFirst), func(t *testing.T) {
			f := newGuildDecayFixture(t, 100, 200)
			ctx := context.Background()
			transition := f.transition(false)
			if decayFirst {
				if err := f.guilds.DecayGuildPoints(ctx, .8); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.gvg.CommitRoundTransition(ctx, transition.Detail.Room.ID, 1, transition); err != nil {
				t.Fatal(err)
			}
			want := []int64{87, 164}
			if !decayFirst {
				if err := f.guilds.DecayGuildPoints(ctx, .8); err != nil {
					t.Fatal(err)
				}
				want = []int64{85, 163}
			}
			if got := f.points(t); !slices.Equal(got, want) {
				t.Fatalf("serial GP=%v want=%v", got, want)
			}
			f.assertSettlement(t, transition)
		})
	}
}

func TestGuildDecayGvGConcurrentStress(t *testing.T) {
	f := newGuildDecayFixture(t, 100, 200)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	win, draw := f.transition(false), f.transition(true)
	cfg := testutil.GetStressConfig()
	stress := testutil.RunConcurrentStressTest(t, cfg, func(worker, _ int) error {
		if worker == 0 {
			return f.guilds.DecayGuildPoints(ctx, .8)
		}
		transition := win
		if worker%2 == 0 {
			transition = draw
		}
		_, err := f.gvg.CommitRoundTransition(ctx, transition.Detail.Room.ID, transition.Result.Round, transition)
		return err
	})
	if stress.Successes != stress.TotalOps {
		t.Fatalf("decay/settlement stress: %+v", stress)
	}
	got := f.points(t)
	// The single winning claim can occur before, between or after the decays.
	serial := false
	for before := 0; before <= cfg.OpsPerWorker; before++ {
		a, b := int64(100), int64(200)
		for i := 0; i <= cfg.OpsPerWorker; i++ {
			if i == before {
				a, b = a+7, b+4
			}
			if i < cfg.OpsPerWorker {
				a, b = a*4/5, b*4/5
			}
		}
		serial = serial || slices.Equal(got, []int64{a, b})
	}
	if !serial {
		t.Fatalf("GP=%v does not match any serial order of one win and %d decays", got, cfg.OpsPerWorker)
	}
	f.assertSettlement(t, win, draw)
}

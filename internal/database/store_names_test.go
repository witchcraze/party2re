package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

type storeNameFixture struct {
	ctx       context.Context
	db        *sql.DB
	repo      *database.StoreRepository
	chars     *database.CharacterRepository
	svc       *store.Service
	now       time.Time
	seedCount int
}

func newStoreNameFixture(t *testing.T) *storeNameFixture {
	t.Helper()
	// Construction counts the whole town, requiring a dedicated database.
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "guilds", "guild_members", "character_stores", "store_town_capacity_lock", "store_name_lock")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	repo, err := database.NewStoreRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	gp, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	svc := store.NewService(repo, chars, nil, nil, database.NewTransactionProvider(db), store.WithGuildPoints(gp), store.WithNowFunc(func() time.Time { return now }))
	return &storeNameFixture{ctx: ctx, db: db, repo: repo, chars: chars, svc: svc, now: now}
}

func (f *storeNameFixture) seed(t *testing.T, name, town string, active bool) store.Store {
	t.Helper()
	_, c, err := database.CreateTestGuildWithLeader(f.ctx, f.db, "SN"+id.New()[:8], 2*store.StorePrice)
	if err != nil {
		t.Fatal(err)
	}
	expires := f.now
	if active {
		expires = f.now.Add(time.Hour)
	}
	f.seedCount++
	s := store.Store{ID: fmt.Sprintf("store-%d", f.seedCount), CharacterID: c.ID, StoreName: name, TownID: town, HouseStyle: "001", Wallpaper: "farm", ExpiresAt: expires, CreatedAt: f.now, UpdatedAt: f.now}
	if err := f.repo.SaveStore(f.ctx, s); err != nil {
		t.Fatal(err)
	}
	return f.load(t, c.ID)
}

func (f *storeNameFixture) load(t *testing.T, actor string) store.Store {
	t.Helper()
	s, err := f.repo.GetStoreByCharacterID(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *storeNameFixture) assets(t *testing.T, actor string) (int, int) {
	t.Helper()
	c, err := f.chars.FindByID(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var points int
	if err := f.db.QueryRowContext(f.ctx, `SELECT g.points FROM guilds g JOIN guild_members m ON m.guild_id = g.id WHERE m.character_id = ?`, actor).Scan(&points); err != nil {
		t.Fatal(err)
	}
	return c.Money, points
}

func (f *storeNameFixture) unchanged(t *testing.T, before store.Store) {
	t.Helper()
	gold, points := f.assets(t, before.CharacterID)
	if got := f.load(t, before.CharacterID); got != before || gold != 2*store.StorePrice || points != 0 {
		t.Fatalf("rejected operation changed state: store=%+v, gold=%d, guild points=%d", got, gold, points)
	}
}

func TestStoreNameSequentialClaims(t *testing.T) {
	for _, op := range []string{"rebuild", "rename"} {
		for _, occupied := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/occupied=%t", op, occupied), func(t *testing.T) {
				f := newStoreNameFixture(t)
				f.seed(t, "共有店", "town1", false) // Earlier expired row must not hide an active match.
				if occupied {
					f.seed(t, "共有店", "town2", true)
				}
				name := "共有店"
				if op == "rename" {
					name = "元の店"
				}
				actor := f.seed(t, name, "town3", op == "rename")
				var err error
				if op == "rebuild" {
					_, err = f.svc.BuildStore(f.ctx, actor.CharacterID, "town4", "021")
				} else {
					err = f.svc.ChangeStoreName(f.ctx, actor.CharacterID, "共有店")
				}
				if occupied {
					if !errors.Is(err, store.ErrStoreNameTaken) {
						t.Fatalf("claim = %v, want name taken", err)
					}
					f.unchanged(t, actor)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				got := f.load(t, actor.CharacterID)
				gold, points := f.assets(t, actor.CharacterID)
				if got.StoreName != "共有店" || got.ID != actor.ID || got.Wallpaper != actor.Wallpaper {
					t.Fatalf("claim lost registered state: %+v", got)
				}
				if op == "rebuild" {
					if !got.ExpiresAt.Equal(f.now.Add(store.StoreCycleDays*24*time.Hour)) || gold != store.StorePrice || points != store.StoreCycleDays*10 {
						t.Fatalf("rebuild state: %+v gold=%d points=%d", got, gold, points)
					}
				} else {
					if gold != 2*store.StorePrice || points != 0 || !got.ExpiresAt.Equal(actor.ExpiresAt) {
						t.Fatal("rename changed assets or expiry")
					}
					if err := f.svc.ChangeStoreName(f.ctx, actor.CharacterID, "共有店"); !errors.Is(err, store.ErrStoreNameTaken) {
						t.Fatalf("own-name rename = %v", err)
					}
				}
			})
		}
	}
}

func TestStoreNameConcurrentClaims(t *testing.T) {
	for _, pair := range [][2]string{{"rebuild", "rename"}, {"rebuild", "rebuild"}, {"rename", "rename"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			f := newStoreNameFixture(t)
			rendezvous := &storeNameRendezvous{StoreRepository: f.repo, both: make(chan struct{})}
			gp, err := database.NewGuildRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			f.svc = store.NewService(rendezvous, f.chars, nil, nil, database.NewTransactionProvider(f.db), store.WithGuildPoints(gp), store.WithNowFunc(func() time.Time { return f.now }))
			var actors [2]store.Store
			var ops [2]func() error
			for i, op := range pair {
				name := "競合店"
				if op == "rename" {
					name = fmt.Sprintf("元店%d", i)
				}
				actors[i] = f.seed(t, name, fmt.Sprintf("town%d", i+1), op == "rename")
				actor := actors[i].CharacterID
				if op == "rebuild" {
					targetTown, style := "town3", "013"
					if i == 1 {
						targetTown, style = "town4", "021"
					}
					ops[i] = func() error { _, err := f.svc.BuildStore(f.ctx, actor, targetTown, style); return err }
				} else {
					ops[i] = func() error { return f.svc.ChangeStoreName(f.ctx, actor, "競合店") }
				}
			}
			wins, active := 0, 0
			for i, err := range testutil.RunRace(ops[:]...) {
				got := f.load(t, actors[i].CharacterID)
				if got.StoreName == "競合店" && got.IsActive(f.now) {
					active++
				}
				switch {
				case err == nil:
					wins++
					gold, points := f.assets(t, actors[i].CharacterID)
					wantGold, wantPoints := 2*store.StorePrice, 0
					if pair[i] == "rebuild" {
						wantGold, wantPoints = store.StorePrice, store.StoreCycleDays*10
					}
					if gold != wantGold || points != wantPoints {
						t.Fatalf("winner assets: gold=%d points=%d", gold, points)
					}
				case errors.Is(err, store.ErrStoreNameTaken):
					f.unchanged(t, actors[i])
				default:
					t.Fatalf("unexpected race result (including deadlock/timeout): %v", err)
				}
			}
			if wins != 1 || active != 1 {
				t.Fatalf("wins=%d active names=%d", wins, active)
			}
		})
	}
}

// Without the shared lock, both claimants observe the free name before either saves.
// With it, the first wait expires while the second is still blocked on the lock.
type storeNameRendezvous struct {
	*database.StoreRepository
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func (r *storeNameRendezvous) ActiveStoreNameExists(ctx context.Context, name string, now time.Time) (bool, error) {
	exists, err := r.StoreRepository.ActiveStoreNameExists(ctx, name, now)
	r.mu.Lock()
	r.arrived++
	if r.arrived == 2 {
		close(r.both)
	}
	r.mu.Unlock()
	select {
	case <-r.both:
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return exists, err
}

type failingStoreNameRepo struct {
	*database.StoreRepository
	stage string
	err   error
}

func (r failingStoreNameRepo) LockStoreNamesForUpdate(ctx context.Context) error {
	if r.stage == "lock" {
		return r.err
	}
	return r.StoreRepository.LockStoreNamesForUpdate(ctx)
}

func (r failingStoreNameRepo) ActiveStoreNameExists(ctx context.Context, name string, now time.Time) (bool, error) {
	if r.stage == "read" {
		return false, r.err
	}
	return r.StoreRepository.ActiveStoreNameExists(ctx, name, now)
}

func (r failingStoreNameRepo) SaveStore(ctx context.Context, s store.Store) error {
	if err := r.StoreRepository.SaveStore(ctx, s); err != nil {
		return err
	}
	return r.err // Fail after a real write to verify the whole transaction rolls back.
}

func TestStoreNameFailuresRollBack(t *testing.T) {
	for _, op := range []string{"rebuild", "rename"} {
		for _, stage := range []string{"lock", "read", "save"} {
			t.Run(op+"/"+stage, func(t *testing.T) {
				f := newStoreNameFixture(t)
				actor := f.seed(t, "元店", "town1", op == "rename")
				injected := errors.New("store repository failure")
				repo := failingStoreNameRepo{f.repo, stage, injected}
				gp, err := database.NewGuildRepository(f.db)
				if err != nil {
					t.Fatal(err)
				}
				svc := store.NewService(repo, f.chars, nil, nil, database.NewTransactionProvider(f.db), store.WithGuildPoints(gp), store.WithNowFunc(func() time.Time { return f.now }))
				if op == "rebuild" {
					_, err = svc.BuildStore(f.ctx, actor.CharacterID, "town2", "005")
				} else {
					err = svc.ChangeStoreName(f.ctx, actor.CharacterID, "新店")
				}
				if !errors.Is(err, injected) {
					t.Fatalf("error = %v, want injected failure", err)
				}
				f.unchanged(t, actor)
			})
		}
	}
}

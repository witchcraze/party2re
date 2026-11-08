package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

// lockedNavigationStore is safe for the concurrent entries in the race test.
type lockedNavigationStore struct {
	mu        sync.Mutex
	selection playercontext.Selection
}

func (s *lockedNavigationStore) Load(context.Context, string) (playercontext.Selection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selection, nil
}

func (s *lockedNavigationStore) Save(_ context.Context, _ string, n playercontext.Selection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selection = n
	return nil
}

type alchemyDB struct {
	router   http.Handler
	actor    corecharacter.Character
	depots   *database.DepotRepository
	service  *alchemy.Service
	hookErr  error
	hookMu   sync.Mutex
	recipeID string
	product  string
}

func newAlchemyDB(t *testing.T, ctx context.Context) *alchemyDB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "Alchemy Gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := chars.Delete(cleanupCtx, actor.ID); err != nil {
			t.Error(err)
		}
	})
	items, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := &alchemyDB{actor: actor, depots: depots, recipeID: "recipe-001", product: "item-002"}
	f.service, err = alchemy.NewService(chars, depots, repo, recipes, items, alchemy.WithTransactionProvider(database.NewTransactionProvider(db)),
		alchemy.WithSynthesisHook(func(context.Context, string, string) error {
			f.hookMu.Lock()
			defer f.hookMu.Unlock()
			return f.hookErr
		}))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.UnlockRecipe(ctx, actor.ID, f.recipeID); err != nil {
		t.Fatal(err)
	}
	dep, err := depot.NewDepot(actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	herbs, err := item.NewInstance("item-001", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(herbs); err != nil {
		t.Fatal(err)
	}
	if err := depots.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(&lockedNavigationStore{selection: playercontext.Selection{Destination: "town"}},
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "alchemy", Parent: "town", Pageable: true}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithAlchemy(f.service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	f.router = h.Router()
	return f
}

func (f *alchemyDB) enter(t *testing.T, ctx context.Context) (int, map[string]json.RawMessage, sceneEnterResult) {
	t.Helper()
	status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", `{"action":"scene_enter","params":{"destination":"alchemy"}}`, ctx)
	var result sceneEnterResult
	if status == 200 {
		if err := json.Unmarshal(got["result"], &result); err != nil {
			t.Fatal(err)
		}
	}
	return status, got, result
}

func (f *alchemyDB) synthesize(t *testing.T, ctx context.Context) {
	t.Helper()
	status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", fmt.Sprintf(`{"action":"alchemy_synthesize","params":{"recipe_id":%q}}`, f.recipeID), ctx)
	if status != 200 {
		t.Fatalf("synthesize: %d %s", status, got)
	}
}

func (f *alchemyDB) quantity(t *testing.T, ctx context.Context, definition string) int {
	t.Helper()
	dep, err := f.depots.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return dep.Quantity(definition)
}

func (f *alchemyDB) state(t *testing.T, ctx context.Context) alchemy.Synthesis {
	t.Helper()
	st, err := f.service.GetStatus(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestAlchemyGatewayEntryDeliversAfterSleep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newAlchemyDB(t, ctx)

	if status, got, result := f.enter(t, ctx); status != 200 || result.AlchemyClaim != nil {
		t.Fatalf("idle entry: %d %s", status, got)
	}
	f.synthesize(t, ctx)
	if f.quantity(t, ctx, "item-001") != 0 || f.state(t, ctx).State != alchemy.StateOngoing {
		t.Fatal("synthesis did not consume materials")
	}
	if status, got, result := f.enter(t, ctx); status != 200 || result.AlchemyClaim != nil || f.quantity(t, ctx, f.product) != 0 {
		t.Fatalf("unfinished entry delivered: %d %s", status, got)
	}
	if err := f.service.CompleteOngoingSynthesis(ctx, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+f.actor.ID+"/context", nil)
	get.Header.Set("Authorization", "Bearer session")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, get)
	if rec.Code != 200 || f.quantity(t, ctx, f.product) != 0 {
		t.Fatalf("GET delivered: %d", rec.Code)
	}
	status, got, result := f.enter(t, ctx)
	if status != 200 || result.AlchemyClaim == nil || result.AlchemyClaim.CreatedItem.DefinitionID != f.product || f.quantity(t, ctx, f.product) != 1 || f.state(t, ctx).State != alchemy.StateNone {
		t.Fatalf("delivery: %d %s", status, got)
	}
	if status, got, result := f.enter(t, ctx); status != 200 || result.AlchemyClaim != nil || f.quantity(t, ctx, f.product) != 1 {
		t.Fatalf("second entry delivered again: %d %s", status, got)
	}
}

func TestAlchemyGatewayEntryRollsBackWhenClaimFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newAlchemyDB(t, ctx)
	f.synthesize(t, ctx)
	if err := f.service.CompleteOngoingSynthesis(ctx, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	f.hookErr = errors.New("private hook failure")
	status, got, _ := f.enter(t, ctx)
	assertGatewayError(t, got, "EXECUTION_FAILED")
	if status != 500 || f.quantity(t, ctx, f.product) != 0 || f.state(t, ctx).State != alchemy.StateCompleted {
		t.Fatalf("failed claim moved assets: %d %s", status, got)
	}
	f.hookErr = nil
	if status, got, result := f.enter(t, ctx); status != 200 || result.AlchemyClaim == nil || f.quantity(t, ctx, f.product) != 1 {
		t.Fatalf("retry: %d %s", status, got)
	}
}

func TestAlchemyGatewayEntryFullDepotDeliversOverCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newAlchemyDB(t, ctx)
	f.synthesize(t, ctx)
	if err := f.service.CompleteOngoingSynthesis(ctx, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	dep, err := f.depots.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep.RefreshCapacity(f.actor.JobLevel, f.actor.OverDepot)
	for len(dep.Items) < dep.Capacity {
		filler, err := item.NewInstance("weapon-01", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := dep.AddItem(filler, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.depots.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}
	// alchemy.cgi:183-201 finishes through send_item, which ignores capacity.
	status, got, result := f.enter(t, ctx)
	if status != 200 || result.AlchemyClaim == nil || f.quantity(t, ctx, f.product) != 1 {
		t.Fatalf("full depot: %d %s", status, got)
	}
	after, err := f.depots.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) <= dep.Capacity {
		t.Fatalf("product must be stored above capacity %d, got %d slots", dep.Capacity, len(after.Items))
	}
}

func TestAlchemyGatewayConcurrentEntriesDeliverOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newAlchemyDB(t, ctx)
	f.synthesize(t, ctx)
	if err := f.service.CompleteOngoingSynthesis(ctx, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	delivered := 0
	enter := func() error {
		status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", `{"action":"scene_enter","params":{"destination":"alchemy"}}`, ctx)
		if status != 200 {
			return fmt.Errorf("entry: %d %s", status, got)
		}
		var result sceneEnterResult
		if err := json.Unmarshal(got["result"], &result); err != nil {
			return err
		}
		if result.AlchemyClaim != nil {
			mu.Lock()
			delivered++
			mu.Unlock()
		}
		return nil
	}
	for _, err := range testutil.RunRace(enter, enter, enter, enter) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if delivered != 1 || f.quantity(t, ctx, f.product) != 1 || f.state(t, ctx).TotalCrafts != 1 {
		t.Fatalf("assets not conserved: delivered=%d quantity=%d crafts=%d", delivered, f.quantity(t, ctx, f.product), f.state(t, ctx).TotalCrafts)
	}
}

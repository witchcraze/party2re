package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestEventPlazaRetiredRoutes(t *testing.T) {
	// Any service call panics: removed routes must stop at the router.
	for _, service := range []apihttp.EventPlazaService{nil, &struct{ apihttp.EventPlazaService }{}} {
		h, err := createTestHandler(apihttp.WithEventPlaza(service))
		if err != nil {
			t.Fatal(err)
		}
		router := h.Router()
		for _, token := range []string{"", "invalid-session", "valid-session"} {
			for _, route := range []struct{ method, path string }{
				{http.MethodGet, "/eventplaza/banquets"},
				{http.MethodPost, "/eventplaza/banquets/retired/toast"},
			} {
				req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"character_id":"char-1"}`))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s %s: status %d, want 404", route.method, route.path, rec.Code)
				}
			}
		}
	}
}

func TestEventPlazaRetiredToastConcurrentRequestsPreserveAssets(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "RetiredToast_"+id.New()[:8], 1234)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewEventPlazaRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventplaza.NewService(repo, characters, inventories, eventplaza.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	h, err := apihttp.NewHandler(
		&stubPlayerService{authenticateFn: func(context.Context, string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: actor.PlayerID}, nil
		}},
		&stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
			return characters.FindByID(ctx, actor.ID)
		}},
		&stubAdventureService{}, &stubShopService{}, apihttp.WithEventPlaza(service))
	if err != nil {
		t.Fatal(err)
	}
	marker := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	if err := repo.RecordPresence(ctx, actor.ID, marker); err != nil {
		t.Fatal(err)
	}
	banquetIDs := []string{id.New(), id.New()}
	for i, banquetID := range banquetIDs {
		if _, err := db.ExecContext(ctx, `INSERT INTO celebration_banquets
			(id, boss_id, boss_name, slayer_character_id, slayer_character_name, tier, toast_count, celebrated_at, expires_at)
			VALUES (?, 'king1', 'King', ?, ?, 1, ?, ?, ?)`, banquetID, actor.ID, actor.Name, i, marker, marker.Add(24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO banquet_toasts (banquet_id, character_id, toasted_at) VALUES (?, ?, ?)", banquetIDs[1], actor.ID, marker); err != nil {
		t.Fatal(err)
	}
	// Every assertion is scoped to this test's fixtures in the shared database.
	assertUnchanged := func() {
		t.Helper()
		char, err := characters.FindByID(ctx, actor.ID)
		if err != nil || char.Money != actor.Money {
			t.Fatalf("wallet changed: %+v, err=%v", char, err)
		}
		var presence time.Time
		if err := db.QueryRowContext(ctx, "SELECT last_seen_at FROM eventplaza_presences WHERE character_id = ?", actor.ID).Scan(&presence); err != nil || !presence.Equal(marker) {
			t.Fatalf("presence changed: %v, err=%v", presence, err)
		}
		var banquets, toasts, toastCount int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), SUM(toast_count) FROM celebration_banquets WHERE slayer_character_id = ?", actor.ID).Scan(&banquets, &toastCount); err != nil {
			t.Fatal(err)
		}
		var toastedAt time.Time
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MIN(toasted_at) FROM banquet_toasts WHERE character_id = ?", actor.ID).Scan(&toasts, &toastedAt); err != nil {
			t.Fatal(err)
		}
		if banquets != 2 || toastCount != 1 || toasts != 1 || !toastedAt.Equal(marker) {
			t.Fatalf("historical records changed: banquets=%d toastCount=%d toasts=%d toastedAt=%v", banquets, toastCount, toasts, toastedAt)
		}
	}
	assertUnchanged()
	router := h.Router()
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
		req := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/"+banquetIDs[0]+"/toast", strings.NewReader(fmt.Sprintf(`{"character_id":%q}`, actor.ID)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			return fmt.Errorf("retired toast status %d, want 404", rec.Code)
		}
		return nil
	})
	if result.Successes != result.TotalOps || result.Failures != 0 {
		t.Errorf("retired toast requests: %+v", result)
	}
	assertUnchanged()
}

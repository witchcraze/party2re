package http_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCasinoExchange_RealDB_HTTP_OverflowRejected(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Create player and character with 0 initial gold in real MariaDB
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "CasinoHTTPGambler", 0)
	if err != nil {
		t.Fatal(err)
	}
	player := coreplayer.Player{ID: char.PlayerID, Username: "casinoplayer"}

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	casinoSvc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == char.ID {
				return charRepo.FindByID(ctx, id)
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithCasino(casinoSvc),
	)
	router := h.Router()

	exchangeURL := fmt.Sprintf("/characters/%s/casino/exchange", char.ID)

	// 2. Initial balance in real DB must be 0 coins
	initialAcc, err := casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if initialAcc.Coins != 0 {
		t.Fatalf("initial coins in DB = %d, want 0", initialAcc.Coins)
	}

	// 3. HTTP POST with 2^62 coins (4611686018427387904) -> must return 400 Bad Request, no coins in DB
	req := jsonRequest(t, http.MethodPost, exchangeURL, `{"direction":"gold_to_coins","coins":4611686018427387904}`)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for 2^62 coins, got %d: %s", rec.Code, rec.Body.String())
	}

	dbAcc, err := casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbAcc.Coins != 0 {
		t.Fatalf("exploit succeeded: 2^62 coins credited to DB! coins=%d", dbAcc.Coins)
	}

	// 4. HTTP POST with MaxInt64 (9223372036854775807) -> must return 400 Bad Request, no coins in DB
	reqMax := jsonRequest(t, http.MethodPost, exchangeURL, fmt.Sprintf(`{"direction":"gold_to_coins","coins":%d}`, int64(math.MaxInt64)))
	reqMax.Header.Set("Authorization", "Bearer valid-token")
	recMax := httptest.NewRecorder()
	router.ServeHTTP(recMax, reqMax)

	if recMax.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for MaxInt64, got %d: %s", recMax.Code, recMax.Body.String())
	}

	dbAcc, err = casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbAcc.Coins != 0 {
		t.Fatalf("MaxInt64 coins credited to DB! coins=%d", dbAcc.Coins)
	}

	// 5. HTTP POST with zero and negative coins -> must return 400 Bad Request, no coins in DB
	for _, invalidCoins := range []string{`0`, `-1`, `-50`} {
		reqBad := jsonRequest(t, http.MethodPost, exchangeURL, `{"direction":"gold_to_coins","coins":`+invalidCoins+`}`)
		reqBad.Header.Set("Authorization", "Bearer valid-token")
		recBad := httptest.NewRecorder()
		router.ServeHTTP(recBad, reqBad)

		if recBad.Code != http.StatusBadRequest {
			t.Fatalf("coins=%s: expected 400 Bad Request, got %d: %s", invalidCoins, recBad.Code, recBad.Body.String())
		}
	}

	dbAcc, err = casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbAcc.Coins != 0 {
		t.Fatalf("coins modified in DB after invalid requests! coins=%d", dbAcc.Coins)
	}

	// 6. Give character 200 gold in real DB and perform legitimate HTTP exchange of 10 coins (costs 200 gold)
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 200, char.ID); err != nil {
		t.Fatal(err)
	}

	reqLegit := jsonRequest(t, http.MethodPost, exchangeURL, `{"direction":"gold_to_coins","coins":10}`)
	reqLegit.Header.Set("Authorization", "Bearer valid-token")
	recLegit := httptest.NewRecorder()
	router.ServeHTTP(recLegit, reqLegit)

	if recLegit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for legitimate exchange, got %d: %s", recLegit.Code, recLegit.Body.String())
	}

	// Verify real DB state
	dbAcc, err = casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbAcc.Coins != 10 {
		t.Fatalf("expected 10 coins in real DB, got %d", dbAcc.Coins)
	}
	dbChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbChar.Money != 0 {
		t.Fatalf("expected 0 gold remaining in real DB, got %d", dbChar.Money)
	}
}

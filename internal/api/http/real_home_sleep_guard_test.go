package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/store"
)

type inMemoryRealTimer struct {
	remainingDurations map[string]time.Duration
	getRemainingErrs   map[string]error
	locks              map[string]bool
	isLockedErrs       map[string]error
}

func newInMemoryRealTimer() *inMemoryRealTimer {
	return &inMemoryRealTimer{
		remainingDurations: make(map[string]time.Duration),
		getRemainingErrs:   make(map[string]error),
		locks:              make(map[string]bool),
		isLockedErrs:       make(map[string]error),
	}
}

func (t *inMemoryRealTimer) SetLock(ctx context.Context, category, targetID string, duration time.Duration) error {
	t.remainingDurations[category+":"+targetID] = duration
	t.locks[category+":"+targetID] = true
	return nil
}

func (t *inMemoryRealTimer) IsLocked(ctx context.Context, category, targetID string) (bool, error) {
	key := category + ":" + targetID
	if err, ok := t.isLockedErrs[key]; ok && err != nil {
		return false, err
	}
	return t.locks[key], nil
}

func (t *inMemoryRealTimer) GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error) {
	key := category + ":" + targetID
	if err, ok := t.getRemainingErrs[key]; ok && err != nil {
		return 0, err
	}
	return t.remainingDurations[key], nil
}

func (t *inMemoryRealTimer) ReleaseLock(ctx context.Context, category, targetID string) error {
	key := category + ":" + targetID
	delete(t.remainingDurations, key)
	delete(t.locks, key)
	return nil
}

func (t *inMemoryRealTimer) ResetDailyQuota(ctx context.Context, action, targetID string) error {
	return nil
}

type inMemoryRealBankRepo struct {
	char corecharacter.Character
}

func (r *inMemoryRealBankRepo) GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if characterID == r.char.ID {
		return r.char, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (r *inMemoryRealBankRepo) Deposit(ctx context.Context, characterID string, amount int64) (corecharacter.Character, error) {
	if characterID != r.char.ID {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	r.char.Money -= int(amount)
	r.char.Deposit += amount
	return r.char, nil
}

func (r *inMemoryRealBankRepo) Withdraw(ctx context.Context, characterID string, amount int64) (corecharacter.Character, int, int64, error) {
	if characterID != r.char.ID {
		return corecharacter.Character{}, 0, 0, corecharacter.ErrNotFound
	}
	r.char.Money += int(amount)
	r.char.Deposit -= amount
	return r.char, int(amount), 0, nil
}

type inMemoryRealHomeRepo struct {
	home.Repository
}

type inMemoryRealCharRepo struct {
	char        corecharacter.Character
	updateCount int
}

func (c *inMemoryRealCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == c.char.ID {
		return c.char, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (c *inMemoryRealCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return c.FindByID(ctx, id)
}

func (c *inMemoryRealCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	c.char = char
	c.updateCount++
	return nil
}

func setupRealHomeGuardTest(
	t *testing.T,
	timerSvc *inMemoryRealTimer,
	bankRepo *inMemoryRealBankRepo,
	charRepo *inMemoryRealCharRepo,
	parkSvc *mockParkService,
	storeSvc *stubStoreService,
) (http.Handler, *home.Service) {
	t.Helper()

	homeSvc, err := home.NewService(
		&inMemoryRealHomeRepo{},
		charRepo,
		home.WithTimer(timerSvc),
		home.WithCharacterUpdater(charRepo),
	)
	if err != nil {
		t.Fatalf("home.NewService failed: %v", err)
	}

	bankSvc, err := bank.NewService(bankRepo)
	if err != nil {
		t.Fatalf("bank.NewService failed: %v", err)
	}

	player := coreplayer.Player{ID: "player-1", Username: "hero"}
	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("invalid session")
		},
	}

	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == charRepo.char.ID {
				return charRepo.char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	h, err := apihttp.NewHandler(
		players,
		chars,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithHome(homeSvc),
		apihttp.WithBank(bankSvc),
		apihttp.WithPark(parkSvc),
		apihttp.WithStore(storeSvc),
	)
	if err != nil {
		t.Fatalf("apihttp.NewHandler failed: %v", err)
	}

	return h.Router(), homeSvc
}

func TestAuditRealHomeSleepGuard(t *testing.T) {
	t.Run("Asleep lookup failure halts execution with 5xx without mutating Bank, Park or Store", func(t *testing.T) {
		charRepo := &inMemoryRealCharRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
				Tired:    60,
				Stats:    corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50},
			},
		}
		bankRepo := &inMemoryRealBankRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
				Money:    100,
				Deposit:  1000,
			},
		}
		timerSvc := newInMemoryRealTimer()
		// Duration lock expired, Asleep lookup returns a storage error
		timerSvc.remainingDurations[timer.CategorySleep+":hero"] = 0
		timerSvc.isLockedErrs[timer.CategoryAsleep+":hero"] = errors.New("storage error in valkey asleep lookup")

		parkPosted := false
		mockPark := &mockParkService{
			postFn: func(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
				parkPosted = true
				return park.Post{}, nil
			},
		}

		storeBuilt := false
		mockStore := &stubStoreService{
			buildStoreFn: func(ctx context.Context, characterID, townID, houseStyle, storeName string) (*store.StoreCheckResult, error) {
				storeBuilt = true
				return nil, nil
			},
		}

		router, homeSvc := setupRealHomeGuardTest(t, timerSvc, bankRepo, charRepo, mockPark, mockStore)

		// 1. Bank deposit fails with 500 and does NOT mutate wallet or deposit
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/characters/hero/bank/deposit", bytes.NewReader([]byte(`{"amount":10}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500 Internal Server Error for bank deposit on asleep error, got %d: %s", rec.Code, rec.Body.String())
			}
			if bankRepo.char.Money != 100 || bankRepo.char.Deposit != 1000 {
				t.Errorf("expected bank state unmutated on error, got money=%d, deposit=%d", bankRepo.char.Money, bankRepo.char.Deposit)
			}
		}

		// 2. Representative Park action fails with 500 and does NOT invoke callback
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader([]byte(`{"character_id":"hero","content":"hello"}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500 Internal Server Error for park post on asleep error, got %d: %s", rec.Code, rec.Body.String())
			}
			if parkPosted {
				t.Errorf("expected park post not to be created on asleep lookup error")
			}
		}

		// 3. Representative Store action fails with 500 and does NOT invoke callback
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/towns/town1/stores", bytes.NewReader([]byte(`{"character_id":"hero","house_style":"001","store_name":"Hero Shop"}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500 Internal Server Error for store build on asleep error, got %d: %s", rec.Code, rec.Body.String())
			}
			if storeBuilt {
				t.Errorf("expected store build callback not to be invoked on asleep lookup error")
			}
		}

		// 4. Wake propagates the failed Asleep prerequisite read without recovery writes or false awake success
		{
			wakeRes, err := homeSvc.Wake(context.Background(), "hero")
			if err == nil {
				t.Fatalf("expected error from Wake on asleep lookup error, got nil: %+v", wakeRes)
			}
			if wakeRes.Success {
				t.Errorf("expected WakeResult.Success = false on asleep lookup error")
			}
			if strings.Contains(wakeRes.Message, "すでに目覚めています") {
				t.Errorf("expected no false awake message on asleep lookup error, got: %s", wakeRes.Message)
			}
			if charRepo.updateCount != 0 {
				t.Errorf("expected 0 character updates on wake failure, got %d", charRepo.updateCount)
			}
			if charRepo.char.Stats.HP != 10 || charRepo.char.Stats.MP != 5 || charRepo.char.Tired != 60 {
				t.Errorf("expected no recovery writes on wake failure, got HP=%d, MP=%d, Tired=%d",
					charRepo.char.Stats.HP, charRepo.char.Stats.MP, charRepo.char.Tired)
			}
		}
	})

	t.Run("Pending recovery contrast returns 409 Conflict without mutating state", func(t *testing.T) {
		charRepo := &inMemoryRealCharRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
			},
		}
		bankRepo := &inMemoryRealBankRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
				Money:    100,
				Deposit:  1000,
			},
		}
		timerSvc := newInMemoryRealTimer()
		// Sleep duration expired, but CategoryAsleep is active
		timerSvc.remainingDurations[timer.CategorySleep+":hero"] = 0
		timerSvc.locks[timer.CategoryAsleep+":hero"] = true

		parkPosted := false
		mockPark := &mockParkService{
			postFn: func(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
				parkPosted = true
				return park.Post{}, nil
			},
		}

		storeBuilt := false
		mockStore := &stubStoreService{
			buildStoreFn: func(ctx context.Context, characterID, townID, houseStyle, storeName string) (*store.StoreCheckResult, error) {
				storeBuilt = true
				return nil, nil
			},
		}

		router, _ := setupRealHomeGuardTest(t, timerSvc, bankRepo, charRepo, mockPark, mockStore)

		// Bank deposit returns 409 Conflict
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/characters/hero/bank/deposit", bytes.NewReader([]byte(`{"amount":10}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("expected 409 Conflict for bank deposit when pending recovery, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "目を覚ましてください") {
				t.Errorf("expected conflict message to prompt wake-up, got: %s", rec.Body.String())
			}
			if bankRepo.char.Money != 100 || bankRepo.char.Deposit != 1000 {
				t.Errorf("expected bank state unmutated on 409, got money=%d, deposit=%d", bankRepo.char.Money, bankRepo.char.Deposit)
			}
		}

		// Park post returns 409 Conflict
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader([]byte(`{"character_id":"hero","content":"hello"}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("expected 409 Conflict for park post when pending recovery, got %d: %s", rec.Code, rec.Body.String())
			}
			if parkPosted {
				t.Errorf("expected park post not to be created when pending recovery")
			}
		}

		// Store build returns 409 Conflict
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/towns/town1/stores", bytes.NewReader([]byte(`{"character_id":"hero","house_style":"001","store_name":"Hero Shop"}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("expected 409 Conflict for store build when pending recovery, got %d: %s", rec.Code, rec.Body.String())
			}
			if storeBuilt {
				t.Errorf("expected store build callback not to be invoked when pending recovery")
			}
		}
	})

	t.Run("Active sleep contrast returns 409 Conflict without mutating state", func(t *testing.T) {
		charRepo := &inMemoryRealCharRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
			},
		}
		bankRepo := &inMemoryRealBankRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
				Money:    100,
				Deposit:  1000,
			},
		}
		timerSvc := newInMemoryRealTimer()
		// Active sleep duration remaining
		timerSvc.remainingDurations[timer.CategorySleep+":hero"] = 5 * time.Minute
		timerSvc.locks[timer.CategoryAsleep+":hero"] = true

		parkPosted := false
		mockPark := &mockParkService{
			postFn: func(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
				parkPosted = true
				return park.Post{}, nil
			},
		}

		router, _ := setupRealHomeGuardTest(t, timerSvc, bankRepo, charRepo, mockPark, nil)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/hero/bank/deposit", bytes.NewReader([]byte(`{"amount":10}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for bank deposit when actively sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
		if bankRepo.char.Money != 100 || bankRepo.char.Deposit != 1000 {
			t.Errorf("expected bank state unmutated on 409, got money=%d, deposit=%d", bankRepo.char.Money, bankRepo.char.Deposit)
		}
		if parkPosted {
			t.Errorf("expected park post not to be created")
		}
	})

	t.Run("Awake contrast succeeds and mutates Bank and Park", func(t *testing.T) {
		charRepo := &inMemoryRealCharRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
			},
		}
		bankRepo := &inMemoryRealBankRepo{
			char: corecharacter.Character{
				ID:       "hero",
				PlayerID: "player-1",
				Name:     "Hero",
				Money:    100,
				Deposit:  1000,
			},
		}
		timerSvc := newInMemoryRealTimer()
		// Both unlocked -> fully awake
		timerSvc.remainingDurations[timer.CategorySleep+":hero"] = 0
		timerSvc.locks[timer.CategoryAsleep+":hero"] = false

		parkPosted := false
		mockPark := &mockParkService{
			postFn: func(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
				parkPosted = true
				return park.Post{ID: "post-1"}, nil
			},
		}

		router, _ := setupRealHomeGuardTest(t, timerSvc, bankRepo, charRepo, mockPark, nil)

		// Bank deposit succeeds with 200 and mutates wallet=90, deposit=1010
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/characters/hero/bank/deposit", bytes.NewReader([]byte(`{"amount":10}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for bank deposit when awake, got %d: %s", rec.Code, rec.Body.String())
			}
			if bankRepo.char.Money != 90 || bankRepo.char.Deposit != 1010 {
				t.Errorf("expected bank deposit mutated state to wallet=90, deposit=1010, got money=%d, deposit=%d",
					bankRepo.char.Money, bankRepo.char.Deposit)
			}
		}

		// Park post succeeds with 201 Created and creates post
		{
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader([]byte(`{"character_id":"hero","content":"hello"}`)))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201 Created for park post when awake, got %d: %s", rec.Code, rec.Body.String())
			}
			if !parkPosted {
				t.Errorf("expected park post to be created when awake")
			}
		}
	})
}

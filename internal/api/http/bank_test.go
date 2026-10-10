package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type stubBankService struct {
	depositFn  func(ctx context.Context, characterID string, amount int64) (bank.DepositResult, error)
	withdrawFn func(ctx context.Context, characterID string, amount int64) (bank.WithdrawResult, error)
	inspectFn  func() bank.NPCInfo
	talkFn     func() string
}

func (s *stubBankService) GetState(ctx context.Context, characterID string) (bank.State, error) {
	return bank.State{
		CharacterID: characterID,
		Money:       5000,
		Deposit:     10000,
		MaxDeposit:  bank.MaxDeposit,
		NPCName:     bank.NPCName,
		Dialogues:   bank.NPCDialogues,
	}, nil
}

func (s *stubBankService) Deposit(ctx context.Context, characterID string, amount int64) (bank.DepositResult, error) {
	if s.depositFn != nil {
		return s.depositFn(ctx, characterID, amount)
	}
	return bank.DepositResult{
		CharacterID: characterID,
		Money:       2000,
		Deposit:     13000,
		Amount:      amount,
		Message:     "3000 Gお預かりいたしました",
	}, nil
}

func (s *stubBankService) Withdraw(ctx context.Context, characterID string, amount int64) (bank.WithdrawResult, error) {
	if s.withdrawFn != nil {
		return s.withdrawFn(ctx, characterID, amount)
	}
	return bank.WithdrawResult{
		CharacterID:     characterID,
		Money:           8000,
		Deposit:         7000,
		Amount:          amount,
		ActualWithdrawn: int(amount),
		Refunded:        0,
		Message:         "3000 Gお返しいたします",
	}, nil
}

func (s *stubBankService) InspectNPC() bank.NPCInfo {
	if s.inspectFn != nil {
		return s.inspectFn()
	}
	return bank.NPCInfo{
		Name:      bank.NPCName,
		Dialogues: bank.NPCDialogues,
	}
}

func (s *stubBankService) TalkNPC() string {
	if s.talkFn != nil {
		return s.talkFn()
	}
	return "いらっしゃいませ。預金ならおまかせください。"
}

func TestBankHTTPHandlers(t *testing.T) {
	player := coreplayer.Player{
		ID:        "p1",
		Username:  "tester",
		CreatedAt: time.Now().UTC(),
	}
	char := corecharacter.Character{
		ID:       "c1",
		PlayerID: player.ID,
		Name:     "Bank Hero",
		Money:    5000,
		Deposit:  10000,
	}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	bankSvc := &stubBankService{}

	readers := &contextReaders{char: char}
	h, err := apihttp.NewHandler(pService, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithBank(bankSvc),
		apihttp.WithPlayerContext(playercontext.NewService(readers, readers, timer.NewService(nil))))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("Gateway bank_deposit success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/characters/c1/actions", bytes.NewBufferString(`{"action":"bank_deposit","params":{"amount":3000}}`))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var response struct {
			Success bool               `json:"success"`
			Result  bank.DepositResult `json:"result"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if !response.Success || response.Result.Amount != 3000 || response.Result.Money != 2000 {
			t.Errorf("unexpected deposit result: %+v", response)
		}
	})

	t.Run("Gateway bank_deposit insufficient funds", func(t *testing.T) {
		bankSvc.depositFn = func(ctx context.Context, characterID string, amount int64) (bank.DepositResult, error) {
			return bank.DepositResult{}, bank.ErrInsufficientFunds
		}
		defer func() { bankSvc.depositFn = nil }()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/characters/c1/actions", bytes.NewBufferString(`{"action":"bank_deposit","params":{"amount":999999}}`))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Gateway bank_withdraw success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/characters/c1/actions", bytes.NewBufferString(`{"action":"bank_withdraw","params":{"amount":3000}}`))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var response struct {
			Success bool                `json:"success"`
			Result  bank.WithdrawResult `json:"result"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if !response.Success || response.Result.Amount != 3000 || response.Result.Money != 8000 {
			t.Errorf("unexpected withdraw result: %+v", response)
		}
	})

	t.Run("Gateway bank_withdraw insufficient balance", func(t *testing.T) {
		bankSvc.withdrawFn = func(ctx context.Context, characterID string, amount int64) (bank.WithdrawResult, error) {
			return bank.WithdrawResult{}, bank.ErrInsufficientBalance
		}
		defer func() { bankSvc.withdrawFn = nil }()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/characters/c1/actions", bytes.NewBufferString(`{"action":"bank_withdraw","params":{"amount":50000}}`))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/bank/inspect success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/bank/inspect", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var info bank.NPCInfo
		if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if info.Name != bank.NPCName {
			t.Errorf("expected NPC name %s, got %s", bank.NPCName, info.Name)
		}
	})

	t.Run("POST /characters/{id}/bank/talk success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/bank/talk", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if resp["message"] == "" {
			t.Error("expected non-empty message")
		}
	})

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/characters/c1/context", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type bankGatewayFixture struct {
	*gatewayFixture
	BankService
	service                     *bank.Service
	depositCalls, withdrawCalls int
	requestedAmount             int64
	repositoryErr               error
}

func newBankGatewayFixture(t *testing.T) *bankGatewayFixture {
	f := &bankGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	f.char.Deposit = 1000
	service, err := bank.NewService(bankGatewayRepository{f})
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	return f
}

func (f *bankGatewayFixture) recordCall(ctx context.Context, actorID string, amount int64) {
	f.checkContext(ctx)
	if actorID != "hero" {
		f.t.Fatalf("wrong bank actor: %s", actorID)
	}
	f.executions++
	f.requestedAmount = amount
}

func (f *bankGatewayFixture) Deposit(ctx context.Context, actorID string, amount int64) (bank.DepositResult, error) {
	f.recordCall(ctx, actorID, amount)
	f.depositCalls++
	result, err := f.service.Deposit(ctx, actorID, amount)
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return result, err
}

func (f *bankGatewayFixture) Withdraw(ctx context.Context, actorID string, amount int64) (bank.WithdrawResult, error) {
	f.recordCall(ctx, actorID, amount)
	f.withdrawCalls++
	result, err := f.service.Withdraw(ctx, actorID, amount)
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return result, err
}

// Reuse Bank calculations as the database repository does, with the shared
// character feeding uncached observations after the real service mutation.
type bankGatewayRepository struct{ *bankGatewayFixture }

func (r bankGatewayRepository) GetCharacter(ctx context.Context, actorID string) (corecharacter.Character, error) {
	r.checkContext(ctx)
	if actorID != r.char.ID {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.char, r.repositoryErr
}

func (r bankGatewayRepository) Deposit(ctx context.Context, actorID string, amount int64) (corecharacter.Character, error) {
	c, err := r.GetCharacter(ctx, actorID)
	if err != nil {
		return corecharacter.Character{}, err
	}
	money, deposit, err := bank.CalculateDeposit(c.Money, c.Deposit, amount)
	if err != nil {
		return corecharacter.Character{}, err
	}
	r.char.Money, r.char.Deposit = money, deposit
	return r.char, nil
}

func (r bankGatewayRepository) Withdraw(ctx context.Context, actorID string, amount int64) (corecharacter.Character, int, int64, error) {
	c, err := r.GetCharacter(ctx, actorID)
	if err != nil {
		return corecharacter.Character{}, 0, 0, err
	}
	money, deposit, actual, refunded, err := bank.CalculateWithdrawal(c.Money, c.Deposit, amount)
	if err != nil {
		return corecharacter.Character{}, 0, 0, err
	}
	r.char.Money, r.char.Deposit = money, deposit
	return r.char, actual, refunded, nil
}

func (f *bankGatewayFixture) router(timers playercontext.TimerReader) http.Handler {
	opts := []Option{
		WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)),
		WithBank(f), func(h *Handler) { h.homes = f.gatewayFixture },
	}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *bankGatewayFixture) request(router http.Handler, action, params string) (int, map[string]json.RawMessage) {
	body := `{"action":"` + action + `"`
	if params != "" {
		body += `,"params":` + params
	}
	return gatewayRequest(f.t, router, "hero", "session", "application/json", body+`}`, f.expectedContext)
}

func (f *bankGatewayFixture) assertCalls(action string, amount int64) {
	f.t.Helper()
	wantDeposit, wantWithdraw := 0, 1
	if action == "bank_deposit" {
		wantDeposit, wantWithdraw = 1, 0
	}
	if f.executions != 1 || f.depositCalls != wantDeposit || f.withdrawCalls != wantWithdraw || f.requestedAmount != amount {
		f.t.Fatalf("wrong bank calls: deposit=%d withdraw=%d amount=%d", f.depositCalls, f.withdrawCalls, f.requestedAmount)
	}
}

func TestBankGatewaySuccessAndRefreshFailure(t *testing.T) {
	for _, tc := range []struct {
		name, action                           string
		money, wantMoney, actual               int
		deposit, amount, wantDeposit, refunded int64
	}{
		{"deposit", "bank_deposit", 100, 60, 0, 1000, 40, 1040, 0},
		{"deposit all wallet funds", "bank_deposit", 100, 0, 0, 1000, 100, 1100, 0},
		{"deposit limit boundary", "bank_deposit", 100, 60, 0, bank.MaxDeposit - 40, 40, bank.MaxDeposit, 0},
		{"withdraw", "bank_withdraw", 100, 140, 40, 1000, 40, 960, 0},
		{"withdraw with empty wallet", "bank_withdraw", 0, 40, 40, 1000, 40, 960, 0},
		{"overflow refund", "bank_withdraw", bank.MaxWallet - 10, bank.MaxWallet, 10, 1000, 40, 990, 30},
		{"full wallet", "bank_withdraw", bank.MaxWallet, bank.MaxWallet, 0, bank.MaxDeposit, bank.MaxDeposit, bank.MaxDeposit, bank.MaxDeposit},
	} {
		for _, failure := range []string{"", "query", "profile"} {
			t.Run(tc.name+"/"+failure, func(t *testing.T) {
				f := newBankGatewayFixture(t)
				f.char.Money, f.char.Deposit = tc.money, tc.deposit
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "bank request")
				f.afterExecute = func() {
					if failure == "query" {
						f.queryErr = errors.New("secret query store")
					}
					if failure == "profile" {
						f.profileErr = errors.New("secret profile store")
					}
				}
				status, got := f.request(f.router(timer.NewService(nil)), tc.action, fmt.Sprintf(`{"amount":%d}`, tc.amount))
				if status != 200 || string(got["success"]) != "true" {
					t.Fatalf("status=%d: %s", status, got)
				}
				f.assertCalls(tc.action, tc.amount)
				if f.char.Money != tc.wantMoney || f.char.Deposit != tc.wantDeposit || int64(f.char.Money)+f.char.Deposit != int64(tc.money)+tc.deposit {
					t.Fatalf("wrong or nonconserving funds: %+v", f.char)
				}
				if tc.action == "bank_deposit" {
					var result bank.DepositResult
					if err := json.Unmarshal(got["result"], &result); err != nil {
						t.Fatal(err)
					}
					want := bank.DepositResult{CharacterID: "hero", Money: tc.wantMoney, Deposit: tc.wantDeposit, Amount: tc.amount, Message: fmt.Sprintf("%d Gお預かりいたしました", tc.amount)}
					if result != want {
						t.Fatalf("lost deposit result: %+v", result)
					}
				} else {
					var result bank.WithdrawResult
					if err := json.Unmarshal(got["result"], &result); err != nil {
						t.Fatal(err)
					}
					want := bank.WithdrawResult{CharacterID: "hero", Money: tc.wantMoney, Deposit: tc.wantDeposit, Amount: tc.amount, ActualWithdrawn: tc.actual, Refunded: tc.refunded, Message: fmt.Sprintf("%d Gお返しいたします", tc.amount)}
					if result != want {
						t.Fatalf("lost withdrawal result: %+v", result)
					}
				}
				if failure != "" {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost refresh failure: %s", got)
					}
					return
				}
				assertGatewayContext(t, got)
				var observation PlayerContextResponse
				if err := json.Unmarshal(got["context"], &observation); err != nil {
					t.Fatal(err)
				}
				if observation.Character.Gold != tc.wantMoney {
					t.Fatalf("stale wallet: %+v", observation)
				}
			})
		}
	}
}

func TestBankGatewayServiceValidation(t *testing.T) {
	for _, tc := range []struct {
		action, code string
		amount       int64
		deposit      int64
	}{
		{"bank_deposit", "BANK_INVALID_AMOUNT", 0, 1000},
		{"bank_deposit", "BANK_INVALID_AMOUNT", -1, 1000},
		{"bank_withdraw", "BANK_INVALID_AMOUNT", 0, 1000},
		{"bank_withdraw", "BANK_INVALID_AMOUNT", -1, 1000},
		{"bank_deposit", "BANK_INSUFFICIENT_FUNDS", 101, 1000},
		{"bank_withdraw", "BANK_INSUFFICIENT_BALANCE", 1001, 1000},
		{"bank_deposit", "BANK_DEPOSIT_LIMIT_EXCEEDED", 40, bank.MaxDeposit - 39},
	} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%d/%t", tc.action, tc.amount, refreshFails), func(t *testing.T) {
				f := newBankGatewayFixture(t)
				f.char.Deposit = tc.deposit
				before := f.char
				if refreshFails {
					f.afterExecute = func() { f.queryErr = errors.New("secret store") }
				}
				status, got := f.request(f.router(timer.NewService(nil)), tc.action, fmt.Sprintf(`{"amount":%d}`, tc.amount))
				if status != 400 || f.char.Money != before.Money || f.char.Deposit != before.Deposit {
					t.Fatalf("status=%d funds=%+v: %s", status, f.char, got)
				}
				f.assertCalls(tc.action, tc.amount)
				assertGatewayError(t, got, tc.code)
				if refreshFails {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost rejection: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestBankGatewayInvalidParamsAndOwnership(t *testing.T) {
	for _, action := range []string{"bank_deposit", "bank_withdraw"} {
		for _, params := range []string{"", `null`, `{}`, `[]`, `{"amount":null}`, `{"amount":"1"}`, `{"amount":1.5}`, `{"amount":true}`, `{"amount":[]}`, `{"amount":9223372036854775808}`, `{"amount":1,"character_id":"other"}`, `{"amount":1,"player_id":"other"}`, `{"amount":1,"actor_id":"other"}`} {
			t.Run(action+params, func(t *testing.T) {
				f := newBankGatewayFixture(t)
				status, got := f.request(f.router(timer.NewService(nil)), action, params)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("status=%d executions=%d: %s", status, f.executions, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
		for _, tc := range []struct {
			path, token string
			status      int
		}{{"hero", "", 401}, {"other", "session", 403}} {
			f := newBankGatewayFixture(t)
			status, got := gatewayRequest(t, f.router(timer.NewService(nil)), tc.path, tc.token, "application/json", `{"action":"`+action+`","params":{"amount":1}}`, nil)
			if status != tc.status || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("status=%d: %s", status, got)
			}
		}
	}
}

func TestBankGatewayGuardsAndReadFailures(t *testing.T) {
	for _, action := range []string{"bank_deposit", "bank_withdraw"} {
		for _, condition := range []string{"sleep", "pending wake", "pending", "overdue pending", "overdue processing", "shared sleep", "shared wake", "character read", "scheduled read", "sleep read", "wake read", "shared sleep read"} {
			t.Run(action+"/"+condition, func(t *testing.T) {
				f := newBankGatewayFixture(t)
				timers := timer.NewService(nil)
				reader := gatewayTimers{TimerReader: timers}
				want := 409
				switch condition {
				case "sleep", "pending wake":
					category := timer.CategorySleep
					if condition == "pending wake" {
						category = timer.CategoryAsleep
					}
					if err := timers.SetLock(context.Background(), category, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				case "pending", "overdue pending", "overdue processing":
					a := scheduling.ScheduledAction{ID: "work", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}
					if condition != "pending" {
						a.ExecuteAt = time.Now().Add(-time.Hour)
					}
					if condition == "overdue processing" {
						a.State = scheduling.StateProcessing
					}
					f.actions = []scheduling.ScheduledAction{a}
				case "shared sleep":
					f.sleep.Sleeping = true
				case "shared wake":
					f.sleep.CanWake = true
				case "character read":
					f.characterErr = errors.New("secret character store")
					want = 500
				case "scheduled read":
					f.queryErr = errors.New("secret scheduled store")
					want = 500
				case "sleep read":
					reader.durationErr = errors.New("secret sleep store")
					want = 500
				case "wake read":
					reader.asleepErr = errors.New("secret wake store")
					want = 500
				case "shared sleep read":
					f.sleepErr = errors.New("secret shared sleep store")
					want = 500
				}
				before := f.char
				status, got := f.request(f.router(reader), action, `{"amount":1}`)
				if status != want || f.executions != 0 || f.char.Money != before.Money || f.char.Deposit != before.Deposit {
					t.Fatalf("status=%d executions=%d: %s", status, f.executions, got)
				}
				if want == 500 {
					assertGatewayError(t, got, "ACTION_PREFLIGHT_FAILED")
				} else {
					assertGatewayError(t, got, "ACTION_UNAVAILABLE")
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestBankGatewayExplicitErrorMappings(t *testing.T) {
	for _, action := range []string{"bank_deposit", "bank_withdraw"} {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{bank.ErrInvalidCharacterID, 400, "BANK_INVALID_CHARACTER_ID"},
			{bank.ErrInvalidAmount, 400, "BANK_INVALID_AMOUNT"},
			{bank.ErrInsufficientFunds, 400, "BANK_INSUFFICIENT_FUNDS"},
			{bank.ErrInsufficientBalance, 400, "BANK_INSUFFICIENT_BALANCE"},
			{bank.ErrDepositLimitExceeded, 400, "BANK_DEPOSIT_LIMIT_EXCEEDED"},
			{corecharacter.ErrNotFound, 404, "CHARACTER_NOT_FOUND"},
			{errors.New("secret ambiguous store failure"), 500, "EXECUTION_FAILED"},
		} {
			t.Run(action+"/"+tc.code, func(t *testing.T) {
				f := newBankGatewayFixture(t)
				f.repositoryErr = fmt.Errorf("wrapped: %w", tc.err)
				status, got := f.request(f.router(timer.NewService(nil)), action, `{"amount":1}`)
				if status != tc.status || f.char.Money != 100 || f.char.Deposit != 1000 {
					t.Fatalf("status=%d: %s", status, got)
				}
				f.assertCalls(action, 1)
				assertGatewayError(t, got, tc.code)
				if tc.status == 500 {
					if _, exists := got["context"]; exists || f.queryCalls != 1 || strings.Contains(string(got["error"]), "secret") {
						t.Fatalf("unknown outcome exposed context/error: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestBankGatewayUnconfigured(t *testing.T) {
	for _, action := range []string{"bank_deposit", "bank_withdraw"} {
		f := newBankGatewayFixture(t)
		h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
			WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil))), WithBank(nil))
		if err != nil {
			t.Fatal(err)
		}
		status, got := f.request(h.Router(), action, `{"amount":1}`)
		if status != 501 || f.executions != 0 || f.queryCalls != 0 {
			t.Fatalf("status=%d: %s", status, got)
		}
		assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
	}
}

package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type bankDepositRequest struct {
	Amount int64 `json:"amount"`
}

type bankWithdrawRequest struct {
	Amount int64 `json:"amount"`
}

// WithBank configures the BankService and its deposit/withdraw Gateway commands.
func WithBank(service BankService) Option {
	return func(h *Handler) {
		h.bank = service
		bankDepositCommand(service)(h)
		bankWithdrawCommand(service)(h)
	}
}

func bankDepositCommand(service BankService) Option {
	if service == nil {
		return withActionCommand[bankDepositRequest]("bank_deposit", nil, nil)
	}
	return withActionCommand("bank_deposit", func(ctx context.Context, actorID string, params bankDepositRequest) (any, error) {
		return service.Deposit(ctx, actorID, params.Amount)
	}, bankActionRejection)
}

func bankWithdrawCommand(service BankService) Option {
	if service == nil {
		return withActionCommand[bankWithdrawRequest]("bank_withdraw", nil, nil)
	}
	return withActionCommand("bank_withdraw", func(ctx context.Context, actorID string, params bankWithdrawRequest) (any, error) {
		return service.Withdraw(ctx, actorID, params.Amount)
	}, bankActionRejection)
}

func bankActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, bank.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "BANK_INVALID_CHARACTER_ID", Message: "Invalid bank character ID."}
	case errors.Is(err, bank.ErrInvalidAmount):
		return http.StatusBadRequest, ErrorDetail{Code: "BANK_INVALID_AMOUNT", Message: "Bank amount must be positive."}
	case errors.Is(err, bank.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "BANK_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, bank.ErrInsufficientBalance):
		return http.StatusBadRequest, ErrorDetail{Code: "BANK_INSUFFICIENT_BALANCE", Message: "Insufficient bank balance."}
	case errors.Is(err, bank.ErrDepositLimitExceeded):
		return http.StatusBadRequest, ErrorDetail{Code: "BANK_DEPOSIT_LIMIT_EXCEEDED", Message: "Bank deposit limit exceeded."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}

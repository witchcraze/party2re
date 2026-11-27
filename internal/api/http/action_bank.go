package http

import (
	"context"
	"errors"
	"fmt"
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

// BankDepositResult and BankWithdrawResult add the teller's reply, which names
// the requested amount even when a withdrawal is refunded (bank.cgi:58,89).
type BankDepositResult struct {
	bank.DepositResult
	Message string `json:"message"`
	commandEcho
}

type BankWithdrawResult struct {
	bank.WithdrawResult
	Message string `json:"message"`
	commandEcho
}

const bankDepositLimitReply = "これ以上お預かりできません"

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
		return withEchoCommand[bankDepositRequest, BankDepositResult]("bank_deposit", nil, nil)
	}
	return withEchoCommand("bank_deposit", func(ctx context.Context, actorID string, params bankDepositRequest) (BankDepositResult, error) {
		target := fmt.Sprintf("%dG", params.Amount)
		res, err := service.Deposit(ctx, actorID, params.Amount)
		if errors.Is(err, bank.ErrDepositLimitExceeded) {
			// Legacy answers the limit with a logged $npc_com that changes nothing (bank.cgi:51-53).
			state, err := service.GetState(ctx, actorID)
			if err != nil {
				return BankDepositResult{}, err
			}
			return BankDepositResult{bank.DepositResult{CharacterID: state.CharacterID, Money: state.Money, Deposit: state.Deposit}, bankDepositLimitReply, commandEcho{target: target, reply: bankDepositLimitReply}}, nil
		}
		if err != nil {
			return BankDepositResult{}, err
		}
		reply := fmt.Sprintf("%d Gお預かりいたしました", res.Amount)
		return BankDepositResult{res, reply, commandEcho{target: target, reply: reply}}, nil
	}, bankActionRejection)
}

func bankWithdrawCommand(service BankService) Option {
	if service == nil {
		return withEchoCommand[bankWithdrawRequest, BankWithdrawResult]("bank_withdraw", nil, nil)
	}
	return withEchoCommand("bank_withdraw", func(ctx context.Context, actorID string, params bankWithdrawRequest) (BankWithdrawResult, error) {
		res, err := service.Withdraw(ctx, actorID, params.Amount)
		if err != nil {
			return BankWithdrawResult{}, err
		}
		reply := fmt.Sprintf("%d Gお返しいたします", res.Amount)
		return BankWithdrawResult{res, reply, commandEcho{target: fmt.Sprintf("%dG", res.Amount), reply: reply}}, nil
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
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}

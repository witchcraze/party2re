package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/bank"
)

// BankService defines bank operations exposed over HTTP.
type BankService interface {
	GetState(ctx context.Context, characterID string) (bank.State, error)
	Deposit(ctx context.Context, characterID string, amount int64) (bank.DepositResult, error)
	Withdraw(ctx context.Context, characterID string, amount int64) (bank.WithdrawResult, error)
}

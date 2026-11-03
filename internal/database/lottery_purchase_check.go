package database

import "context"

// SetTicketPurchaseCheck adds buyer admission to the repository-owned purchase
// transaction after Rank 2 locking. Draw/prize delivery remains independent.
// Configure before serving requests.
func (r *LotteryRepository) SetTicketPurchaseCheck(check func(context.Context, string) error) {
	r.ticketPurchaseCheck = check
}

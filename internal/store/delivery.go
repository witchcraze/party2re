package store

import "context"

// DeliveryService writes receipts and recipient notices in the caller's transaction.
// The caller holds both characters' locks before invoking it.
type DeliveryService interface {
	HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error
	NotifyItem(ctx context.Context, recipientID, senderName, itemID string) error
}

func WithDeliveryService(delivery DeliveryService) Option {
	return func(s *Service) { s.delivery = delivery }
}

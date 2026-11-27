package http_test

import (
	"context"

	"github.com/witchcraze/party2re/internal/depot"
)

type stubDepotService struct{}

func (s *stubDepotService) GetDepot(ctx context.Context, characterID string) (depot.Depot, error) {
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) DepositItem(ctx context.Context, characterID, itemID string) (depot.Receipt, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: characterID, Capacity: 50}}, nil
}

func (s *stubDepotService) WithdrawItem(ctx context.Context, characterID, itemID string) (depot.Receipt, []depot.Remittance, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: characterID, Capacity: 50}}, nil, nil
}

func (s *stubDepotService) PendingRemittances(ctx context.Context, characterID string) ([]depot.Remittance, error) {
	return nil, nil
}

func (s *stubDepotService) SellItem(ctx context.Context, characterID, itemID string) (depot.Receipt, int, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: characterID, Capacity: 50}}, 250, nil
}

func (s *stubDepotService) SellItems(ctx context.Context, characterID string, itemIDs []string) (depot.Receipt, int, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: characterID, Capacity: 50}}, 500, nil
}

func (s *stubDepotService) SortItems(ctx context.Context, characterID string) (depot.Depot, error) {
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) Expand(ctx context.Context, characterID string) (depot.Depot, error) {
	return depot.Depot{CharacterID: characterID, Capacity: 55, ExDepot: 1}, nil
}

func (s *stubDepotService) SendMoney(ctx context.Context, fromID, toID string, amount int) (depot.Receipt, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: fromID, Capacity: 50}}, nil
}

func (s *stubDepotService) SendItem(ctx context.Context, fromID, toID, itemID string) (depot.Receipt, error) {
	return depot.Receipt{Depot: depot.Depot{CharacterID: fromID, Capacity: 50}}, nil
}

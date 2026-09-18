package party

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	battleadapter "github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type mockBattleSettler struct {
	err  error
	resp battleadapter.ApplyPostBattleResponse
}

func (m *mockBattleSettler) ApplyPostBattleResult(_ context.Context, _ battleadapter.ApplyPostBattleRequest) (battleadapter.ApplyPostBattleResponse, error) {
	if m.err != nil {
		return battleadapter.ApplyPostBattleResponse{}, m.err
	}
	return m.resp, nil
}

func TestSettleFallback_AddMoneyError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-1"
	c := corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 100,
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: -50, // Invalid negative gold triggers ErrInvalidAmount in AddMoney
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on negative gold in settleFallback, got nil")
	}
	if !strings.Contains(err.Error(), "adding gold for character") {
		t.Errorf("expected error message to contain 'adding gold for character', got: %v", err)
	}
}

func TestSettleFallback_AddCrystalError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-2"
	c := corecharacter.Character{
		ID:      charID,
		Name:    "Mage",
		Crystal: 10,
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalCrystals: -10, // Invalid negative crystals triggers ErrInvalidAmount in AddCrystal
		Outcome:       corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on negative crystals in settleFallback, got nil")
	}
	if !strings.Contains(err.Error(), "adding crystals for character") {
		t.Errorf("expected error message to contain 'adding crystals for character', got: %v", err)
	}
}

func TestSettleFallback_CharacterUpdateError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-3"
	c := corecharacter.Character{
		ID:   charID,
		Name: "Cleric",
	}
	charRepo.chars[charID] = c

	injectedErr := errors.New("db error updating character")
	charRepo.updateErr = injectedErr

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: 100,
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on character repo update failure, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error %v, got %v", injectedErr, err)
	}
}

func TestSettleWithBattleSettler_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	injectedErr := errors.New("battle adapter settlement failure")
	settler := &mockBattleSettler{err: injectedErr}

	svc, err := NewService(
		newMockPartyRepository(),
		charRepo,
		invRepo,
		stageProv,
		monsterProv,
		nil,
		WithPostBattleSettler(settler),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-4"
	c := corecharacter.Character{
		ID:   charID,
		Name: "Warrior",
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: 50,
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settlePostBattle(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error from battle settler, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error %v, got %v", injectedErr, err)
	}
}

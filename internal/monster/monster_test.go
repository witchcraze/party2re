package monster_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/monster"
)

type mockCharacterRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{
		characters: make(map[string]corecharacter.Character),
	}
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

type mockMonsterRepo struct {
	mu       sync.Mutex
	monsters map[string]monster.MonsterInstance
}

func newMockMonsterRepo() *mockMonsterRepo {
	return &mockMonsterRepo{
		monsters: make(map[string]monster.MonsterInstance),
	}
}

func (m *mockMonsterRepo) ListByCharacterID(ctx context.Context, characterID string) ([]monster.MonsterInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []monster.MonsterInstance
	for _, inst := range m.monsters {
		if inst.CharacterID == characterID {
			list = append(list, inst)
		}
	}
	return list, nil
}

func (m *mockMonsterRepo) ListByCharacterIDAndLocation(ctx context.Context, characterID, location string) ([]monster.MonsterInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []monster.MonsterInstance
	for _, inst := range m.monsters {
		if inst.CharacterID == characterID && inst.Location == location {
			list = append(list, inst)
		}
	}
	return list, nil
}

func (m *mockMonsterRepo) FindByID(ctx context.Context, id string) (monster.MonsterInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.monsters[id]
	if !ok {
		return monster.MonsterInstance{}, monster.ErrMonsterNotFound
	}
	return inst, nil
}

func (m *mockMonsterRepo) FindByIDForUpdate(ctx context.Context, id string) (monster.MonsterInstance, error) {
	return m.FindByID(ctx, id)
}

func (m *mockMonsterRepo) Save(ctx context.Context, inst monster.MonsterInstance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.monsters[inst.ID] = inst
	return nil
}

func (m *mockMonsterRepo) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.monsters, id)
	return nil
}

func (m *mockMonsterRepo) CountByLocation(ctx context.Context, characterID, location string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, inst := range m.monsters {
		if inst.CharacterID == characterID && inst.Location == location {
			count++
		}
	}
	return count, nil
}

type mockTxProvider struct{}

func (tp *mockTxProvider) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

func TestValidateMonsterName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid ascii", "Slime", false},
		{"valid japanese", "スライム", false},
		{"valid max length 8", "12345678", false},
		{"too long 9 chars", "123456789", true},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"contains space", "S lime", true},
		{"contains fullwidth space", "S　lime", true},
		{"contains comma", "S,lime", true},
		{"contains semicolon", "S;lime", true},
		{"contains single quote", "S'lime", true},
		{"contains double quote", "S\"lime", true},
		{"contains ampersand", "S&lime", true},
		{"contains brackets", "S<lime>", true},
		{"contains at sign", "@スライム", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monster.ValidateMonsterName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMonsterName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestBoxCapacityForCharacter(t *testing.T) {
	tests := []struct {
		overMonster int
		wantCap     int
	}{
		{0, 50},
		{1, 100},
		{2, 150},
		{3, 200},
		{4, 250},
		{5, 300},
		{6, 300}, // capped at tier 5
		{-1, 50}, // floored at tier 0
	}

	for _, tt := range tests {
		char := corecharacter.Character{OverMonster: tt.overMonster}
		got := monster.BoxCapacityForCharacter(char)
		if got != tt.wantCap {
			t.Errorf("BoxCapacityForCharacter(tier %d) = %d, want %d", tt.overMonster, got, tt.wantCap)
		}
	}
}

func TestMonsterService_TameAndSummary(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Hero", OverMonster: 0}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	// Initial summary
	summary, err := svc.GetSummary(ctx, "c1", "")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.BoxCount != 0 || summary.BoxCapacity != 50 || summary.HomeCount != 0 || summary.HomeCapacity != 8 {
		t.Errorf("unexpected initial summary: %+v", summary)
	}

	// Tame first monster
	inst, err := svc.TameMonster(ctx, "c1", "slime", "スラりん")
	if err != nil {
		t.Fatalf("TameMonster failed: %v", err)
	}
	if inst.CustomName != "スラりん" || inst.Location != monster.LocationBox {
		t.Errorf("unexpected tamed instance: %+v", inst)
	}

	// Tame with empty custom name defaults to monster ID
	inst2, err := svc.TameMonster(ctx, "c1", "goblin", "")
	if err != nil {
		t.Fatalf("TameMonster 2 failed: %v", err)
	}
	if inst2.CustomName != "goblin" {
		t.Errorf("expected default custom name 'goblin', got %q", inst2.CustomName)
	}

	summary, err = svc.GetSummary(ctx, "c1", monster.LocationBox)
	if err != nil {
		t.Fatalf("GetSummary box filter failed: %v", err)
	}
	if summary.BoxCount != 2 || len(summary.Monsters) != 2 {
		t.Errorf("expected 2 monsters in box, got %d (list len %d)", summary.BoxCount, len(summary.Monsters))
	}
}

func TestMonsterService_BringToHomeAndDeposit(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Hero", OverMonster: 0}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	inst1, _ := svc.TameMonster(ctx, "c1", "slime", "スラりん")
	inst2, _ := svc.TameMonster(ctx, "c1", "slime", "スラりん2")

	// Bring to home
	homeInst, err := svc.BringToHome(ctx, "c1", inst1.ID)
	if err != nil {
		t.Fatalf("BringToHome failed: %v", err)
	}
	if homeInst.Location != monster.LocationHome {
		t.Errorf("expected location home, got %s", homeInst.Location)
	}

	// Duplicate name check at home: rename inst2 to "スラりん" and try to bring to home
	_, err = svc.Rename(ctx, "c1", inst2.ID, "スラりん")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	_, err = svc.BringToHome(ctx, "c1", inst2.ID)
	if err != monster.ErrDuplicatePetNameAtHome {
		t.Errorf("expected ErrDuplicatePetNameAtHome, got %v", err)
	}

	// Rename back and bring to home
	_, err = svc.Rename(ctx, "c1", inst2.ID, "スラきち")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	_, err = svc.BringToHome(ctx, "c1", inst2.ID)
	if err != nil {
		t.Fatalf("BringToHome inst2 failed: %v", err)
	}

	// Summary check
	summary, err := svc.GetSummary(ctx, "c1", "")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.BoxCount != 0 || summary.HomeCount != 2 {
		t.Errorf("expected 0 box, 2 home, got box %d, home %d", summary.BoxCount, summary.HomeCount)
	}

	// Deposit inst1 back to box
	boxInst, err := svc.DepositToBox(ctx, "c1", inst1.ID)
	if err != nil {
		t.Fatalf("DepositToBox failed: %v", err)
	}
	if boxInst.Location != monster.LocationBox {
		t.Errorf("expected location box, got %s", boxInst.Location)
	}
}

func TestMonsterService_MaxCapacityLimits(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Hero", OverMonster: 0}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	// Fill home with 8 pets
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("pet%d", i)
		inst, err := svc.TameMonster(ctx, "c1", "slime", name)
		if err != nil {
			t.Fatalf("TameMonster failed at %d: %v", i, err)
		}
		_, err = svc.BringToHome(ctx, "c1", inst.ID)
		if err != nil {
			t.Fatalf("BringToHome failed at %d: %v", i, err)
		}
	}

	// 9th pet cannot be brought home
	inst9, _ := svc.TameMonster(ctx, "c1", "slime", "pet9")
	_, err := svc.BringToHome(ctx, "c1", inst9.ID)
	if err != monster.ErrHomeFull {
		t.Errorf("expected ErrHomeFull, got %v", err)
	}

	// Fill box to 50 (inst9 is already 1st in box, add 49 more)
	for i := 1; i <= 49; i++ {
		name := fmt.Sprintf("box%d", i)
		_, err := svc.TameMonster(ctx, "c1", "slime", name)
		if err != nil {
			t.Fatalf("TameMonster failed at %d: %v", i, err)
		}
	}

	// 51st taming into box fails with ErrBoxFull
	_, err = svc.TameMonster(ctx, "c1", "slime", "box50")
	if err != monster.ErrBoxFull {
		t.Errorf("expected ErrBoxFull, got %v", err)
	}
}

func TestMonsterService_SendAndRelease(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Alice"}
	charRepo.characters["c2"] = corecharacter.Character{ID: "c2", Name: "Bob"}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	inst, err := svc.TameMonster(ctx, "c1", "dragon", "ドラごん")
	if err != nil {
		t.Fatalf("TameMonster failed: %v", err)
	}

	// Cannot send to self
	err = svc.SendMonster(ctx, "c1", "c1", inst.ID)
	if err != monster.ErrCannotSendToSelf {
		t.Errorf("expected ErrCannotSendToSelf, got %v", err)
	}

	// Cannot send to non-existent recipient
	err = svc.SendMonster(ctx, "c1", "c999", inst.ID)
	if err != monster.ErrRecipientNotFound {
		t.Errorf("expected ErrRecipientNotFound, got %v", err)
	}

	// Send to Bob
	err = svc.SendMonster(ctx, "c1", "c2", inst.ID)
	if err != nil {
		t.Fatalf("SendMonster failed: %v", err)
	}

	// Verify Bob owns the dragon in box
	bobSummary, err := svc.GetSummary(ctx, "c2", "")
	if err != nil {
		t.Fatalf("GetSummary for Bob failed: %v", err)
	}
	if bobSummary.BoxCount != 1 || len(bobSummary.Monsters) != 1 || bobSummary.Monsters[0].CharacterID != "c2" {
		t.Errorf("unexpected Bob summary: %+v", bobSummary)
	}

	// Alice summary is empty
	aliceSummary, _ := svc.GetSummary(ctx, "c1", "")
	if aliceSummary.BoxCount != 0 {
		t.Errorf("expected Alice box 0, got %d", aliceSummary.BoxCount)
	}

	// Bob releases the dragon
	err = svc.ReleaseMonster(ctx, "c2", inst.ID)
	if err != nil {
		t.Fatalf("ReleaseMonster failed: %v", err)
	}

	bobSummary, _ = svc.GetSummary(ctx, "c2", "")
	if bobSummary.BoxCount != 0 {
		t.Errorf("expected Bob box 0 after release, got %d", bobSummary.BoxCount)
	}
}

func TestMonsterService_GetDialogue(t *testing.T) {
	svc := monster.NewService(newMockCharacterRepo(), newMockMonsterRepo())
	d := svc.GetDialogue()
	if d.NPCName != "@モンジィ" || d.Title != "モンスターじいさん" || len(d.Phrases) == 0 {
		t.Errorf("unexpected dialogue: %+v", d)
	}
}

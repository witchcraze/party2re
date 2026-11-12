package monster_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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

type mockHomeCompanionStore struct {
	mu         sync.Mutex
	companions map[string]monster.HomeCompanion
	deleteErr  error
}

func newMockHomeCompanionStore() *mockHomeCompanionStore {
	return &mockHomeCompanionStore{
		companions: make(map[string]monster.HomeCompanion),
	}
}

func (s *mockHomeCompanionStore) ListHomeCompanions(_ context.Context, characterID string) ([]monster.HomeCompanion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []monster.HomeCompanion
	for _, c := range s.companions {
		if c.CharacterID == characterID {
			res = append(res, c)
		}
	}
	return res, nil
}

func (s *mockHomeCompanionStore) FindHomeCompanionForUpdate(_ context.Context, id string) (monster.HomeCompanion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.companions[id]
	if !ok {
		return monster.HomeCompanion{}, monster.ErrMonsterNotFound
	}
	return c, nil
}

func (s *mockHomeCompanionStore) DeleteHomeCompanion(_ context.Context, characterID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	c, ok := s.companions[id]
	if !ok || c.CharacterID != characterID {
		return monster.ErrMonsterNotFound
	}
	delete(s.companions, id)
	return nil
}

func TestValidateMonsterName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"valid ascii", "Slime", nil},
		{"valid japanese", "スライム", nil},
		{"valid ascii with at mark", "@スライム", nil},
		{"valid ascii at sign internal", "Slime@1", nil},
		{"valid max length 8 ascii", "12345678", nil},
		{"valid max length 8 japanese", "一二三四五六七八", nil},
		{"too long 9 chars ascii", "123456789", monster.ErrNameTooLong},
		{"too long 9 chars japanese", "一二三四五六七八九", monster.ErrNameTooLong},
		{"empty string", "", monster.ErrInvalidName},
		{"whitespace only", "   ", monster.ErrInvalidName},
		{"leading space", " Slime", monster.ErrInvalidName},
		{"trailing space", "Slime ", monster.ErrInvalidName},
		{"leading and trailing spaces", " Slime ", monster.ErrInvalidName},
		{"leading fullwidth space", "　スライム", monster.ErrInvalidName},
		{"trailing fullwidth space", "スライム　", monster.ErrInvalidName},
		{"contains space", "S lime", monster.ErrInvalidName},
		{"contains tab", "S\tlime", monster.ErrInvalidName},
		{"contains fullwidth space", "S　lime", monster.ErrInvalidName},
		{"contains comma", "S,lime", monster.ErrInvalidName},
		{"contains semicolon", "S;lime", monster.ErrInvalidName},
		{"contains single quote", "S'lime", monster.ErrInvalidName},
		{"contains double quote", "S\"lime", monster.ErrInvalidName},
		{"contains ampersand", "S&lime", monster.ErrInvalidName},
		{"contains brackets", "S<lime>", monster.ErrInvalidName},
		{"contains fullwidth at sign", "＠スライム", monster.ErrInvalidName},
		{"contains fullwidth at sign inside", "スラ＠りん", monster.ErrInvalidName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monster.ValidateMonsterName(tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateMonsterName(%q) unexpected error = %v", tt.input, err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateMonsterName(%q) error = %v, want %v", tt.input, err, tt.wantErr)
				}
			}
		})
	}
}

func TestMonsterService_Rename_Validation(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Hero"}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	inst, err := svc.TameMonster(ctx, "c1", "slime", "スラりん")
	if err != nil {
		t.Fatalf("TameMonster failed: %v", err)
	}

	invalidCases := []struct {
		name    string
		newName string
		wantErr error
	}{
		{"leading whitespace", " スラりん", monster.ErrInvalidName},
		{"trailing whitespace", "スラりん ", monster.ErrInvalidName},
		{"surrounding whitespace", "  スラりん  ", monster.ErrInvalidName},
		{"fullwidth at symbol", "＠スラりん", monster.ErrInvalidName},
		{"nine runes japanese", "一二三四五六七八九", monster.ErrNameTooLong},
		{"forbidden comma", "スラ,りん", monster.ErrInvalidName},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Rename(ctx, "c1", inst.ID, tc.newName)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Rename(%q) error = %v, want %v", tc.newName, err, tc.wantErr)
			}

			// Verify in storage that custom name was not modified
			stored, err := monRepo.FindByID(ctx, inst.ID)
			if err != nil {
				t.Fatalf("FindByID failed: %v", err)
			}
			if stored.CustomName != "スラりん" {
				t.Fatalf("CustomName modified after failed rename: got %q, want %q", stored.CustomName, "スラりん")
			}
		})
	}

	// Valid rename with ASCII @
	updated, err := svc.Rename(ctx, "c1", inst.ID, "@スラりん")
	if err != nil {
		t.Fatalf("Rename with ASCII @ failed: %v", err)
	}
	if updated.CustomName != "@スラりん" {
		t.Fatalf("updated CustomName = %q, want %q", updated.CustomName, "@スラりん")
	}

	stored, err := monRepo.FindByID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if stored.CustomName != "@スラりん" {
		t.Fatalf("stored CustomName = %q, want %q", stored.CustomName, "@スラりん")
	}
}

func TestBoxCapacityForCharacter(t *testing.T) {
	tests := []struct {
		name        string
		jobLevel    int
		level       int
		overMonster int
		wantCap     int
	}{
		{"standard base", 0, 50, 0, 50},
		{"standard tier 1", 0, 50, 1, 100},
		{"standard tier 2", 0, 50, 2, 150},
		{"standard tier 3", 0, 50, 3, 200},
		{"standard tier 4", 0, 50, 4, 250},
		{"standard tier 5 (max)", 0, 50, 5, 300},
		{"standard tier 6 (clamped)", 0, 50, 6, 300},
		{"negative tier (floored)", 0, 50, -1, 50},
		{"high level but low job level does not grant veteran bonus", 0, 100, 0, 50},
		{"high level 99 and low job level", 0, 99, 0, 50},
		{"veteran job level 100 with level 1", 100, 1, 0, 100},
		{"veteran job level 100 with level 1 and tier 5 max", 100, 1, 5, 350},
		{"veteran job level 120 with level 50 and tier 2", 120, 50, 2, 200},
		{"veteran job level 100 with level 1 and tier 6 (clamped)", 100, 1, 6, 350},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := corecharacter.Character{
				JobLevel:    tt.jobLevel,
				Level:       tt.level,
				OverMonster: tt.overMonster,
			}
			got := monster.BoxCapacityForCharacter(char)
			if got != tt.wantCap {
				t.Errorf("BoxCapacityForCharacter(%+v) = %d, want %d", char, got, tt.wantCap)
			}
		})
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
	if summary.BoxCount != 0 || summary.BoxCapacity != 50 || summary.HomeCount != 0 || summary.HomeCapacity != 50 {
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

	// Home capacity equals the box capacity (50 at tier 0)
	for i := 1; i <= 50; i++ {
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

	// 51st pet cannot be brought home
	inst9, _ := svc.TameMonster(ctx, "c1", "slime", "pet51")
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

// Gift capacity counts the recipient's box against the sender's limit (legacy get_monster_c).
func TestMonsterService_SendCapacityUsesSenderLimit(t *testing.T) {
	recipientSetups := map[string]corecharacter.Character{
		"novice":  {JobLevel: 99, OverMonster: 0},
		"veteran": {JobLevel: 100, OverMonster: 5},
	}
	senders := []struct {
		name               string
		jobLevel, overTier int
		limit              int
	}{
		{"JL99/OM0", 99, 0, 50},
		{"JL99/OM5", 99, 5, 300},
		{"JL100/OM0", 100, 0, 100},
		{"JL100/OM5", 100, 5, 350},
	}
	for recipientName, recipient := range recipientSetups {
		for _, sender := range senders {
			for _, tc := range []struct {
				boxCount int
				wantErr  error
			}{{sender.limit - 1, nil}, {sender.limit, monster.ErrRecipientBoxFull}} {
				t.Run(fmt.Sprintf("%s to %s with %d", sender.name, recipientName, tc.boxCount), func(t *testing.T) {
					ctx := context.Background()
					charRepo := newMockCharacterRepo()
					monRepo := newMockMonsterRepo()
					charRepo.characters["s"] = corecharacter.Character{ID: "s", JobLevel: sender.jobLevel, OverMonster: sender.overTier}
					r := recipient
					r.ID = "r"
					charRepo.characters["r"] = r
					for i := 0; i < tc.boxCount; i++ {
						id := fmt.Sprintf("r%d", i)
						monRepo.monsters[id] = monster.MonsterInstance{ID: id, CharacterID: "r", MonsterID: "slime", CustomName: id, Location: monster.LocationBox}
					}
					monRepo.monsters["gift"] = monster.MonsterInstance{ID: "gift", CharacterID: "s", MonsterID: "slime", CustomName: "gift", Location: monster.LocationBox}
					svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

					if err := svc.SendMonster(ctx, "s", "r", "gift"); !errors.Is(err, tc.wantErr) {
						t.Fatalf("SendMonster: got %v, want %v", err, tc.wantErr)
					}
					wantOwner := "r"
					if tc.wantErr != nil {
						wantOwner = "s"
					}
					if got := monRepo.monsters["gift"].CharacterID; got != wantOwner {
						t.Fatalf("gift owner = %s, want %s", got, wantOwner)
					}
				})
			}
		}
	}
}

func TestMonsterService_LocationAndOwnershipMatrix(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Alice", OverMonster: 0}
	charRepo.characters["c2"] = corecharacter.Character{ID: "c2", Name: "Bob", OverMonster: 0}

	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(&mockTxProvider{}))

	// Create a home pet for c1 and fill c1's box to capacity (50) to verify full-box bypass is blocked
	homePet, err := svc.TameMonster(ctx, "c1", "slime", "家スラ")
	if err != nil {
		t.Fatalf("TameMonster homePet failed: %v", err)
	}
	if _, err := svc.BringToHome(ctx, "c1", homePet.ID); err != nil {
		t.Fatalf("BringToHome failed: %v", err)
	}
	for i := 0; i < monster.BaseBoxCapacity; i++ {
		if _, err := svc.TameMonster(ctx, "c1", "slime", fmt.Sprintf("箱%d", i)); err != nil {
			t.Fatalf("TameMonster box fill %d failed: %v", i, err)
		}
	}

	// Verify DepositToBox fails with ErrBoxFull when box is full
	if _, err := svc.DepositToBox(ctx, "c1", homePet.ID); !errors.Is(err, monster.ErrBoxFull) {
		t.Fatalf("DepositToBox on full box: got %v, want %v", err, monster.ErrBoxFull)
	}

	// Create a boxed monster for c2
	otherBoxMon, err := svc.TameMonster(ctx, "c2", "goblin", "他箱ゴブ")
	if err != nil {
		t.Fatalf("TameMonster otherBoxMon failed: %v", err)
	}

	// 1. Owned LocationHome is rejected with ErrInvalidLocation across Rename, SendMonster, ReleaseMonster
	if _, err := svc.Rename(ctx, "c1", homePet.ID, "改名不可"); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("Rename on home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}
	if err := svc.SendMonster(ctx, "c1", "c2", homePet.ID); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("SendMonster on home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}
	if err := svc.ReleaseMonster(ctx, "c1", homePet.ID); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("ReleaseMonster on home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}

	// Verify home pet state was not mutated
	storedHome, err := monRepo.FindByID(ctx, homePet.ID)
	if err != nil {
		t.Fatalf("FindByID homePet failed: %v", err)
	}
	if storedHome.CharacterID != "c1" || storedHome.Location != monster.LocationHome || storedHome.CustomName != "家スラ" {
		t.Fatalf("homePet state mutated after rejected ops: %+v", storedHome)
	}

	// 2. Unowned monster (both LocationHome and LocationBox) is rejected with ErrForbidden
	if _, err := svc.Rename(ctx, "c2", homePet.ID, "乗っ取り"); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("Rename unowned home pet: got %v, want %v", err, monster.ErrForbidden)
	}
	if _, err := svc.Rename(ctx, "c1", otherBoxMon.ID, "乗っ取り"); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("Rename unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}
	if err := svc.SendMonster(ctx, "c1", "c2", otherBoxMon.ID); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("SendMonster unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}
	if err := svc.ReleaseMonster(ctx, "c1", otherBoxMon.ID); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("ReleaseMonster unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}

	storedOther, err := monRepo.FindByID(ctx, otherBoxMon.ID)
	if err != nil {
		t.Fatalf("FindByID otherBoxMon failed: %v", err)
	}
	if storedOther.CharacterID != "c2" || storedOther.Location != monster.LocationBox || storedOther.CustomName != "他箱ゴブ" {
		t.Fatalf("otherBoxMon state mutated after rejected ops: %+v", storedOther)
	}
}

func TestMonsterService_GetDialogue(t *testing.T) {
	svc := monster.NewService(newMockCharacterRepo(), newMockMonsterRepo())
	d := svc.GetDialogue()
	if d.NPCName != "@モンジィ" || d.Title != "モンスターじいさん" || len(d.Phrases) == 0 {
		t.Errorf("unexpected dialogue: %+v", d)
	}

	foundCapacityPhrase := false
	for _, phrase := range d.Phrases {
		if strings.Contains(phrase, "転職100回以上で100匹") && strings.Contains(phrase, "最大350匹") {
			foundCapacityPhrase = true
			break
		}
	}
	if !foundCapacityPhrase {
		t.Errorf("expected dialogue phrase explaining 100+ reclass and 350 max capacity, got: %+v", d.Phrases)
	}
}

func TestMonsterService_GodHomeCompanions(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	monRepo := newMockMonsterRepo()
	compStore := newMockHomeCompanionStore()
	charRepo.characters["c1"] = corecharacter.Character{ID: "c1", Name: "Alice", OverMonster: 0}
	charRepo.characters["c2"] = corecharacter.Character{ID: "c2", Name: "Bob", OverMonster: 0}

	svc := monster.NewService(
		charRepo,
		monRepo,
		monster.WithTransactionProvider(&mockTxProvider{}),
		monster.WithHomeCompanionStore(compStore),
	)

	createdAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	compStore.companions["comp-ortega"] = monster.HomeCompanion{
		ID:          "comp-ortega",
		CharacterID: "c1",
		MonsterID:   "chr/029.gif",
		CustomName:  "オルテガ",
		CreatedAt:   createdAt,
	}
	compStore.companions["comp-cat"] = monster.HomeCompanion{
		ID:          "comp-cat",
		CharacterID: "c1",
		MonsterID:   "mon/211.gif",
		CustomName:  "白猫",
		CreatedAt:   createdAt,
	}

	// 1. GetSummary reflects God companions in HomeCount and Monsters
	summaryAll, err := svc.GetSummary(ctx, "c1", "")
	if err != nil {
		t.Fatalf("GetSummary all failed: %v", err)
	}
	if summaryAll.HomeCount != 2 || summaryAll.BoxCount != 0 || len(summaryAll.Monsters) != 2 {
		t.Fatalf("unexpected GetSummary all: %+v", summaryAll)
	}
	summaryHome, err := svc.GetSummary(ctx, "c1", monster.LocationHome)
	if err != nil {
		t.Fatalf("GetSummary home failed: %v", err)
	}
	if summaryHome.HomeCount != 2 || len(summaryHome.Monsters) != 2 {
		t.Fatalf("unexpected GetSummary home: %+v", summaryHome)
	}
	summaryBox, err := svc.GetSummary(ctx, "c1", monster.LocationBox)
	if err != nil {
		t.Fatalf("GetSummary box failed: %v", err)
	}
	if summaryBox.HomeCount != 2 || len(summaryBox.Monsters) != 0 {
		t.Fatalf("unexpected GetSummary box: %+v", summaryBox)
	}

	// 2. Duplicate name collision against God companion at home is rejected
	dupMon, err := svc.TameMonster(ctx, "c1", "slime", "オルテガ")
	if err != nil {
		t.Fatalf("TameMonster dupMon failed: %v", err)
	}
	if _, err := svc.BringToHome(ctx, "c1", dupMon.ID); !errors.Is(err, monster.ErrDuplicatePetNameAtHome) {
		t.Fatalf("BringToHome colliding with God companion name: got %v, want %v", err, monster.ErrDuplicatePetNameAtHome)
	}

	// 3. Mixed capacity enforcement: 2 God companions + 48 character_monsters = 50 total -> 51st rejected with ErrHomeFull
	for i := 1; i <= 48; i++ {
		m, err := svc.TameMonster(ctx, "c1", "slime", fmt.Sprintf("ペット%d", i))
		if err != nil {
			t.Fatalf("TameMonster %d failed: %v", i, err)
		}
		if _, err := svc.BringToHome(ctx, "c1", m.ID); err != nil {
			t.Fatalf("BringToHome %d failed: %v", i, err)
		}
	}
	extraMon, err := svc.TameMonster(ctx, "c1", "slime", "ペット49")
	if err != nil {
		t.Fatalf("TameMonster extraMon failed: %v", err)
	}
	if _, err := svc.BringToHome(ctx, "c1", extraMon.ID); !errors.Is(err, monster.ErrHomeFull) {
		t.Fatalf("BringToHome 51st combined pet: got %v, want %v", err, monster.ErrHomeFull)
	}

	// 4. Unowned God companion deposit is rejected with ErrForbidden
	if _, err := svc.DepositToBox(ctx, "c2", "comp-ortega"); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("DepositToBox unowned God companion: got %v, want %v", err, monster.ErrForbidden)
	}

	// 5. Full box blocks depositing a God companion without deleting it from home_members
	// Currently c1 has 2 monsters in box (dupMon, extraMon); add 48 more to reach 50
	var tempBoxIDs []string
	for i := 1; i <= 48; i++ {
		m, err := svc.TameMonster(ctx, "c1", "slime", fmt.Sprintf("箱満%d", i))
		if err != nil {
			t.Fatalf("TameMonster fill %d failed: %v", i, err)
		}
		tempBoxIDs = append(tempBoxIDs, m.ID)
	}
	if _, err := svc.DepositToBox(ctx, "c1", "comp-ortega"); !errors.Is(err, monster.ErrBoxFull) {
		t.Fatalf("DepositToBox God companion on full box: got %v, want %v", err, monster.ErrBoxFull)
	}
	if _, ok := compStore.companions["comp-ortega"]; !ok {
		t.Fatalf("comp-ortega was removed from HomeCompanionStore despite ErrBoxFull")
	}
	if _, err := monRepo.FindByID(ctx, "comp-ortega"); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("comp-ortega should not exist in character_monsters after ErrBoxFull, got %v", err)
	}

	// Free one box slot
	if err := svc.ReleaseMonster(ctx, "c1", tempBoxIDs[0]); err != nil {
		t.Fatalf("ReleaseMonster failed: %v", err)
	}

	// 6. DepositToBox moves God companion from HomeCompanionStore to character_monsters (location=box),
	// preserving ID, CharacterID, MonsterID (icon), CustomName, and CreatedAt.
	deposited, err := svc.DepositToBox(ctx, "c1", "comp-ortega")
	if err != nil {
		t.Fatalf("DepositToBox comp-ortega failed: %v", err)
	}
	if deposited.ID != "comp-ortega" || deposited.CharacterID != "c1" || deposited.MonsterID != "chr/029.gif" || deposited.CustomName != "オルテガ" || deposited.Location != monster.LocationBox || !deposited.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected deposited companion: %+v", deposited)
	}
	if _, ok := compStore.companions["comp-ortega"]; ok {
		t.Fatalf("comp-ortega still present in HomeCompanionStore after DepositToBox")
	}

	// 7. BringToHome brings the deposited God companion back to home, preserving identity, icon, and ownership
	broughtBack, err := svc.BringToHome(ctx, "c1", "comp-ortega")
	if err != nil {
		t.Fatalf("BringToHome comp-ortega failed: %v", err)
	}
	if broughtBack.ID != "comp-ortega" || broughtBack.CharacterID != "c1" || broughtBack.MonsterID != "chr/029.gif" || broughtBack.CustomName != "オルテガ" || broughtBack.Location != monster.LocationHome {
		t.Fatalf("unexpected broughtBack companion: %+v", broughtBack)
	}
}

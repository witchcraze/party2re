package home

import (
	"context"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockCostumeApplier struct {
	applied []appliedCostume
}

type appliedCostume struct {
	characterID string
	itemNo      int
	itemName    string
	icon        string
	expiresAt   time.Time
}

func (m *mockCostumeApplier) ApplyCostume(_ context.Context, charID string, itemNo int, itemName, icon string, exp time.Time) error {
	m.applied = append(m.applied, appliedCostume{
		characterID: charID,
		itemNo:      itemNo,
		itemName:    itemName,
		icon:        icon,
		expiresAt:   exp,
	})
	return nil
}

func TestUseHomeItem_CostumeConsumption(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		itemName   string
		gender     string
		wantItemNo int
		wantIcon   string
		wantMsgSub string
	}{
		{
			name:       "Pink skirt (44)",
			itemName:   "ピンクスカート",
			gender:     "female",
			wantItemNo: 44,
			wantIcon:   "chr/001.gif",
			wantMsgSub: "ピンクスカートのコスプレをした！",
		},
		{
			name:       "Butler costume (46) - Male",
			itemName:   "チョビヒゲタクシード",
			gender:     "male",
			wantItemNo: 46,
			wantIcon:   "chr/012.gif",
			wantMsgSub: "チョビヒゲタクシードのコスプレをした！",
		},
		{
			name:       "Maid costume (46) - Female",
			itemName:   "チョビヒゲタクシード",
			gender:     "female",
			wantItemNo: 46,
			wantIcon:   "chr/007.gif",
			wantMsgSub: "チョビヒゲタクシードのコスプレをした！",
		},
		{
			name:       "Cleric costume (55) - Male",
			itemName:   "聖職者の衣装",
			gender:     "m",
			wantItemNo: 55,
			wantIcon:   "chr/016.gif",
			wantMsgSub: "聖職者のコスプレをした！",
		},
		{
			name:       "Cleric costume (55) - Female",
			itemName:   "聖職者の衣装",
			gender:     "f",
			wantItemNo: 55,
			wantIcon:   "chr/017.gif",
			wantMsgSub: "聖職者のコスプレをした！",
		},
		{
			name:       "Royal costume (56) - Male",
			itemName:   "王族の衣装",
			gender:     "m",
			wantItemNo: 56,
			wantIcon:   "chr/002.gif",
			wantMsgSub: "王様のコスプレをした！",
		},
		{
			name:       "Royal costume (56) - Female",
			itemName:   "王族の衣装",
			gender:     "f",
			wantItemNo: 56,
			wantIcon:   "chr/018.gif",
			wantMsgSub: "王様のコスプレをした！",
		},
		{
			name:       "Hero costume (140) - Male",
			itemName:   "英雄の衣装",
			gender:     "male",
			wantItemNo: 140,
			wantIcon:   "chr/034.gif",
			wantMsgSub: "英雄のコスプレをした！",
		},
		{
			name:       "Hero costume (140) - Female",
			itemName:   "英雄の衣装",
			gender:     "female",
			wantItemNo: 140,
			wantIcon:   "chr/028.gif",
			wantMsgSub: "英雄のコスプレをした！",
		},
		{
			name:       "Transformation scroll (141)",
			itemName:   "変身の巻物",
			gender:     "male",
			wantItemNo: 141,
			wantIcon:   "chr/", // prefix matches random roll
			wantMsgSub: "変身の巻物を読んだ！",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := corecharacter.Character{
				ID:     "char-1",
				Name:   "テストキャラ",
				Gender: tt.gender,
			}
			charsMap := map[string]corecharacter.Character{"char-1": char}
			charReader := &mockCharReader{chars: charsMap}
			charUpdater := &mockCharUpdater{chars: charsMap}
			homeRepo := &mockHomeRepo{
				homes: make(map[string]CharacterHome),
			}
			applier := &mockCostumeApplier{}

			defID := "item-test"
			catalog := &mockCatalog{defs: map[string]coreitem.Definition{
				defID: {
					ID:            defID,
					Name:          tt.itemName,
					UsageCategory: coreitem.UsageCategoryAnytime,
				},
			}}

			invMgr := &mockInventoryManager{
				invs: make(map[string]coreinventory.Inventory),
			}
			inv, _ := coreinventory.New("char-1")
			inst, _ := coreitem.NewInstance(defID, 1)
			_ = inv.Add(inst)
			invMgr.invs["char-1"] = inv

			svc, err := NewService(
				homeRepo,
				charReader,
				WithCharacterUpdater(charUpdater),
				WithInventoryManager(invMgr),
				WithItemCatalog(catalog),
				WithCostumeApplier(applier),
			)
			if err != nil {
				t.Fatalf("NewService failed: %v", err)
			}

			res, err := svc.UseHomeItem(ctx, "char-1", inst.ID, "inventory")
			if err != nil {
				t.Fatalf("UseHomeItem failed: %v", err)
			}

			if !res.Consumed {
				t.Errorf("expected item to be consumed")
			}
			if !strings.Contains(res.Message, tt.wantMsgSub) {
				t.Errorf("expected message to contain %q, got %q", tt.wantMsgSub, res.Message)
			}

			if len(applier.applied) != 1 {
				t.Fatalf("expected 1 applied costume, got %d", len(applier.applied))
			}
			applied := applier.applied[0]
			if applied.itemNo != tt.wantItemNo {
				t.Errorf("expected itemNo %d, got %d", tt.wantItemNo, applied.itemNo)
			}
			if !strings.HasPrefix(applied.icon, tt.wantIcon) {
				t.Errorf("expected icon prefix %q, got %q", tt.wantIcon, applied.icon)
			}
		})
	}
}

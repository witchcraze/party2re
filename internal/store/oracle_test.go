package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/store"
)

type mockHomeWallpaperRepo struct {
	wallpapers map[string]string
}

func (m *mockHomeWallpaperRepo) UpdateHomeWallpaper(_ context.Context, charID string, wallpaper string) error {
	if m.wallpapers == nil {
		m.wallpapers = make(map[string]string)
	}
	m.wallpapers[charID] = wallpaper
	return nil
}

func TestAvailableCostumes_LegacyGatingParity(t *testing.T) {
	tests := []struct {
		name      string
		jobLevel  int
		wantCount int
		wantItems []int
		unwanted  []int
	}{
		{
			name:      "JobLevel 0 has 2 items (44..45)",
			jobLevel:  0,
			wantCount: 2,
			wantItems: []int{44, 45},
			unwanted:  []int{46, 56, 138, 141},
		},
		{
			name:      "JobLevel 1 has 3 items (44..46)",
			jobLevel:  1,
			wantCount: 3,
			wantItems: []int{44, 45, 46},
			unwanted:  []int{47, 56, 138},
		},
		{
			name:      "JobLevel 5 has 7 items (44..50)",
			jobLevel:  5,
			wantCount: 7,
			wantItems: []int{44, 45, 46, 47, 48, 49, 50},
			unwanted:  []int{51, 56, 138},
		},
		{
			name:      "JobLevel 10 has 12 items (44..55)",
			jobLevel:  10,
			wantCount: 12,
			wantItems: []int{44, 55},
			unwanted:  []int{56, 138},
		},
		{
			name:      "JobLevel 11 has all 13 base items (44..56)",
			jobLevel:  11,
			wantCount: 13,
			wantItems: []int{44, 56},
			unwanted:  []int{138, 141},
		},
		{
			name:      "JobLevel 15 has all 13 base items (not yet extended)",
			jobLevel:  15,
			wantCount: 13,
			wantItems: []int{44, 56},
			unwanted:  []int{138, 141},
		},
		{
			name:      "JobLevel 16 has 17 items (44..56 and 138..141)",
			jobLevel:  16,
			wantCount: 17,
			wantItems: []int{44, 56, 138, 139, 140, 141},
			unwanted:  []int{},
		},
		{
			name:      "JobLevel 50 has 17 items",
			jobLevel:  50,
			wantCount: 17,
			wantItems: []int{44, 56, 138, 141},
			unwanted:  []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.AvailableCostumes(tt.jobLevel)
			if len(got) != tt.wantCount {
				t.Fatalf("AvailableCostumes(%d) returned %d items, want %d", tt.jobLevel, len(got), tt.wantCount)
			}

			itemsMap := make(map[int]store.OracleItem)
			for _, it := range got {
				itemsMap[it.ItemNo] = it
			}

			for _, wantNo := range tt.wantItems {
				if _, ok := itemsMap[wantNo]; !ok {
					t.Errorf("expected item %d to be in available costumes for job_lv %d", wantNo, tt.jobLevel)
				}
			}

			for _, unwantedNo := range tt.unwanted {
				if _, ok := itemsMap[unwantedNo]; ok {
					t.Errorf("item %d should NOT be available for job_lv %d", unwantedNo, tt.jobLevel)
				}
			}
		})
	}
}

func TestCostumeIcon_GenderParity(t *testing.T) {
	tests := []struct {
		itemNo   int
		gender   string
		wantIcon string
	}{
		{itemNo: 44, gender: "male", wantIcon: "chr/001.gif"},
		{itemNo: 44, gender: "female", wantIcon: "chr/001.gif"},
		// 46: チョビヒゲタクシード - male 012, female 007
		{itemNo: 46, gender: "male", wantIcon: "chr/012.gif"},
		{itemNo: 46, gender: "female", wantIcon: "chr/007.gif"},
		{itemNo: 46, gender: "f", wantIcon: "chr/007.gif"},
		// 55: 聖職者の衣装 - male 016, female 017
		{itemNo: 55, gender: "male", wantIcon: "chr/016.gif"},
		{itemNo: 55, gender: "female", wantIcon: "chr/017.gif"},
		// 56: 王族の衣装 - male 002, female 018
		{itemNo: 56, gender: "male", wantIcon: "chr/002.gif"},
		{itemNo: 56, gender: "female", wantIcon: "chr/018.gif"},
		// 140: 英雄の衣装 - male 034, female 028
		{itemNo: 140, gender: "male", wantIcon: "chr/034.gif"},
		{itemNo: 140, gender: "female", wantIcon: "chr/028.gif"},
	}

	for _, tt := range tests {
		got := store.CostumeIcon(tt.itemNo, tt.gender)
		if got != tt.wantIcon {
			t.Errorf("CostumeIcon(%d, %q) = %q, want %q", tt.itemNo, tt.gender, got, tt.wantIcon)
		}
	}
}

func TestOracleTalk_CanonicalWords(t *testing.T) {
	words := store.OracleWords
	if len(words) != 5 {
		t.Fatalf("expected 5 canonical talk dialogues, got %d", len(words))
	}

	talk := store.OracleTalk()
	found := false
	for _, w := range words {
		if talk == w {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("OracleTalk() returned unrecognized dialogue: %q", talk)
	}
}

func TestOracleInspect_LegacyParity(t *testing.T) {
	// JobLevel < 15: No hint
	res := store.OracleInspect(10)
	if !strings.Contains(res.Message, "おっ？なんじゃなんじゃ？わしゃ何も知らんよ") {
		t.Errorf("expected legacy inspect message, got: %q", res.Message)
	}
	if res.Hint != "" {
		t.Errorf("expected no hint for jobLevel < 15, got: %q", res.Hint)
	}

	// JobLevel >= 15: Shows Black Market hint
	res15 := store.OracleInspect(15)
	if res15.Hint != "＠やみいちば に行きたい" {
		t.Errorf("expected black market hint for jobLevel >= 15, got: %q", res15.Hint)
	}
}

func TestDiscoverBlackMarket_Gating(t *testing.T) {
	if err := store.DiscoverBlackMarket(14); !errors.Is(err, store.ErrBlackMarketNotDiscovered) {
		t.Errorf("expected ErrBlackMarketNotDiscovered for jobLevel 14, got: %v", err)
	}

	if err := store.DiscoverBlackMarket(15); err != nil {
		t.Errorf("expected nil error for jobLevel 15, got: %v", err)
	}
}

func TestService_OracleDelegates(t *testing.T) {
	svc := store.NewService(nil, nil, nil, nil, nil)
	talk := svc.OracleTalk()
	if talk == "" {
		t.Error("expected non-empty talk message")
	}

	inspect := svc.OracleInspect(15)
	if inspect.Hint != "＠やみいちば に行きたい" {
		t.Errorf("expected hint from inspect, got %q", inspect.Hint)
	}

	if err := svc.DiscoverBlackMarket(10); !errors.Is(err, store.ErrBlackMarketNotDiscovered) {
		t.Errorf("expected ErrBlackMarketNotDiscovered, got %v", err)
	}
	if err := svc.DiscoverBlackMarket(15); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestOracleService_GetOracleStatus(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	char := corecharacter.Character{
		ID:       "char-1",
		Name:     "勇者",
		JobLevel: 16,
		Money:    5000,
	}
	_ = charRepo.Save(ctx, char)

	costumeRepo := store.NewMemoryCostumeRepository()
	wallpaperRepo := &mockHomeWallpaperRepo{}

	svc := store.NewService(
		newMockStoreRepo(),
		charRepo,
		&mockDepotRepo{depots: make(map[string]depot.Depot)},
		&mockCatalog{items: make(map[string]coreitem.Definition)},
		&mockTxProvider{},
		store.WithCostumeRepository(costumeRepo),
		store.WithHomeWallpaperRepository(wallpaperRepo),
	)

	status, err := svc.GetOracleStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("GetOracleStatus failed: %v", err)
	}

	if status.LocationName != "オラクル屋" || status.NPCName != "@ラクル" || status.BgImg != "bgimg/goods.gif" {
		t.Errorf("unexpected oracle metadata: %+v", status)
	}
	if !status.CanUnlockBlackMarket {
		t.Errorf("expected CanUnlockBlackMarket to be true for job_lv 16")
	}
	if len(status.AvailableCostumes) != 17 {
		t.Errorf("expected 17 costumes for job_lv 16, got %d", len(status.AvailableCostumes))
	}
	if len(status.AvailableWallpapers) == 0 {
		t.Errorf("expected wallpapers to be listed")
	}
	if status.ActiveCostume != nil {
		t.Errorf("expected no active costume initially")
	}
}

func TestOracleService_RentCostume_And_Reset(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	char := corecharacter.Character{
		ID:       "char-1",
		Name:     "アリス",
		Gender:   "female",
		JobLevel: 5,
		Money:    1000,
	}
	_ = charRepo.Save(ctx, char)

	costumeRepo := store.NewMemoryCostumeRepository()
	svc := store.NewService(
		newMockStoreRepo(),
		charRepo,
		&mockDepotRepo{depots: make(map[string]depot.Depot)},
		&mockCatalog{items: make(map[string]coreitem.Definition)},
		&mockTxProvider{},
		store.WithCostumeRepository(costumeRepo),
	)

	// Item 56 requires jobLevel > 10, but char has jobLevel 5 -> ErrCostumeNotAvailable
	_, err := svc.RentCostume(ctx, "char-1", 56)
	if !errors.Is(err, store.ErrCostumeNotAvailable) {
		t.Fatalf("expected ErrCostumeNotAvailable for ungated item 56, got: %v", err)
	}

	// Item 44 (ピンクスカート, 300 G) is available at jobLevel 5
	res, err := svc.RentCostume(ctx, "char-1", 44)
	if err != nil {
		t.Fatalf("RentCostume failed: %v", err)
	}

	if res.ActiveCostume.ItemNo != 44 || res.ActiveCostume.ItemName != "ピンクスカート" {
		t.Errorf("unexpected active costume: %+v", res.ActiveCostume)
	}
	if res.ActiveCostume.Icon != "chr/001.gif" {
		t.Errorf("expected icon chr/001.gif, got %s", res.ActiveCostume.Icon)
	}

	// Check character money deducted
	updatedChar, _ := charRepo.FindByID(ctx, "char-1")
	if updatedChar.Money != 700 {
		t.Errorf("expected money 700, got %d", updatedChar.Money)
	}

	// Status reflects rented costume
	status, err := svc.GetOracleStatus(ctx, "char-1")
	if err != nil || status.ActiveCostume == nil {
		t.Fatalf("expected active costume in status: %+v (err: %v)", status, err)
	}
	if status.ActiveCostume.ItemNo != 44 {
		t.Errorf("expected item 44 in status, got %d", status.ActiveCostume.ItemNo)
	}

	// Reset costume (e.g. from sleep wake or job change)
	if err := svc.ResetCostume(ctx, "char-1"); err != nil {
		t.Fatalf("ResetCostume failed: %v", err)
	}

	// Active costume should now be cleared
	statusAfterReset, err := svc.GetOracleStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("GetOracleStatus failed: %v", err)
	}
	if statusAfterReset.ActiveCostume != nil {
		t.Errorf("expected active costume to be cleared after reset, got %+v", statusAfterReset.ActiveCostume)
	}
}

func TestOracleService_BuyHomeWallpaper(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "ボブ",
		Money: 5000,
	}
	_ = charRepo.Save(ctx, char)

	wallpaperRepo := &mockHomeWallpaperRepo{}
	svc := store.NewService(
		newMockStoreRepo(),
		charRepo,
		&mockDepotRepo{depots: make(map[string]depot.Depot)},
		&mockCatalog{items: make(map[string]coreitem.Definition)},
		&mockTxProvider{},
		store.WithHomeWallpaperRepository(wallpaperRepo),
	)

	// Invalid wallpaper
	_, err := svc.BuyHomeWallpaper(ctx, "char-1", "invalid_wallpaper")
	if !errors.Is(err, store.ErrInvalidWallpaper) {
		t.Fatalf("expected ErrInvalidWallpaper, got: %v", err)
	}

	// Valid wallpaper: "goods" (2500 G)
	res, err := svc.BuyHomeWallpaper(ctx, "char-1", "goods")
	if err != nil {
		t.Fatalf("BuyHomeWallpaper failed: %v", err)
	}

	if res.Wallpaper != "goods.gif" {
		t.Errorf("expected goods.gif, got %s", res.Wallpaper)
	}
	if !strings.Contains(res.Message, "ボブの家の壁紙を goods に、張り替えておいたよん") {
		t.Errorf("unexpected message: %s", res.Message)
	}

	// Check wallpaper updated in repo
	if wallpaperRepo.wallpapers["char-1"] != "goods.gif" {
		t.Errorf("expected home wallpaper goods.gif, got %s", wallpaperRepo.wallpapers["char-1"])
	}

	// Check money deducted (5000 - 2500 = 2500)
	updatedChar, _ := charRepo.FindByID(ctx, "char-1")
	if updatedChar.Money != 2500 {
		t.Errorf("expected 2500 G remaining, got %d", updatedChar.Money)
	}

	// Insufficient funds for expensive wallpaper: "stage20" (50,000 G)
	_, err = svc.BuyHomeWallpaper(ctx, "char-1", "stage20")
	if !errors.Is(err, store.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got: %v", err)
	}
}

func TestOracleService_ReturnAndResetCostume(t *testing.T) {
	ctx := context.Background()

	// 1. Without costumeRepo (nil safe)
	svcNoRepo := store.NewService(nil, nil, nil, nil, nil)
	if err := svcNoRepo.ReturnCostume(ctx, "char-1"); err != nil {
		t.Errorf("expected nil error on nil costumeRepo, got %v", err)
	}
	if err := svcNoRepo.ResetCostume(ctx, "char-1"); err != nil {
		t.Errorf("expected nil error on nil costumeRepo reset, got %v", err)
	}

	// 2. With costumeRepo
	costumeRepo := store.NewMemoryCostumeRepository()
	_ = costumeRepo.SaveActiveCostume(ctx, store.ActiveCostume{
		CharacterID: "char-1",
		ItemNo:      46,
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)

	svcWithRepo := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo))
	got, _ := costumeRepo.GetActiveCostume(ctx, "char-1")
	if got == nil {
		t.Fatal("expected active costume before return")
	}

	if err := svcWithRepo.ReturnCostume(ctx, "char-1"); err != nil {
		t.Fatalf("ReturnCostume failed: %v", err)
	}
	got, _ = costumeRepo.GetActiveCostume(ctx, "char-1")
	if got != nil {
		t.Errorf("expected costume cleared after return, got %+v", got)
	}

	// Test ResetCostume
	_ = costumeRepo.SaveActiveCostume(ctx, store.ActiveCostume{
		CharacterID: "char-1",
		ItemNo:      55,
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)
	if err := svcWithRepo.ResetCostume(ctx, "char-1"); err != nil {
		t.Fatalf("ResetCostume failed: %v", err)
	}
	got, _ = costumeRepo.GetActiveCostume(ctx, "char-1")
	if got != nil {
		t.Errorf("expected costume cleared after reset, got %+v", got)
	}
}

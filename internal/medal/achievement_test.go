package medal_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/medal"
)

type mockAchievementRepo struct {
	mu           sync.Mutex
	records      map[string]map[string]medal.AchievementRecord // charID -> achID -> record
	medals       map[string][]medal.CharacterMedal             // charID -> medals
	progressErr  error
	getErr       error
	claimErr     error
	saveMedalErr error
}

func newMockAchievementRepo() *mockAchievementRepo {
	return &mockAchievementRepo{
		records: make(map[string]map[string]medal.AchievementRecord),
		medals:  make(map[string][]medal.CharacterMedal),
	}
}

func (m *mockAchievementRepo) RecordProgress(ctx context.Context, charID string, metric medal.MetricType, amount int, matching []medal.Achievement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.progressErr != nil {
		return m.progressErr
	}

	charRecords, exists := m.records[charID]
	if !exists {
		charRecords = make(map[string]medal.AchievementRecord)
		m.records[charID] = charRecords
	}

	now := time.Now().UTC()
	for _, ach := range matching {
		rec, found := charRecords[ach.ID]
		if !found {
			rec = medal.AchievementRecord{
				CharacterID:   charID,
				AchievementID: ach.ID,
			}
		}
		rec.CurrentProgress += amount
		if rec.CurrentProgress >= ach.Threshold && !rec.IsCompleted {
			rec.IsCompleted = true
			rec.CompletedAt = &now
		}
		charRecords[ach.ID] = rec
	}
	return nil
}

func (m *mockAchievementRepo) GetCharacterAchievements(ctx context.Context, charID string) ([]medal.AchievementRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	var res []medal.AchievementRecord
	if charRecords, ok := m.records[charID]; ok {
		for _, rec := range charRecords {
			res = append(res, rec)
		}
	}
	return res, nil
}

func (m *mockAchievementRepo) GetAchievementForUpdate(ctx context.Context, charID string, achievementID string) (medal.AchievementRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if charRecords, ok := m.records[charID]; ok {
		if rec, found := charRecords[achievementID]; found {
			return rec, nil
		}
	}
	return medal.AchievementRecord{
		CharacterID:   charID,
		AchievementID: achievementID,
	}, nil
}

func (m *mockAchievementRepo) MarkAchievementClaimed(ctx context.Context, charID string, achievementID string, claimedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimErr != nil {
		return m.claimErr
	}
	charRecords, ok := m.records[charID]
	if !ok {
		charRecords = make(map[string]medal.AchievementRecord)
		m.records[charID] = charRecords
	}
	rec, ok := charRecords[achievementID]
	if !ok {
		rec = medal.AchievementRecord{
			CharacterID:   charID,
			AchievementID: achievementID,
		}
	}
	rec.IsClaimed = true
	rec.ClaimedAt = &claimedAt
	charRecords[achievementID] = rec
	return nil
}

func (m *mockAchievementRepo) SaveMedal(ctx context.Context, charMedal medal.CharacterMedal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveMedalErr != nil {
		return m.saveMedalErr
	}
	m.medals[charMedal.CharacterID] = append(m.medals[charMedal.CharacterID], charMedal)
	return nil
}

func (m *mockAchievementRepo) GetCharacterMedals(ctx context.Context, charID string) ([]medal.CharacterMedal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.medals[charID], nil
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
	mu    sync.Mutex
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{chars: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, value corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chars[value.ID] = value
	return nil
}

type mockInvRepo struct{}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, charID string) (coreinventory.Inventory, error) {
	inv, _ := coreinventory.New(charID)
	return inv, nil
}
func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, charID string) (coreinventory.Inventory, error) {
	inv, _ := coreinventory.New(charID)
	return inv, nil
}
func (m *mockInvRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	return nil
}

func TestInitialAchievements_CatalogIntegrity(t *testing.T) {
	achievements, err := medal.InitialAchievements()
	if err != nil {
		t.Fatalf("failed to load initial achievements: %v", err)
	}

	if len(achievements) == 0 {
		t.Fatal("expected achievements to not be empty")
	}

	idMap := make(map[string]bool)
	medalIDMap := make(map[string]bool)
	for _, a := range achievements {
		if a.ID == "" {
			t.Errorf("achievement has empty ID: %+v", a)
		}
		if idMap[a.ID] {
			t.Errorf("duplicate achievement ID found: %s", a.ID)
		}
		idMap[a.ID] = true

		if a.Name == "" {
			t.Errorf("achievement %s has empty name", a.ID)
		}
		if a.Metric == "" {
			t.Errorf("achievement %s has empty metric", a.ID)
		}
		if a.Threshold <= 0 {
			t.Errorf("achievement %s has non-positive threshold %d", a.ID, a.Threshold)
		}
		if a.MedalID == "" {
			t.Errorf("achievement %s has empty medal ID", a.ID)
		}
		if medalIDMap[a.MedalID] {
			t.Errorf("duplicate medal ID found: %s", a.MedalID)
		}
		medalIDMap[a.MedalID] = true
		if a.MedalName == "" {
			t.Errorf("achievement %s has empty medal name", a.ID)
		}
		if a.SmallMedalsReward <= 0 {
			t.Errorf("achievement %s has non-positive small medal reward %d", a.ID, a.SmallMedalsReward)
		}
	}
}

func TestService_RecordProgress_And_Unlock(t *testing.T) {
	charRepo := newMockCharRepo()
	char, _ := corecharacter.New("Hero")
	charRepo.chars[char.ID] = char

	invRepo := &mockInvRepo{}
	achRepo := newMockAchievementRepo()

	customCatalog := []medal.Achievement{
		{
			ID:                "test_adv_1",
			Name:              "冒険の第一歩",
			Metric:            medal.MetricAdventureVictories,
			Threshold:         1,
			MedalID:           "test_medal_1",
			MedalName:         "テスト勲章1",
			SmallMedalsReward: 2,
		},
		{
			ID:                "test_adv_10",
			Name:              "歴戦の冒険者",
			Metric:            medal.MetricAdventureVictories,
			Threshold:         10,
			MedalID:           "test_medal_10",
			MedalName:         "テスト勲章10",
			SmallMedalsReward: 5,
		},
	}

	svc, err := medal.NewService(
		charRepo,
		invRepo,
		"",
		medal.WithAchievementRepository(achRepo, customCatalog...),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Initial state: progress should be 0
	list, err := svc.GetAchievements(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAchievements failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 achievements, got %d", len(list))
	}
	if list[0].CurrentProgress != 0 || list[0].IsCompleted {
		t.Fatalf("expected 0 progress and not completed, got %+v", list[0])
	}

	// 2. Record 1 adventure victory -> test_adv_1 completes, test_adv_10 has 1/10
	err = svc.RecordProgress(ctx, char.ID, medal.MetricAdventureVictories, 1)
	if err != nil {
		t.Fatalf("RecordProgress failed: %v", err)
	}

	list, err = svc.GetAchievements(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAchievements failed: %v", err)
	}
	if !list[0].IsCompleted || list[0].CurrentProgress != 1 || list[0].CompletionPercentage != 100.0 {
		t.Fatalf("expected test_adv_1 to be completed, got %+v", list[0])
	}
	if list[1].IsCompleted || list[1].CurrentProgress != 1 || list[1].CompletionPercentage != 10.0 {
		t.Fatalf("expected test_adv_10 to not be completed, got %+v", list[1])
	}

	// 3. Emit DomainEvent with 9 more victories
	err = svc.HandleEvent(ctx, medal.DomainEvent{
		CharacterID: char.ID,
		Metric:      medal.MetricAdventureVictories,
		Amount:      9,
	})
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	list, err = svc.GetAchievements(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAchievements failed: %v", err)
	}
	if !list[1].IsCompleted || list[1].CurrentProgress != 10 || list[1].CompletionPercentage != 100.0 {
		t.Fatalf("expected test_adv_10 to be completed, got %+v", list[1])
	}
}

func TestService_ClaimAchievement(t *testing.T) {
	charRepo := newMockCharRepo()
	char, _ := corecharacter.New("Hero")
	charRepo.chars[char.ID] = char

	invRepo := &mockInvRepo{}
	achRepo := newMockAchievementRepo()

	customCatalog := []medal.Achievement{
		{
			ID:                "test_adv_1",
			Name:              "冒険の第一歩",
			Metric:            medal.MetricAdventureVictories,
			Threshold:         1,
			MedalID:           "test_medal_1",
			MedalName:         "テスト勲章1",
			MedalDescription:  "テスト記念",
			Category:          "adventure",
			SmallMedalsReward: 3,
		},
	}

	svc, err := medal.NewService(
		charRepo,
		invRepo,
		"",
		medal.WithAchievementRepository(achRepo, customCatalog...),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Claim before completing -> ErrAchievementNotCompleted
	_, err = svc.ClaimAchievement(ctx, char.ID, "test_adv_1")
	if !errors.Is(err, medal.ErrAchievementNotCompleted) {
		t.Fatalf("expected ErrAchievementNotCompleted, got %v", err)
	}

	// Complete achievement
	_ = svc.RecordProgress(ctx, char.ID, medal.MetricAdventureVictories, 1)

	// Claim completed achievement
	claimRes, err := svc.ClaimAchievement(ctx, char.ID, "test_adv_1")
	if err != nil {
		t.Fatalf("ClaimAchievement failed: %v", err)
	}

	if claimRes.AchievementID != "test_adv_1" {
		t.Errorf("expected achievement ID test_adv_1, got %s", claimRes.AchievementID)
	}
	if claimRes.SmallMedalsAwarded != 3 {
		t.Errorf("expected 3 small medals awarded, got %d", claimRes.SmallMedalsAwarded)
	}
	if claimRes.Character.SmallMedals != 3 {
		t.Errorf("expected character small medals to be 3, got %d", claimRes.Character.SmallMedals)
	}
	if claimRes.Medal.MedalID != "test_medal_1" {
		t.Errorf("expected medal ID test_medal_1, got %s", claimRes.Medal.MedalID)
	}

	// Attempting duplicate claim -> ErrAchievementAlreadyClaimed
	_, err = svc.ClaimAchievement(ctx, char.ID, "test_adv_1")
	if !errors.Is(err, medal.ErrAchievementAlreadyClaimed) {
		t.Fatalf("expected ErrAchievementAlreadyClaimed, got %v", err)
	}

	// Verify medals collection query
	medals, err := svc.GetCharacterMedals(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterMedals failed: %v", err)
	}
	if len(medals) != 1 || medals[0].MedalID != "test_medal_1" {
		t.Fatalf("expected 1 medal with ID test_medal_1, got %+v", medals)
	}
}

func TestService_ClaimAchievement_InvalidCases(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := &mockInvRepo{}
	achRepo := newMockAchievementRepo()

	svc, _ := medal.NewService(
		charRepo,
		invRepo,
		"",
		medal.WithAchievementRepository(achRepo),
	)

	ctx := context.Background()

	// Empty char ID
	_, err := svc.ClaimAchievement(ctx, "", "adv_novice")
	if !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}

	// Non-existent achievement
	_, err = svc.ClaimAchievement(ctx, "char-1", "non_existent_ach")
	if !errors.Is(err, medal.ErrAchievementNotFound) {
		t.Fatalf("expected ErrAchievementNotFound, got %v", err)
	}
}

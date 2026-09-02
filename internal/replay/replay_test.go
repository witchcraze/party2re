package replay_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/replay"
)

type mockReplayRepo struct {
	replays map[string]replay.BattleReplay
}

func newMockReplayRepo() *mockReplayRepo {
	return &mockReplayRepo{
		replays: make(map[string]replay.BattleReplay),
	}
}

func (m *mockReplayRepo) Save(ctx context.Context, r replay.BattleReplay) error {
	m.replays[r.ID] = r
	return nil
}

func (m *mockReplayRepo) FindByID(ctx context.Context, id string) (*replay.BattleReplay, error) {
	r, ok := m.replays[id]
	if !ok {
		return nil, replay.ErrReplayNotFound
	}
	return &r, nil
}

func (m *mockReplayRepo) FindByCharacter(ctx context.Context, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error) {
	var list []replay.ReplayHeader
	for _, r := range m.replays {
		if r.InitiatorID == characterID || r.OpponentID == characterID {
			if combatType == "" || r.CombatType == combatType {
				list = append(list, replay.ReplayHeader{
					ID:            r.ID,
					CombatType:    r.CombatType,
					InitiatorID:   r.InitiatorID,
					InitiatorName: r.InitiatorName,
					OpponentID:    r.OpponentID,
					OpponentName:  r.OpponentName,
					Outcome:       r.Outcome,
					WinnerID:      r.WinnerID,
					TotalTurns:    r.TotalTurns,
					CreatedAt:     r.CreatedAt,
				})
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockReplayRepo) FindByCharacterByCursor(ctx context.Context, characterID string, combatType string, limit int, beforeTime time.Time, beforeID string) ([]replay.ReplayHeader, error) {
	var list []replay.ReplayHeader
	for _, r := range m.replays {
		if r.InitiatorID == characterID || r.OpponentID == characterID {
			if combatType == "" || r.CombatType == combatType {
				if beforeTime.IsZero() && beforeID == "" {
					list = append(list, toReplayHeader(r))
				} else if !beforeTime.IsZero() && beforeID != "" {
					if r.CreatedAt.Before(beforeTime) || (r.CreatedAt.Equal(beforeTime) && r.ID < beforeID) {
						list = append(list, toReplayHeader(r))
					}
				} else if !beforeTime.IsZero() {
					if r.CreatedAt.Before(beforeTime) {
						list = append(list, toReplayHeader(r))
					}
				} else {
					if r.ID < beforeID {
						list = append(list, toReplayHeader(r))
					}
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockReplayRepo) FindRecent(ctx context.Context, combatType string, limit int) ([]replay.ReplayHeader, error) {
	var list []replay.ReplayHeader
	for _, r := range m.replays {
		if combatType == "" || r.CombatType == combatType {
			list = append(list, toReplayHeader(r))
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockReplayRepo) FindRecentByCursor(ctx context.Context, combatType string, limit int, beforeTime time.Time, beforeID string) ([]replay.ReplayHeader, error) {
	var list []replay.ReplayHeader
	for _, r := range m.replays {
		if combatType == "" || r.CombatType == combatType {
			if beforeTime.IsZero() && beforeID == "" {
				list = append(list, toReplayHeader(r))
			} else if !beforeTime.IsZero() && beforeID != "" {
				if r.CreatedAt.Before(beforeTime) || (r.CreatedAt.Equal(beforeTime) && r.ID < beforeID) {
					list = append(list, toReplayHeader(r))
				}
			} else if !beforeTime.IsZero() {
				if r.CreatedAt.Before(beforeTime) {
					list = append(list, toReplayHeader(r))
				}
			} else {
				if r.ID < beforeID {
					list = append(list, toReplayHeader(r))
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func toReplayHeader(r replay.BattleReplay) replay.ReplayHeader {
	return replay.ReplayHeader{
		ID:            r.ID,
		CombatType:    r.CombatType,
		InitiatorID:   r.InitiatorID,
		InitiatorName: r.InitiatorName,
		OpponentID:    r.OpponentID,
		OpponentName:  r.OpponentName,
		Outcome:       r.Outcome,
		WinnerID:      r.WinnerID,
		TotalTurns:    r.TotalTurns,
		CreatedAt:     r.CreatedAt,
	}
}

func (m *mockReplayRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	for id, r := range m.replays {
		if r.CreatedAt.Before(cutoff) {
			delete(m.replays, id)
			count++
		}
	}
	return count, nil
}

func TestRecordBattle_SuccessAndValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			{ID: "char1", HP: 100, Attack: 25, Defense: 10},
			{ID: "char2", HP: 80, Attack: 20, Defense: 8},
		},
	}
	result, err := battleEngine.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Invalid Request (missing combat type)
	_, err = service.RecordBattle(ctx, replay.SaveReplayRequest{
		CombatType: "",
		Initiator:  replay.ParticipantSnapshot{ID: "char1", Name: "Player 1"},
		Opponent:   replay.ParticipantSnapshot{ID: "char2", Name: "Player 2"},
	})
	if !errors.Is(err, replay.ErrInvalidCombatType) {
		t.Errorf("expected ErrInvalidCombatType, got %v", err)
	}

	// 2. Success Record
	replayID, err := service.RecordBattle(ctx, replay.SaveReplayRequest{
		CombatType:   replay.CombatTypePvP,
		Initiator:    replay.ParticipantSnapshot{ID: "char1", Name: "Player 1", Level: 15},
		Opponent:     replay.ParticipantSnapshot{ID: "char2", Name: "Player 2", Level: 14},
		BattleResult: result,
	})
	if err != nil {
		t.Fatalf("RecordBattle failed: %v", err)
	}
	if replayID == "" {
		t.Errorf("expected non-empty replay ID")
	}

	// 3. GetReplay
	fetched, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay failed: %v", err)
	}
	if fetched.CombatType != replay.CombatTypePvP || fetched.TotalTurns != result.Turns || len(fetched.TurnLogs) == 0 {
		t.Errorf("unexpected fetched replay: %#v", fetched)
	}
	if fetched.InitiatorName != "Player 1" || fetched.OpponentName != "Player 2" {
		t.Errorf("unexpected participant names: %s vs %s", fetched.InitiatorName, fetched.OpponentName)
	}
}

func TestGetCharacterHistoryAndRecent(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}
	res, _ := battleEngine.Resolve(corebattle.Request{
		Participants: []corebattle.Participant{
			{ID: "hero", HP: 100, Attack: 30, Defense: 10},
			{ID: "boss", HP: 200, Attack: 25, Defense: 15},
		},
	})

	_, _ = service.RecordBattle(ctx, replay.SaveReplayRequest{
		CombatType:   replay.CombatTypeBoss,
		Initiator:    replay.ParticipantSnapshot{ID: "hero", Name: "Hero"},
		Opponent:     replay.ParticipantSnapshot{ID: "boss", Name: "Boss King"},
		BattleResult: res,
	})

	_, _ = service.RecordBattle(ctx, replay.SaveReplayRequest{
		CombatType:   replay.CombatTypePvP,
		Initiator:    replay.ParticipantSnapshot{ID: "other", Name: "Rival"},
		Opponent:     replay.ParticipantSnapshot{ID: "hero", Name: "Hero"},
		BattleResult: res,
	})

	// 1. Character History (should return both matches involving hero)
	history, err := service.GetCharacterHistory(ctx, "hero", "", 10)
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 history items, got %d (err: %v)", len(history), err)
	}

	// 2. Filtered Character History (Boss only)
	bossHistory, err := service.GetCharacterHistory(ctx, "hero", replay.CombatTypeBoss, 10)
	if err != nil || len(bossHistory) != 1 {
		t.Fatalf("expected 1 boss history item, got %d (err: %v)", len(bossHistory), err)
	}

	// 3. Global Recent Replays
	recent, err := service.GetRecentReplays(ctx, "", 10)
	if err != nil || len(recent) != 2 {
		t.Fatalf("expected 2 recent replays, got %d", len(recent))
	}
}

func TestPruneOldReplays(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Old replay (60 days ago)
	oldTime := time.Now().UTC().AddDate(0, 0, -60)
	repo.replays["old-rep"] = replay.BattleReplay{
		ID:          "old-rep",
		CombatType:  replay.CombatTypePvP,
		InitiatorID: "char1",
		OpponentID:  "char2",
		CreatedAt:   oldTime,
	}

	// Fresh replay
	repo.replays["new-rep"] = replay.BattleReplay{
		ID:          "new-rep",
		CombatType:  replay.CombatTypePvP,
		InitiatorID: "char1",
		OpponentID:  "char2",
		CreatedAt:   time.Now().UTC(),
	}

	deleted, err := service.PruneOldReplays(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 replay pruned, got %d", deleted)
	}
	if _, ok := repo.replays["old-rep"]; ok {
		t.Errorf("expected old replay to be deleted")
	}
	if _, ok := repo.replays["new-rep"]; !ok {
		t.Errorf("expected new replay to remain")
	}
}

func TestGetCharacterHistoryByCursor(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		repID := "rep-" + string(rune('0'+i))
		repo.replays[repID] = replay.BattleReplay{
			ID:          repID,
			CombatType:  replay.CombatTypePvP,
			InitiatorID: "char-1",
			OpponentID:  "char-2",
			Outcome:     corebattle.OutcomeWin,
			WinnerID:    "char-1",
			CreatedAt:   now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := service.GetCharacterHistoryByCursor(ctx, "char-1", replay.CombatTypePvP, 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}

func TestGetRecentReplaysByCursor(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		repID := "rep-" + string(rune('0'+i))
		repo.replays[repID] = replay.BattleReplay{
			ID:          repID,
			CombatType:  replay.CombatTypeBoss,
			InitiatorID: "char-1",
			OpponentID:  "boss-1",
			Outcome:     corebattle.OutcomeWin,
			WinnerID:    "char-1",
			CreatedAt:   now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := service.GetRecentReplaysByCursor(ctx, replay.CombatTypeBoss, 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}

package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestBanquetAttendeesForTier(t *testing.T) {
	tests := []struct {
		tier     int
		expected int
	}{
		{tier: 0, expected: 10},
		{tier: 1, expected: 10},
		{tier: 2, expected: 20},
		{tier: 3, expected: 30},
		{tier: 5, expected: 30},
		{tier: 10, expected: 30},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("tier-%d", tt.tier), func(t *testing.T) {
			got := BanquetAttendeesForTier(tt.tier)
			if got != tt.expected {
				t.Errorf("BanquetAttendeesForTier(%d) = %d, expected %d", tt.tier, got, tt.expected)
			}
		})
	}
}

func TestValkeyPresenceTracker_RecordPresence(t *testing.T) {
	var executedCmds [][]string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		executedCmds = append(executedCmds, cmd.Commands())
		return valkeytest.MakeOKResult()
	}))

	tracker := NewValkeyPresenceTracker(client)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	err := tracker.RecordPresence(context.Background(), "char-123", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executedCmds) != 2 {
		t.Fatalf("expected 2 commands (ZADD, EXPIRE), got %d: %v", len(executedCmds), executedCmds)
	}

	// 1. ZADD
	if executedCmds[0][0] != "ZADD" || executedCmds[0][1] != ValkeyPresenceKey {
		t.Errorf("unexpected ZADD command: %v", executedCmds[0])
	}
	if executedCmds[0][2] != strconv.FormatInt(now.Unix(), 10) || executedCmds[0][3] != "char-123" {
		t.Errorf("unexpected ZADD score/member: %v", executedCmds[0])
	}

	// 2. EXPIRE
	if executedCmds[1][0] != "EXPIRE" || executedCmds[1][1] != ValkeyPresenceKey {
		t.Errorf("unexpected EXPIRE command: %v", executedCmds[1])
	}
}

func TestValkeyPresenceTracker_RecordBanquetPresence(t *testing.T) {
	var executedCmds [][]string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		executedCmds = append(executedCmds, cmd.Commands())
		return valkeytest.MakeOKResult()
	}))

	tracker := NewValkeyPresenceTracker(client)
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	duration := time.Hour
	expiry := at.Add(duration)

	err := tracker.RecordBanquetPresenceAt(context.Background(), "banquet-king1", 10, at, duration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executedCmds) != 2 {
		t.Fatalf("expected 2 commands (ZADD, EXPIRE), got %d: %v", len(executedCmds), executedCmds)
	}

	// 1. ZADD with 10 members
	zaddCmd := executedCmds[0]
	if zaddCmd[0] != "ZADD" || zaddCmd[1] != ValkeyPresenceKey {
		t.Fatalf("unexpected ZADD command: %v", zaddCmd)
	}
	// args: ZADD key score member score member ...
	// total args = 2 + 10 * 2 = 22
	if len(zaddCmd) != 22 {
		t.Fatalf("expected 22 args in ZADD for 10 members, got %d: %v", len(zaddCmd), zaddCmd)
	}
	expectedScoreStr := strconv.FormatInt(expiry.Unix(), 10)
	for i := 0; i < 10; i++ {
		scoreArg := zaddCmd[2+i*2]
		memberArg := zaddCmd[3+i*2]
		if scoreArg != expectedScoreStr {
			t.Errorf("member %d: expected score %s, got %s", i, expectedScoreStr, scoreArg)
		}
		expectedMember := fmt.Sprintf("banquet:banquet-king1:npc:%d", i)
		if memberArg != expectedMember {
			t.Errorf("member %d: expected %s, got %s", i, expectedMember, memberArg)
		}
	}

	// 2. EXPIRE
	expireCmd := executedCmds[1]
	if expireCmd[0] != "EXPIRE" || expireCmd[1] != ValkeyPresenceKey {
		t.Errorf("unexpected EXPIRE command: %v", expireCmd)
	}
	expectedTTL := strconv.FormatInt(int64(duration.Seconds())+ValkeyPresenceTTL, 10)
	if expireCmd[2] != expectedTTL {
		t.Errorf("expected TTL %s, got %s", expectedTTL, expireCmd[2])
	}
}

func TestValkeyPresenceTracker_CountActiveParticipants(t *testing.T) {
	var executedCmds [][]string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		executedCmds = append(executedCmds, cmd.Commands())
		if cmd.Commands()[0] == "ZCARD" {
			return valkeytest.MakeIntResult(25)
		}
		return valkeytest.MakeOKResult()
	}))

	tracker := NewValkeyPresenceTracker(client)
	cutoff := time.Date(2026, 9, 14, 12, 55, 0, 0, time.UTC)
	count, err := tracker.CountActiveParticipants(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 25 {
		t.Errorf("expected count 25, got %d", count)
	}

	if len(executedCmds) != 2 {
		t.Fatalf("expected 2 commands (ZREMRANGEBYSCORE, ZCARD), got %d: %v", len(executedCmds), executedCmds)
	}

	// 1. ZREMRANGEBYSCORE key -inf cutoff-1
	remCmd := executedCmds[0]
	if remCmd[0] != "ZREMRANGEBYSCORE" || remCmd[1] != ValkeyPresenceKey {
		t.Errorf("unexpected ZREMRANGEBYSCORE: %v", remCmd)
	}
	expectedMax := strconv.FormatInt(cutoff.Unix()-1, 10)
	if remCmd[2] != "-inf" || remCmd[3] != expectedMax {
		t.Errorf("expected range -inf to %s, got %s to %s", expectedMax, remCmd[2], remCmd[3])
	}

	// 2. ZCARD key
	cardCmd := executedCmds[1]
	if cardCmd[0] != "ZCARD" || cardCmd[1] != ValkeyPresenceKey {
		t.Errorf("unexpected ZCARD: %v", cardCmd)
	}
}

func TestValkeyPresenceTracker_NilClient(t *testing.T) {
	tracker := NewValkeyPresenceTracker(nil)
	ctx := context.Background()

	if err := tracker.RecordPresence(ctx, "char-1", time.Now()); err != nil {
		t.Errorf("expected nil error on nil client, got %v", err)
	}
	if err := tracker.RecordBanquetPresence(ctx, "b-1", 10, time.Hour); err != nil {
		t.Errorf("expected nil error on nil client, got %v", err)
	}
	count, err := tracker.CountActiveParticipants(ctx, time.Now())
	if err != nil || count != 0 {
		t.Errorf("expected (0, nil) on nil client, got (%d, %v)", count, err)
	}
}

func TestValkeyPresenceTracker_ErrorHandling(t *testing.T) {
	errZadd := errors.New("valkey zadd failure")
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "ZADD" {
			return valkeytest.MakeErrorResult(errZadd)
		}
		return valkeytest.MakeOKResult()
	}))

	tracker := NewValkeyPresenceTracker(client)
	ctx := context.Background()

	if err := tracker.RecordPresence(ctx, "char-1", time.Now()); !errors.Is(err, errZadd) {
		t.Errorf("expected errZadd on RecordPresence, got %v", err)
	}
	if err := tracker.RecordBanquetPresence(ctx, "b-1", 10, time.Hour); !errors.Is(err, errZadd) {
		t.Errorf("expected errZadd on RecordBanquetPresence, got %v", err)
	}
}

type mockSortedSetStore struct {
	members map[string]float64
}

func newMockSortedSetStore() *mockSortedSetStore {
	return &mockSortedSetStore{members: make(map[string]float64)}
}

func (s *mockSortedSetStore) Client() valkey.Client {
	return valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		args := cmd.Commands()
		switch args[0] {
		case "ZADD":
			// ZADD key score member [score member ...]
			for i := 2; i < len(args); i += 2 {
				score, _ := strconv.ParseFloat(args[i], 64)
				s.members[args[i+1]] = score
			}
			return valkeytest.MakeIntResult(int64((len(args) - 2) / 2))
		case "ZREMRANGEBYSCORE":
			// ZREMRANGEBYSCORE key min max
			maxScore, _ := strconv.ParseFloat(args[3], 64)
			var removed int64
			for m, sc := range s.members {
				if sc <= maxScore {
					delete(s.members, m)
					removed++
				}
			}
			return valkeytest.MakeIntResult(removed)
		case "ZCARD":
			return valkeytest.MakeIntResult(int64(len(s.members)))
		case "EXPIRE":
			return valkeytest.MakeIntResult(1)
		default:
			return valkeytest.MakeOKResult()
		}
	}))
}

func TestValkeyPresenceTracker_BanquetLifecycleAndNaturalPurge(t *testing.T) {
	store := newMockSortedSetStore()
	tracker := NewValkeyPresenceTracker(store.Client())
	ctx := context.Background()

	startTime := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	duration := 1 * time.Hour

	// 1. Defeating King Boss Tier 2 injects 20 attendees
	err := tracker.RecordBanquetPresenceAt(ctx, "banquet-king2", 20, startTime, duration)
	if err != nil {
		t.Fatalf("RecordBanquetPresenceAt failed: %v", err)
	}

	// 2. Querying presence 30 minutes in (cutoff = 12:25:00)
	countDuring, err := tracker.CountActiveParticipants(ctx, startTime.Add(25*time.Minute))
	if err != nil {
		t.Fatalf("CountActiveParticipants failed: %v", err)
	}
	if countDuring != 20 {
		t.Errorf("expected 20 active participants during banquet, got %d", countDuring)
	}

	// 3. Querying presence after 1 hour (expiry = 13:00:00; cutoff at 13:00:01)
	// The participants naturally expire and are purged by ZREMRANGEBYSCORE
	countAfter, err := tracker.CountActiveParticipants(ctx, startTime.Add(1*time.Hour+1*time.Second))
	if err != nil {
		t.Fatalf("CountActiveParticipants failed: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 active participants after banquet expired, got %d", countAfter)
	}
}

func TestEventPlazaService_VictoryBanquet_UnlocksTravelingMerchant(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
	store := newMockSortedSetStore()
	tracker := NewValkeyPresenceTracker(store.Client())
	repo := newMemoryRepo()
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()

	svc, err := NewService(
		repo,
		charRepo,
		invRepo,
		WithPresenceTracker(tracker),
		WithClock(clock),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Initial state: 0 participants, Traveling merchant on journey (tier 0)
	status, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status.ActiveParticipants != 0 || status.MerchantTier != 0 {
		t.Fatalf("expected 0 participants and tier 0, got %d, %d", status.ActiveParticipants, status.MerchantTier)
	}

	// 1. King Boss Tier 1 victory: records 10 attendees -> unlocks Tier 1 (Bronze)
	banquet1, err := svc.RecordVictoryBanquet(ctx, "king1", "King Slime", "char-hero", "Hero", 1)
	if err != nil {
		t.Fatalf("RecordVictoryBanquet tier 1 failed: %v", err)
	}
	if banquet1.Tier != 1 {
		t.Errorf("expected banquet tier 1, got %d", banquet1.Tier)
	}

	status1, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status1.ActiveParticipants != 10 {
		t.Errorf("expected 10 active participants, got %d", status1.ActiveParticipants)
	}
	if status1.MerchantTier != 1 {
		t.Errorf("expected MerchantTier 1 (Bronze), got %d (%s)", status1.MerchantTier, status1.MerchantTierName)
	}

	// 2. King Boss Tier 2 victory: records 20 attendees -> unlocks Tier 2 (Silver)
	// Reset store to isolate tier 2 test
	store.members = make(map[string]float64)
	banquet2, err := svc.RecordVictoryBanquet(ctx, "king2", "King Goblin", "char-hero", "Hero", 2)
	if err != nil {
		t.Fatalf("RecordVictoryBanquet tier 2 failed: %v", err)
	}
	if banquet2.Tier != 2 {
		t.Errorf("expected banquet tier 2, got %d", banquet2.Tier)
	}

	status2, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status2.ActiveParticipants != 20 {
		t.Errorf("expected 20 active participants, got %d", status2.ActiveParticipants)
	}
	if status2.MerchantTier != 2 {
		t.Errorf("expected MerchantTier 2 (Silver), got %d (%s)", status2.MerchantTier, status2.MerchantTierName)
	}

	// 3. King Boss Tier 3 victory: records 30 attendees -> unlocks Tier 3 (Gold)
	store.members = make(map[string]float64)
	banquet3, err := svc.RecordVictoryBanquet(ctx, "king3", "King Dragon", "char-hero", "Hero", 3)
	if err != nil {
		t.Fatalf("RecordVictoryBanquet tier 3 failed: %v", err)
	}
	if banquet3.Tier != 3 {
		t.Errorf("expected banquet tier 3, got %d", banquet3.Tier)
	}

	status3, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status3.ActiveParticipants != 30 {
		t.Errorf("expected 30 active participants, got %d", status3.ActiveParticipants)
	}
	if status3.MerchantTier != 3 {
		t.Errorf("expected MerchantTier 3 (Gold), got %d (%s)", status3.MerchantTier, status3.MerchantTierName)
	}

	// 4. Advance clock by 1 hour + 6 minutes (beyond 1 hour duration + 5 min presence cutoff)
	clock.now = clock.now.Add(1*time.Hour + 6*time.Minute)
	statusExpired, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if statusExpired.ActiveParticipants != 0 {
		t.Errorf("expected 0 active participants after expiry, got %d", statusExpired.ActiveParticipants)
	}
	if statusExpired.MerchantTier != 0 {
		t.Errorf("expected MerchantTier 0 after expiry, got %d", statusExpired.MerchantTier)
	}
}

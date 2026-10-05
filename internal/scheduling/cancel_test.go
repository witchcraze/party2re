package scheduling

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core_character "github.com/witchcraze/party2re/internal/core/character"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestCancelByActorID_WriteFailureMatrix(t *testing.T) {
	ctx := context.Background()
	injectedErr := errors.New("valkey injected write error")

	tests := []struct {
		name               string
		actions            []string
		failAtCommand      string // "ZREM", "DEL_ACTION", "DEL_LOCK", "DEL_ACTOR"
		failActionID       string // which action ID fails (for multi-action)
		expectedCancelled  int
		expectedError      error
		disallowedCommands []string // commands that must NOT have been called
	}{
		{
			name:               "ZREM failure stops cleanup and returns 0",
			actions:            []string{"work-1"},
			failAtCommand:      "ZREM",
			failActionID:       "work-1",
			expectedCancelled:  0,
			expectedError:      injectedErr,
			disallowedCommands: []string{"DEL " + actionKeyPrefix + "work-1", "DEL " + lockKeyPrefix + "work-1", "DEL " + actorKeyPrefix + "hero"},
		},
		{
			name:               "DEL action payload failure stops cleanup and returns 0",
			actions:            []string{"work-1"},
			failAtCommand:      "DEL_ACTION",
			failActionID:       "work-1",
			expectedCancelled:  0,
			expectedError:      injectedErr,
			disallowedCommands: []string{"DEL " + lockKeyPrefix + "work-1", "DEL " + actorKeyPrefix + "hero"},
		},
		{
			name:               "DEL lock failure stops cleanup and returns 0",
			actions:            []string{"work-1"},
			failAtCommand:      "DEL_LOCK",
			failActionID:       "work-1",
			expectedCancelled:  0,
			expectedError:      injectedErr,
			disallowedCommands: []string{"DEL " + actorKeyPrefix + "hero"},
		},
		{
			name:               "DEL actor index failure returns action count and surfaces error",
			actions:            []string{"work-1"},
			failAtCommand:      "DEL_ACTOR",
			failActionID:       "work-1",
			expectedCancelled:  1,
			expectedError:      injectedErr,
			disallowedCommands: nil,
		},
		{
			name:               "Multi-action second action ZREM failure returns first count and stops cleanup",
			actions:            []string{"act-1", "act-2"},
			failAtCommand:      "ZREM",
			failActionID:       "act-2",
			expectedCancelled:  1,
			expectedError:      injectedErr,
			disallowedCommands: []string{"DEL " + actionKeyPrefix + "act-2", "DEL " + lockKeyPrefix + "act-2", "DEL " + actorKeyPrefix + "hero"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				cmdStrs := cmd.Commands()
				cmdName := cmdStrs[0]

				if cmdName == "SMEMBERS" {
					return valkeytest.MakeStringSliceResult(tc.actions)
				}

				if tc.failAtCommand == "ZREM" && cmdName == "ZREM" && len(cmdStrs) >= 3 && cmdStrs[2] == tc.failActionID {
					return valkeytest.MakeErrorResult(tc.expectedError)
				}
				if tc.failAtCommand == "DEL_ACTION" && cmdName == "DEL" && cmdStrs[1] == actionKeyPrefix+tc.failActionID {
					return valkeytest.MakeErrorResult(tc.expectedError)
				}
				if tc.failAtCommand == "DEL_LOCK" && cmdName == "DEL" && cmdStrs[1] == lockKeyPrefix+tc.failActionID {
					return valkeytest.MakeErrorResult(tc.expectedError)
				}
				if tc.failAtCommand == "DEL_ACTOR" && cmdName == "DEL" && cmdStrs[1] == actorKeyPrefix+"hero" {
					return valkeytest.MakeErrorResult(tc.expectedError)
				}

				return valkeytest.MakeOKResult()
			}))

			repo := NewValkeyRepository(client)
			cancelled, err := repo.CancelByActorID(ctx, "hero")

			if !errors.Is(err, tc.expectedError) {
				t.Fatalf("expected error %v, got %v", tc.expectedError, err)
			}
			if cancelled != tc.expectedCancelled {
				t.Fatalf("expected cancelled %d, got %d", tc.expectedCancelled, cancelled)
			}

			// Verify disallowed commands were not executed
			recorded := client.RecordedCommandStrings()
			for _, disallowed := range tc.disallowedCommands {
				parts := strings.Split(disallowed, " ")
				for _, r := range recorded {
					if len(r) >= 2 && r[0] == parts[0] && r[1] == parts[1] {
						t.Errorf("disallowed command executed: %v (recorded all: %v)", disallowed, recorded)
					}
				}
			}
		})
	}
}

func TestService_ClearActiveActions_WriteFailure(t *testing.T) {
	ctx := context.Background()
	injectedErr := errors.New("valkey failure")

	// When all writes fail (as described in reproduction)
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "SMEMBERS" {
			return valkeytest.MakeStringSliceResult([]string{"work"})
		}
		// Inject failure for subsequent ZREM and DEL
		return valkeytest.MakeErrorResult(injectedErr)
	}))

	repo := NewValkeyRepository(client)
	svc := NewService(repo)

	cleared, err := svc.ClearActiveActions(ctx, "hero")
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected error %v, got %v", injectedErr, err)
	}
	if cleared {
		t.Fatalf("expected cleared == false when writes fail, got cleared == true")
	}
}

// statefulValkeyAdapter implements a minimal in-memory Valkey client for scheduling and timer tests.
type statefulValkeyAdapter struct {
	actions      map[string]string              // key -> payload
	pendingQueue map[string]float64             // actionID -> score
	actorSets    map[string]map[string]struct{} // actorKey -> set of actionIDs
	locks        map[string]string              // key -> value

	failZrem   error
	failDelKey map[string]error
}

func newStatefulValkeyAdapter() *statefulValkeyAdapter {
	return &statefulValkeyAdapter{
		actions:      make(map[string]string),
		pendingQueue: make(map[string]float64),
		actorSets:    make(map[string]map[string]struct{}),
		locks:        make(map[string]string),
		failDelKey:   make(map[string]error),
	}
}

func (s *statefulValkeyAdapter) toMockClient() valkey.Client {
	return valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStrs := cmd.Commands()
		name := cmdStrs[0]

		switch name {
		case "SET":
			key := cmdStrs[1]
			val := cmdStrs[2]
			if strings.HasPrefix(key, lockKeyPrefix) || strings.HasPrefix(key, "party2:timer:") {
				s.locks[key] = val
			} else {
				s.actions[key] = val
			}
			return valkeytest.MakeOKResult()

		case "GET":
			key := cmdStrs[1]
			val, ok := s.actions[key]
			if !ok {
				return valkeytest.MakeNilResult()
			}
			return valkeytest.MakeStringResult(val)

		case "MGET":
			var results []valkey.ValkeyResult
			for i := 1; i < len(cmdStrs); i++ {
				k := cmdStrs[i]
				val, ok := s.actions[k]
				if !ok {
					results = append(results, valkeytest.MakeNilResult())
				} else {
					results = append(results, valkeytest.MakeStringResult(val))
				}
			}
			// Build array result
			arr := make([]string, len(cmdStrs)-1)
			for i := 1; i < len(cmdStrs); i++ {
				arr[i-1] = s.actions[cmdStrs[i]]
			}
			return valkeytest.MakeStringSliceResult(arr)

		case "SADD":
			key := cmdStrs[1]
			member := cmdStrs[2]
			if s.actorSets[key] == nil {
				s.actorSets[key] = make(map[string]struct{})
			}
			s.actorSets[key][member] = struct{}{}
			return valkeytest.MakeIntResult(1)

		case "SMEMBERS":
			key := cmdStrs[1]
			set := s.actorSets[key]
			if set == nil || len(set) == 0 {
				return valkeytest.MakeNilResult()
			}
			var members []string
			for m := range set {
				members = append(members, m)
			}
			return valkeytest.MakeStringSliceResult(members)

		case "ZADD":
			key := cmdStrs[1]
			if key == pendingQueueKey {
				member := cmdStrs[len(cmdStrs)-1]
				s.pendingQueue[member] = 100
			}
			return valkeytest.MakeIntResult(1)

		case "ZREM":
			if s.failZrem != nil {
				return valkeytest.MakeErrorResult(s.failZrem)
			}
			key := cmdStrs[1]
			if key == pendingQueueKey {
				member := cmdStrs[2]
				delete(s.pendingQueue, member)
			}
			return valkeytest.MakeIntResult(1)

		case "DEL":
			key := cmdStrs[1]
			if err, fail := s.failDelKey[key]; fail {
				return valkeytest.MakeErrorResult(err)
			}
			delete(s.actions, key)
			delete(s.locks, key)
			delete(s.actorSets, key)
			return valkeytest.MakeIntResult(1)

		case "EXISTS":
			key := cmdStrs[1]
			if _, ok := s.locks[key]; ok {
				return valkeytest.MakeIntResult(1)
			}
			return valkeytest.MakeIntResult(0)

		case "TTL":
			return valkeytest.MakeIntResult(600)
		}

		return valkeytest.MakeOKResult()
	}))
}

func TestCancelByActorID_StatefulDiscoverability(t *testing.T) {
	ctx := context.Background()

	t.Run("ZREM failure retains discoverability in actor set", func(t *testing.T) {
		adapter := newStatefulValkeyAdapter()
		client := adapter.toMockClient()
		repo := NewValkeyRepository(client)
		svc := NewService(repo)

		// Schedule action
		now := time.Now().UTC()
		actionID := "action-disco-1"
		action := core_scheduling.ScheduledAction{
			ID:          actionID,
			ActionType:  "adventure:step",
			ActorID:     "hero-disco",
			State:       core_scheduling.StatePending,
			ScheduledAt: now,
			ExecuteAt:   now.Add(time.Hour),
		}
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule failed: %v", err)
		}

		// Verify initially discoverable
		pending, err := repo.FindPendingByActorID(ctx, "hero-disco")
		if err != nil || len(pending) != 1 {
			t.Fatalf("expected 1 pending action, got %v (err: %v)", pending, err)
		}

		// Inject ZREM failure
		injectedErr := errors.New("zrem storage crash")
		adapter.failZrem = injectedErr

		// Attempt ClearActiveActions
		cleared, err := svc.ClearActiveActions(ctx, "hero-disco")
		if !errors.Is(err, injectedErr) {
			t.Fatalf("expected error %v, got %v", injectedErr, err)
		}
		if cleared {
			t.Fatalf("expected cleared == false on failure")
		}

		// CRITICAL: Work MUST retain discoverability after cancellation failure
		adapter.failZrem = nil // clear for query read
		stillPending, err := repo.FindPendingByActorID(ctx, "hero-disco")
		if err != nil {
			t.Fatalf("FindPendingByActorID failed: %v", err)
		}
		if len(stillPending) != 1 || stillPending[0].ID != actionID {
			t.Fatalf("expected action %s to remain discoverable after cancellation failure, got %v", actionID, stillPending)
		}
	})

	t.Run("DEL action payload failure retains discoverability", func(t *testing.T) {
		adapter := newStatefulValkeyAdapter()
		client := adapter.toMockClient()
		repo := NewValkeyRepository(client)
		svc := NewService(repo)

		now := time.Now().UTC()
		actionID := "action-disco-2"
		action := core_scheduling.ScheduledAction{
			ID:          actionID,
			ActionType:  "adventure:step",
			ActorID:     "hero-disco-2",
			State:       core_scheduling.StatePending,
			ScheduledAt: now,
			ExecuteAt:   now.Add(time.Hour),
		}
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule failed: %v", err)
		}

		// Inject DEL action payload failure
		injectedErr := errors.New("del action payload crash")
		adapter.failDelKey[actionKeyPrefix+actionID] = injectedErr

		cleared, err := svc.ClearActiveActions(ctx, "hero-disco-2")
		if !errors.Is(err, injectedErr) {
			t.Fatalf("expected error %v, got %v", injectedErr, err)
		}
		if cleared {
			t.Fatalf("expected cleared == false on failure")
		}

		// Work MUST retain discoverability
		stillPending, err := repo.FindPendingByActorID(ctx, "hero-disco-2")
		if err != nil {
			t.Fatalf("FindPendingByActorID failed: %v", err)
		}
		if len(stillPending) != 1 || stillPending[0].ID != actionID {
			t.Fatalf("expected action %s to remain discoverable, got %v", actionID, stillPending)
		}
	})
}

type memoryRescueRepo struct {
	saved []rescue.RescueRecord
}

func (r *memoryRescueRepo) Save(_ context.Context, record rescue.RescueRecord) error {
	r.saved = append(r.saved, record)
	return nil
}

func (r *memoryRescueRepo) FindLatestByCharacterID(_ context.Context, characterID string) (rescue.RescueRecord, error) {
	for i := len(r.saved) - 1; i >= 0; i-- {
		if r.saved[i].CharacterID == characterID {
			return r.saved[i], nil
		}
	}
	return rescue.RescueRecord{}, rescue.ErrNoRescueRecord
}

type memoryCharRepo struct {
	chars map[string]core_character.Character
}

func (r *memoryCharRepo) FindByID(_ context.Context, id string) (core_character.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return core_character.Character{}, core_character.ErrNotFound
	}
	return c, nil
}

func (r *memoryCharRepo) Update(_ context.Context, c core_character.Character) error {
	r.chars[c.ID] = c
	return nil
}

func TestEmergencyRescue_WithRealSchedulingCleaner_FailureAndRecovery(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	adapter := newStatefulValkeyAdapter()
	client := adapter.toMockClient()
	schedRepo := NewValkeyRepository(client)
	schedSvc := NewService(schedRepo)
	timerSvc := timer.NewService(client)

	rescueRepo := &memoryRescueRepo{}
	charRepo := &memoryCharRepo{
		chars: map[string]core_character.Character{
			"hero-1": {ID: "hero-1", Name: "Hero"},
			"idle-1": {ID: "idle-1", Name: "Idle"},
		},
	}

	rescueService := rescue.NewService(rescueRepo, charRepo, schedSvc, timerSvc)

	// 1. Enqueue action for hero-1
	_, err := schedSvc.Schedule(ctx, "adventure:step", "hero-1", nil, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	// 2. Inject cancellation write failure
	injectedErr := errors.New("valkey write connection reset")
	adapter.failZrem = injectedErr

	// 3. EmergencyRescue MUST return execution failure and record NO penalty
	rec, err := rescueService.EmergencyRescue(ctx, "hero-1", "Screen frozen", now)
	if err == nil {
		t.Fatalf("expected EmergencyRescue to fail on cancellation error, got rec=%+v", rec)
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected error %v, got %v", injectedErr, err)
	}
	if len(rescueRepo.saved) != 0 {
		t.Fatalf("expected 0 rescue records saved after prerequisite failure, got %d", len(rescueRepo.saved))
	}
	locked, _ := timerSvc.IsLocked(ctx, timer.CategorySleep, "hero-1")
	if locked {
		t.Fatalf("expected sleep timer not locked after prerequisite failure")
	}

	// 4. Healthy cancellation after error resolved
	adapter.failZrem = nil
	recSuccess, err := rescueService.EmergencyRescue(ctx, "hero-1", "Screen frozen retry", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed on healthy retry: %v", err)
	}
	if recSuccess.PenaltySeconds != rescue.DefaultPenaltySeconds {
		t.Errorf("expected penalty %d, got %d", rescue.DefaultPenaltySeconds, recSuccess.PenaltySeconds)
	}
	if len(rescueRepo.saved) != 1 {
		t.Fatalf("expected 1 rescue record saved, got %d", len(rescueRepo.saved))
	}

	// 5. Idle character (no actions remaining) returns 0 penalty cooldown without recording
	recIdle, err := rescueService.EmergencyRescue(ctx, "idle-1", "Accidental click in town", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed for idle character: %v", err)
	}
	if recIdle.PenaltySeconds != 0 {
		t.Errorf("expected 0 penalty for idle character, got %d", recIdle.PenaltySeconds)
	}
	if len(rescueRepo.saved) != 1 {
		t.Errorf("expected still only 1 rescue record saved, got %d", len(rescueRepo.saved))
	}
}

package activity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type activityRepositoryStub struct {
	mu         sync.Mutex
	value      Activity
	characters *characterRepositoryStub
}

func (r *activityRepositoryStub) Save(_ context.Context, value Activity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
	return nil
}
func (r *activityRepositoryStub) FindByID(_ context.Context, _ string) (Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" {
		return Activity{}, ErrNotFound
	}
	return r.value, nil
}
func (r *activityRepositoryStub) ClaimAndApply(_ context.Context, id string, character corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID != id {
		return ErrNotFound
	}
	if r.value.Claimed {
		return ErrAlreadyClaimed
	}
	r.value.Claimed = true
	r.characters.mu.Lock()
	defer r.characters.mu.Unlock()
	r.characters.value = character
	return nil
}

type characterRepositoryStub struct {
	mu    sync.Mutex
	value corecharacter.Character
}

func (r *characterRepositoryStub) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" || r.value.ID != id {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.value, nil
}
func (r *characterRepositoryStub) Update(_ context.Context, value corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
	return nil
}

type testScheduler struct {
	mu           sync.Mutex
	scheduledIDs []string
	err          error
}

func (s *testScheduler) Schedule(ctx context.Context, actionType, actorID string, params map[string]string, executeAt time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
	}
	s.scheduledIDs = append(s.scheduledIDs, "mock-id")
	return "mock-id", nil
}

type testLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func newTestService(t *testing.T) (*Service, *testClock, *activityRepositoryStub, *characterRepositoryStub) {
	t.Helper()
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	activities := &activityRepositoryStub{}
	characters := &characterRepositoryStub{value: character}
	activities.characters = characters
	scheduler := &testScheduler{}
	logger := nopLogger{}
	service, err := NewService(activities, characters, scheduler, logger)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service.clock = clock
	return service, clock, activities, characters
}

func TestStartTrainingSchedulesActivity(t *testing.T) {
	service, clock, repository, characters := newTestService(t)

	got, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("StartTraining() error = %v", err)
	}
	if got.Type != TrainingType || got.StartedAt != clock.now || got.AvailableAt != clock.now.Add(TrainingDuration) {
		t.Fatalf("StartTraining() = %#v", got)
	}
	if repository.value.ID != got.ID {
		t.Fatal("activity was not saved")
	}
	scheduler := service.scheduler.(*testScheduler)
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if len(scheduler.scheduledIDs) != 1 {
		t.Fatal("scheduler was not called")
	}
}

func TestStartTrainingSchedulerFailsLogsWarningAndSucceeds(t *testing.T) {
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	activities := &activityRepositoryStub{}
	characters := &characterRepositoryStub{value: character}
	activities.characters = characters
	scheduler := &testScheduler{err: errors.New("valkey connection error")}
	logger := &testLogger{}
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service, err := NewServiceWithClock(activities, characters, scheduler, logger, clock)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("StartTraining() expected success even when scheduler fails, got error: %v", err)
	}
	if got.ID == "" {
		t.Fatal("activity ID is empty")
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.warns) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(logger.warns))
	}
}

func TestStartTrainingNilSchedulerSucceeds(t *testing.T) {
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	activities := &activityRepositoryStub{}
	characters := &characterRepositoryStub{value: character}
	activities.characters = characters
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service, err := NewServiceWithClock(activities, characters, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("StartTraining() expected success with nil scheduler, got error: %v", err)
	}
	if got.ID == "" {
		t.Fatal("activity ID is empty")
	}
}

func TestClaimTrainingBeforeReadyIsRejected(t *testing.T) {
	service, _, _, characters := newTestService(t)
	activity, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Claim(context.Background(), activity.ID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Claim() error = %v, want %v", err, ErrNotReady)
	}
}

func TestClaimTrainingAwardsExperienceOnce(t *testing.T) {
	service, clock, _, characters := newTestService(t)
	activity, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = activity.AvailableAt

	got, err := service.Claim(context.Background(), activity.ID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !got.Claimed || characters.value.Experience != TrainingReward || characters.value.Level != 2 {
		t.Fatalf("Claim() result = %#v, character = %#v", got, characters.value)
	}
	if _, err := service.Claim(context.Background(), activity.ID); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second Claim() error = %v, want %v", err, ErrAlreadyClaimed)
	}
}

func TestClaimTrainingAppliesRewardAtMostOnceConcurrently(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	value, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, claimErr := service.Claim(context.Background(), value.ID)
			errs <- claimErr
		}()
	}
	wg.Wait()
	close(errs)

	var successes, alreadyClaimed int
	for claimErr := range errs {
		switch {
		case claimErr == nil:
			successes++
		case errors.Is(claimErr, ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("Claim() error = %v", claimErr)
		}
	}
	if successes != 1 || alreadyClaimed != 1 {
		t.Fatalf("concurrent claims: successes = %d, already claimed = %d", successes, alreadyClaimed)
	}
	if characters.value.Experience != TrainingReward || !repository.value.Claimed {
		t.Fatalf("concurrent claim state = activity %#v, character %#v", repository.value, characters.value)
	}
}

func TestActivityNewServiceNilDependencies(t *testing.T) {
	activities := &activityRepositoryStub{}
	characters := &characterRepositoryStub{}
	clock := &testClock{}
	scheduler := &testScheduler{}
	logger := nopLogger{}

	if _, err := NewService(nil, characters, scheduler, logger); err == nil {
		t.Fatal("NewService(nil, ...) expected error, got nil")
	}
	if _, err := NewService(activities, nil, scheduler, logger); err == nil {
		t.Fatal("NewService(..., nil, ...) expected error, got nil")
	}
	if _, err := NewServiceWithClock(activities, characters, scheduler, logger, nil); err == nil {
		t.Fatal("NewServiceWithClock(..., nil) expected error, got nil")
	}
	if _, err := NewServiceWithClock(activities, characters, scheduler, logger, clock); err != nil {
		t.Fatalf("NewServiceWithClock(...) error = %v", err)
	}

	svc, err := NewService(activities, characters, scheduler, logger)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestActivityStartTrainingInvalidCharacter(t *testing.T) {
	service, _, _, _ := newTestService(t)

	if _, err := service.StartTraining(context.Background(), ""); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("StartTraining(\"\") error = %v, want %v", err, corecharacter.ErrNotFound)
	}
	if _, err := service.StartTraining(context.Background(), "nonexistent_char"); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("StartTraining(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}

func TestActivityClaimNotFound(t *testing.T) {
	service, _, _, _ := newTestService(t)

	if _, err := service.Claim(context.Background(), "nonexistent_activity"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Claim(nonexistent) error = %v, want %v", err, ErrNotFound)
	}
}

func TestActivityRealClock(t *testing.T) {
	clock := realClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("realClock.Now() returned zero time")
	}
}

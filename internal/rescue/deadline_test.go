package rescue

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

func TestEmergencyRescue_PublicationUsesSavedDeadline(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "elapsed publication", true: "expired obligation"}[expired], func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			until := now.Add(2 * time.Minute)
			if expired {
				until = now.Add(-time.Minute)
			}
			repo := &stubRescueRepo{records: []RescueRecord{{
				ID: "receipt", CharacterID: "char-1", Reason: "stuck",
				CreatedAt: now.Add(-10 * time.Minute), SleepUntil: until, PenaltySeconds: DefaultPenaltySeconds,
			}}}
			timers := timer.NewService(nil)
			svc := newTestService(repo, heroRepo(), &stubActionCleaner{stuckCharacters: map[string]bool{}}, timers)
			if _, err := svc.EmergencyRescue(ctx, "char-1", "retry", now.Add(-5*time.Minute)); err != nil {
				t.Fatal(err)
			}
			remaining, err := timers.GetRemainingLock(ctx, timer.CategorySleep, "char-1")
			if err != nil || remaining > 2*time.Minute || (expired && remaining != 0) || (!expired && remaining < time.Minute) {
				t.Fatalf("sleep=%s err=%v expired=%v", remaining, err, expired)
			}
			if repo.records[0].AppliedAt.IsZero() {
				t.Fatal("receipt was not confirmed")
			}
		})
	}
}

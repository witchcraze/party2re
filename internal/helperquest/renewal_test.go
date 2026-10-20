package helperquest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/helperquest"
)

func renewalService(t *testing.T, questRepo *mockQuestRepo, tx helperquest.TransactionProvider) *helperquest.Service {
	t.Helper()
	dep, err := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	mats, err := item.NewInstance("weapon-01", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(mats); err != nil {
		t.Fatal(err)
	}
	depotRepo := newMockDepotRepo()
	if err := depotRepo.Save(context.Background(), dep); err != nil {
		t.Fatal(err)
	}
	charRepo := &mockCharRepo{characters: map[string]corecharacter.Character{"char-1": {ID: "char-1", Name: "Hero"}}}
	return helperquest.NewService(questRepo, charRepo, nil, tx, helperquest.WithDepotRepository(depotRepo))
}

func renewalQuest(id string, createdAt, expiresAt time.Time) helperquest.Quest {
	return helperquest.Quest{
		ID: id, Title: id, Kind: helperquest.KindWeapon, TargetID: "weapon-01", TargetName: "ヒノキの棒",
		RequiredCount: 2, RewardItemID: "item-128", CreatedAt: createdAt, ExpiresAt: expiresAt,
	}
}

func seed(t *testing.T, repo *mockQuestRepo, quests ...helperquest.Quest) {
	t.Helper()
	for _, q := range quests {
		if err := repo.Save(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompleteQuest_ExchangesExpiredNonTargetRequestsOneForOne(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := newMockQuestRepo()
	completedAt := now.Add(-time.Hour)
	doneExpired := renewalQuest("done-expired", now.Add(-8*24*time.Hour), now.Add(-2*24*time.Hour))
	doneExpired.CompletedAt, doneExpired.CompletedBy = &completedAt, "someone"
	seed(t, repo,
		renewalQuest("target", now, now.Add(time.Hour)),
		renewalQuest("live-other", now, now.Add(time.Hour)),
		renewalQuest("expired-1", now.Add(-8*24*time.Hour), now.Add(-2*24*time.Hour)),
		renewalQuest("expired-2", now.Add(-7*24*time.Hour), now.Add(-time.Second)),
		doneExpired,
	)
	svc := renewalService(t, repo, &mockTxProvider{})

	if _, err := svc.CompleteQuest(ctx, "char-1", "target", now); err != nil {
		t.Fatalf("CompleteQuest: %v", err)
	}

	for _, id := range []string{"expired-1", "expired-2"} {
		if _, ok := repo.quests[id]; ok {
			t.Errorf("expired request %s was not exchanged", id)
		}
	}
	if _, ok := repo.quests["done-expired"]; !ok {
		t.Error("completed request must be retained, not exchanged")
	}
	if _, ok := repo.quests["live-other"]; !ok {
		t.Error("unexpired request must be retained")
	}
	// live-other + 2 exchanged + 1 completion replacement; the target is completed.
	active, _ := repo.ListUncompleted(ctx)
	if len(active) != 4 {
		t.Fatalf("active requests = %d, want 4", len(active))
	}
}

func TestCompleteQuest_RenewalOnlyExchangesExpiredRequests(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := newMockQuestRepo()
	seed(t, repo,
		renewalQuest("expired-1", now.Add(-8*24*time.Hour), now.Add(-time.Hour)),
		renewalQuest("expired-2", now.Add(-8*24*time.Hour), now.Add(-time.Hour)),
	)
	svc := renewalService(t, repo, &mockTxProvider{})

	res, err := svc.CompleteQuest(ctx, "char-1", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if res.RenewedCount != 2 || res.CompletedQuest != nil || res.Character != nil || res.Depot != nil || res.NewQuest != nil {
		t.Fatalf("renewal-only result = %+v", res)
	}

	if len(repo.quests) != 2 {
		t.Fatalf("stored requests = %d, want 2 exchanged one-for-one", len(repo.quests))
	}
	active, _ := repo.ListUncompleted(ctx)
	if len(active) != 2 {
		t.Fatalf("active requests = %d, want 2 after all-expired exchange", len(active))
	}

	// A retry exchanges nothing more.
	if res, err := svc.CompleteQuest(ctx, "char-1", "", now); err != nil || res.RenewedCount != 0 {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	if len(repo.quests) != 2 {
		t.Fatalf("stored requests after retry = %d, want 2", len(repo.quests))
	}
}

func TestCompleteQuest_ExpiredTargetCompletes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := newMockQuestRepo()
	seed(t, repo, renewalQuest("target", now.Add(-8*24*time.Hour), now.Add(-time.Hour)))
	svc := renewalService(t, repo, &mockTxProvider{})

	res, err := svc.CompleteQuest(ctx, "char-1", "target", now)
	if err != nil {
		t.Fatal(err)
	}
	if res.CompletedQuest.ID != "target" || res.Character.HelpCount != 1 || res.RenewedCount != 0 {
		t.Fatalf("expired target completion = %+v", res)
	}
	if len(repo.quests) != 2 || repo.quests["target"].CompletedAt == nil {
		t.Fatalf("stored requests = %+v, want completed target and replacement", repo.quests)
	}
}

func TestCompleteQuest_ExpiryBoundaryIsSixDaysExclusive(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := newMockQuestRepo()
	seed(t, repo, renewalQuest("target", start, start.Add(time.Hour)))
	svc := renewalService(t, repo, &mockTxProvider{})
	if _, err := svc.CompleteQuest(ctx, "char-1", "target", start); err != nil {
		t.Fatal(err)
	}
	var replacement helperquest.Quest
	active, _ := repo.ListUncompleted(ctx)
	if len(active) != 1 {
		t.Fatalf("active = %d, want 1 completion replacement", len(active))
	}
	replacement = active[0]
	deadline := start.Add(6 * 24 * time.Hour)
	if !replacement.ExpiresAt.Equal(deadline) {
		t.Fatalf("expires_at = %v, want %v", replacement.ExpiresAt, deadline)
	}

	for _, tc := range []struct {
		name      string
		now       time.Time
		exchanged bool
	}{
		{"just before deadline", deadline.Add(-time.Second), false},
		{"at deadline", deadline, false},
		{"just after deadline", deadline.Add(time.Second), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newMockQuestRepo()
			seed(t, r, replacement)
			s := renewalService(t, r, &mockTxProvider{})
			if _, err := s.CompleteQuest(ctx, "char-1", "", tc.now); err != nil {
				t.Fatal(err)
			}
			_, kept := r.quests[replacement.ID]
			if kept == tc.exchanged {
				t.Fatalf("kept original = %t, want %t", kept, !tc.exchanged)
			}
		})
	}
}

func TestCompleteQuest_EmptyStoreStaysEmpty(t *testing.T) {
	repo := newMockQuestRepo()
	svc := renewalService(t, repo, &mockTxProvider{})
	if _, err := svc.CompleteQuest(context.Background(), "char-1", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(repo.quests) != 0 {
		t.Fatalf("empty store gained %d requests; initial supply is zero (#1226)", len(repo.quests))
	}
}

func TestCompleteQuest_UnknownTargetDoesNotRenewBoard(t *testing.T) {
	repo := newMockQuestRepo()
	now := time.Now().UTC()
	q := renewalQuest("expired", now.Add(-7*24*time.Hour), now.Add(-time.Second))
	seed(t, repo, q)
	svc := renewalService(t, repo, &mockTxProvider{})
	if _, err := svc.CompleteQuest(context.Background(), "char-1", "unknown", now); !errors.Is(err, helperquest.ErrQuestNotFound) {
		t.Fatalf("error = %v, want ErrQuestNotFound", err)
	}
	if len(repo.quests) != 1 || repo.quests[q.ID] != q {
		t.Fatalf("unknown target changed board: %+v", repo.quests)
	}
}

func TestCompleteQuest_RenewalFailurePropagates(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := newMockQuestRepo()
	seed(t, repo,
		renewalQuest("target", now, now.Add(time.Hour)),
		renewalQuest("expired", now.Add(-8*24*time.Hour), now.Add(-time.Hour)),
	)
	repo.failReplacementSave = true
	tx := &mockTxProvider{}
	svc := renewalService(t, repo, tx)

	if _, err := svc.CompleteQuest(ctx, "char-1", "", now); err == nil {
		t.Fatal("expected replacement save failure")
	}
	if !tx.rollbackCalled {
		t.Error("expected renewal transaction rollback")
	}
	if repo.quests["target"].CompletedAt != nil {
		t.Error("completion must not run after a failed renewal")
	}
}

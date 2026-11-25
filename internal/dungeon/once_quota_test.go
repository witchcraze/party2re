package dungeon_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// Legacy dungeon 16 (quest.cgi:654,1224–1228): starting consumes the leader's
// once quota, the menu hides the map while it is consumed, and joiners are
// not checked. Map16 is not native yet, so a quota copy of map0 stands in.
func TestOnceQuota_StartListingAndJoiners(t *testing.T) {
	ctx := context.Background()
	quotaMap := mapDef(t, dungeon.NativeMap0ID)
	quotaMap.ID, quotaMap.Once = "quota-map", true
	quota := timer.NewService(nil)
	f := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), createTestChar("lead", 1, 100, 10, 10),
		dungeon.WithCustomDungeons([]dungeon.Dungeon{quotaMap}), dungeon.WithOnceQuota(quota))
	for _, id := range []string{"busy", "spent"} {
		f.chars.chars[id] = createTestChar(id, 1, 100, 10, 10)
	}
	used := func(id string) bool {
		t.Helper()
		locked, err := quota.IsLocked(ctx, timer.CategoryDungeonOnce, id)
		if err != nil {
			t.Fatal(err)
		}
		return locked
	}
	listed := func(id string) bool {
		t.Helper()
		ds, err := f.svc.ListDungeons(ctx, f.chars.chars[id])
		if err != nil {
			t.Fatal(err)
		}
		return len(ds) == 1
	}

	// A failed start releases the leader's quota: busy already has a run.
	if _, err := f.svc.StartExpedition(ctx, "busy", quotaMap.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.StartPartyExpedition(ctx, "lead", []string{"lead", "busy"}, quotaMap.ID, "p"); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Fatalf("party with busy member err=%v", err)
	}
	if used("lead") || !listed("lead") {
		t.Fatal("failed start kept the leader's quota")
	}

	// A joiner whose quota is consumed is accepted; only the leader consumes.
	if ok, err := quota.TryLockWithToken(ctx, timer.CategoryDungeonOnce, "spent", "earlier", 0); err != nil || !ok {
		t.Fatalf("lock spent: %v %v", ok, err)
	}
	if listed("spent") {
		t.Fatal("listing shows the quota map to a consumed viewer")
	}
	run, err := f.svc.StartPartyExpedition(ctx, "lead", []string{"lead", "spent"}, quotaMap.ID, "p")
	if err != nil {
		t.Fatalf("party with consumed joiner: %v", err)
	}
	if !used("lead") || listed("lead") {
		t.Fatal("start did not consume the leader's quota")
	}
	if ok, err := quota.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, "lead", run.ID); err != nil || !ok {
		t.Fatalf("quota token is not the run ID: %v %v", ok, err)
	}
	if ok, err := quota.TryLockWithToken(ctx, timer.CategoryDungeonOnce, "lead", run.ID, 0); err != nil || !ok {
		t.Fatalf("relock: %v %v", ok, err)
	}

	// A consumed leader is rejected before any run check, keeping the quota.
	if _, err := f.svc.StartExpedition(ctx, "lead", quotaMap.ID); !errors.Is(err, dungeon.ErrOnceQuotaUsed) {
		t.Fatalf("second start err=%v", err)
	}
	if !used("lead") {
		t.Fatal("rejected start released the earlier quota")
	}

	// Without a configured quota the map is neither listed nor startable.
	g := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), createTestChar("solo", 1, 100, 10, 10),
		dungeon.WithCustomDungeons([]dungeon.Dungeon{quotaMap}))
	if ds, err := g.svc.ListDungeons(ctx, g.chars.chars["solo"]); err != nil || len(ds) != 0 {
		t.Fatalf("unconfigured listing %v %v", ds, err)
	}
	if _, err := g.svc.StartExpedition(ctx, "solo", quotaMap.ID); err == nil {
		t.Fatal("unconfigured quota admitted a start")
	}
}

// saveAckLostStore fails every save, after writing it when written is set.
type saveAckLostStore struct {
	dungeon.ActiveExpeditionStore
	written bool
}

func (s *saveAckLostStore) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	if s.written {
		if err := s.ActiveExpeditionStore.SaveActiveExpedition(ctx, exp); err != nil {
			return err
		}
	}
	return errors.New("save acknowledgement lost")
}

// A failed save may have published the run, so it never returns the quota:
// a verified run is the start result, and an absent one is an unknown outcome.
func TestOnceQuota_FailedSaveKeepsQuota(t *testing.T) {
	ctx := context.Background()
	quotaMap := mapDef(t, dungeon.NativeMap0ID)
	quotaMap.ID, quotaMap.Once = "quota-map", true
	for _, written := range []bool{true, false} {
		quota := timer.NewService(nil)
		store := &saveAckLostStore{ActiveExpeditionStore: dungeon.NewMemoryExpeditionRepository(), written: written}
		f := newNativeFixtureOn(t, store, createTestChar("lead", 1, 100, 10, 10),
			dungeon.WithCustomDungeons([]dungeon.Dungeon{quotaMap}), dungeon.WithOnceQuota(quota))
		run, err := f.svc.StartExpedition(ctx, "lead", quotaMap.ID)
		if written && (err != nil || run == nil) {
			t.Fatalf("written save: run must be returned: %+v %v", run, err)
		}
		if !written && !errors.Is(err, dungeon.ErrStartOutcomeUnknown) {
			t.Fatalf("unwritten save err=%v", err)
		}
		if locked, err := quota.IsLocked(ctx, timer.CategoryDungeonOnce, "lead"); err != nil || !locked {
			t.Fatalf("written=%v: quota released after a failed save: %v %v", written, locked, err)
		}
	}
}

package costume_test

import (
	"context"
	"os"
	"testing"
	"time"

	vk "github.com/witchcraze/party2re/internal/valkey"

	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/id"
)

// restoreContract: restore only while the applied value is current; previous nil clears.
func restoreContract(t *testing.T, repo costume.Repository) {
	t.Helper()
	ctx := context.Background()
	char := "restore-" + id.New()
	t.Cleanup(func() { _ = repo.ClearActiveCostume(ctx, char) })
	now := time.Now().UTC().Truncate(time.Second)
	prev := costume.ActiveCostume{CharacterID: char, ItemNo: 44, ItemName: "prev", Icon: "a", RentedAt: now, ExpiresAt: now.Add(time.Hour)}
	applied := costume.ActiveCostume{CharacterID: char, ItemNo: 58, ItemName: "book", Icon: "b", RentedAt: now, ExpiresAt: now.Add(time.Hour)}
	newer := costume.ActiveCostume{CharacterID: char, ItemNo: 45, ItemName: "newer", Icon: "c", RentedAt: now, ExpiresAt: now.Add(time.Hour)}
	save := func(c costume.ActiveCostume) {
		if err := repo.SaveActiveCostume(ctx, c, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	get := func() *costume.ActiveCostume {
		c, err := repo.GetActiveCostume(ctx, char)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	save(prev)
	save(applied)
	if ok, err := repo.RestoreActiveCostume(ctx, applied, &prev); err != nil || !ok {
		t.Fatalf("restore current: %v %v", ok, err)
	}
	if got := get(); got == nil || got.ItemNo != prev.ItemNo || got.Icon != prev.Icon {
		t.Fatalf("previous not restored: %+v", got)
	}

	save(applied)
	save(newer) // a later writer
	if ok, err := repo.RestoreActiveCostume(ctx, applied, &prev); err != nil || ok {
		t.Fatalf("restore over newer: %v %v", ok, err)
	}
	if got := get(); got == nil || got.ItemNo != newer.ItemNo {
		t.Fatalf("newer overwritten: %+v", got)
	}

	save(applied)
	if ok, err := repo.RestoreActiveCostume(ctx, applied, nil); err != nil || !ok || get() != nil {
		t.Fatalf("nil previous must clear: %v %v", ok, err)
	}
	if ok, err := repo.RestoreActiveCostume(ctx, applied, &prev); err != nil || ok || get() != nil {
		t.Fatalf("a reset must not be revived: %v %v", ok, err)
	}
}

func TestMemoryCostumeRepository_RestoreActiveCostume(t *testing.T) {
	restoreContract(t, costume.NewMemoryCostumeRepository())
}

func TestValkeyCostumeRepository_RestoreActiveCostume(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	restoreContract(t, costume.NewValkeyCostumeRepository(client))
}

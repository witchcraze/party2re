package costume_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyCostumeRepository_NilClient(t *testing.T) {
	ctx := context.Background()
	repo := costume.NewValkeyCostumeRepository(nil)

	activeCostume, err := repo.GetActiveCostume(ctx, "c1")
	if err != nil || activeCostume != nil {
		t.Fatalf("expected nil costume and nil error, got: %v, %v", activeCostume, err)
	}

	err = repo.SaveActiveCostume(ctx, costume.ActiveCostume{CharacterID: "c1"}, time.Hour)
	if err != nil {
		t.Fatalf("expected nil error on nil client save, got: %v", err)
	}

	err = repo.ClearActiveCostume(ctx, "c1")
	if err != nil {
		t.Fatalf("expected nil error on nil client clear, got: %v", err)
	}
}

func TestValkeyCostumeRepository_GetActiveCostume(t *testing.T) {
	ctx := context.Background()

	// 1. Key not found (Nil)
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repo := costume.NewValkeyCostumeRepository(client)
	c, err := repo.GetActiveCostume(ctx, "c1")
	if err != nil || c != nil {
		t.Fatalf("expected nil, nil for missing key, got: %v, %v", c, err)
	}

	// 2. Client error
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("valkey connection broken"))
	}))
	repoErr := costume.NewValkeyCostumeRepository(clientErr)
	_, err = repoErr.GetActiveCostume(ctx, "c1")
	if err == nil {
		t.Fatal("expected error from broken valkey connection")
	}

	// 3. Valid JSON active costume
	future := time.Now().Add(2 * time.Hour).UTC()
	active := costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "チョビヒゲタクシード",
		Icon:        "chr/012.gif",
		RentedAt:    time.Now().Add(-1 * time.Hour).UTC(),
		ExpiresAt:   future,
	}
	raw, _ := json.Marshal(active)
	clientOK := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult(string(raw))
	}))
	repoOK := costume.NewValkeyCostumeRepository(clientOK)
	got, err := repoOK.GetActiveCostume(ctx, "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.CharacterID != "c1" || got.ItemNo != 46 {
		t.Fatalf("unexpected costume returned: %+v", got)
	}

	// 4. Expired costume in cache returns nil
	expired := costume.ActiveCostume{
		CharacterID: "c1",
		ExpiresAt:   time.Now().Add(-1 * time.Minute).UTC(),
	}
	rawExpired, _ := json.Marshal(expired)
	clientExpired := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult(string(rawExpired))
	}))
	repoExpired := costume.NewValkeyCostumeRepository(clientExpired)
	gotExpired, err := repoExpired.GetActiveCostume(ctx, "c1")
	if err != nil || gotExpired != nil {
		t.Fatalf("expected nil for expired costume, got: %v, %v", gotExpired, err)
	}

	// 5. Corrupt JSON
	clientBadJSON := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("invalid json{")
	}))
	repoBadJSON := costume.NewValkeyCostumeRepository(clientBadJSON)
	_, err = repoBadJSON.GetActiveCostume(ctx, "c1")
	if err == nil {
		t.Fatal("expected unmarshal error on corrupt JSON")
	}
}

func TestValkeyCostumeRepository_SaveActiveCostume(t *testing.T) {
	ctx := context.Background()

	var recordedCmd []string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		recordedCmd = cmd.Commands()
		return valkeytest.MakeOKResult()
	}))
	repo := costume.NewValkeyCostumeRepository(client)

	active := costume.ActiveCostume{
		CharacterID: "char-123",
		ItemNo:      55,
		ItemName:    "水着",
		Icon:        "m55.gif",
	}
	err := repo.SaveActiveCostume(ctx, active, 3600*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPrefix := []string{"SET", "party2:daily:costume:char-123"}
	if len(recordedCmd) < 5 || !reflect.DeepEqual(recordedCmd[:2], expectedPrefix) || recordedCmd[3] != "EX" || recordedCmd[4] != "3600" {
		t.Fatalf("unexpected commands recorded: %v", recordedCmd)
	}

	// TTL <= 0 fallback to 86400
	err = repo.SaveActiveCostume(ctx, active, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recordedCmd[4] != "86400" {
		t.Fatalf("expected fallback TTL 86400, got: %s", recordedCmd[4])
	}
}

func TestValkeyCostumeRepository_ClearActiveCostume(t *testing.T) {
	ctx := context.Background()

	var recordedCmd []string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		recordedCmd = cmd.Commands()
		return valkeytest.MakeOKResult()
	}))
	repo := costume.NewValkeyCostumeRepository(client)

	err := repo.ClearActiveCostume(ctx, "char-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCmd := []string{"DEL", "party2:daily:costume:char-123"}
	if !reflect.DeepEqual(recordedCmd, expectedCmd) {
		t.Fatalf("expected %v, got %v", expectedCmd, recordedCmd)
	}
}

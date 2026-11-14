package casino_test

import (
	"context"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/id"
)

func TestCasinoValkeyFixtureCleanup(t *testing.T) {
	observer := openIsolatedTestValkey(t, 0)
	t.Cleanup(observer.Close)
	ctx := context.Background()
	prefix := "cleanup-" + id.New()
	room, actor, unrelated := prefix+"-room", prefix+"-actor", prefix+"-unrelated"
	roomKey := casino.DefaultRoomKeyPrefix + room
	actorKey := casino.DefaultCharacterKeyPrefix + actor
	unrelatedKey := casino.DefaultRoomKeyPrefix + unrelated
	t.Cleanup(func() {
		if err := observer.Do(ctx, observer.B().Del().Key(roomKey, actorKey, unrelatedKey).Build()).Error(); err != nil {
			t.Error(err)
		}
		if err := observer.Do(ctx, observer.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(room, unrelated).Build()).Error(); err != nil {
			t.Error(err)
		}
	})
	for _, key := range []string{roomKey, actorKey, unrelatedKey} {
		if err := observer.Do(ctx, observer.B().Set().Key(key).Value("fixture").Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}
	if err := observer.Do(ctx, observer.B().Zadd().Key(casino.DefaultRoomsActiveIndexKey).ScoreMember().ScoreMember(1, room).ScoreMember(2, unrelated).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	t.Run("fixture returns before normal teardown", func(t *testing.T) {
		writer := openIsolatedTestValkey(t, 0)
		t.Cleanup(writer.Close)
		registerCasinoValkeyCleanup(t, writer, []string{room}, []string{actor})
	})
	if count, err := observer.Do(ctx, observer.B().Exists().Key(roomKey, actorKey).Build()).AsInt64(); err != nil || count != 0 {
		t.Fatalf("owned keys after cleanup = %d, %v", count, err)
	}
	if err := observer.Do(ctx, observer.B().Zscore().Key(casino.DefaultRoomsActiveIndexKey).Member(room).Build()).Error(); !valkeygo.IsValkeyNil(err) {
		t.Fatalf("owned index member survived cleanup: %v", err)
	}
	if value, err := observer.Do(ctx, observer.B().Get().Key(unrelatedKey).Build()).ToString(); err != nil || value != "fixture" {
		t.Fatalf("unrelated payload changed: %q, %v", value, err)
	}
	if score, err := observer.Do(ctx, observer.B().Zscore().Key(casino.DefaultRoomsActiveIndexKey).Member(unrelated).Build()).AsFloat64(); err != nil || score != 2 {
		t.Fatalf("unrelated index member changed: %v, %v", score, err)
	}
}

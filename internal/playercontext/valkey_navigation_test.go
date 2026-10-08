package playercontext

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyNavigationCommandsAndBounds(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	n := Selection{Destination: "shop_weapon", Subject: Subject{Kind: "item", ID: "weapon-01"}}
	var commands [][]string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(got context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if got != ctx {
			t.Fatal("lost request context")
		}
		args := append([]string(nil), cmd.Commands()...)
		commands = append(commands, args)
		if args[0] == "GET" {
			return valkeytest.MakeStringResult(commands[0][2])
		}
		return valkeytest.MakeOKResult()
	}))
	r := NewValkeyNavigationRepository(client)
	if err := r.Save(ctx, "hero", n); err != nil {
		t.Fatal(err)
	}
	got, err := r.Load(ctx, "hero")
	if err != nil || got != n {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	if len(commands) != 2 || !reflect.DeepEqual(commands[0][3:], []string{"EX", "604800"}) || !reflect.DeepEqual(commands[1], []string{"GET", navigationKeyPrefix + "hero"}) {
		t.Fatalf("unexpected commands: %v", commands)
	}
	for _, invalid := range []Selection{{}, {Destination: "town", Offset: -1}, {Destination: "town", Limit: 101}, {Destination: "town", Subject: Subject{Kind: "item", ID: strings.Repeat("x", 129)}}} {
		if err := r.Save(ctx, "hero", invalid); !errors.Is(err, ErrInvalidSelection) {
			t.Fatal(err)
		}
	}
	if len(commands) != 2 {
		t.Fatal("invalid writes reached Valkey")
	}
	for _, raw := range []string{`null`, `{}`, `{"destination":"town","unknown":1}`, `{"destination":"town"} {}`, strings.Repeat(" ", 2049), `{"destination":"town","limit":101}`} {
		client.DoFn = func(context.Context, valkey.Completed) valkey.ValkeyResult { return valkeytest.MakeStringResult(raw) }
		if _, err := r.Load(ctx, "hero"); err == nil {
			t.Fatalf("accepted corrupt record %q", raw)
		}
	}
	client.DoFn = func(context.Context, valkey.Completed) valkey.ValkeyResult { return valkeytest.MakeNilResult() }
	if n, err := r.Load(ctx, "hero"); err != nil || n != (Selection{}) {
		t.Fatalf("missing: %+v %v", n, err)
	}
	wantErr := errors.New("Valkey failed")
	client.DoFn = func(context.Context, valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(wantErr)
	}
	if _, err := r.Load(ctx, "hero"); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	if err := r.Save(ctx, "hero", n); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	if _, err := NewValkeyNavigationRepository(nil).Load(ctx, "hero"); !errors.Is(err, ErrNavigationNotConfigured) {
		t.Fatal(err)
	}
	if err := NewValkeyNavigationRepository(nil).Save(ctx, "hero", n); !errors.Is(err, ErrNavigationNotConfigured) {
		t.Fatal(err)
	}
}

func TestValkeyNavigationLifetimeAndSharedClients(t *testing.T) {
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR required for integration test")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	actorID := id.New()
	key := navigationKeyPrefix + actorID
	t.Cleanup(func() {
		if err := client.Do(context.Background(), client.B().Del().Key(key).Build()).Error(); err != nil {
			t.Error(err)
		}
	})
	r, second := NewValkeyNavigationRepository(client), NewValkeyNavigationRepository(client)
	if n, err := r.Load(ctx, actorID); err != nil || n != (Selection{}) {
		t.Fatalf("missing %+v %v", n, err)
	}
	if exists, err := client.Do(ctx, client.B().Exists().Key(key).Build()).ToInt64(); err != nil || exists != 0 {
		t.Fatalf("GET created record: %d %v", exists, err)
	}
	if err := r.Save(ctx, actorID, Selection{Destination: "bank"}); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Expire().Key(key).Seconds(60).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if n, err := second.Load(ctx, actorID); err != nil || n.Destination != "bank" {
		t.Fatalf("shared read %+v %v", n, err)
	}
	if ttl, err := client.Do(ctx, client.B().Ttl().Key(key).Build()).ToInt64(); err != nil || ttl > 60 || ttl <= 0 {
		t.Fatalf("GET renewed TTL: %d %v", ttl, err)
	}
	cursor := ""
	if err := second.Save(ctx, actorID, Selection{Destination: "home_inbox", Cursor: &cursor}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Load(ctx, actorID); err != nil || n.Destination != "home_inbox" || n.Cursor == nil || *n.Cursor != "" {
		t.Fatalf("last write %+v %v", n, err)
	}
	if ttl, err := client.Do(ctx, client.B().Ttl().Key(key).Build()).ToInt64(); err != nil || ttl < 604795 || ttl > 604800 {
		t.Fatalf("write TTL: %d %v", ttl, err)
	}
	if kind, err := client.Do(ctx, client.B().Type().Key(key).Build()).ToString(); err != nil || kind != "string" {
		t.Fatalf("type %s %v", kind, err)
	}
	if err := client.Do(ctx, client.B().Expire().Key(key).Seconds(0).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Load(ctx, actorID); err != nil || n != (Selection{}) {
		t.Fatalf("expired %+v %v", n, err)
	}
}

package placelog_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestEntryVisibleTo(t *testing.T) {
	public := placelog.Entry{SpeakerCharacterID: "a"}
	whisper := placelog.Entry{SpeakerCharacterID: "a", RecipientCharacterID: "b"}
	for _, tc := range []struct {
		e      placelog.Entry
		viewer string
		want   bool
	}{
		{public, "z", true}, {public, "", true},
		{whisper, "a", true}, {whisper, "b", true}, {whisper, "z", false}, {whisper, "", false},
	} {
		if got := tc.e.VisibleTo(tc.viewer); got != tc.want {
			t.Errorf("%+v.VisibleTo(%q) = %v, want %v", tc.e, tc.viewer, got, tc.want)
		}
	}
}

func TestReadFailures(t *testing.T) {
	ctx := context.Background()
	place := placelog.Place{Kind: placelog.KindFacility, ID: "bank"}
	broken := placelog.New(valkeytest.NewMockClient(valkeytest.WithDoHandler(func(context.Context, valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("down"))
	})), placelog.Config{})
	if _, err := broken.Read(ctx, place, "c"); err == nil {
		t.Fatal("store failure must not read as an empty log")
	}
	if _, err := broken.Read(ctx, placelog.Place{Kind: "facility"}, "c"); err == nil {
		t.Fatal("empty place ID must be rejected")
	}
	if err := broken.Append(ctx, placelog.Place{Kind: "a:b", ID: "x"}, placelog.Entry{}); err == nil {
		t.Fatal("kind containing ':' must be rejected")
	}
}

func realLog(t *testing.T, cfg placelog.Config) (*placelog.Log, valkey.Client) {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(client.Close)
	return placelog.New(client, cfg), client
}

func testPlace(t *testing.T, client valkey.Client) placelog.Place {
	p := placelog.Place{Kind: "test", ID: id.New()}
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key("party2:placelog:test:"+p.ID).Build())
	})
	return p
}

func TestValkeyAppendReadTrimAndTTL(t *testing.T) {
	log, client := realLog(t, placelog.Config{Limit: 3, TTL: time.Hour})
	ctx := context.Background()
	p := testPlace(t, client)

	empty, err := log.Read(ctx, p, "a")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("missing log = %v, %v; want non-nil empty", empty, err)
	}
	row := func(text string) placelog.Entry {
		return placelog.Entry{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "a", Name: "A", Color: "#fff", Text: text}
	}
	if err := log.Append(ctx, p, row("1"), row("2")); err != nil {
		t.Fatal(err)
	}
	npc := placelog.Entry{SpeakerKind: placelog.SpeakerNPC, Name: "@N", Color: "#FF69B4", Text: "reply"}
	if err := log.Append(ctx, p, row("3"), npc); err != nil {
		t.Fatal(err)
	}
	got, err := log.Read(ctx, p, "z")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Text != "reply" || got[1].Text != "3" || got[2].Text != "2" {
		t.Fatalf("rows = %+v; want newest-first reply,3,2 trimmed to 3", got)
	}
	ttl, err := client.Do(ctx, client.B().Ttl().Key("party2:placelog:test:"+p.ID).Build()).AsInt64()
	if err != nil || ttl <= 0 || ttl > 3600 {
		t.Fatalf("ttl = %d, %v; want refreshed within 1h", ttl, err)
	}
}

func TestValkeyWhisperFiltering(t *testing.T) {
	log, client := realLog(t, placelog.Config{})
	ctx := context.Background()
	p := testPlace(t, client)
	if err := log.Append(ctx, p,
		placelog.Entry{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "a", Text: "public"},
		placelog.Entry{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "a", RecipientCharacterID: "b", Text: "secret"},
	); err != nil {
		t.Fatal(err)
	}
	for viewer, want := range map[string]int{"a": 2, "b": 2, "c": 1} {
		got, err := log.Read(ctx, p, viewer)
		if err != nil || len(got) != want {
			t.Errorf("viewer %s sees %d rows (%v), want %d", viewer, len(got), err, want)
		}
	}
}

func TestValkeyCorruptRowFailsRead(t *testing.T) {
	log, client := realLog(t, placelog.Config{})
	ctx := context.Background()
	p := testPlace(t, client)
	client.Do(ctx, client.B().Lpush().Key("party2:placelog:test:"+p.ID).Element("{not json").Build())
	if _, err := log.Read(ctx, p, "a"); err == nil {
		t.Fatal("corrupt row must fail the read")
	}
}

func TestValkeyConcurrentAppendsRespectCap(t *testing.T) {
	const limit = 30
	log, client := realLog(t, placelog.Config{Limit: limit, TTL: time.Hour})
	ctx := context.Background()
	p := testPlace(t, client)
	cfg := testutil.GetStressConfig()
	res := testutil.RunConcurrentStressTest(t, cfg, func(w, op int) error {
		return log.Append(ctx, p,
			placelog.Entry{SpeakerKind: placelog.SpeakerCharacter, Text: fmt.Sprintf("%d-%d", w, op)},
			placelog.Entry{SpeakerKind: placelog.SpeakerNPC, Text: fmt.Sprintf("%d-%d-npc", w, op)})
	})
	if res.Failures != 0 {
		t.Fatalf("append failures: %d", res.Failures)
	}
	n, err := client.Do(ctx, client.B().Llen().Key("party2:placelog:test:"+p.ID).Build()).AsInt64()
	if err != nil || n != limit {
		t.Fatalf("len = %d, %v; want %d", n, err, limit)
	}
	rows, err := log.Read(ctx, p, "")
	if err != nil {
		t.Fatal(err)
	}
	// Atomic appends keep each actor/NPC pair adjacent: the newest row is an NPC reply.
	for i := 0; i+1 < len(rows); i += 2 {
		if rows[i].Text != rows[i+1].Text+"-npc" {
			t.Fatalf("pair broken at %d: %q / %q", i, rows[i].Text, rows[i+1].Text)
		}
	}
}

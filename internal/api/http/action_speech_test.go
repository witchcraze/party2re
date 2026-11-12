package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type fakeCharacterNames map[string]string

func (f fakeCharacterNames) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	if id, ok := f[name]; ok {
		return corecharacter.Character{ID: id, Name: name}, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func speechRouter(t *testing.T, log PlaceLog) (*bankGatewayFixture, timer.Service, http.Handler) {
	t.Helper()
	return actionWaitRouterWith(t, "bank", timer.NewService(nil), playercontext.DefaultActionWait,
		WithPlaceLog(log), WithCharacterNames(fakeCharacterNames{"Bob": "bob"}))
}

func speechResult(t *testing.T, raw map[string]json.RawMessage) SpeechResult {
	t.Helper()
	var r SpeechResult
	if err := json.Unmarshal(raw["result"], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPlaceSayLogsAPublicRowAndIgnoresTheActionWait(t *testing.T) {
	log := &fakePlaceLog{}
	f, timers, router := speechRouter(t, log)
	f.char.Name, f.char.Color = "Hero", "#00ff00"
	if err := timers.SetLock(context.Background(), timer.CategoryActionWait, "hero", time.Minute); err != nil {
		t.Fatal(err)
	}
	payload := `<script>alert(1)</script>'; DROP TABLE characters;--`
	body, _ := json.Marshal(map[string]any{"action": "place_say", "params": map[string]string{"text": "a\r\nb " + payload}})
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil)
	want := "ab " + payload
	if status != 200 || speechResult(t, raw) != (SpeechResult{Text: want}) {
		t.Fatalf("place_say during the wait: %d %s %s", status, raw["result"], raw["error"])
	}
	got := log.rows[0]
	got.OccurredAt = time.Time{}
	if len(log.rows) != 1 || got != (placelog.Entry{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "hero", Name: "Hero", Color: "#00ff00", Text: want}) {
		t.Fatalf("rows: %+v", log.rows)
	}
	if log.places[0] != (placelog.Place{Kind: placelog.KindFacility, ID: "bank"}) {
		t.Fatalf("place: %+v", log.places)
	}
	if remaining, _ := timers.GetRemainingLock(context.Background(), timer.CategoryActionWait, "hero"); remaining <= 30*time.Second {
		t.Fatalf("speech changed the wait: %v", remaining)
	}
}

func TestPlaceSayDoesNotStartTheActionWait(t *testing.T) {
	_, _, router := speechRouter(t, &fakePlaceLog{})
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"hi"}}`, nil)
	if status != 200 || waitMS(t, raw) != 0 {
		t.Fatalf("place_say: %d %s", status, raw["error"])
	}
}

func TestPlaceWhisperIsPrivateToARecipientAndObeysTheWait(t *testing.T) {
	log := &fakePlaceLog{}
	f, timers, router := speechRouter(t, log)
	f.char.Name = "Hero"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Nobody","text":"hi"}}`, nil)
	if status != http.StatusNotFound || actionErrorCode(t, raw) != "RECIPIENT_NOT_FOUND" || !strings.Contains(string(raw["error"]), "Nobodyというプレイヤーが存在しません") || len(log.rows) != 0 {
		t.Fatalf("unknown recipient: %d %s rows %d", status, raw["error"], len(log.rows))
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Bob","text":"hi"}}`, nil)
	if status != 200 || waitMS(t, raw) != 0 || speechResult(t, raw) != (SpeechResult{Text: "＠ささやき>Bob hi", Private: true}) {
		t.Fatalf("whisper: %d %s %s", status, raw["result"], raw["error"])
	}
	if len(log.rows) != 1 || log.rows[0].RecipientCharacterID != "bob" || log.rows[0].SpeakerCharacterID != "hero" {
		t.Fatalf("rows: %+v", log.rows)
	}
	var observation PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &observation); err != nil || len(observation.Scene.PlaceLog) != 1 || !observation.Scene.PlaceLog[0].Private {
		t.Fatalf("sender view: %+v %v", observation.Scene.PlaceLog, err)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Bob","text":" \n "}}`, nil)
	if status != 200 || speechResult(t, raw).Text != "＠ささやき>Bob" {
		t.Fatalf("whisper without a body: %d %s %s", status, raw["result"], raw["error"])
	}
	if err := timers.SetLock(context.Background(), timer.CategoryActionWait, "hero", time.Minute); err != nil {
		t.Fatal(err)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Bob","text":"hi"}}`, nil)
	if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" || len(log.rows) != 2 {
		t.Fatalf("whisper during the wait: %d %s", status, raw["error"])
	}
}

func TestSpeechRejectsInvalidInputWithoutWriting(t *testing.T) {
	log := &fakePlaceLog{}
	_, _, router := speechRouter(t, log)
	for _, body := range []string{
		`{"action":"place_say","params":{}}`,
		`{"action":"place_say","params":{"text":null}}`,
		`{"action":"place_say","params":{"text":"   "}}`,
		`{"action":"place_say","params":{"text":"hi","to":"x"}}`,
		`{"action":"place_say","params":{"text":"a‮b"}}`,
		`{"action":"place_say","params":{"text":"a​b"}}`,
		`{"action":"place_say","params":{"text":"` + strings.Repeat("a", maxSpeechLength+1) + `"}}`,
		`{"action":"place_say","params":{"text":"` + strings.Repeat("&", maxSpeechLength/5+1) + `"}}`,
		`{"action":"place_whisper","params":{"text":"hi"}}`,
		`{"action":"place_whisper","params":{"recipient_name":"Bob"}}`,
		`{"action":"place_whisper","params":{"recipient_name":"Bob","text":"\u200b"}}`,
		`{"action":"place_whisper","params":{"recipient_name":"Bob","text":"\t"}}`,
		`{"action":"place_whisper","params":{"recipient_name":"Bob","text":"` + strings.Repeat(" ", maxSpeechLength+1) + `"}}`,
		`{"action":"place_whisper","params":{"recipient_name":"B o b","text":"hi"}}`,
	} {
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", body, nil); status != http.StatusBadRequest {
			t.Errorf("%.80s: %d %s", body, status, raw["error"])
		}
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"`+strings.Repeat("あ", maxSpeechLength)+`"}}`, nil); status != 200 {
		t.Fatalf("text at the limit: %d %s", status, raw["error"])
	}
	if len(log.rows) != 1 {
		t.Fatalf("rejected input wrote rows: %d", len(log.rows))
	}
}

func TestSpeechAppendFailureIsAnUnknownOutcome(t *testing.T) {
	_, _, router := speechRouter(t, &fakePlaceLog{err: errors.New("valkey down")})
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"hi"}}`, nil)
	if status != http.StatusInternalServerError || actionErrorCode(t, raw) != "EXECUTION_FAILED" {
		t.Fatalf("append failure: %d %s", status, raw["error"])
	}
}

func TestSpeechInTownAndRejectedOutsideAQualifiedPlace(t *testing.T) {
	log := &fakePlaceLog{}
	_, _, router := actionWaitRouterWith(t, "town", timer.NewService(nil), playercontext.DefaultActionWait, WithPlaceLog(log))
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"hi"}}`, nil); status != 200 {
		t.Fatalf("town speech: %d %s", status, raw["error"])
	}
	if len(log.places) == 0 || log.places[0] != (placelog.Place{Kind: placelog.KindFacility, ID: "town"}) {
		t.Fatalf("town place: %+v", log.places)
	}
	// A qualification lost after entry leaves the place, as for NPCs.
	f, store, _, router := secretShopSceneRouterWith(t, []Option{WithPlaceLog(log)})
	store.selection, f.char.JobLevel = playercontext.Selection{Destination: "secretshop"}, 6
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"hi"}}`, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "PLACE_UNAVAILABLE" {
		t.Fatalf("unqualified place: %d %s", status, raw["error"])
	}
	if len(log.rows) != 1 {
		t.Fatalf("rejected speech wrote a row: %+v", log.rows)
	}
}

// Real Valkey: speech during a held wait and whisper visibility per viewer.
// Rows are identified by unique text, so the shared bank log is never cleared.
func TestSpeechWithRealValkey(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx := context.Background()
	timers := timer.NewService(client)
	log := placelog.New(client, placelog.Config{})
	_ = timers.ReleaseLock(ctx, timer.CategoryActionWait, "hero")
	t.Cleanup(func() { _ = timers.ReleaseLock(context.Background(), timer.CategoryActionWait, "hero") })
	bob, tag := "bob-"+id.New(), id.New()
	_, _, router := actionWaitRouterWith(t, "bank", timers, time.Minute, WithPlaceLog(log), WithCharacterNames(fakeCharacterNames{"Bob": bob}))
	if err := timers.SetLock(ctx, timer.CategoryActionWait, "hero", time.Minute); err != nil {
		t.Fatal(err)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"say `+tag+`"}}`, nil); status != 200 {
		t.Fatalf("speech during the wait: %d %s", status, raw["error"])
	}
	if err := timers.ReleaseLock(ctx, timer.CategoryActionWait, "hero"); err != nil {
		t.Fatal(err)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Bob","text":"psst `+tag+`"}}`, nil); status != 200 {
		t.Fatalf("whisper: %d %s", status, raw["error"])
	}
	for viewer, want := range map[string]int{"hero": 2, bob: 2, "carol-" + tag: 1} {
		rows, err := log.Read(ctx, placelog.Place{Kind: placelog.KindFacility, ID: "bank"}, viewer)
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, r := range rows {
			if strings.HasSuffix(r.Text, tag) {
				got++
			}
		}
		if got != want {
			t.Fatalf("%s sees %d of this test's rows, want %d: %+v", viewer, got, want, rows)
		}
	}
}

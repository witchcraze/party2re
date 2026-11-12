package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type fakePlaceLog struct {
	rows   []placelog.Entry
	err    error
	places []placelog.Place
	viewer string
}

func (f *fakePlaceLog) Append(_ context.Context, p placelog.Place, entries ...placelog.Entry) error {
	if f.err != nil {
		return f.err
	}
	f.places = append(f.places, p)
	for _, e := range entries { // the last entry becomes the newest, as in placelog.Log
		f.rows = append([]placelog.Entry{e}, f.rows...)
	}
	return nil
}

func (f *fakePlaceLog) Read(_ context.Context, p placelog.Place, viewer string) ([]placelog.Entry, error) {
	f.places, f.viewer = append(f.places, p), viewer
	if f.err != nil {
		return nil, f.err
	}
	var out []placelog.Entry
	for _, r := range f.rows {
		if r.VisibleTo(viewer) {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestFacilitySceneObservesPlaceLog(t *testing.T) {
	at := time.Date(2026, 11, 8, 1, 2, 3, 0, time.UTC)
	log := &fakePlaceLog{rows: []placelog.Entry{
		{OccurredAt: at, SpeakerKind: placelog.SpeakerNPC, Name: "@タクシード", Color: "#FF69B4", Text: "100 Gお預かりいたしました"},
		{OccurredAt: at, SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "other", Name: "B", Color: "#fff", Text: "secret", RecipientCharacterID: "hero"},
		{OccurredAt: at, SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "x", Name: "C", Color: "#fff", Text: "not for hero", RecipientCharacterID: "y"},
	}}
	_, store, _, router := bankSceneRouter(t, WithPlaceLog(log))

	status, observation := navigationGET(t, router)
	want := []PlaceLogEntry{
		{OccurredAt: at, SpeakerKind: "npc", Name: "@タクシード", Color: "#FF69B4", Text: "100 Gお預かりいたしました"},
		{OccurredAt: at, SpeakerKind: "character", Name: "B", Color: "#fff", Text: "secret", Private: true},
	}
	if status != 200 || len(observation.Scene.PlaceLog) != 2 || observation.Scene.PlaceLog[0] != want[0] || observation.Scene.PlaceLog[1] != want[1] {
		t.Fatalf("place_log = %d %+v", status, observation.Scene.PlaceLog)
	}
	if len(log.places) != 1 || log.places[0] != (placelog.Place{Kind: placelog.KindFacility, ID: "bank"}) || log.viewer != "hero" || store.writes != 0 {
		t.Fatalf("read %+v as %q, writes %d", log.places, log.viewer, store.writes)
	}

	// Character IDs never leave the server.
	raw, _ := json.Marshal(observation.Scene.PlaceLog)
	for _, leaked := range []string{"other", "speaker_character_id", "recipient_character_id"} {
		if bytes.Contains(raw, []byte(leaked)) {
			t.Fatalf("place_log leaks %q: %s", leaked, raw)
		}
	}
}

func TestFacilitySceneEmptyAndFailedPlaceLog(t *testing.T) {
	_, _, _, router := bankSceneRouter(t, WithPlaceLog(&fakePlaceLog{}))
	if status, observation := navigationGET(t, router); status != 200 || observation.Scene.PlaceLog == nil || len(observation.Scene.PlaceLog) != 0 {
		t.Fatalf("empty log must be []: %d %+v", status, observation.Scene.PlaceLog)
	}
	_, _, _, router = bankSceneRouter(t, WithPlaceLog(&fakePlaceLog{err: errors.New("valkey down")}))
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("required place-log read failure must fail the observation, got %d", status)
	}
}

func TestTownSceneObservesItsOwnPlaceLog(t *testing.T) {
	log := &fakePlaceLog{}
	_, store, _, router := bankSceneRouter(t, WithPlaceLog(log))
	store.selection = playercontext.Selection{Destination: "town"}
	if status, observation := navigationGET(t, router); status != 200 || observation.Scene.PlaceLog == nil || len(log.places) != 1 || log.places[0].ID != "town" {
		t.Fatalf("town: %d %+v reads %+v", status, observation.Scene.PlaceLog, log.places)
	}
}

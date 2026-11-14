package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type trajectoryAlchemy struct {
	apihttp.AlchemyService
	percent int
}

func (a trajectoryAlchemy) GetCompendium(context.Context, string) (alchemy.Compendium, error) {
	return alchemy.Compendium{CompletionPercentage: a.percent}, nil
}

// The trajectory is public like legacy player.cgi: no session, and the response
// is exactly the milestone log and the four whole-percent completion rates.
func TestCharacterTrajectory(t *testing.T) {
	at := time.Date(2026, 11, 14, 1, 2, 3, 0, time.UTC)
	progress := func(percent float64) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
		return nil, collection.CompletionProgress{CompletionPercentage: percent}, nil
	}
	characters := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id != "c1" {
				return corecharacter.Character{}, corecharacter.ErrNotFound
			}
			return corecharacter.Character{ID: "c1", PlayerID: "private-player", Name: "Hero", Money: 987654}, nil
		},
		trajectoryFn: func(_ context.Context, id string) ([]character.Memory, error) {
			return []character.Memory{{
				Kind: corecharacter.MemoryJobMaster, Content: "★ 戦士 Job Master! ★", Name: "Hero", GuildName: "Knights",
				Color: "#ffffff", Level: 30, JobID: "job-01", Gender: "m", SP: 80, CreatedAt: at,
			}}, nil
		},
	}
	collections := &stubCollectionService{
		getWeaponCollectionFn: func(context.Context, string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
			return progress(99.9)
		},
		getArmorCollectionFn: func(context.Context, string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
			return progress(0)
		},
		getItemCollectionFn: func(_ context.Context, _, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
			if category != "item" {
				return nil, collection.CompletionProgress{}, errors.New("unexpected category " + category)
			}
			return progress(100)
		},
	}
	h, err := apihttp.NewHandler(&stubPlayerService{}, characters, &stubAdventureService{}, &stubShopService{},
		apihttp.WithCollection(collections), apihttp.WithAlchemy(trajectoryAlchemy{percent: 120}))
	if err != nil {
		t.Fatal(err)
	}
	get := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/characters/"+id+"/trajectory", nil))
		return rec
	}

	rec := get("c1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"name":             "Hero",
		"completion_rates": map[string]any{"weapon": 99.0, "armor": 0.0, "item": 100.0, "alchemy": 100.0},
		"memories": []any{map[string]any{
			"kind": "job_master", "content": "★ 戦士 Job Master! ★", "name": "Hero", "guild_name": "Knights",
			"color": "#ffffff", "level": 30.0, "job_id": "job-01", "gender": "m", "sp": 80.0, "created_at": "2026-11-14T01:02:03Z",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %v, want exactly %v", got, want)
	}

	if rec := get("missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown character status %d, want 404", rec.Code)
	}
	characters.trajectoryFn = func(context.Context, string) ([]character.Memory, error) { return nil, errors.New("store down") }
	if rec := get("c1"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("store failure status %d, want 500", rec.Code)
	}
}

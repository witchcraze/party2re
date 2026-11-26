package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type gvgRecoveryStandings struct{ gvg.StandingRepository }

func (gvgRecoveryStandings) FindRoundTransition(context.Context, string) (gvg.RoundTransition, bool, error) {
	return gvg.RoundTransition{}, false, nil
}

// gvgMappingFailure fails the first reverse-mapping cleanup after the departure committed.
type gvgMappingFailure struct {
	*gvg.MemoryRoomRepository
	fail bool
}

func (r *gvgMappingFailure) DeleteCharacterRoom(ctx context.Context, characterID, roomID string) error {
	if r.fail {
		r.fail = false
		return errors.New("mapping store down")
	}
	return r.MemoryRoomRepository.DeleteCharacterRoom(ctx, characterID, roomID)
}

type gvgGatewayChars struct {
	apihttp.CharacterService
	chars *casinoActivityCharStub
}

func (c gvgGatewayChars) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	return c.chars.FindByID(ctx, id)
}

func (c gvgGatewayChars) GetProfile(ctx context.Context, id string) (character.ProfileView, error) {
	ch, err := c.chars.FindByID(ctx, id)
	return character.ProfileView{Character: ch}, err
}

// The Gateway is the only departure route, so a departure interrupted before its
// mapping cleanup must stay reachable through context and gvg_leave.
func TestGvGInterruptedDepartureRecoversThroughGateway(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining bool
	}{{"room kept", true}, {"last member", false}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			chars := &casinoActivityCharStub{chars: map[string]corecharacter.Character{
				"hero": {ID: "hero", PlayerID: "owner", Name: "Hero", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
			}}
			rooms := &gvgMappingFailure{MemoryRoomRepository: gvg.NewMemoryRoomRepository(), fail: true}
			service, err := gvg.NewService(rooms, gvgRecoveryStandings{}, &struct{ gvg.GuildRepository }{}, chars, &struct{ gvg.BattleEngine }{})
			if err != nil {
				t.Fatal(err)
			}
			members := []gvg.GvGMember{{RoomID: "g1", CharacterID: "hero", CharacterName: "Hero", IsLeader: true}}
			if tc.remaining {
				members = append(members, gvg.GvGMember{RoomID: "g1", CharacterID: "ally", CharacterName: "Ally"})
			}
			if err := rooms.SaveRoom(ctx, gvg.GvGRoom{ID: "g1", Status: gvg.StatusRecruiting, LeaderCharacterID: "hero", MaxMembers: 8}, members); err != nil {
				t.Fatal(err)
			}
			if err := rooms.SetCharacterRoom(ctx, "hero", "g1"); err != nil {
				t.Fatal(err)
			}

			f := &activityFixtures{}
			readers := contextActivityReaders{party: f, pvp: &pvpActivityFixture{}, gvg: service, dungeon: f, challenge: f, casino: f}
			pc := playercontext.NewService(chars, depotGatewayObservation{}, timer.NewService(nil), playercontext.WithActivities(readers.read))
			h, err := apihttp.NewHandler(depotGatewayPlayer{owner: "owner"}, gvgGatewayChars{chars: chars},
				&struct{ apihttp.AdventureService }{}, &struct{ apihttp.ShopService }{}, apihttp.WithGvG(service), apihttp.WithPlayerContext(pc))
			if err != nil {
				t.Fatal(err)
			}
			router := h.Router()
			leave := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/hero/actions", strings.NewReader(`{"action":"gvg_leave","params":{"room_id":"g1"}}`))
				r.Header.Set("Authorization", "Bearer session")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w
			}
			offered := func() []string {
				t.Helper()
				r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil)
				r.Header.Set("Authorization", "Bearer session")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				var out apihttp.PlayerContextResponse
				if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &out) != nil {
					t.Fatalf("context: %d %s", w.Code, w.Body.String())
				}
				var ids []string
				for _, a := range out.AvailableActions {
					if a.Action == "gvg_leave" && a.ParamsTemplate["room_id"] != "g1" {
						t.Fatalf("gvg_leave bound to %v", a.ParamsTemplate)
					}
					ids = append(ids, a.Action)
				}
				return ids
			}

			if w := leave(); w.Code != http.StatusInternalServerError {
				t.Fatalf("interrupted departure: %d %s", w.Code, w.Body.String())
			}
			if got, _ := chars.FindByID(ctx, "hero"); got.Tired != 1 {
				t.Fatalf("departure fatigue = %d, want 1", got.Tired)
			}
			if !slices.Contains(offered(), "gvg_leave") {
				t.Fatal("interrupted departure hid gvg_leave")
			}
			if w := leave(); w.Code != http.StatusOK {
				t.Fatalf("recovery: %d %s", w.Code, w.Body.String())
			}
			if _, err := rooms.GetCharacterRoom(ctx, "hero"); !errors.Is(err, gvg.ErrRoomNotFound) {
				t.Fatalf("mapping left after recovery: %v", err)
			}
			if got, _ := chars.FindByID(ctx, "hero"); got.Tired != 1 {
				t.Fatalf("recovery repeated departure effects: tired %d", got.Tired)
			}
			detail, err := rooms.GetRoom(ctx, "g1")
			if tc.remaining && (err != nil || len(detail.Members) != 1 || detail.Room.LeaderCharacterID != "ally") {
				t.Fatalf("remaining room changed: %+v %v", detail, err)
			}
			if !tc.remaining && !errors.Is(err, gvg.ErrRoomNotFound) {
				t.Fatalf("empty room kept: %+v %v", detail, err)
			}
			if slices.Contains(offered(), "gvg_leave") {
				t.Fatal("gvg_leave still offered after recovery")
			}
			if w := leave(); w.Code != http.StatusConflict {
				t.Fatalf("repeated recovery: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type casinoProjectionAccounts struct{ casino.Repository }

type casinoViewerCharacters struct {
	casino.CharacterRepository
	chars map[string]corecharacter.Character
}

func (r casinoViewerCharacters) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return c, corecharacter.ErrNotFound
	}
	return c, nil
}

func TestCasinoAuthenticatedProjection(t *testing.T) {
	ctx := context.Background()
	chars := casinoViewerCharacters{chars: map[string]corecharacter.Character{
		"owned": {ID: "owned", PlayerID: "owner"}, "victim": {ID: "victim", PlayerID: "victim-player"}, "outsider": {ID: "outsider", PlayerID: "owner"},
	}}
	repo := casino.NewMemoryRoomRepository()
	room := casino.Room{ID: "room", Name: "room", GameType: casino.GameTypeHighLow, Status: casino.RoomStatusWaiting, UpdatedAt: time.Now(), AllowSpectators: false, HasPassword: true}
	if err := repo.CreateRoom(ctx, room, casino.RoomMember{RoomID: "room", CharacterID: "owned", Card: -1}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, casino.RoomMember{RoomID: "room", CharacterID: "victim", Card: -1}); err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoProjectionAccounts{}, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	// Start through the existing command service and compare the GET projection.
	room.LeaderCharacterID = "owned"
	if err := repo.UpdateRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	result, err := svc.StartGame(ctx, "room", "owned")
	if err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "owner"})}, &stubCharacterService{getFn: chars.FindByID}, &stubAdventureService{}, &stubShopService{}, apihttp.WithCasino(svc))
	for _, tc := range []struct {
		path   string
		auth   bool
		status int
	}{
		{"/casino/rooms", false, 200},
		{"/casino/rooms/room", false, 401},
		{"/casino/rooms/room?character_id=victim", false, 401},
		{"/casino/rooms/room?character_id=victim", true, 403},
		{"/casino/rooms/room?character_id=outsider", true, 403},
		{"/casino/rooms/room?character_id=owned", true, 200},
		{"/casino/rooms/missing?character_id=owned", true, 404},
		{"/casino/rooms/room", true, 400},
	} {
		t.Run(tc.path+"/"+http.StatusText(tc.status), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.auth {
				req.Header.Set("Authorization", "Bearer token")
			}
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == 200 && tc.path == "/casino/rooms" {
				for _, key := range []string{"card", "action", "members", "pot", "victim"} {
					if strings.Contains(rec.Body.String(), key) {
						t.Fatalf("lobby leaked %s", rec.Body.String())
					}
				}
			}
			if tc.status == 200 && strings.Contains(tc.path, "character_id") {
				var body struct {
					Room casino.RoomView `json:"room"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				for _, m := range body.Room.Members {
					for _, commandMember := range result.Members {
						if m.CharacterID == commandMember.CharacterID && (m.Card != commandMember.Card || m.Action != commandMember.Action) {
							t.Fatal("GET disagrees with post-command projection")
						}
					}
				}
			}
		})
	}
	members, err := repo.ListMembers(ctx, "room")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatal("GET admitted an outsider")
	}
}

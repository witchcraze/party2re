package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestChallengeReadOwnershipAndPendingResult(t *testing.T) {
	p := coreplayer.Player{ID: "p1"}
	calls := 0
	service := &stubChallengeService{
		activeFn: func(_ context.Context, actor string) (*challenge.ChallengeSession, error) {
			calls++
			if actor != "c1" {
				t.Fatal("foreign actor forwarded")
			}
			return &challenge.ChallengeSession{ID: "run", RecoveryPending: true}, nil
		},
		commandFn: func(_ context.Context, actor, run string, rev int) (*challenge.CommandResult, error) {
			calls++
			if actor != "c1" || run != "run" || rev != 3 {
				t.Fatalf("receipt identity: %s %s %d", actor, run, rev)
			}
			return &challenge.CommandResult{ActorID: actor, Action: "exit", ExpectedRevision: rev}, nil
		},
	}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(p)}, &stubCharacterService{getFn: func(_ context.Context, actor string) (corecharacter.Character, error) {
		owner := "p1"
		if actor == "c2" {
			owner = "p2"
		}
		return corecharacter.Character{ID: actor, PlayerID: owner}, nil
	}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithChallenge(service))
	for _, suffix := range []string{"active", "run/commands/3"} {
		for _, actor := range []string{"c1", "c2"} {
			before := calls
			req := httptest.NewRequest(http.MethodGet, "/characters/"+actor+"/challenges/"+suffix, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if actor == "c1" {
				if rec.Code != 200 || calls != before+1 {
					t.Fatalf("owned observation: %d %s", rec.Code, rec.Body.String())
				}
				if suffix == "active" && !strings.Contains(rec.Body.String(), `"recovery_pending":true`) {
					t.Fatal("pending state omitted")
				}
			} else if rec.Code != http.StatusForbidden || calls != before {
				t.Fatalf("foreign observation: %d calls=%d", rec.Code, calls)
			}
		}
	}
}

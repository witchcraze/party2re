package challenge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Authentication is a test double; observations and settlement use real SQL/Valkey.
type protocolGateway struct {
	apihttp.PlayerService
	apihttp.CharacterService
	*protocolFixture
}

func (g protocolGateway) Authenticate(ctx context.Context, token string) (coreplayer.Player, error) {
	c, err := g.chars.FindByID(ctx, token)
	return coreplayer.Player{ID: c.PlayerID}, err
}

func (g protocolGateway) Get(ctx context.Context, actor string) (corecharacter.Character, error) {
	return g.chars.FindByID(ctx, actor)
}

func (g protocolGateway) GetProfile(ctx context.Context, actor string) (character.ProfileView, error) {
	c, err := g.Get(ctx, actor)
	return character.ProfileView{Character: c}, err
}

func (g protocolGateway) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func (g protocolGateway) router(t *testing.T) http.Handler {
	t.Helper()
	pc := playercontext.NewService(g.chars, g, timer.NewService(nil), playercontext.WithActivities(func(ctx context.Context, _, actor string) ([]playercontext.Activity, error) {
		run, err := g.svc.GetActiveSession(ctx, actor)
		if err != nil || run == nil {
			return nil, err
		}
		a := playercontext.Activity{Kind: "challenge", ID: run.ID, Revision: run.Revision, Phase: string(run.Status), Actions: []string{}}
		if run.RecoveryPending {
			a.Phase = "recovery_pending"
		} else if run.Status == challenge.StatusActive {
			a.Actions = []string{"challenge_retire"}
		}
		return []playercontext.Activity{a}, nil
	}))
	h, err := apihttp.NewHandler(g, g, &struct{ apihttp.AdventureService }{}, &struct{ apihttp.ShopService }{}, apihttp.WithChallenge(g.svc), apihttp.WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func protocolGatewayRequest(t *testing.T, router http.Handler, actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+actor)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
	}
	return w
}

func protocolGatewayExit(t *testing.T, router http.Handler, actor string, run *challenge.ChallengeSession) challenge.CommandResult {
	t.Helper()
	w := protocolGatewayRequest(t, router, actor, "POST", "/api/v1/characters/"+actor+"/actions", fmt.Sprintf(`{"action":"challenge_retire","params":{"session_id":%q,"expected_revision":%d}}`, run.ID, run.Revision))
	var response struct {
		Success bool
		Result  challenge.CommandResult
		Context *apihttp.PlayerContextResponse
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.Result.ActorID != actor || response.Result.ExpectedRevision != run.Revision || response.Context == nil {
		t.Fatalf("lost exit/refresh: %s", w.Body.String())
	}
	return response.Result
}

func TestChallengeGatewayParticipantSettlementIntegration(t *testing.T) {
	for _, n := range []int{1, 3} {
		for _, wins := range []int{0, 1} {
			t.Run(fmt.Sprintf("party%d/wins%d", n, wins), func(t *testing.T) {
				f := newProtocolFixture(t, n)
				router := (protocolGateway{protocolFixture: f}).router(t)
				run := f.start(t)
				for i := 0; i < wins; i++ {
					run = f.advance(t, run)
				}
				rounds := wins
				order := []int{0}
				if n > 1 {
					order = []int{1, 0, 2}
				}
				for _, i := range order {
					actor := f.actors[i].ID
					before := *run
					out := protocolGatewayExit(t, router, actor, run)
					f.assertSettled(t, i, rounds, rounds)
					remaining := []challenge.ChallengeMember{}
					for _, m := range before.Members {
						if m.CharacterID != actor {
							remaining = append(remaining, m)
						}
					}
					if len(out.Session.Members) != len(remaining) || (len(remaining) > 0 && !reflect.DeepEqual(out.Session.Members, remaining)) || out.Session.CurrentRound != before.CurrentRound {
						t.Fatalf("survivor state changed: %+v", out)
					}
					for _, m := range remaining {
						c, err := f.chars.FindByID(context.Background(), m.CharacterID)
						if err != nil {
							t.Fatal(err)
						}
						for _, original := range f.actors {
							if original.ID == m.CharacterID && !reflect.DeepEqual(c, original) {
								t.Fatal("survivor persisted by another actor's exit")
							}
						}
					}
					if len(remaining) > 0 && out.Session.CharacterID != remaining[0].CharacterID {
						t.Fatal("wrong leader after exit")
					}
					if len(remaining) == 0 && out.Session.Status != challenge.StatusClaimed {
						t.Fatal("last exit did not end run")
					}
					var replacement *challenge.ChallengeSession
					if n > 1 && i == 0 {
						var err error
						replacement, err = f.svc.StartSession(context.Background(), actor, "test")
						if err != nil {
							t.Fatal(err)
						}
					}
					// GET receipt/context and explicit replay must not settle a second time.
					protocolGatewayRequest(t, router, actor, "GET", fmt.Sprintf("/characters/%s/challenges/%s/commands/%d", actor, run.ID, run.Revision), "")
					protocolGatewayRequest(t, router, actor, "GET", "/api/v1/characters/"+actor+"/context", "")
					protocolGatewayExit(t, router, actor, run)
					f.assertSettled(t, i, rounds, rounds)
					if replacement != nil {
						active, err := f.svc.GetActiveSession(context.Background(), actor)
						if err != nil || active == nil || active.ID != replacement.ID {
							t.Fatalf("replay damaged former leader's new run: %+v %v", active, err)
						}
					}
					run = &out.Session
					if len(remaining) > 0 {
						w := protocolGatewayRequest(t, router, run.CharacterID, "POST", "/characters/"+run.CharacterID+"/challenges/advance", fmt.Sprintf(`{"session_id":%q,"expected_revision":%d}`, run.ID, run.Revision))
						var advanced struct{ Session *challenge.ChallengeSession }
						if err := json.Unmarshal(w.Body.Bytes(), &advanced); err != nil {
							t.Fatal(err)
						}
						if advanced.Session == nil || len(advanced.Session.Members) != len(remaining) || advanced.Session.CurrentRound != run.CurrentRound+1 {
							t.Fatalf("survivors could not continue: %s", w.Body.String())
						}
						run = advanced.Session
						rounds++
					}
				}
			})
		}
	}
}

func TestChallengeGatewayConcurrentExitIntegration(t *testing.T) {
	f := newProtocolFixture(t, 2)
	router := (protocolGateway{protocolFixture: f}).router(t)
	run := f.advance(t, f.start(t))
	actor := f.actors[0].ID
	body := fmt.Sprintf(`{"action":"challenge_retire","params":{"session_id":%q,"expected_revision":%d}}`, run.ID, run.Revision)
	out := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error {
		r := httptest.NewRequest("POST", "/api/v1/characters/"+actor+"/actions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+actor)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			return fmt.Errorf("exit status=%d: %s", w.Code, w.Body.String())
		}
		return nil
	})
	if out.Failures != 0 {
		t.Fatalf("concurrent HTTP exits failed: %d", out.Failures)
	}
	f.assertSettled(t, 0, 1, 1)
	survivor, err := f.chars.FindByID(context.Background(), f.actors[1].ID)
	if err != nil || !reflect.DeepEqual(survivor, f.actors[1]) {
		t.Fatalf("concurrent exits changed survivor: %+v %v", survivor, err)
	}
}

func TestChallengeGatewayPendingPublicationIntegration(t *testing.T) {
	f := newProtocolFixture(t, 2)
	run := f.advance(t, f.start(t))
	ctx := context.Background()
	if _, err := f.db.ExecContext(ctx, `CREATE TRIGGER reject_gateway_ack BEFORE UPDATE ON challenge_run_transitions FOR EACH ROW BEGIN IF OLD.pending=TRUE AND NEW.pending=FALSE THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='gateway ack failed'; END IF; END`); err != nil {
		t.Fatal(err)
	}
	router := (protocolGateway{protocolFixture: f}).router(t)
	actor := f.actors[0].ID
	body := fmt.Sprintf(`{"action":"challenge_retire","params":{"session_id":%q,"expected_revision":%d}}`, run.ID, run.Revision)
	w := protocolGatewayRequest(t, router, actor, "POST", "/api/v1/characters/"+actor+"/actions", body)
	var response struct {
		Success bool
		Result  struct {
			RecoveryPending bool `json:"recovery_pending"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || !response.Result.RecoveryPending {
		t.Fatalf("lost committed/pending distinction: %s", w.Body.String())
	}
	f.assertSettled(t, 0, 1, 1)
	protocolGatewayRequest(t, router, actor, "GET", fmt.Sprintf("/characters/%s/challenges/%s/commands/%d", actor, run.ID, run.Revision), "")
	protocolGatewayRequest(t, router, actor, "GET", "/api/v1/characters/"+actor+"/context", "")
	transition, err := f.repo.FindTransition(ctx, run.ID)
	if err != nil || !transition.Pending {
		t.Fatalf("GET repaired pending publication: %+v %v", transition, err)
	}
	if _, err := f.db.ExecContext(ctx, "DROP TRIGGER reject_gateway_ack"); err != nil {
		t.Fatal(err)
	}
	w = protocolGatewayRequest(t, router, actor, "POST", "/api/v1/characters/"+actor+"/actions", body)
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.Result.RecoveryPending {
		t.Fatalf("explicit replay did not finish publication: %s", w.Body.String())
	}
	f.assertSettled(t, 0, 1, 1)
}

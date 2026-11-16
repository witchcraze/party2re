package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/timer"
	jobapp "github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type jobTempleFixture struct {
	*gatewayFixture
	JobService
	status    jobapp.TempleStatus
	statusErr error
	calls     []string
}

func (f *jobTempleFixture) ListDefinitions() []corejob.Definition {
	return []corejob.Definition{{ID: "starter", Name: "----"}, {ID: "job-01", Name: "戦士"}, {ID: "job-05", Name: "僧侶"}}
}

func (f *jobTempleFixture) TempleStatus(context.Context, string) (jobapp.TempleStatus, error) {
	return f.status, f.statusErr
}

func (f *jobTempleFixture) done(ctx context.Context, actor, call string) (corecharacter.Character, corejob.CharacterJob, error) {
	f.calls = append(f.calls, call)
	_, err := f.execute(ctx, actor, 0)
	return corecharacter.Character{ID: actor, Name: "勇者", JobID: "job-05"}, corejob.CharacterJob{CurrentJobID: "job-05"}, err
}

func (f *jobTempleFixture) ChangeJob(ctx context.Context, actor, target string) (corecharacter.Character, corejob.CharacterJob, error) {
	return f.done(ctx, actor, "change "+target)
}

func (f *jobTempleFixture) ExchangeJob(ctx context.Context, actor, job, old string) (corecharacter.Character, corejob.CharacterJob, error) {
	return f.done(ctx, actor, "exchange "+job+" "+old)
}

func (f *jobTempleFixture) RestoreJobMemory(ctx context.Context, actor string) (corecharacter.Character, corejob.CharacterJob, error) {
	return f.done(ctx, actor, "restore")
}

func (f *jobTempleFixture) RecallFutureMemory(ctx context.Context, actor, memory string) (corecharacter.Character, corejob.CharacterJob, error) {
	return f.done(ctx, actor, "future "+memory)
}

func newJobTempleFixture(t *testing.T) (*jobTempleFixture, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := &jobTempleFixture{gatewayFixture: newGatewayFixture(t), status: jobapp.TempleStatus{
		Changeable:     []corejob.Definition{{ID: "job-01", Name: "戦士"}},
		Recallable:     []corejob.Definition{{ID: "job-01", Name: "戦士"}, {ID: "job-05", Name: "僧侶"}},
		RecallItem:     "item-168",
		FutureMemories: []corecharacter.FutureMemory{{ID: "fm1", JobID: "job-05", OldJobID: "job-01", Level: 40, MaxHP: 300}},
	}}
	f.char.Name, f.char.MonsterKills, f.char.CasinoWins = "勇者", 12, 3
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "job"}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "job", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithJob(f), WithPlayerContext(pc), WithPlaceLog(&fakePlaceLog{}), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func TestJobTempleSceneObservesChoices(t *testing.T) {
	f, store, router := newJobTempleFixture(t)
	status, observation := navigationGET(t, router)
	data := decodeShopScene[JobTempleSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "転職の神殿" || len(data.Jobs) != 1 || data.Jobs[0] != (JobOption{JobID: "job-01", Name: "戦士"}) ||
		len(data.RecallJobs) != 2 || data.JobMemory != "" || len(data.FutureMemories) != 1 ||
		data.FutureMemories[0] != (JobFutureMemory{ID: "fm1", JobName: "僧侶", OldJobName: "戦士", Level: 40, MaxHP: 300}) {
		t.Fatalf("scene: %d %+v", status, data)
	}
	for _, action := range []string{"job_change", "job_recall", "job_future_recall", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing %s", action)
		}
	}
	if offered(observation, "job_restore") {
		t.Fatal("job_restore offered without a persistent memory")
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET mutated: writes %d executions %d", store.writes, f.executions)
	}
}

func TestJobTempleOffersMemoryCommandsByState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		memory  *corecharacter.JobMemory
		noItems bool
		offer   map[string]bool
	}{
		{"persistent memory", &corecharacter.JobMemory{Kind: corecharacter.JobMemoryKindPersistent},
			false, map[string]bool{"job_restore": true, "job_recall": false, "job_future_recall": false}},
		{"temporary memory", &corecharacter.JobMemory{Kind: corecharacter.JobMemoryKindTemporary},
			false, map[string]bool{"job_restore": false, "job_recall": false, "job_future_recall": false}},
		{"no recall item", nil, true, map[string]bool{"job_restore": false, "job_recall": false, "job_future_recall": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, router := newJobTempleFixture(t)
			f.char.JobMemory = tc.memory
			if tc.noItems {
				f.status.RecallItem = ""
			}
			_, observation := navigationGET(t, router)
			for action, want := range tc.offer {
				if offered(observation, action) != want {
					t.Fatalf("%s offered=%v", action, !want)
				}
			}
		})
	}
}

func TestJobTempleSceneFailsOnRequiredRead(t *testing.T) {
	f, _, router := newJobTempleFixture(t)
	f.statusErr = errors.New("private")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
}

func TestJobTempleCommands(t *testing.T) {
	for _, tc := range []struct {
		action, params, call, message string
	}{
		{"job_change", `{"job_id":"job-05"}`, "change job-05", "勇者よ！僧侶となり新たな道を歩むが良い"},
		// job_change.cgi:164-179 names the consumed item before the blessing.
		{"job_change", `{"job_id":"job-33"}`, "change job-33", "賢者の悟りを使いました！ 勇者よ！僧侶となり新たな道を歩むが良い"},
		{"job_recall", `{"job_id":"job-05","old_job_id":"job-01"}`, "exchange job-05 job-01", "勇者の記憶を呼び起こしたぞ！"},
		{"job_restore", `{}`, "restore", "勇者を元の職業に戻したぞ！"},
		{"job_future_recall", `{"memory_id":"fm1"}`, "future fm1", "勇者の記憶を呼び起こしたぞ"},
	} {
		t.Run(tc.action+tc.call, func(t *testing.T) {
			f, _, router := newJobTempleFixture(t)
			f.char.JobMemory = &corecharacter.JobMemory{Kind: corecharacter.JobMemoryKindPersistent} // offers job_restore
			if tc.action != "job_restore" {
				f.char.JobMemory = nil
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			got := tavernResult[JobTempleResult](t, raw)
			if status != 200 || len(f.calls) != 1 || got.Message != tc.message || got.Character.JobID != "job-05" {
				t.Fatalf("%d %v %+v", status, f.calls, got)
			}
		})
	}
}

func TestJobTempleRejectsInvalidParams(t *testing.T) {
	f, _, router := newJobTempleFixture(t)
	for _, body := range []string{
		saleGatewayBody("job_change", `{}`), saleGatewayBody("job_change", `{"job_id":"job-01","x":1}`),
		saleGatewayBody("job_recall", `{"job_id":"job-05"}`), saleGatewayBody("job_recall", `{"job_id":"job-05","old_job_id":"job-01","item_id":"item-168"}`),
		saleGatewayBody("job_future_recall", `{"memory_id":1}`),
	} {
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", body, nil)
		if status/100 != 4 || len(f.calls) != 0 {
			t.Fatalf("%s executed: %d %s", body, status, raw)
		}
	}
}

func TestJobTempleCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{corejob.ErrJobUnavailable, "JOB_UNAVAILABLE", 409},
		{corejob.ErrDefinitionNotFound, "JOB_UNAVAILABLE", 409},
		{jobapp.ErrRequiredItem, "JOB_REQUIRED_ITEM", 409},
		{jobapp.ErrRequiredArmor, "JOB_REQUIRED_ARMOR", 409},
		{jobapp.ErrFutureMemoryNotFound, "JOB_FUTURE_MEMORY_NOT_FOUND", 409},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f, _, router := newJobTempleFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("job_change", `{"job_id":"job-01"}`), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestJobTemplePriestInspectsRecords(t *testing.T) {
	_, _, router := newJobTempleFixture(t)
	_, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil)
	if r := npcResult(t, raw); r.NPCName != "@神官" || !r.Private || r.Dialogue != "モンスター撃退数 12回\nプレイヤー撃退数 0回\n封印解いた回数 0回\n封印した回数 0回\nカジノ勝利数 3回" {
		t.Fatalf("npc_inspect %+v", r)
	}
	for range 40 {
		_, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
		if r := npcResult(t, raw); r.NPCName != "@神官" || strings.Contains(r.Dialogue, "----") || strings.Contains(r.Dialogue, "ダーマ") {
			t.Fatalf("npc_talk %+v", r)
		}
	}
}

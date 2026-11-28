package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	homeapp "github.com/witchcraze/party2re/internal/home"
)

// TestGameplayScenario_JobTemple changes job at the temple, recalls and
// restores a job memory, then recalls a future memory saved at home.
func TestGameplayScenario_JobTemple(t *testing.T) {
	a := newScenarioApp(t)
	ctx := context.Background()
	token, hero := a.register()

	// Fixture: job_change.cgi:146 needs level 20. Items 168 (recall) and 207
	// (future memory) come only from random prizes and synthesis.
	const startSP = 7
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET level = 20, sp = ? WHERE id = ?`, startSP, hero); err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := inventories.FindByCharacterID(ctx, hero)
	if err != nil {
		t.Fatal(err)
	}
	recallItem, err := item.NewInstance("item-168", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(recallItem); err != nil {
		t.Fatal(err)
	}
	if err := inventories.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}
	shard, err := item.NewInstance("item-207", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, a.db, hero, 0, []item.Instance{shard}); err != nil {
		t.Fatal(err)
	}

	type option struct {
		JobID string `json:"job_id"`
		Name  string `json:"name"`
	}
	type temple struct {
		scenarioContext
		Data struct {
			Jobs           []option `json:"jobs"`
			RecallJobs     []option `json:"recall_jobs"`
			RecallItem     string   `json:"recall_item"`
			JobMemory      string   `json:"job_memory"`
			FutureMemories []struct {
				ID string `json:"id"`
			} `json:"future_memories"`
		}
	}
	// observe decodes the temple data locally to keep it out of the shared scenarioContext (#2772).
	observe := func() temple {
		t.Helper()
		var body json.RawMessage
		a.call(token, http.MethodGet, "/api/v1/characters/"+hero+"/context", "", http.StatusOK, &body)
		var out temple
		var data struct {
			Scene struct {
				Data json.RawMessage `json:"data"`
			} `json:"scene"`
		}
		if err := json.Unmarshal(body, &out.scenarioContext); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatal(err)
		}
		if out.Scene.LocationID != "job" {
			t.Fatalf("not in the temple: %s", out.Scene.LocationID)
		}
		if err := json.Unmarshal(data.Scene.Data, &out.Data); err != nil {
			t.Fatal(err)
		}
		return out
	}
	act := func(action string, inputs map[string]any) {
		t.Helper()
		a.elapseActionWait(hero)
		a.offered(token, hero, observe().scenarioContext, action, inputs, http.StatusOK)
	}
	type stored struct {
		job, oldJob, memoryJob, memoryOldJob, memoryKind       string
		sp, oldSP, memorySP, memoryOldSP, level, jobLevel      int
		hp, maxHP, mp, maxMP, at, df, ag, recallItems, futures int
		gender                                                 string
	}
	load := func() stored {
		t.Helper()
		var s stored
		if err := a.db.QueryRowContext(ctx, `SELECT job_id, old_job_id, job_memory_job_id, job_memory_old_job_id, job_memory_kind,
			sp, old_sp, job_memory_sp, job_memory_old_sp, level, job_level, hp, max_hp, mp, max_mp, attack, defense, agility, gender,
			(SELECT COALESCE(SUM(quantity), 0) FROM inventory_items WHERE character_id = c.id AND definition_id = 'item-168'),
			(SELECT COUNT(*) FROM character_future_memories WHERE character_id = c.id)
			FROM characters c WHERE id = ?`, hero).Scan(&s.job, &s.oldJob, &s.memoryJob, &s.memoryOldJob, &s.memoryKind,
			&s.sp, &s.oldSP, &s.memorySP, &s.memoryOldSP, &s.level, &s.jobLevel, &s.hp, &s.maxHP, &s.mp, &s.maxMP, &s.at, &s.df, &s.ag, &s.gender,
			&s.recallItems, &s.futures); err != nil {
			t.Fatal(err)
		}
		return s
	}
	trajectory := func() []string {
		t.Helper()
		rows, err := a.db.QueryContext(ctx, `SELECT content FROM character_memories WHERE character_id = ? AND kind LIKE 'job%' ORDER BY id`, hero)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Item 207 at home (_data.cgi:1828-1848) saves a future memory of the level-20 warrior.
	saved := load()
	home := a.enter(token, hero, a.context(token, hero), "home")
	i := slices.IndexFunc(home.Scene.Data.View.Private.Items, func(it homeapp.HomeUsableItem) bool { return it.DefinitionID == "item-207" })
	if i < 0 {
		t.Fatalf("home items: %+v", home.Scene.Data.View.Private.Items)
	}
	shardItem := home.Scene.Data.View.Private.Items[i]
	a.offered(token, hero, home, "home_item_use", map[string]any{"instance_id": shardItem.InstanceID, "source": shardItem.Source}, http.StatusOK)
	if s := load(); s.futures != 1 || s.job != saved.job || s.level != saved.level || s.maxHP != saved.maxHP {
		t.Fatalf("after saving the future memory: %+v; before %+v", s, saved)
	}
	a.elapseActionWait(hero)
	town := a.offered(token, hero, a.context(token, hero), "scene_back", nil, http.StatusOK).Context
	a.elapseActionWait(hero)
	a.enter(token, hero, town, "job")
	start := observe()
	name := func(opts []option, jobID string) string {
		t.Helper()
		i := slices.IndexFunc(opts, func(o option) bool { return o.JobID == jobID })
		if i < 0 {
			t.Fatalf("%s is not offered: %+v", jobID, opts)
		}
		return opts[i].Name
	}
	warrior, priest := name(start.Data.Jobs, "job-01"), name(start.Data.Jobs, "job-05")
	// job_change.cgi:118-136 offers recall only with a recorded job pair.
	if start.offers("job_recall") || start.offers("job_restore") || !start.offers("job_future_recall") || start.Data.RecallItem != "item-168" {
		t.Fatalf("temple before changing: %+v %+v", start.Data, start.AvailableActions)
	}

	// ＠てんしょく (job_change.cgi:144-238): SP moves to the old job, level 1, stats halved (at least 10).
	before := load()
	act("job_change", map[string]any{"job_id": "job-05"})
	half := func(v int) int { return max(v/2, 10) }
	changed := load()
	if changed.job != "job-05" || changed.oldJob != "job-01" || changed.sp != 0 || changed.oldSP != startSP ||
		changed.level != 1 || changed.jobLevel != before.jobLevel+1 || changed.maxHP != half(before.maxHP) || changed.hp != changed.maxHP ||
		changed.maxMP != half(before.maxMP) || changed.mp != changed.maxMP || changed.at != half(before.at) || changed.df != half(before.df) ||
		changed.ag != half(before.ag) || changed.recallItems != 1 || changed.memoryJob != "" {
		t.Fatalf("after changing: %+v; before %+v", changed, before)
	}

	// The common action wait rejects the next command before it reaches the temple.
	rejected := a.command(token, hero, "job_change", map[string]any{"job_id": "job-01"}, http.StatusConflict)
	if rejected.Error.Code != "ACTION_WAIT" || load() != changed {
		t.Fatalf("change during action wait: %+v %+v", rejected, load())
	}
	// Only a departed job is recorded (add_job_master), so recall still waits.
	a.elapseActionWait(hero)
	if v := observe(); len(v.Data.Jobs) != 0 || v.offers("job_recall") {
		t.Fatalf("temple at level 1: %+v %+v", v.Data, v.AvailableActions)
	}

	// Fixture: regain level 20 with SP earned as the priest. Returning to the
	// old job takes back its SP (job_change.cgi:216-220).
	const priestSP = 3
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET level = 20, sp = ? WHERE id = ?`, priestSP, hero); err != nil {
		t.Fatal(err)
	}
	name(observe().Data.Jobs, "job-01")
	act("job_change", map[string]any{"job_id": "job-01"})
	if s := load(); s.job != "job-01" || s.sp != startSP || s.oldJob != "job-05" || s.oldSP != priestSP || s.level != 1 {
		t.Fatalf("after returning: %+v", s)
	}

	// ＠おもいだす with item 168 (job_change.cgi:360-427): take the recorded SP
	// of the chosen pair and remember the jobs left.
	a.elapseActionWait(hero)
	recall := observe()
	name(recall.Data.RecallJobs, "job-01")
	name(recall.Data.RecallJobs, "job-05")
	act("job_recall", map[string]any{"job_id": "job-05", "old_job_id": "job-01"})
	recalled := load()
	if recalled.job != "job-05" || recalled.sp != priestSP || recalled.oldJob != "job-01" || recalled.oldSP != startSP ||
		recalled.memoryKind != string(corecharacter.JobMemoryKindPersistent) || recalled.memoryJob != "job-01" || recalled.memorySP != startSP ||
		recalled.memoryOldJob != "job-05" || recalled.memoryOldSP != priestSP || recalled.recallItems != 0 || recalled.level != 1 {
		t.Fatalf("after recalling: %+v", recalled)
	}
	// job_change.cgi:118-136,150: while remembering, only ＠おもいだす (to restore) remains.
	a.elapseActionWait(hero)
	if v := observe(); len(v.Data.Jobs) != 0 || v.Data.JobMemory != string(corecharacter.JobMemoryKindPersistent) ||
		!v.offers("job_restore") || v.offers("job_recall") || v.offers("job_future_recall") {
		t.Fatalf("temple while remembering: %+v %+v", v.Data, v.AvailableActions)
	}

	// ＠おもいだす>はい (job_reverse, job_change.cgi:514-540) returns to the jobs left.
	act("job_restore", nil)
	restored := load()
	if restored.job != "job-01" || restored.sp != startSP || restored.oldJob != "job-05" || restored.oldSP != priestSP || restored.memoryJob != "" {
		t.Fatalf("after restoring: %+v", restored)
	}

	// ＠よびおこす (job_call_future, job_change.cgi:604-660) restores the saved warrior
	// with the recorded warrior SP. The empty old job has no record, so the old SP
	// stays; the current gender stays too. The snapshot is consumed.
	a.elapseActionWait(hero)
	listed := observe()
	if len(listed.Data.FutureMemories) != 1 {
		t.Fatalf("future memories: %+v", listed.Data.FutureMemories)
	}
	act("job_future_recall", map[string]any{"memory_id": listed.Data.FutureMemories[0].ID})
	want := saved
	want.sp, want.oldSP, want.memoryKind, want.jobLevel, want.hp, want.mp, want.recallItems = startSP, priestSP, restored.memoryKind, restored.jobLevel, saved.maxHP, saved.maxMP, 0
	if got := load(); got != want {
		t.Fatalf("after the future recall: %+v; want %+v", got, want)
	}

	// Legacy writes no memory for ＠よびおこす.
	wantTrajectory := []string{warrior + "から" + priest + "に転職", priest + "から" + warrior + "に転職",
		priest + "と" + warrior + "の記憶を思い出す", warrior + "と" + priest + "に戻る"}
	a.elapseActionWait(hero)
	a.restart()
	for range 2 {
		if got := load(); got != want {
			t.Fatalf("after restart: %+v; want %+v", got, want)
		}
		if got := trajectory(); !slices.Equal(got, wantTrajectory) {
			t.Fatalf("trajectory %q; want %q", got, wantTrajectory)
		}
		if v := observe(); len(v.Data.FutureMemories) != 0 || v.offers("job_future_recall") || v.offers("job_restore") {
			t.Fatalf("temple after the journey: %+v %+v", v.Data, v.AvailableActions)
		}
	}
}

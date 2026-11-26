package http

import (
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func TestDungeonOutcomePresentation(t *testing.T) {
	group := []dungeon.DungeonMonster{{Name: "SlimeA", ExpReward: 7}, {Name: "SlimeB", ExpReward: 13}}
	for _, tc := range []struct {
		name     string
		event    dungeon.TileEventType
		outcome  corebattle.Outcome
		monsters []dungeon.DungeonMonster
		want     string
	}{
		{"move", dungeon.EventMove, "", nil, ""},
		{"treasure", dungeon.EventTreasure, "", nil, ""},
		{"escape", dungeon.EventEscape, "", nil, "探索を中断し、戦利品を持って無事に脱出した！"},
		{"resumed escape", dungeon.EventEscape, "", nil, "探索を中断し、戦利品を持って無事に脱出した！"},
		{"wipeout", dungeon.EventWipeout, "", nil, "探索に失敗した…"},
		{"victory", dungeon.EventBattle, corebattle.OutcomeWin, group, "SlimeA、SlimeB との戦闘に勝利した！ (EXP: +20, G: +30)"},
		{"boss victory", dungeon.EventBoss, corebattle.OutcomeWin, group[:1], "SlimeA との戦闘に勝利した！ (EXP: +20, G: +30)"},
		{"defeat", dungeon.EventBattle, corebattle.OutcomeDefeat, group, "SlimeA、SlimeB との戦いに敗れ、全滅してしまった…"},
		{"draw", dungeon.EventBattle, corebattle.OutcomeDraw, group, "SlimeA、SlimeB との戦いは時間切れで決着がつかなかった… (引き分け)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The win message reports the step's credited rewards, not a member's source values.
			facts := dungeon.ExpeditionStepResult{EventType: tc.event, CombatOutcome: tc.outcome, Monsters: tc.monsters, ExpEarned: 20, GoldFound: 30}
			got := toDungeonStepResult(facts)
			if got.Message != tc.want || got.BattleResult != nil {
				t.Fatalf("got %+v want %q", got, tc.want)
			}
		})
	}
}

func TestMonsterTameGatewayPresentation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		success, full bool
		want          string
	}{
		{"success", true, false, "なんと スライム が起き上がりこちらを見ている。スライムはうれしそうにアレックスのモンスター預かり所に向かった"},
		{"full box", false, true, "なんと スライム が起き上がりこちらを見ている。しかし、アレックスのモンスター預かり所はいっぱいだった。スライムは悲しそうに去っていった…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunGatewayFixture(t)
			f.runs.tames = []battle.MonsterTameResult{{MonsterID: "monster-002", MonsterName: "スライム", TamerName: "アレックス", Success: tc.success, BoxFull: tc.full}}
			status, response := f.post(f.router(), "run_operation", `{"run_id":"run","version":3,"command_id":"c","operation":"attack","target_id":"foe"}`)
			if status != http.StatusOK {
				t.Fatalf("status=%d response=%s", status, response)
			}
			got := decodeRunReceipt(t, response).Tames
			if len(got) != 1 || got[0].Message != tc.want || got[0].Success != tc.success || got[0].BoxFull != tc.full || got[0].MonsterID != "monster-002" {
				t.Fatalf("lost taming outcome: %s", response)
			}
		})
	}
	if toMonsterTames(nil) != nil {
		t.Fatal("empty outcomes must stay absent")
	}
}

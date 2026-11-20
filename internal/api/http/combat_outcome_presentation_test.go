package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestDungeonOutcomePresentation(t *testing.T) {
	monster := &dungeon.DungeonMonster{Name: "Slime", ExpReward: 20, GoldReward: 30}
	for _, tc := range []struct {
		name    string
		event   dungeon.TileEventType
		outcome corebattle.Outcome
		monster *dungeon.DungeonMonster
		want    string
	}{
		{"move", dungeon.EventMove, "", nil, ""},
		{"treasure", dungeon.EventTreasure, "", nil, ""},
		{"escape", dungeon.EventEscape, "", nil, "探索を中断し、戦利品を持って無事に脱出した！"},
		{"resumed escape", dungeon.EventEscape, "", nil, "探索を中断し、戦利品を持って無事に脱出した！"},
		{"wipeout", dungeon.EventWipeout, "", nil, "探索に失敗した…"},
		{"victory", dungeon.EventBattle, corebattle.OutcomeWin, monster, "Slime との戦闘に勝利した！ (EXP: +20, G: +30)"},
		{"boss victory", dungeon.EventBoss, corebattle.OutcomeWin, monster, "Slime との戦闘に勝利した！ (EXP: +20, G: +30)"},
		{"defeat", dungeon.EventBattle, corebattle.OutcomeDefeat, monster, "Slime との戦いに敗れ、全滅してしまった…"},
		{"draw", dungeon.EventBattle, corebattle.OutcomeDraw, monster, "Slime との戦いは時間切れで決着がつかなかった… (引き分け)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := dungeon.ExpeditionStepResult{EventType: tc.event, CombatOutcome: tc.outcome, Monster: tc.monster}
			got := toDungeonStepResult(facts)
			if got.Message != tc.want || got.BattleResult != nil || got.ExpEarned != 0 {
				t.Fatalf("got %+v want %q", got, tc.want)
			}
		})
	}
}

type presentationAdventureService struct {
	AdventureService
	result adventure.Adventure
}

func (s presentationAdventureService) StartStage(context.Context, string, string) (adventure.Adventure, error) {
	return s.result, nil
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
			f := newGatewayFixture(t)
			service := presentationAdventureService{result: adventure.Adventure{MonsterTames: map[string][]battle.MonsterTameResult{"hero": {{MonsterID: "monster-002", MonsterName: "スライム", TamerName: "アレックス", Success: tc.success, BoxFull: tc.full}}}}}
			h, err := NewHandler(f, f, service, &struct{ ShopService }{}, WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))), func(h *Handler) { h.homes = f })
			if err != nil {
				t.Fatal(err)
			}
			status, response := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"adventure_start","params":{"stage_id":"stage-01"}}`, nil)
			if status != http.StatusOK {
				t.Fatalf("status=%d response=%s", status, response)
			}
			var result adventureResponse
			if err := json.Unmarshal(response["result"], &result); err != nil {
				t.Fatal(err)
			}
			got := result.MonsterTames["hero"]
			if len(got) != 1 || got[0].Message != tc.want || got[0].Success != tc.success || got[0].BoxFull != tc.full || got[0].MonsterID != "monster-002" {
				t.Fatalf("lost taming outcome: %s", response)
			}
		})
	}
	if toMonsterTames(nil) != nil {
		t.Fatal("empty outcomes must stay absent")
	}
}

package http

import (
	"fmt"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
)

type dungeonStepResultResponse struct {
	dungeon.ExpeditionStepResult
	BattleResult *battleResultResponse `json:"battle_result,omitempty"`
	Message      string                `json:"message"`
}

func toDungeonStepResult(result dungeon.ExpeditionStepResult) dungeonStepResultResponse {
	response := dungeonStepResultResponse{ExpeditionStepResult: result}
	if result.BattleResult != nil {
		value := toBattleResult(*result.BattleResult)
		response.BattleResult = &value
	}
	switch {
	case result.EventType == dungeon.EventEscape:
		response.Message = "探索を中断し、戦利品を持って無事に脱出した！"
	case result.Monster != nil:
		m := result.Monster
		switch result.CombatOutcome {
		case corebattle.OutcomeWin:
			response.Message = fmt.Sprintf("%s との戦闘に勝利した！ (EXP: +%d, G: +%d)", m.Name, m.ExpReward, m.GoldReward)
		case corebattle.OutcomeDefeat:
			response.Message = fmt.Sprintf("%s との戦いに敗れ、全滅してしまった…", m.Name)
		case corebattle.OutcomeDraw:
			response.Message = fmt.Sprintf("%s との戦いは時間切れで決着がつかなかった… (引き分け)", m.Name)
		}
	}
	return response
}

type bossBattleResultResponse struct {
	boss.SealingBattleResult
	BattleResult partyBattleResultResponse `json:"battle_result"`
}

func toBossBattleResult(result boss.SealingBattleResult) bossBattleResultResponse {
	return bossBattleResultResponse{SealingBattleResult: result, BattleResult: toPartyBattleResult(result.BattleResult)}
}

type challengeRoundResultResponse struct {
	*challenge.RoundResult
	BattleResult battleResultResponse `json:"battle_result"`
}

func toChallengeRoundResult(result *challenge.RoundResult) *challengeRoundResultResponse {
	if result == nil {
		return nil
	}
	return &challengeRoundResultResponse{RoundResult: result, BattleResult: toBattleResult(result.BattleResult)}
}

type challengeCommandResultResponse struct {
	*challenge.CommandResult
	Round *challengeRoundResultResponse `json:"round,omitempty"`
}

type pvpRoundResponse struct {
	pvp.RoundResolution
	BattleLog []turnLogResponse `json:"battle_log,omitempty"`
}

type gvgRoundResponse struct {
	gvg.RoundResolution
	BattleLog []turnLogResponse `json:"battle_log,omitempty"`
}

type partyAdventureResponse struct {
	party.PartyAdventureResult
	MonsterTames map[string][]monsterTameResponse `json:"monster_tames,omitempty"`
	BattleResult partyBattleResultResponse        `json:"battle_result"`
}

type monsterTameResponse struct {
	battle.MonsterTameResult
	Message string `json:"message"`
}

func toMonsterTames(results map[string][]battle.MonsterTameResult) map[string][]monsterTameResponse {
	if len(results) == 0 {
		return nil
	}
	response := make(map[string][]monsterTameResponse, len(results))
	for id, tames := range results {
		for _, result := range tames {
			message := ""
			if result.BoxFull {
				message = fmt.Sprintf("なんと %s が起き上がりこちらを見ている。しかし、%sのモンスター預かり所はいっぱいだった。%sは悲しそうに去っていった…", result.MonsterName, result.TamerName, result.MonsterName)
			} else if result.Success {
				message = fmt.Sprintf("なんと %s が起き上がりこちらを見ている。%sはうれしそうに%sのモンスター預かり所に向かった", result.MonsterName, result.MonsterName, result.TamerName)
			}
			response[id] = append(response[id], monsterTameResponse{MonsterTameResult: result, Message: message})
		}
	}
	return response
}

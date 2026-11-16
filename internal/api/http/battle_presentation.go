package http

import (
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type turnLogResponse struct {
	corebattle.TurnLog
	Message string `json:"message"`
}

func toTurnLogs(logs []corebattle.TurnLog) []turnLogResponse {
	if logs == nil {
		return nil
	}
	result := make([]turnLogResponse, len(logs))
	for i, log := range logs {
		result[i] = turnLogResponse{TurnLog: log, Message: battleLogMessage(log)}
	}
	return result
}

func battleLogMessage(log corebattle.TurnLog) string {
	actor, target := log.ActorName, log.TargetName
	if actor == "" {
		actor = log.ActorID
	}
	if target == "" {
		target = log.TargetID
	}
	prefix := fmt.Sprintf("%s の %s！", actor, log.ActionName)
	if log.Source == "item" {
		prefix = fmt.Sprintf("%s は %s をつかった！", actor, log.ActionName)
	}
	if log.Source == "custom" {
		prefix = fmt.Sprintf("%s 「%s」 %s！", actor, log.Incantation, log.ActionName)
	}
	var message string
	switch log.Kind {
	case "defend":
		message = fmt.Sprintf("%s は身を固めている！", actor)
	case "miss":
		message = fmt.Sprintf("%s ミス！%s は攻撃をかわした！", prefix, target)
	case "damage":
		switch {
		case log.Immunity == "magic":
			message = fmt.Sprintf("%s の %s！ %s は魔法をうけつけない！", actor, log.ActionName, target)
		case log.Immunity == "physical":
			message = fmt.Sprintf("%s の %s！ %s は攻撃をうけつけない！", actor, log.ActionName, target)
		case log.IsCritical:
			message = fmt.Sprintf("%s の会心の一撃！！ %s に %d のダメージ！", actor, target, log.DamageDealt)
		default:
			message = fmt.Sprintf("%s %s に %d のダメージ！", prefix, target, log.DamageDealt)
		}
	case "heal":
		message = fmt.Sprintf("%s %s のHPが %d 回復した！", prefix, target, log.HealingDone)
	case "buff":
		message = fmt.Sprintf("%s %s の%sが %d 上がった！", prefix, target, log.BuffStat, log.BuffAmount)
	case "buff_blocked":
		message = fmt.Sprintf("%s は鎖縛により能力があがらない！", target)
	case "status":
		message = fmt.Sprintf("%s %s は状態異常（%s）になった！", prefix, target, log.Status)
	case "banish":
		message = fmt.Sprintf("%s が異空間へと吸い込まれた！", target)
	case "field":
		message = fmt.Sprintf("%s %sの属性フィールドが展開された！", prefix, log.Element)
	case "anti_field":
		message = prefix + " アンチフィールドが展開された！"
	case "magic_sealed":
		message = fmt.Sprintf("しかし、%s は魔法が封じられていた", actor)
	case "confused":
		message = fmt.Sprintf("%s は混乱している！", actor)
	case "immobilized":
		switch log.Status {
		case corebattle.StatusParalyze:
			message = fmt.Sprintf("%s は麻痺して動くことができない！", actor)
		case corebattle.StatusSleep:
			message = fmt.Sprintf("%s は眠っている！", actor)
		default:
			message = fmt.Sprintf("しかし、%s は動くことができない！", actor)
		}
		if log.StatusCured {
			message += fmt.Sprintf(" %s は鎖から解放された！", actor)
		}
	case "status_cured":
		switch log.Status {
		case corebattle.StatusMagicSeal:
			message = fmt.Sprintf("%s は魔法が使えるようになりました！", actor)
		case corebattle.StatusSleep:
			message = fmt.Sprintf("%s は眠りから覚めた！", actor)
		case corebattle.StatusConfusion:
			message = fmt.Sprintf("%s の混乱が治った！", actor)
		case corebattle.StatusParalyze:
			message = fmt.Sprintf("%s の麻痺が治った！", actor)
		case corebattle.StatusPoison:
			message = fmt.Sprintf("%s の毒が治った！", actor)
		}
	case "status_damage":
		status := log.Status
		switch status {
		case corebattle.StatusKinju:
			status = "禁呪"
		case corebattle.StatusPoison:
			status = "毒"
		case corebattle.StatusDeadlyPoison:
			status = "猛毒"
		case corebattle.StatusVirulentPoison:
			status = "劇毒"
		}
		message = fmt.Sprintf("%s は%sにより %d のダメージをうけた！", actor, status, log.DamageDealt)
	case "crystals":
		message = fmt.Sprintf("刻印晶を手に入れた！ (%d個)", log.Crystals)
	}
	if log.Revival != nil && log.Revival.Revived {
		name := target
		if log.TargetID == "" {
			name = actor
		}
		if log.Revival.Source == "pharaoh" {
			message += fmt.Sprintf(" %sは不死の呪いでよみがえった！", name)
		} else {
			message += fmt.Sprintf(" %sは瀕死でよみがえった！", name)
		}
		if log.Revival.Cursed {
			message += fmt.Sprintf(" %sは呪われた！", name)
		}
	}
	if log.BuffsReset {
		message += fmt.Sprintf(" %s のステータスが元に戻った！", target)
	}
	return message
}

type battleResultResponse struct {
	corebattle.Result
	Logs []turnLogResponse `json:"logs,omitempty"`
}

func toBattleResult(result corebattle.Result) battleResultResponse {
	return battleResultResponse{Result: result, Logs: toTurnLogs(result.Logs)}
}

type partyBattleResultResponse struct {
	corebattle.PartyBattleResult
	Logs []turnLogResponse `json:"logs,omitempty"`
}

func toPartyBattleResult(result corebattle.PartyBattleResult) partyBattleResultResponse {
	return partyBattleResultResponse{PartyBattleResult: result, Logs: toTurnLogs(result.Logs)}
}

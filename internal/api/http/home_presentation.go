package http

import (
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
)

type homeSleepResponse struct {
	home.SleepResult
	Message string `json:"message"`
}

type wakeResponse struct {
	Recovered bool              `json:"recovered"`
	Message   string            `json:"message"`
	Character characterResponse `json:"character"`
}

type homeItemUseResponse struct {
	*home.UseHomeItemResult
	Message string `json:"message"`
}

func newHomeSleepResponse(r home.SleepResult) homeSleepResponse {
	return homeSleepResponse{SleepResult: r, Message: r.CharacterName + "はベッドにもぐりこんだ！"}
}

func newWakeResponse(r home.WakeResult) wakeResponse {
	msg := r.Character.Name + "はすでに目覚めています"
	if r.Recovered {
		msg = r.Character.Name + "のHP・MP・疲労度が回復した！"
	}
	return wakeResponse{Recovered: r.Recovered, Message: msg, Character: toCharacterResponse(r.Character)}
}

// sleepingMessage is the remaining-time line legacy sleep.cgi shows while asleep.
func sleepingMessage(remainingSeconds int) string {
	return fmt.Sprintf("お休み中「Zzz...」 目覚めるまで %d分%02d秒", remainingSeconds/60, remainingSeconds%60)
}

// newHomeItemUseResponse composes ＠つかう's text. Legacy announces
// "<item>をつかった！" before a committed held or Depot category-2 callback,
// never before a Depot category-4 one or a no-effect reply (lib/home.cgi:287,323,329–333).
func newHomeItemUseResponse(r *home.UseHomeItemResult) homeItemUseResponse {
	var msg string
	switch {
	case r.Inspection != nil:
		label := "武器名"
		if r.Kind == 2 {
			label = "防具名"
		}
		msg = fmt.Sprintf("%s：%s / 強さ：%d / 重さ：%d / 価格：%dG", label, r.ItemName, r.Inspection.Power, r.Inspection.Weight, r.Inspection.Price)
	case r.Outcome != nil:
		var actor string
		if r.Character != nil {
			actor = r.Character.Name
		}
		msg = homeItemEffectText(actor, r.ItemName, *r.Outcome)
		if r.Action == "consumed" && !r.Deferred {
			msg = r.ItemName + "をつかった！" + msg
		}
	}
	return homeItemUseResponse{UseHomeItemResult: r, Message: msg}
}

var homeStatLabels = map[string]string{
	home.HomeStatMaxHP: "HP", home.HomeStatMaxMP: "MP", home.HomeStatAttack: "攻撃力",
	home.HomeStatDefense: "守備力", home.HomeStatAgility: "素早さ", home.HomeStatSP: "SP",
}

func homeItemEffectText(actor, itemName string, o home.HomeItemOutcome) string {
	switch o.Effect {
	case home.HomeItemEffectNoEffect:
		return "しかし、何も起こらなかった…"
	case home.HomeItemEffectStatGain:
		return fmt.Sprintf("%sの%sが %d あがった！", actor, homeStatLabels[o.Stat], o.Amount)
	case home.HomeItemEffectLevelUpReserved:
		return "次のクエスト時にレベルアップ！"
	case home.HomeItemEffectTiredReset:
		return fmt.Sprintf("元気全快！%sの疲労が回復した！", actor)
	case home.HomeItemEffectGodVisit:
		if o.GodVisit == corecharacter.GodVisitUnderworld {
			return actor + "は、黒い天馬に乗り天界?へと導かれた！"
		}
		return actor + "は、天馬に乗り天界へと導かれた！"
	case home.HomeItemEffectOrbOffered:
		return itemName + "を復活の祭壇にささげた！"
	case home.HomeItemEffectOrbAlreadyOffered:
		return "すでにささげられています"
	case home.HomeItemEffectSmallMedal:
		return "メダル伯爵にメダルを１枚献上しました"
	case home.HomeItemEffectSealRemoved:
		return fmt.Sprintf("武器に施された刻印が剥がれ落ちた！刻印晶を %d 個回収した！", o.Amount)
	case home.HomeItemEffectFutureMemory:
		return "未来のカケラに記憶が写った！"
	case home.HomeItemEffectFutureMemoryFull:
		return "これ以上職業を覚えられない"
	case home.HomeItemEffectVeteranReward:
		return fmt.Sprintf("これまでの戦いの報酬として %d Gもらった!", o.Amount)
	case home.HomeItemEffectPersonality:
		return fmt.Sprintf("%sは%sをじっくり読んだ………%sの性格が%sになった！", actor, itemName, actor, o.Personality)
	case home.HomeItemEffectCostume:
		return fmt.Sprintf("%sは%sのコスプレをした！", actor, cosplayLabel(itemName))
	case home.HomeItemEffectTransformScroll:
		return fmt.Sprintf("%sは%sを読んだ！", actor, itemName)
	case home.HomeItemEffectRecipeLearned:
		return fmt.Sprintf("%s は錬金レシピを読んだ！【%s × %s ＝ ？？？】の錬金方法を習得した！", actor, o.Recipe.BaseName, o.Recipe.MaterialName)
	case home.HomeItemEffectRecipeExhausted:
		return "この錬金レシピからこれ以上習得できる錬金方法はないようだ…"
	case home.HomeItemEffectRewardItems:
		if itemName == "太古の財宝" {
			return fmt.Sprintf("なんと、太古の財宝 は %s だった！", strings.Join(o.Items, "、"))
		}
		return fmt.Sprintf("なんと、%sの中身は %s だった！", itemName, strings.Join(o.Items, "、"))
	case home.HomeItemEffectSealedBoxOpened:
		return fmt.Sprintf("異封の鍵をつかって異封の箱は開けた！　なんと中には %s が入っていた", strings.Join(o.Items, "、"))
	case home.HomeItemEffectNoSealedBox:
		return "何かの箱を開けるための鍵みたいだ"
	default: // HomeItemEffectNone
		return ""
	}
}

// itemRefusalMessage is legacy's ＠つかう refusal (lib/home.cgi:284,293,335).
func itemRefusalMessage(r home.ItemRefusal) string {
	if r.CombatOnly {
		return r.ItemName + "は戦闘中でしか使えません"
	}
	return r.ItemName + "はここでは使えません"
}

// cosplayLabel is the costume name legacy puts before のコスプレをした.
func cosplayLabel(itemName string) string {
	if itemName == "王族の衣装" {
		return "王様"
	}
	return strings.TrimSuffix(itemName, "の衣装")
}

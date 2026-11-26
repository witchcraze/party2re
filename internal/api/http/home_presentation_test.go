package http

import (
	"fmt"
	"net/http"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeSleepAndWakeMessages(t *testing.T) {
	if got := newHomeSleepResponse(home.SleepResult{Sleeping: true, CharacterName: "Hero"}).Message; got != "Heroはベッドにもぐりこんだ！" {
		t.Errorf("sleep message = %q", got)
	}
	char := corecharacter.Character{ID: "hero", Name: "Hero"}
	if got := newWakeResponse(home.WakeResult{Recovered: true, Character: char}); !got.Recovered || got.Message != "HeroのHP・MP・疲労度が回復した！" {
		t.Errorf("recovered wake = %+v", got)
	}
	if got := newWakeResponse(home.WakeResult{Character: char}); got.Recovered || got.Message != "Heroはすでに目覚めています" {
		t.Errorf("already awake wake = %+v", got)
	}
	if got := sleepingMessage(305); got != "お休み中「Zzz...」 目覚めるまで 5分05秒" {
		t.Errorf("sleeping message = %q", got)
	}
}

func TestHomeItemUseMessage(t *testing.T) {
	hero := &corecharacter.Character{ID: "hero", Name: "Hero"}
	outcome := func(o home.HomeItemOutcome) *home.HomeItemOutcome { return &o }
	consumed := func(name string, o home.HomeItemOutcome) *home.UseHomeItemResult {
		return &home.UseHomeItemResult{Action: "consumed", ItemName: name, Kind: 3, Consumed: true, Outcome: outcome(o), Character: hero}
	}
	tests := []struct {
		name   string
		result *home.UseHomeItemResult
		want   string
	}{
		{"weapon inspection", &home.UseHomeItemResult{Action: "inspect", ItemName: "銅の剣", Kind: 1, Inspection: &home.HomeItemInspection{Power: 14, Weight: 9, Price: 400}, Character: hero},
			"武器名：銅の剣 / 強さ：14 / 重さ：9 / 価格：400G"},
		{"armor inspection", &home.UseHomeItemResult{Action: "inspect", ItemName: "布の服", Kind: 2, Inspection: &home.HomeItemInspection{Power: 3, Price: 20}, Character: hero},
			"防具名：布の服 / 強さ：3 / 重さ：0 / 価格：20G"},
		{"held or Depot category-2 use is announced", consumed("命の木の実", home.HomeItemOutcome{Effect: home.HomeItemEffectStatGain, Stat: home.HomeStatMaxHP, Amount: 3}),
			"命の木の実をつかった！HeroのHPが 3 あがった！"},
		{"zero over-level gain", consumed("力の種", home.HomeItemOutcome{Effect: home.HomeItemEffectStatGain, Stat: home.HomeStatAttack}),
			"力の種をつかった！Heroの攻撃力が 0 あがった！"},
		{"no-stat item keeps only the announcement", consumed("命のブレスレット", home.HomeItemOutcome{Effect: home.HomeItemEffectNone}),
			"命のブレスレットをつかった！"},
		{"Depot category-4 use is not announced", &home.UseHomeItemResult{Action: "consumed", ItemName: "異封の鍵", Kind: 3, Consumed: true, Deferred: true,
			Outcome: outcome(home.HomeItemOutcome{Effect: home.HomeItemEffectSealedBoxOpened, Items: []string{"A", "B"}}), Character: hero},
			"異封の鍵をつかって異封の箱は開けた！　なんと中には A、B が入っていた"},
		{"Depot category-4 no-effect still consumes silently", &home.UseHomeItemResult{Action: "consumed", ItemName: "異封の鍵", Kind: 3, Consumed: true, Deferred: true,
			Outcome: outcome(home.HomeItemOutcome{Effect: home.HomeItemEffectNoSealedBox}), Character: hero},
			"何かの箱を開けるための鍵みたいだ"},
		{"retained no-effect is not announced", &home.UseHomeItemResult{Action: "no_effect", ItemName: "水晶の原石", Kind: 3,
			Outcome: outcome(home.HomeItemOutcome{Effect: home.HomeItemEffectNoEffect}), Character: hero},
			"しかし、何も起こらなかった…"},
		{"already offered orb", &home.UseHomeItemResult{Action: "no_effect", ItemName: "レッドオーブ", Kind: 3,
			Outcome: outcome(home.HomeItemOutcome{Effect: home.HomeItemEffectOrbAlreadyOffered}), Character: hero},
			"すでにささげられています"},
		{"underworld reins", consumed("黒い天馬のたづな", home.HomeItemOutcome{Effect: home.HomeItemEffectGodVisit, GodVisit: corecharacter.GodVisitUnderworld}),
			"黒い天馬のたづなをつかった！Heroは、黒い天馬に乗り天界?へと導かれた！"},
		{"heaven reins", consumed("天馬のたづな", home.HomeItemOutcome{Effect: home.HomeItemEffectGodVisit, GodVisit: corecharacter.GodVisitHeaven}),
			"天馬のたづなをつかった！Heroは、天馬に乗り天界へと導かれた！"},
		{"costume label", consumed("王族の衣装", home.HomeItemOutcome{Effect: home.HomeItemEffectCostume}),
			"王族の衣装をつかった！Heroは王様のコスプレをした！"},
		{"costume suffix", consumed("冒険家の衣装", home.HomeItemOutcome{Effect: home.HomeItemEffectCostume}),
			"冒険家の衣装をつかった！Heroは冒険家のコスプレをした！"},
		{"transform scroll", consumed("変身の巻物", home.HomeItemOutcome{Effect: home.HomeItemEffectTransformScroll}),
			"変身の巻物をつかった！Heroは変身の巻物を読んだ！"},
		{"recipe", consumed("基本錬金レシピ", home.HomeItemOutcome{Effect: home.HomeItemEffectRecipeLearned, Recipe: &home.LearnedRecipe{BaseName: "薬草", MaterialName: "薬草"}}),
			"基本錬金レシピをつかった！Hero は錬金レシピを読んだ！【薬草 × 薬草 ＝ ？？？】の錬金方法を習得した！"},
		{"bag", consumed("福袋", home.HomeItemOutcome{Effect: home.HomeItemEffectRewardItems, Items: []string{"福袋"}}),
			"福袋をつかった！なんと、福袋の中身は 福袋 だった！"},
		{"ancient treasure", consumed("太古の財宝", home.HomeItemOutcome{Effect: home.HomeItemEffectRewardItems, Items: []string{"A", "B"}}),
			"太古の財宝をつかった！なんと、太古の財宝 は A、B だった！"},
		{"personality", consumed("エッチな本", home.HomeItemOutcome{Effect: home.HomeItemEffectPersonality, Personality: "ムッツリスケベ"}),
			"エッチな本をつかった！Heroはエッチな本をじっくり読んだ………Heroの性格がムッツリスケベになった！"},
		{"veteran reward", consumed("歴戦の褒章", home.HomeItemOutcome{Effect: home.HomeItemEffectVeteranReward, Amount: 10}),
			"歴戦の褒章をつかった！これまでの戦いの報酬として 10 Gもらった!"},
		{"future memory without a slot", &home.UseHomeItemResult{Action: "no_effect", ItemName: "未来のカケラ", Kind: 3,
			Outcome: outcome(home.HomeItemOutcome{Effect: home.HomeItemEffectFutureMemoryFull}), Character: hero},
			"これ以上職業を覚えられない"},
		{"seal removed", consumed("水晶の原石", home.HomeItemOutcome{Effect: home.HomeItemEffectSealRemoved, Amount: 25}),
			"水晶の原石をつかった！武器に施された刻印が剥がれ落ちた！刻印晶を 25 個回収した！"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := newHomeItemUseResponse(tc.result).Message; got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHomeItemRefusalRejection(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{home.ItemRefusal{ItemName: "薬草", CombatOnly: true}, "薬草は戦闘中でしか使えません"},
		{fmt.Errorf("wrapped: %w", home.ItemRefusal{ItemName: "薬草"}), "薬草はここでは使えません"},
		{home.ErrCannotUseHere, "ここでは使えません"},
	} {
		status, detail := homeActionRejection(tc.err)
		if status != http.StatusBadRequest || detail.Code != "HOME_ITEM_CANNOT_USE" || detail.Message != tc.want {
			t.Errorf("%v: %d %+v, want %q", tc.err, status, detail, tc.want)
		}
	}
}

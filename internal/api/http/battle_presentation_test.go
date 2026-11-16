package http

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBattleLogPresentation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts corebattle.TurnLog
		want  string
	}{
		{"attack", corebattle.TurnLog{Kind: "damage", DamageDealt: 12}, "Hero の 技！ Slime に 12 のダメージ！"},
		{"miss", corebattle.TurnLog{Kind: "miss"}, "Hero の 技！ ミス！Slime は攻撃をかわした！"},
		{"critical", corebattle.TurnLog{Kind: "damage", IsCritical: true, DamageDealt: 20}, "Hero の会心の一撃！！ Slime に 20 のダメージ！"},
		{"magic immunity precedes critical", corebattle.TurnLog{Kind: "damage", Immunity: "magic", IsCritical: true}, "Hero の 技！ Slime は魔法をうけつけない！"},
		{"physical immunity", corebattle.TurnLog{Kind: "damage", Immunity: "physical"}, "Hero の 技！ Slime は攻撃をうけつけない！"},
		{"custom attack", corebattle.TurnLog{Kind: "damage", Source: "custom", Incantation: "燃えよ", DamageDealt: 30}, "Hero 「燃えよ」 技！ Slime に 30 のダメージ！"},
		{"item heal at cap", corebattle.TurnLog{Kind: "heal", Source: "item", HealingDone: 0}, "Hero は 技 をつかった！ Slime のHPが 0 回復した！"},
		{"skill heal", corebattle.TurnLog{Kind: "heal", HealingDone: 10}, "Hero の 技！ Slime のHPが 10 回復した！"},
		{"custom heal", corebattle.TurnLog{Kind: "heal", Source: "custom", Incantation: "癒せ", HealingDone: 10}, "Hero 「癒せ」 技！ Slime のHPが 10 回復した！"},
		{"item buff", corebattle.TurnLog{Kind: "buff", Source: "item", BuffStat: "attack", BuffAmount: 5}, "Hero は 技 をつかった！ Slime のattackが 5 上がった！"},
		{"skill buff", corebattle.TurnLog{Kind: "buff", BuffStat: "defense", BuffAmount: 5}, "Hero の 技！ Slime のdefenseが 5 上がった！"},
		{"custom buff", corebattle.TurnLog{Kind: "buff", Source: "custom", Incantation: "護れ", BuffStat: "defense", BuffAmount: 5}, "Hero 「護れ」 技！ Slime のdefenseが 5 上がった！"},
		{"blocked buff", corebattle.TurnLog{Kind: "buff_blocked"}, "Slime は鎖縛により能力があがらない！"},
		{"item status", corebattle.TurnLog{Kind: "status", Source: "item", Status: "sleep"}, "Hero は 技 をつかった！ Slime は状態異常（sleep）になった！"},
		{"skill status", corebattle.TurnLog{Kind: "status", Status: "sleep"}, "Hero の 技！ Slime は状態異常（sleep）になった！"},
		{"banish", corebattle.TurnLog{Kind: "banish"}, "Slime が異空間へと吸い込まれた！"},
		{"field", corebattle.TurnLog{Kind: "field", Source: "custom", Incantation: "燃えよ", Element: "fire"}, "Hero 「燃えよ」 技！ fireの属性フィールドが展開された！"},
		{"anti field", corebattle.TurnLog{Kind: "anti_field", Source: "custom", Incantation: "消えよ"}, "Hero 「消えよ」 技！ アンチフィールドが展開された！"},
		{"defend", corebattle.TurnLog{Kind: "defend"}, "Hero は身を固めている！"},
		{"magic sealed", corebattle.TurnLog{Kind: "magic_sealed"}, "しかし、Hero は魔法が封じられていた"},
		{"confusion", corebattle.TurnLog{Kind: "confused"}, "Hero は混乱している！"},
		{"action sealed", corebattle.TurnLog{Kind: "immobilized", Status: corebattle.StatusDofuu}, "しかし、Hero は動くことができない！"},
		{"paralysis", corebattle.TurnLog{Kind: "immobilized", Status: corebattle.StatusParalyze}, "Hero は麻痺して動くことができない！"},
		{"sleep", corebattle.TurnLog{Kind: "immobilized", Status: corebattle.StatusSleep}, "Hero は眠っている！"},
		{"bound", corebattle.TurnLog{Kind: "immobilized", Status: corebattle.StatusSabaku}, "しかし、Hero は動くことができない！"},
		{"bound then released", corebattle.TurnLog{Kind: "immobilized", Status: corebattle.StatusSabaku, StatusCured: true}, "しかし、Hero は動くことができない！ Hero は鎖から解放された！"},
		{"magic unsealed", corebattle.TurnLog{Kind: "status_cured", Status: corebattle.StatusMagicSeal}, "Hero は魔法が使えるようになりました！"},
		{"confusion cured", corebattle.TurnLog{Kind: "status_cured", Status: corebattle.StatusConfusion}, "Hero の混乱が治った！"},
		{"paralysis cured", corebattle.TurnLog{Kind: "status_cured", Status: corebattle.StatusParalyze}, "Hero の麻痺が治った！"},
		{"woken", corebattle.TurnLog{Kind: "status_cured", Status: corebattle.StatusSleep}, "Hero は眠りから覚めた！"},
		{"poison cured", corebattle.TurnLog{Kind: "status_cured", Status: corebattle.StatusPoison}, "Hero の毒が治った！"},
		{"kinju", corebattle.TurnLog{Kind: "status_damage", Status: corebattle.StatusKinju, DamageDealt: 10}, "Hero は禁呪により 10 のダメージをうけた！"},
		{"poison", corebattle.TurnLog{Kind: "status_damage", Status: corebattle.StatusDeadlyPoison, DamageDealt: 10}, "Hero は猛毒により 10 のダメージをうけた！"},
		{"crystals", corebattle.TurnLog{Kind: "crystals", Crystals: 2}, "刻印晶を手に入れた！ (2個)"},
		{"dispel after hit", corebattle.TurnLog{Kind: "damage", DamageDealt: 12, BuffsReset: true}, "Hero の 技！ Slime に 12 のダメージ！ Slime のステータスが元に戻った！"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := tc.facts
			log.ActorID, log.TargetID, log.ActorName, log.TargetName, log.ActionName = "hero", "slime", "Hero", "Slime", "技"
			log.RemainingHP = map[string]int{"hero": 100, "slime": 0}
			before, err := json.Marshal(log)
			if err != nil {
				t.Fatal(err)
			}
			if got := toTurnLogs([]corebattle.TurnLog{log})[0]; got.Message != tc.want || !reflect.DeepEqual(got.TurnLog, log) {
				t.Fatalf("got %+v, want %q", got, tc.want)
			}
			var restored corebattle.TurnLog
			if err := json.Unmarshal(before, &restored); err != nil {
				t.Fatal(err)
			}
			if got := battleLogMessage(restored); got != tc.want {
				t.Fatalf("after save/reload: %q want %q", got, tc.want)
			}
			after, err := json.Marshal(log)
			if err != nil || string(before) != string(after) {
				t.Fatal("presentation mutated domain facts")
			}
		})
	}
	if toTurnLogs(nil) != nil {
		t.Fatal("nil log changed")
	}
	if got := battleLogMessage(corebattle.TurnLog{Kind: "defend", ActorID: "unnamed"}); got != "unnamed は身を固めている！" {
		t.Fatal(got)
	}
}

func TestBattleRevivalPresentation(t *testing.T) {
	for _, tc := range []struct {
		source string
		cursed bool
		want   string
	}{
		{"undying", false, "瀕死でよみがえった！"}, {"revive", false, "瀕死でよみがえった！"},
		{"touki_shield", false, "瀕死でよみがえった！"}, {"dokuro_amulet", false, "瀕死でよみがえった！"},
		{"pharaoh", false, "不死の呪いでよみがえった！"}, {"cursed_revive", true, "瀕死でよみがえった！ Slimeは呪われた！"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			log := corebattle.TurnLog{Kind: "damage", ActorID: "hero", ActorName: "Hero", TargetID: "slime", TargetName: "Slime", ActionName: "攻撃", DamageDealt: 12, Revival: &corebattle.RevivalResult{Revived: true, Source: tc.source, Cursed: tc.cursed}}
			want := "Hero の 攻撃！ Slime に 12 のダメージ！ Slimeは" + tc.want
			if got := battleLogMessage(log); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
	log := corebattle.TurnLog{Kind: "status_damage", ActorID: "hero", ActorName: "Hero", Status: corebattle.StatusKinju, DamageDealt: 10, Revival: &corebattle.RevivalResult{Revived: true, Source: "undying"}}
	if got := battleLogMessage(log); got != "Hero は禁呪により 10 のダメージをうけた！ Heroは瀕死でよみがえった！" {
		t.Fatal(got)
	}
}

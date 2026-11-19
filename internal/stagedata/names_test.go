package stagedata_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/stagedata"
)

// replacedNames maps each franchise-distinctive legacy name (rule 00 §3) to
// the one generic name every shipped catalog uses instead (#2402).
var replacedNames = map[string]string{
	"おおめだま": "巨大眼球", "ひくいどり": "火炎鳥", "アカテンダー": "紅トゲサボテン",
	"オバケカボチャ": "ジャックランタン", "オバケキノコ": "ゴーストマッシュ", "キラーパンサー": "シャドウパンサー",
	"キラーマシン": "自動戦闘機械", "キングベヒーモス": "ベヒーモスロード", "ギガンテス": "一つ目の巨人",
	"コロヒーロー": "コボルト勇者", "コロファイター": "コボルト戦士", "コロプリースト": "コボルト僧侶",
	"コロマージ": "コボルト魔道士", "ゴーストスライム": "レイススライム", "ゴールデンスライム": "金塊スライム",
	"ゴールドラキー": "ゴールドバット", "サボテンダー": "トゲサボテン", "スペクテッド": "浮遊眼",
	"スライムつむり": "シェルスライム", "スライムまどう": "メイジスライム", "スライムバット": "バットスライム",
	"スライムヒューマ": "ヒューマンスライム", "スライムファラオ": "ファラオスライム", "スライムベス": "レッドスライム",
	"ダースドラゴン": "黒竜", "デスマシン": "死神の魔導機械", "デンデンリュウ": "マイマイ竜",
	"トラップボックス": "罠の小箱", "ドクキノコ": "ポイズンマッシュ", "ドラキー": "ナイトバット",
	"ナスビーラ": "魔界ナス", "ハグレメタル": "流銀スライム", "パンドラボックス": "パンドラの魔箱",
	"ビックドラキー": "ジャイアントバット", "ブチスライム": "ドットスライム", "ブラッドハンド": "血塗られた手",
	"プチヒーロー": "ちび勇者", "プチファイター": "ちび戦士", "プチプリースト": "ちび僧侶",
	"プチマージ": "ちび魔道士", "ベビーパンサー": "ベビーレオ", "ベホマスライム": "ハイヒーラースライム",
	"ホイミスライム": "ヒーラースライム", "マッドハンド": "泥の魔手", "マネマネ": "ものまね妖魔",
	"マージマタンゴ": "メイジマッシュ", "ミニデーモン": "小悪魔インプ", "メイジドラキー": "メイジバット",
	"メガザルロック": "守護の岩石", "メタルキング": "王様流銀スライム", "メタルスライム": "銀滴スライム",
	"モヒカント": "ビーストマスター", "ラボス": "深淵の巨魁", "人面樹": "トレント人面樹",
	"片翼の天使": "隻翼の堕天使", "黒ドラキー": "ダークバット", "黒マネマネ": "漆黒のものまね妖魔",
}

var suffix = regexp.MustCompile(`[A-HＡ-Ｈ]$`)

func checkNames(t *testing.T, file string, names []string, seen map[string]bool) {
	t.Helper()
	for _, n := range names {
		if strings.HasPrefix(n, "Stage ") {
			t.Errorf("%s: placeholder name %q", file, n)
		}
		base := suffix.ReplaceAllString(n, "")
		if _, banned := replacedNames[base]; banned {
			t.Errorf("%s: franchise name %q", file, n)
		}
		seen[base] = true
	}
}

func TestShippedEnemyNamesUseSourceOrGenericNames(t *testing.T) {
	monsters, err := stagedata.Monsters()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(monsters))
	byID := map[string]string{}
	for i, m := range monsters {
		names[i], byID[m.ID] = m.Name, m.Name
	}
	seen := map[string]bool{}
	checkNames(t, "stagedata", names, seen)
	// One entry per name source: unchanged source name, batch-catalog
	// replacement, earlier documented table and approved alias (#2373).
	for id, want := range map[string]string{
		"ordinary-05-boss-10":     "闇の魔術士",
		"ordinary-05-monster-120": "小悪魔インプ",
		"ordinary-00-monster-65":  "ナイトバット",
		"ordinary-05-monster-149": "浮遊眼",
		"ordinary-16-boss-11":     "王様流銀スライムA", // suffix kept
	} {
		if byID[id] != want {
			t.Errorf("%s = %q, want %q", id, byID[id], want)
		}
	}

	// Other shipped catalogs that display the same enemies.
	for _, file := range []string{"../adventure/data/monsters.json", "../challenge/data/challenge_tiers.json"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var found []string
		collectNames(t, raw, &found)
		if len(found) == 0 {
			t.Fatalf("%s: no names found", file)
		}
		checkNames(t, file, found, seen)
	}
	for legacy, generic := range replacedNames {
		if !seen[generic] {
			t.Errorf("replacement %s → %s is used by no shipped catalog", legacy, generic)
		}
	}
}

// collectNames gathers every "name" string in a JSON document.
func collectNames(t *testing.T, raw []byte, out *[]string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if s, ok := e.(string); ok && k == "name" {
					*out = append(*out, s)
				}
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
}

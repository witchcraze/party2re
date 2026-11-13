package http

import (
	"testing"
)

type divinationDraws []int

func (s *divinationDraws) Intn(int) int { v := (*s)[0]; *s = (*s)[1:]; return v }

func TestDivinateUsesLegacyListsAndLine(t *testing.T) {
	for _, tc := range []struct {
		draws []int
		want  string
	}{
		{[]int{0, 0}, "今日の勇者さんの運勢はずばり…大吉です♪ラッキーカラーは黒色ですよ〜。"},
		// Index 7 is the repeated 大吉; the last entries close both lists.
		{[]int{7, 26}, "今日の勇者さんの運勢はずばり…大吉です♪ラッキーカラーはパール色ですよ〜。"},
		{[]int{21, 15}, "今日の勇者さんの運勢はずばり…何か起きますです♪ラッキーカラーはワインレッド色ですよ〜。"},
	} {
		draws := divinationDraws(tc.draws)
		if got := divination(draws.Intn, "勇者"); got != tc.want {
			t.Errorf("draws %v: got %q, want %q", tc.draws, got, tc.want)
		}
	}
}

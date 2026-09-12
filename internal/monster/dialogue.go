package monster

import (
	"fmt"
)

// Dialogue represents NPC @モンジィ dialogue.
type Dialogue struct {
	NPCName  string   `json:"npc_name"`
	Title    string   `json:"title"`
	Greeting string   `json:"greeting"`
	Phrases  []string `json:"phrases"`
}

// GetDialogue returns NPC @モンジィ dialogue.
func (s *Service) GetDialogue() Dialogue {
	return Dialogue{
		NPCName:  "@モンジィ",
		Title:    "モンスターじいさん",
		Greeting: "わしが有名な@モンジィじゃ。モンスターのことなら何でも聞いてくれい",
		Phrases: []string{
			"わしが有名な@モンジィじゃ。モンスターのことなら何でも聞いてくれい",
			"何度かモンスターを倒していると、なついてくるモンスターがいるのじゃ",
			"人間を好むモンスターもいるということじゃ",
			"純粋な強さにモンスターはひきつけられるのじゃ",
			fmt.Sprintf("自分の家には%d匹までペットを連れて行くことができるぞい", MaxHomePets),
			"モンスターは最大50匹（限界突破で最大300匹）まで預かっておけるぞい。それ以上は、残念じゃが＠わかれるしかないのぉ…",
			"モンスター預かり所がまんぱんの状態だと、モンスターは仲間にならんから注意じゃ",
			"自分が相手より強い方が仲間になりやすいぞい",
			"ふがふがふがふがふがふがふが",
		},
	}
}

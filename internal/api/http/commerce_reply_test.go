package http

import (
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// Branches not reached by the gateway fixtures.
func TestCommerceReplyBranches(t *testing.T) {
	for name, tc := range map[string]struct{ got, want string }{
		"alchemy known result": {
			alchemySynthesisReply(alchemy.SynthesisResult{IngredientNames: []string{"薬草", "聖水"}, ResultItemName: "エリクサー"}).Message,
			"薬草 と 聖水 じゃな！ふむ、この組み合わせなら エリクサー ができるぞ！完成する頃にまた来るがよい",
		},
		"black market u-rare": {
			blackMarketSacrificeReply(&blackmarket.SacrificeResult{ItemName: "x", URarePointsGained: 2}).Message,
			"これは……! ……いいだろう…。お前の特別なレアポイントを2加算しておこう…",
		},
		"gem store item": {
			gemStoreAppraiseReply(gemstore.AppraiseResult{Character: corecharacter.Character{Name: "Hero"}, IdentifiedName: "鉄の剣"}).Message,
			"これは… 鉄の剣ですね",
		},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", name, tc.got, tc.want)
		}
	}
}

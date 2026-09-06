package tavern

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	NPCName      = "@エレナ"
	LocationName = "冒険者の酒場"
)

// Random dialogue list for the tavern barkeep NPC.
var barkeepDialogues = []string{
	"いらっしゃい！冒険者の酒場へようこそ。美味しいご飯と飲み物を用意してるわよ♪",
	"食材にはMPを回復させる魔法の聖水や、HPを回復させる新鮮な薬草が含まれているのよ。",
	"HPを回復させたいならボリューム満点のご飯やデザートを食べていくといいわ。",
	"MPを回復させたいなら香り高いコーヒーやハーブティーを飲んでいくといいわね。",
	"お腹がいっぱいのときは無理に食べちゃダメよ？冒険で身体を動かしてからまた来てね！",
	"冒険終わりに温かい食事を食べたいなら、事前に「でりばりー」を予約しておくと便利よ♪",
	"お酒は大人になってからね！未成年にはもぎたて果実ジュースがおすすめよ。",
}

// TalkResult represents a conversation response with the tavern barkeep.
type TalkResult struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	Message      string `json:"message"`
}

// Talk returns a randomized friendly greeting and advice from the tavern barkeep.
func (s *Service) Talk(ctx context.Context, characterID string) (TalkResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return TalkResult{}, ErrInvalidCharacterID
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return TalkResult{}, ErrCharacterNotFound
		}
		return TalkResult{}, err
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(barkeepDialogues))))
	var msg string
	if err != nil {
		msg = barkeepDialogues[0]
	} else {
		msg = barkeepDialogues[n.Int64()]
	}

	// Personalize if character name exists
	if char.Name != "" {
		msg = strings.ReplaceAll(msg, "いらっしゃい！", fmt.Sprintf("いらっしゃい、%sさん！", char.Name))
	}

	return TalkResult{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		Message:      msg,
	}, nil
}

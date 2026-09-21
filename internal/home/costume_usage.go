package home

import (
	"context"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

// CostumeApplier applies active costume state for a character.
type CostumeApplier interface {
	ApplyCostume(ctx context.Context, characterID string, itemNo int, itemName, icon string, expiresAt time.Time) error
}

// SetCostumeApplier registers a cross-domain costume application hook.
func (s *Service) SetCostumeApplier(a CostumeApplier) {
	s.costumeApplier = a
}

// WithCostumeApplier configures the costume applier on Service initialization.
func WithCostumeApplier(a CostumeApplier) ServiceOption {
	return func(s *Service) {
		s.costumeApplier = a
	}
}

func isCostumeItem(name string) bool {
	switch name {
	case "ピンクスカート", "タンクトップハンマー", "チョビヒゲタクシード", "ネコミミメイド",
		"ホビット", "ハナメガネ", "ぬいぐるみ", "冒険家の衣装", "騎士団の衣装",
		"老人の衣装", "精霊の衣装", "聖職者の衣装", "王族の衣装",
		"タクシード", "闇人の衣装", "英雄の衣装", "変身の巻物", "モシャスの巻物":
		return true
	default:
		return false
	}
}

func (s *Service) applyCostumeConsumable(ctx context.Context, char *corecharacter.Character, itemName string) (string, bool, error) {
	if !isCostumeItem(itemName) {
		return "", false, nil
	}

	var itemNo int
	var icon string
	var msg string

	isMale := !strings.HasPrefix(strings.ToLower(strings.TrimSpace(char.Gender)), "f") && char.Gender != "2"

	switch itemName {
	case "ピンクスカート":
		itemNo = 44
		icon = "chr/001.gif"
		msg = fmt.Sprintf("%sはピンクスカートのコスプレをした！", char.Name)
	case "タンクトップハンマー":
		itemNo = 45
		icon = "chr/005.gif"
		msg = fmt.Sprintf("%sはタンクトップハンマーのコスプレをした！", char.Name)
	case "チョビヒゲタクシード":
		itemNo = 46
		if isMale {
			icon = "chr/012.gif"
		} else {
			icon = "chr/007.gif"
		}
		msg = fmt.Sprintf("%sはチョビヒゲタクシードのコスプレをした！", char.Name)
	case "ネコミミメイド":
		itemNo = 47
		icon = "chr/004.gif"
		msg = fmt.Sprintf("%sはネコミミメイドのコスプレをした！", char.Name)
	case "ホビット":
		itemNo = 48
		icon = "chr/014.gif"
		msg = fmt.Sprintf("%sはホビットのコスプレをした！", char.Name)
	case "ハナメガネ":
		itemNo = 49
		icon = "chr/021.gif"
		msg = fmt.Sprintf("%sはハナメガネのコスプレをした！", char.Name)
	case "ぬいぐるみ":
		itemNo = 50
		icon = "chr/023.gif"
		msg = fmt.Sprintf("%sはぬいぐるみのコスプレをした！", char.Name)
	case "冒険家の衣装":
		itemNo = 51
		icon = "chr/003.gif"
		msg = fmt.Sprintf("%sは冒険家のコスプレをした！", char.Name)
	case "騎士団の衣装":
		itemNo = 52
		icon = "chr/015.gif"
		msg = fmt.Sprintf("%sは騎士団のコスプレをした！", char.Name)
	case "老人の衣装":
		itemNo = 53
		icon = "chr/013.gif"
		msg = fmt.Sprintf("%sは老人のコスプレをした！", char.Name)
	case "精霊の衣装":
		itemNo = 54
		icon = "chr/011.gif"
		msg = fmt.Sprintf("%sは精霊のコスプレをした！", char.Name)
	case "聖職者の衣装":
		itemNo = 55
		if isMale {
			icon = "chr/016.gif"
		} else {
			icon = "chr/017.gif"
		}
		msg = fmt.Sprintf("%sは聖職者のコスプレをした！", char.Name)
	case "王族の衣装":
		itemNo = 56
		if isMale {
			icon = "chr/002.gif"
		} else {
			icon = "chr/018.gif"
		}
		msg = fmt.Sprintf("%sは王様のコスプレをした！", char.Name)
	case "タクシード":
		itemNo = 138
		icon = "chr/027.gif"
		msg = fmt.Sprintf("%sはタクシードのコスプレをした！", char.Name)
	case "闇人の衣装":
		itemNo = 139
		icon = "chr/025.gif"
		msg = fmt.Sprintf("%sは闇人のコスプレをした！", char.Name)
	case "英雄の衣装":
		itemNo = 140
		if isMale {
			icon = "chr/034.gif"
		} else {
			icon = "chr/028.gif"
		}
		msg = fmt.Sprintf("%sは英雄のコスプレをした！", char.Name)
	case "変身の巻物", "モシャスの巻物":
		itemNo = 141
		roll := s.randomInt(40) + 1
		icon = fmt.Sprintf("chr/%03d.gif", roll)
		msg = fmt.Sprintf("%sは%sを読んだ！", char.Name, itemName)
	}

	if s.costumeApplier != nil {
		now := s.nowFunc()
		expiresAt := timer.NextMidnightJST(now).UTC()
		if err := s.costumeApplier.ApplyCostume(ctx, char.ID, itemNo, itemName, icon, expiresAt); err != nil {
			return "", false, err
		}
	}

	return msg, true, nil
}

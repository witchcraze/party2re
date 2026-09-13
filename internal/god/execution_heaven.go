package god

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

func (s *Service) executeHeavenWish(
	ctx context.Context,
	char *corecharacter.Character,
	wishID string,
	res *WishResult,
) error {
	var wishName string
	var desc string
	var msg string

	switch wishID {
	case WishStats:
		if char.OverLevel {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上そなたを強くできんのだ…", ErrWishRequirement)
		}
		wishName = "強くなりたい"
		desc = "全ステータス 40 アップ"
		char.Stats.MaxHP += 40
		char.Stats.HP += 40
		char.Stats.MaxMP += 40
		char.Stats.MP += 40
		char.Stats.Attack += 40
		char.Stats.Defense += 40
		char.Stats.Agility += 40
		msg = "全ステータスが 40 上昇しました！"

	case WishSP:
		wishName = "スキルを覚えたい"
		desc = "Sp 50 アップ"
		_ = char.AddSP(50)
		msg = "スキルポイントが 50 増加しました！"

	case WishMoney:
		wishName = "お金がほしい"
		desc = "10 万G"
		_ = char.AddMoney(100000)
		msg = "100,000 G を獲得しました！"

	case WishCasinoCoins:
		wishName = "カジノコインがほしい"
		desc = "5 万枚"
		if s.casino != nil {
			if _, err := s.casino.AdjustCoins(ctx, char.ID, 50000); err != nil {
				return err
			}
		}
		msg = "カジノコイン 50,000 枚を獲得しました！"

	case WishSmallMedals:
		wishName = "小さなメダルがほしい"
		desc = "20 枚"
		_ = char.AddSmallMedals(20)
		msg = "小さなメダル 20 枚を獲得しました！"

	case WishLotteryTickets:
		wishName = "福引券がほしい"
		desc = "1000 枚"
		if s.lottery != nil {
			if _, err := s.lottery.AddRaffleTickets(ctx, char.ID, 1000); err != nil {
				return err
			}
		}
		msg = "福引券 1,000 枚を獲得しました！"

	case WishGuildRank:
		wishName = "ギルドランクをあげたい"
		desc = "1000 ポイント"
		if s.guilds == nil {
			return ErrWishRequirement
		}
		g, _, err := s.guilds.GetGuildByCharacter(ctx, char.ID)
		if err != nil {
			return fmt.Errorf("%w: ギルドに所属していません", ErrWishRequirement)
		}
		if err := s.guilds.AddPoints(ctx, g.ID, 1000); err != nil {
			return err
		}
		msg = "所属ギルドに 1,000 ポイントが加算されました！"

	case WishGuildGorgeous:
		wishName = "ギルドをゴージャスにしたい"
		desc = "ギルドが…"
		if s.guilds == nil {
			return ErrWishRequirement
		}
		g, _, err := s.guilds.GetGuildByCharacter(ctx, char.ID)
		if err != nil {
			return fmt.Errorf("%w: ギルドに所属していません", ErrWishRequirement)
		}
		if err := s.guilds.UpdateBgimg(ctx, g.ID, "god.gif"); err != nil {
			return err
		}
		msg = "ギルドが神々しい佇まいに生まれ変わりました！"

	case WishRefresh, WishFullRecovery:
		wishName = "元気いっぱいになりたい"
		desc = "疲労度 -150 % (HP・MP完全回復)"
		char.ReduceTired(150)
		char.Stats.HP = char.Stats.MaxHP
		char.Stats.MP = char.Stats.MaxMP
		msg = "疲労度が 150% 回復し、HPとMPが完全に回復しました！"

	case WishAllOrbs:
		wishName = "新しい冒険場所に行きたい"
		desc = "全オーブ"
		for _, r := range corecharacter.ValidOrbRunes {
			char.AddOrb(r)
		}
		msg = "すべてのオーブを獲得しました！"

	case WishCelestialDragon:
		wishName = "天竜人になりたい"
		desc = "転職 (空竜の民)"
		if char.JobID == "job-70" || char.OldJobID == "job-70" || char.JobID == "70" || char.OldJobID == "70" {
			return fmt.Errorf("%w: すでに天竜人です", ErrWishRequirement)
		}
		if err := char.ApplyJobChange("job-70", 0); err != nil {
			return err
		}
		msg = "空竜の民 (天竜人) へ転職しました！"

	case WishGodOfNewWorld:
		wishName = "新世界の神になりたい"
		desc = "自分の家が…"
		if s.profiles != nil {
			_ = s.profiles.UpdateAvatar(ctx, char.ID, "chr/052.gif")
		}
		if s.homes != nil {
			_ = s.homes.UpdateBgimg(ctx, char.ID, "god.gif")
		}
		msg = "新世界の神の姿と神殿のような住処を手に入れました！"

	case WishOrtega:
		wishName = "オルテガを生き返らして"
		desc = "自分の家に…"
		if s.homeMembers != nil {
			err := s.homeMembers.AddMember(ctx, HomeMember{
				CharacterID: char.ID,
				IsNPC:       true,
				Name:        "オルテガ",
				Icon:        "chr/029.gif",
			})
			if err != nil {
				return err
			}
		}
		msg = "父オルテガが自宅に帰還しました！"

	case WishCat:
		wishName = "猫を飼いたい"
		desc = "自分の家に…"
		catName := "白猫"
		catIcon := "chr/030.gif"
		if s.randFloat() >= 0.5 {
			catName = "黒猫"
			catIcon = "chr/031.gif"
		}
		if s.homeMembers != nil {
			err := s.homeMembers.AddMember(ctx, HomeMember{
				CharacterID: char.ID,
				IsNPC:       true,
				Name:        catName,
				Icon:        catIcon,
			})
			if err != nil {
				return err
			}
		}
		msg = fmt.Sprintf("自宅に可愛い%sがやってきました！", catName)

	case WishSecretMaid:
		wishName = "メイドを雇いたい"
		desc = "お世話係"
		if s.homeMembers != nil {
			members, err := s.homeMembers.FindByCharacterID(ctx, char.ID)
			if err == nil {
				for _, m := range members {
					if m.Name == "メイド" {
						return fmt.Errorf("%w: メイドはすでに雇用されています", ErrWishRequirement)
					}
				}
			}
			err = s.homeMembers.AddMember(ctx, HomeMember{
				CharacterID: char.ID,
				IsNPC:       true,
				Name:        "メイド",
				Icon:        "chr/026.gif",
			})
			if err != nil {
				return err
			}
		}
		msg = "お世話係のメイドが自宅に加わりました！"

	case WishEroticBook:
		wishName = "エッチな本がほしい"
		desc = "アイテム (預かり所へ送致)"
		if s.depots == nil {
			return ErrNilDependency
		}
		dep, err := s.depots.FindByCharacterIDForUpdate(ctx, char.ID)
		if errors.Is(err, depot.ErrNotFound) {
			dep, err = depot.NewDepotWithCapacity(char.ID, 0, 0, char.OverDepot)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		inst, err := coreitem.NewInstance("item-058", 1)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			return err
		}
		if err := s.depots.Save(ctx, dep); err != nil {
			return err
		}
		msg = "「エッチな本」を預かり所に送致しました！"

	case WishAlchemyRecipe:
		wishName = "錬金レシピがほしい"
		desc = "アイテム (預かり所へ送致)"
		if s.depots == nil {
			return ErrNilDependency
		}
		recipeID := "item-128"
		if s.randFloat() >= 0.5 {
			recipeID = "item-129"
		}
		dep, err := s.depots.FindByCharacterIDForUpdate(ctx, char.ID)
		if errors.Is(err, depot.ErrNotFound) {
			dep, err = depot.NewDepotWithCapacity(char.ID, 0, 0, char.OverDepot)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		inst, err := coreitem.NewInstance(recipeID, 1)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			return err
		}
		if err := s.depots.Save(ctx, dep); err != nil {
			return err
		}
		msg = "錬金レシピを預かり所に送致しました！"

	case WishLover:
		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          WishLover,
				Name:        "素敵な恋人がほしい",
				Realm:       RealmHeaven,
				Description: "恋人が…？",
				Available:   true,
			},
			Message:      "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…",
			NPCSpeech:    "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…",
			NextLocation: "",
		}
		return nil

	case WishLimitBreakLevel:
		if char.Level < 99 || char.OverLevel {
			return fmt.Errorf("%w: Lv99に到達している必要があります", ErrWishRequirement)
		}
		wishName = "もっと強くなりたい"
		desc = "Lv上限を上げる"
		char.OverLevel = true
		msg = "レベル上限が150へ限界突破しました！"

	case WishRestoreLevelLimit:
		if !char.OverLevel {
			return fmt.Errorf("%w: 限界突破中ではありません", ErrWishRequirement)
		}
		wishName = "もとの強さに戻りたい"
		desc = "Lv上限を元に戻す"
		char.OverLevel = false
		msg = "レベル上限を通常(99)に戻しました。"

	default:
		return ErrWishNotFound
	}

	if err := s.characters.Update(ctx, *char); err != nil {
		return err
	}

	*res = WishResult{
		Character: *char,
		Wish: Wish{
			ID:          wishID,
			Name:        wishName,
			Realm:       RealmHeaven,
			Description: desc,
			Available:   true,
		},
		Message:      msg,
		NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「%s」だな。<br />%sの願いを叶えたぞ…。機会があればまたあえるだろう…。さらばだ…", char.Name, wishName, char.Name),
		NextLocation: "home",
	}
	return nil
}

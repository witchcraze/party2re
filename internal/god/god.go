package god

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/depot"
)

var (
	ErrNilDependency      = errors.New("god dependency is nil")
	ErrCharacterNotFound  = errors.New("character not found")
	ErrInvalidCharacterID = errors.New("invalid character ID")
	ErrInvalidWishID      = errors.New("invalid wish ID")
	ErrInvalidRealm       = errors.New("invalid realm, must be 'heaven' or 'underworld'")
	ErrWishNotFound       = errors.New("wish not found in catalog")
	ErrWishRequirement    = errors.New("character does not meet requirements for this wish")
	ErrLimitBreakMaxed    = errors.New("limit break is already at maximum tier")
)

type Realm string

const (
	RealmHeaven     Realm = "heaven"
	RealmUnderworld Realm = "underworld"
)

const (
	MaxLimitBreakTier = 5
)

// Wish represents a wish available in Heaven or Underworld.
type Wish struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Realm       Realm  `json:"realm"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	CurrentTier int    `json:"current_tier,omitempty"`
	MaxTier     int    `json:"max_tier,omitempty"`
	Note        string `json:"note,omitempty"`
}

// WishResult represents the outcome of having a wish granted.
type WishResult struct {
	Character corecharacter.Character `json:"character"`
	Wish      Wish                    `json:"wish"`
	Message   string                  `json:"message"`
	NPCSpeech string                  `json:"npc_speech"`
}

// CharacterRepository defines character persistence methods required by the god service.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// DepotRepository defines depot persistence methods for capacity adjustments.
type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

// InventoryRepository defines inventory persistence methods.
type InventoryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inv coreinventory.Inventory) error
}

// TransactionProvider executes operations within a database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

// WithDepotRepository configures the optional depot repository.
func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depots = repo
	}
}

// WithInventoryRepository configures the optional inventory repository.
func WithInventoryRepository(repo InventoryRepository) Option {
	return func(s *Service) {
		s.inventories = repo
	}
}

// WithTransactionProvider configures the transaction provider.
func WithTransactionProvider(provider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = provider
	}
}

// Service manages endgame wishes and limit break progression.
type Service struct {
	characters  CharacterRepository
	depots      DepotRepository
	inventories InventoryRepository
	txProvider  TransactionProvider
}

// NewService creates a new Service instance.
func NewService(characters CharacterRepository, opts ...Option) (*Service, error) {
	if characters == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		characters: characters,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// GetWishes returns the list of available wishes for the character in the specified realm.
func (s *Service) GetWishes(ctx context.Context, characterID string, realm Realm) ([]Wish, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return nil, err
	}

	switch realm {
	case RealmHeaven:
		return s.buildHeavenWishes(char), nil
	case RealmUnderworld:
		return s.buildUnderworldWishes(char), nil
	default:
		return nil, ErrInvalidRealm
	}
}

// GrantWish executes the specified wish for the character in the given realm.
func (s *Service) GrantWish(ctx context.Context, characterID, wishID string, realm Realm) (WishResult, error) {
	characterID = strings.TrimSpace(characterID)
	wishID = strings.TrimSpace(wishID)
	if characterID == "" {
		return WishResult{}, ErrInvalidCharacterID
	}
	if wishID == "" {
		return WishResult{}, ErrInvalidWishID
	}

	var res WishResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		switch realm {
		case RealmHeaven:
			return s.executeHeavenWish(txCtx, &char, wishID, &res)
		case RealmUnderworld:
			return s.executeUnderworldWish(txCtx, &char, wishID, &res)
		default:
			return ErrInvalidRealm
		}
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return WishResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return WishResult{}, err
		}
	}

	return res, nil
}

// GetDialogue returns NPC greeting speech for the given realm.
func (s *Service) GetDialogue(realm Realm) []string {
	switch realm {
	case RealmHeaven:
		return []string{
			"神「よくぞここまで来た。そなたの願いを一つだけ叶えてやろう」",
			"神「強き心を持つ者よ。己が望む未来を選ぶがよい」",
		}
	case RealmUnderworld:
		return []string{
			"神?「よくぞ裏天界まで辿り着いたな…。そなたの限界を押し広げてやろう」",
			"神?「器を広げ、さらなる高みを目指すのだ…」",
		}
	default:
		return nil
	}
}

// -------------------------------------------------------------------
// Internal wish builders & execution
// -------------------------------------------------------------------

func (s *Service) buildHeavenWishes(char corecharacter.Character) []Wish {
	wishes := []Wish{
		{
			ID:          "wish_stats",
			Name:        "強くなりたい",
			Realm:       RealmHeaven,
			Description: "全ステータス 40 アップ",
			Available:   !char.OverLevel,
			Note:        "限界突破中は選択できません",
		},
		{
			ID:          "wish_money",
			Name:        "お金がほしい",
			Realm:       RealmHeaven,
			Description: "100,000 G 獲得",
			Available:   true,
		},
		{
			ID:          "wish_small_medals",
			Name:        "小さなメダルがほしい",
			Realm:       RealmHeaven,
			Description: "ちいさなメダル 20 枚獲得",
			Available:   true,
		},
		{
			ID:          "wish_full_recovery",
			Name:        "元気いっぱいになりたい",
			Realm:       RealmHeaven,
			Description: "HP・MP完全回復",
			Available:   true,
		},
		{
			ID:          "wish_lover",
			Name:        "素敵な恋人がほしい",
			Realm:       RealmHeaven,
			Description: "恋人が…？",
			Available:   true,
		},
		{
			ID:          "wish_secret_maid",
			Name:        "メイドを雇いたい",
			Realm:       RealmHeaven,
			Description: "お世話係のメイド",
			Available:   true,
		},
	}

	if char.Level >= 99 && !char.OverLevel {
		wishes = append(wishes, Wish{
			ID:          "wish_limit_break_level",
			Name:        "もっと強くなりたい",
			Realm:       RealmHeaven,
			Description: "Lv上限を150へ限界突破",
			Available:   true,
		})
	}

	if char.OverLevel {
		wishes = append(wishes, Wish{
			ID:          "wish_restore_level_limit",
			Name:        "もとの強さに戻りたい",
			Realm:       RealmHeaven,
			Description: "Lv上限を通常(99)に戻す",
			Available:   true,
		})
	}

	return wishes
}

func (s *Service) buildUnderworldWishes(char corecharacter.Character) []Wish {
	return []Wish{
		{
			ID:          "wish_expand_depot",
			Name:        "もっとアイテムを預けたい",
			Realm:       RealmUnderworld,
			Description: "預かり所の上限アップ (+10枠)",
			Available:   char.OverDepot < MaxLimitBreakTier,
			CurrentTier: char.OverDepot,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          "wish_expand_monster",
			Name:        "もっとモンスターを預けたい",
			Realm:       RealmUnderworld,
			Description: "モンスター預入上限アップ (+50枠)",
			Available:   char.OverMonster < MaxLimitBreakTier,
			CurrentTier: char.OverMonster,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          "wish_expand_job_memory",
			Name:        "もっと職業を覚えたい",
			Realm:       RealmUnderworld,
			Description: "未来のカケラの記憶上限アップ (+1枠)",
			Available:   char.OverFuture < MaxLimitBreakTier,
			CurrentTier: char.OverFuture,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          "wish_expand_flea_market",
			Name:        "もっとフリーマーケットで出品したい",
			Realm:       RealmUnderworld,
			Description: "フリーマーケット出品数上限アップ (+1枠)",
			Available:   char.OverFlea < MaxLimitBreakTier,
			CurrentTier: char.OverFlea,
			MaxTier:     MaxLimitBreakTier,
		},
		{
			ID:          "wish_expand_shop_store",
			Name:        "もっとお店で出品したい",
			Realm:       RealmUnderworld,
			Description: "お店の出品数上限アップ (+1枠)",
			Available:   char.OverStore < MaxLimitBreakTier,
			CurrentTier: char.OverStore,
			MaxTier:     MaxLimitBreakTier,
		},
	}
}

func (s *Service) executeHeavenWish(
	ctx context.Context,
	char *corecharacter.Character,
	wishID string,
	res *WishResult,
) error {
	switch wishID {
	case "wish_stats":
		if char.OverLevel {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上そなたを強くできんのだ…", ErrWishRequirement)
		}
		char.Stats.MaxHP += 40
		char.Stats.HP += 40
		char.Stats.MaxMP += 40
		char.Stats.MP += 40
		char.Stats.Attack += 40
		char.Stats.Defense += 40
		char.Stats.Agility += 40

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "強くなりたい",
				Realm:       RealmHeaven,
				Description: "全ステータス 40 アップ",
				Available:   true,
			},
			Message:   "全ステータスが 40 上昇しました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「強くなりたい」だな。\n願いを叶えたぞ…。機会があればまた会えるだろう…。さらばだ…", char.Name),
		}
		return nil

	case "wish_money":
		_ = char.AddMoney(100000)
		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "お金がほしい",
				Realm:       RealmHeaven,
				Description: "100,000 G 獲得",
				Available:   true,
			},
			Message:   "100,000 G を獲得しました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「お金がほしい」だな。\n願いを叶えたぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_small_medals":
		_ = char.AddSmallMedals(20)
		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "小さなメダルがほしい",
				Realm:       RealmHeaven,
				Description: "ちいさなメダル 20 枚獲得",
				Available:   true,
			},
			Message:   "ちいさなメダルを 20 枚獲得しました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「小さなメダルがほしい」だな。\n願いを叶えたぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_full_recovery":
		char.Stats.HP = char.Stats.MaxHP
		char.Stats.MP = char.Stats.MaxMP
		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "元気いっぱいになりたい",
				Realm:       RealmHeaven,
				Description: "HP・MP完全回復",
				Available:   true,
			},
			Message:   "HPとMPが完全に回復しました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「元気いっぱいになりたい」だな。\n願いを叶えたぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_limit_break_level":
		if char.Level < 99 || char.OverLevel {
			return fmt.Errorf("%w: Lv99に到達している必要があります", ErrWishRequirement)
		}
		char.OverLevel = true
		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっと強くなりたい",
				Realm:       RealmHeaven,
				Description: "Lv上限を150へ限界突破",
				Available:   true,
			},
			Message:   "レベル上限が150へ限界突破しました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっと強くなりたい」だな。\nLv上限を引き上げたぞ…。さらなる高みを目指すがよい…", char.Name),
		}
		return nil

	case "wish_restore_level_limit":
		if !char.OverLevel {
			return fmt.Errorf("%w: 限界突破中ではありません", ErrWishRequirement)
		}
		char.OverLevel = false
		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もとの強さに戻りたい",
				Realm:       RealmHeaven,
				Description: "Lv上限を通常(99)に戻す",
				Available:   true,
			},
			Message:   "レベル上限を通常(99)に戻しました。",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もとの強さに戻りたい」だな。\nLv上限を通常に戻したぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_lover":
		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "素敵な恋人がほしい",
				Realm:       RealmHeaven,
				Description: "恋人が…？",
				Available:   true,
			},
			Message:   "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…",
			NPCSpeech: "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…",
		}
		return nil

	case "wish_secret_maid":
		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "メイドを雇いたい",
				Realm:       RealmHeaven,
				Description: "お世話係のメイド",
				Available:   true,
			},
			Message:   "お世話係のメイドが仲間に加わりました！",
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「メイドを雇いたい」だな。\n願いを叶えたぞ…。さらばだ…", char.Name),
		}
		return nil

	default:
		return ErrWishNotFound
	}
}

func (s *Service) executeUnderworldWish(
	ctx context.Context,
	char *corecharacter.Character,
	wishID string,
	res *WishResult,
) error {
	switch wishID {
	case "wish_expand_depot":
		if char.OverDepot >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverDepot++

		if s.depots != nil {
			dep, err := s.depots.FindByCharacterIDForUpdate(ctx, char.ID)
			if err == nil {
				dep.Capacity = depot.DefaultDepotCapacity + (char.OverDepot * 10)
				_ = s.depots.Save(ctx, dep)
			}
		}

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとアイテムを預けたい",
				Realm:       RealmUnderworld,
				Description: "預かり所の上限アップ (+10枠)",
				Available:   char.OverDepot < MaxLimitBreakTier,
				CurrentTier: char.OverDepot,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:   fmt.Sprintf("預かり所の預入上限が +10 拡張されました！ (段階: %d/5)", char.OverDepot),
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっとアイテムを預けたい」だな。\n上限を広げてやったぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_expand_monster":
		if char.OverMonster >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverMonster++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとモンスターを預けたい",
				Realm:       RealmUnderworld,
				Description: "モンスター預入上限アップ (+50枠)",
				Available:   char.OverMonster < MaxLimitBreakTier,
				CurrentTier: char.OverMonster,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:   fmt.Sprintf("モンスター預入上限が +50 拡張されました！ (段階: %d/5)", char.OverMonster),
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっとモンスターを預けたい」だな。\n上限を広げてやったぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_expand_job_memory":
		if char.OverFuture >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverFuture++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっと職業を覚えたい",
				Realm:       RealmUnderworld,
				Description: "未来のカケラの記憶上限アップ (+1枠)",
				Available:   char.OverFuture < MaxLimitBreakTier,
				CurrentTier: char.OverFuture,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:   fmt.Sprintf("職業記憶上限が +1 拡張されました！ (段階: %d/5)", char.OverFuture),
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっと職業を覚えたい」だな。\n上限を広げてやったぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_expand_flea_market":
		if char.OverFlea >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverFlea++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとフリーマーケットで出品したい",
				Realm:       RealmUnderworld,
				Description: "フリーマーケット出品数上限アップ (+1枠)",
				Available:   char.OverFlea < MaxLimitBreakTier,
				CurrentTier: char.OverFlea,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:   fmt.Sprintf("フリーマーケット出品枠が +1 拡張されました！ (段階: %d/5, 最大 %d 出品)", char.OverFlea, 5+char.OverFlea),
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっとフリーマーケットで出品したい」だな。\n上限を広げてやったぞ…。さらばだ…", char.Name),
		}
		return nil

	case "wish_expand_shop_store":
		if char.OverStore >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverStore++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとお店で出品したい",
				Realm:       RealmUnderworld,
				Description: "お店の出品数上限アップ (+1枠)",
				Available:   char.OverStore < MaxLimitBreakTier,
				CurrentTier: char.OverStore,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:   fmt.Sprintf("店舗出品枠が +1 拡張されました！ (段階: %d/5)", char.OverStore),
			NPCSpeech: fmt.Sprintf("ふむ。%sの願いは「もっとお店で出品したい」だな。\n上限を広げてやったぞ…。さらばだ…", char.Name),
		}
		return nil

	default:
		return ErrWishNotFound
	}
}

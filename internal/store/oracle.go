package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/witchcraze/party2re/internal/core/random"
)

const (
	OracleLocationName    = "オラクル屋"
	OracleNPCName         = "@ラクル"
	OracleBgImg           = "bgimg/goods.gif"
	OracleInspectMessage  = "@ラクル「おっ？なんじゃなんじゃ？わしゃ何も知らんよ」"
	OracleBlackMarketHint = "＠やみいちば に行きたい"
	MinBlackMarketJobLv   = 15
)

var (
	ErrCostumeNotAvailable      = errors.New("costume is not available for your job level")
	ErrBlackMarketNotDiscovered = errors.New("you have not discovered the black market")
)

var OracleWords = []string{
	"おっ！久しぶりのお客だ！よく来たよく来た！ここはオラクル屋だよ",
	"＠かべがみでそなたの家をオシャレにすることができるよん",
	"壁紙は買ってしばらくしたら、こちらで勝手に張り替えておくよん。すぐ確認したい人は自分の家で更新ボタンを押すといいよ",
	"衣装は着た時からその日限りのレンタルだよ、次の日には返してもらうよ",
	"衣装は転職するときにも返してもらうよ",
}

type OracleItem struct {
	ItemNo   int    `json:"item_no"`
	Name     string `json:"name"`
	Price    int    `json:"price"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
}

type ActiveCostume struct {
	CharacterID string    `json:"character_id"`
	ItemNo      int       `json:"item_no"`
	ItemName    string    `json:"item_name"`
	Icon        string    `json:"icon"`
	RentedAt    time.Time `json:"rented_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (c ActiveCostume) IsActive(now time.Time) bool {
	return now.Before(c.ExpiresAt)
}

type OracleStatus struct {
	CharacterID          string          `json:"character_id"`
	LocationName         string          `json:"location_name"`
	NPCName              string          `json:"npc_name"`
	BgImg                string          `json:"bgimg"`
	AvailableCostumes    []OracleItem    `json:"available_costumes"`
	AvailableWallpapers  []WallpaperInfo `json:"available_wallpapers"`
	ActiveCostume        *ActiveCostume  `json:"active_costume,omitempty"`
	CanUnlockBlackMarket bool            `json:"can_unlock_black_market"`
}

type WallpaperInfo struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

type OracleInspectResult struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type CostumeRentalResult struct {
	ActiveCostume ActiveCostume `json:"active_costume"`
	Message       string        `json:"message"`
}

type HomeWallpaperResult struct {
	Wallpaper string `json:"wallpaper"`
	Message   string `json:"message"`
}

type CostumeRepository interface {
	GetActiveCostume(ctx context.Context, characterID string) (*ActiveCostume, error)
	SaveActiveCostume(ctx context.Context, costume ActiveCostume, ttl time.Duration) error
	ClearActiveCostume(ctx context.Context, characterID string) error
}

type HomeWallpaperRepository interface {
	UpdateHomeWallpaper(ctx context.Context, characterID string, wallpaper string) error
}

type CostumeResetter interface {
	ResetCostume(ctx context.Context, characterID string) error
}

// MemoryCostumeRepository provides thread-safe in-memory costume storage.
type MemoryCostumeRepository struct {
	mu       sync.RWMutex
	costumes map[string]ActiveCostume
}

func NewMemoryCostumeRepository() *MemoryCostumeRepository {
	return &MemoryCostumeRepository{
		costumes: make(map[string]ActiveCostume),
	}
}

func (r *MemoryCostumeRepository) GetActiveCostume(_ context.Context, characterID string) (*ActiveCostume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.costumes[characterID]
	if !ok {
		return nil, nil
	}
	if !c.IsActive(time.Now().UTC()) {
		return nil, nil
	}
	res := c
	return &res, nil
}

func (r *MemoryCostumeRepository) SaveActiveCostume(_ context.Context, costume ActiveCostume, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.costumes[costume.CharacterID] = costume
	return nil
}

func (r *MemoryCostumeRepository) ClearActiveCostume(_ context.Context, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.costumes, characterID)
	return nil
}

// rawOracleItems maps legacy item No to name, base price, male icon, and female icon.
var rawOracleItems = map[int]struct {
	name       string
	price      int
	maleIcon   string
	femaleIcon string
}{
	44:  {name: "ピンクスカート", price: 300, maleIcon: "chr/001.gif", femaleIcon: "chr/001.gif"},
	45:  {name: "タンクトップハンマー", price: 300, maleIcon: "chr/005.gif", femaleIcon: "chr/005.gif"},
	46:  {name: "チョビヒゲタクシード", price: 300, maleIcon: "chr/012.gif", femaleIcon: "chr/007.gif"},
	47:  {name: "ネコミミメイド", price: 300, maleIcon: "chr/004.gif", femaleIcon: "chr/004.gif"},
	48:  {name: "ホビット", price: 300, maleIcon: "chr/014.gif", femaleIcon: "chr/014.gif"},
	49:  {name: "ハナメガネ", price: 300, maleIcon: "chr/021.gif", femaleIcon: "chr/021.gif"},
	50:  {name: "ぬいぐるみ", price: 300, maleIcon: "chr/023.gif", femaleIcon: "chr/023.gif"},
	51:  {name: "冒険家の衣装", price: 300, maleIcon: "chr/003.gif", femaleIcon: "chr/003.gif"},
	52:  {name: "騎士団の衣装", price: 300, maleIcon: "chr/015.gif", femaleIcon: "chr/015.gif"},
	53:  {name: "老人の衣装", price: 300, maleIcon: "chr/013.gif", femaleIcon: "chr/013.gif"},
	54:  {name: "精霊の衣装", price: 300, maleIcon: "chr/011.gif", femaleIcon: "chr/011.gif"},
	55:  {name: "聖職者の衣装", price: 400, maleIcon: "chr/016.gif", femaleIcon: "chr/017.gif"},
	56:  {name: "王族の衣装", price: 500, maleIcon: "chr/002.gif", femaleIcon: "chr/018.gif"},
	138: {name: "タクシード", price: 500, maleIcon: "chr/027.gif", femaleIcon: "chr/027.gif"},
	139: {name: "闇人の衣装", price: 600, maleIcon: "chr/025.gif", femaleIcon: "chr/025.gif"},
	140: {name: "英雄の衣装", price: 700, maleIcon: "chr/034.gif", femaleIcon: "chr/028.gif"},
	141: {name: "モシャスの巻物", price: 1500, maleIcon: "chr/001.gif", femaleIcon: "chr/001.gif"},
}

// AvailableCostumes returns the list of available costume items according to legacy goods.cgi gating rules:
// - Base pool: items 44..56 if job_lv > 10, else items 44..(45 + job_lv)
// - Extended pool: push items 138..141 if job_lv > 15
func AvailableCostumes(jobLevel int) []OracleItem {
	if jobLevel < 0 {
		jobLevel = 0
	}

	var itemNos []int
	maxBase := 45 + jobLevel
	if jobLevel > 10 {
		maxBase = 56
	}
	if maxBase > 56 {
		maxBase = 56
	}

	for i := 44; i <= maxBase; i++ {
		itemNos = append(itemNos, i)
	}

	if jobLevel > 15 {
		itemNos = append(itemNos, 138, 139, 140, 141)
	}

	var results []OracleItem
	for _, no := range itemNos {
		if raw, ok := rawOracleItems[no]; ok {
			results = append(results, OracleItem{
				ItemNo:   no,
				Name:     raw.name,
				Price:    raw.price,
				Icon:     raw.maleIcon,
				Category: "costume",
			})
		}
	}
	return results
}

// CostumeIcon returns the authentic costume icon path for a given item No and gender.
func CostumeIcon(itemNo int, gender string) string {
	raw, ok := rawOracleItems[itemNo]
	if !ok {
		return "chr/001.gif"
	}
	g := strings.ToLower(strings.TrimSpace(gender))
	if strings.HasPrefix(g, "f") {
		return raw.femaleIcon
	}
	return raw.maleIcon
}

// OracleTalk returns a random dialogue from legacy @words.
func OracleTalk() string {
	idx := random.IntN(len(OracleWords))
	return OracleWords[idx]
}

// OracleInspect returns legacy NPC inspection dialogue and Black Market hint if eligible (job_lv >= 15).
func OracleInspect(jobLevel int) OracleInspectResult {
	res := OracleInspectResult{
		Message: OracleInspectMessage,
	}
	if jobLevel >= MinBlackMarketJobLv {
		res.Hint = OracleBlackMarketHint
	}
	return res
}

// DiscoverBlackMarket checks if character jobLevel is eligible to discover Black Market.
func DiscoverBlackMarket(jobLevel int) error {
	if jobLevel < MinBlackMarketJobLv {
		return ErrBlackMarketNotDiscovered
	}
	return nil
}

// OracleTalk forwards to OracleTalk.
func (s *Service) OracleTalk() string {
	return OracleTalk()
}

// OracleInspect forwards to OracleInspect.
func (s *Service) OracleInspect(jobLevel int) OracleInspectResult {
	return OracleInspect(jobLevel)
}

// DiscoverBlackMarket forwards to DiscoverBlackMarket.
func (s *Service) DiscoverBlackMarket(jobLevel int) error {
	return DiscoverBlackMarket(jobLevel)
}

// GetOracleStatus retrieves the current status, catalog, wallpapers, and active costume.
func (s *Service) GetOracleStatus(ctx context.Context, characterID string) (*OracleStatus, error) {
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, err
	}

	costumes := AvailableCostumes(char.JobLevel)

	// Collect wallpapers sorted by price
	var wallpapers []WallpaperInfo
	for name, price := range WallpaperPrices {
		wallpapers = append(wallpapers, WallpaperInfo{
			Name:  name,
			Price: price,
		})
	}
	slices.SortFunc(wallpapers, func(a, b WallpaperInfo) int {
		if a.Price != b.Price {
			return a.Price - b.Price
		}
		return strings.Compare(a.Name, b.Name)
	})

	var activeCostume *ActiveCostume
	if s.costumeRepo != nil {
		active, err := s.costumeRepo.GetActiveCostume(ctx, characterID)
		if err == nil && active != nil && active.IsActive(s.nowFunc().UTC()) {
			activeCostume = active
		}
	}

	return &OracleStatus{
		CharacterID:          char.ID,
		LocationName:         OracleLocationName,
		NPCName:              OracleNPCName,
		BgImg:                OracleBgImg,
		AvailableCostumes:    costumes,
		AvailableWallpapers:  wallpapers,
		ActiveCostume:        activeCostume,
		CanUnlockBlackMarket: char.JobLevel >= MinBlackMarketJobLv,
	}, nil
}

// RentCostume rents a costume from available sales pool for the day.
func (s *Service) RentCostume(ctx context.Context, characterID string, itemNo int) (*CostumeRentalResult, error) {
	now := s.nowFunc().UTC()

	var result *CostumeRentalResult
	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		available := AvailableCostumes(char.JobLevel)
		var selected *OracleItem
		for _, it := range available {
			if it.ItemNo == itemNo {
				selected = &it
				break
			}
		}
		if selected == nil {
			return ErrCostumeNotAvailable
		}

		if char.Money < selected.Price {
			return ErrInsufficientFunds
		}

		if err := char.DeductMoney(selected.Price); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Save(txCtx, char); err != nil {
			return err
		}

		// Calculate expiration until next midnight JST
		jst := time.FixedZone("JST", 9*3600)
		nowJST := now.In(jst)
		midnightJST := time.Date(nowJST.Year(), nowJST.Month(), nowJST.Day()+1, 0, 0, 0, 0, jst)
		expiresAt := midnightJST.UTC()
		ttl := expiresAt.Sub(now)
		if ttl <= 0 {
			ttl = 24 * time.Hour
			expiresAt = now.Add(ttl)
		}

		icon := CostumeIcon(selected.ItemNo, char.Gender)
		activeCostume := ActiveCostume{
			CharacterID: characterID,
			ItemNo:      selected.ItemNo,
			ItemName:    selected.Name,
			Icon:        icon,
			RentedAt:    now,
			ExpiresAt:   expiresAt,
		}

		if s.costumeRepo != nil {
			if err := s.costumeRepo.SaveActiveCostume(txCtx, activeCostume, ttl); err != nil {
				return err
			}
		}

		result = &CostumeRentalResult{
			ActiveCostume: activeCostume,
			Message:       fmt.Sprintf("%sの衣装をレンタルしたよん。次の日には返してもらうよ", selected.Name),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReturnCostume returns the rented costume, clearing active rental state.
func (s *Service) ReturnCostume(ctx context.Context, characterID string) error {
	if s.costumeRepo == nil {
		return nil
	}
	return s.costumeRepo.ClearActiveCostume(ctx, characterID)
}

// ResetCostume implements CostumeResetter, resetting costume on rest or job change.
func (s *Service) ResetCostume(ctx context.Context, characterID string) error {
	return s.ReturnCostume(ctx, characterID)
}

// BuyHomeWallpaper purchases a wallpaper for the character's private home.
func (s *Service) BuyHomeWallpaper(ctx context.Context, characterID string, wallpaper string) (*HomeWallpaperResult, error) {
	cleanWallpaper := strings.TrimSpace(wallpaper)
	cleanWallpaper = strings.TrimSuffix(cleanWallpaper, ".gif")

	price, ok := WallpaperPrices[cleanWallpaper]
	if !ok {
		return nil, ErrInvalidWallpaper
	}

	var result *HomeWallpaperResult
	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.Money < price {
			return ErrInsufficientFunds
		}

		if price > 0 {
			if err := char.DeductMoney(price); err != nil {
				return ErrInsufficientFunds
			}
			if err := s.charRepo.Save(txCtx, char); err != nil {
				return err
			}
		}

		savedWallpaper := cleanWallpaper + ".gif"
		if s.homeWallpaperRepo != nil {
			if err := s.homeWallpaperRepo.UpdateHomeWallpaper(txCtx, characterID, savedWallpaper); err != nil {
				return err
			}
		}

		result = &HomeWallpaperResult{
			Wallpaper: savedWallpaper,
			Message:   fmt.Sprintf("%sの家の壁紙を %s に、張り替えておいたよん", char.Name, cleanWallpaper),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

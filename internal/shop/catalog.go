package shop

import (
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/core/item"
)

type ShopType string

const (
	ShopTypeWeapon ShopType = "weapon"
	ShopTypeArmor  ShopType = "armor"
	ShopTypeItem   ShopType = "item"
)

const SecretShopHint = "＠ひみつのみせ に行きたい"

var (
	ErrInvalidShopType       = errors.New("invalid shop type")
	ErrItemUnavailable       = errors.New("item is currently unavailable (active helper quest)")
	ErrEmptyPurchaseList     = errors.New("purchase list cannot be empty")
	ErrDepotFull             = errors.New("depot is full")
	ErrDepotNotConfigured    = errors.New("depot repository is not configured")
	ErrSecretShopRequirement = errors.New("insufficient job level to access secret shop")
)

type CatalogItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BasePrice   int       `json:"base_price"`
	RetailPrice int       `json:"retail_price"`
	Slot        item.Slot `json:"slot,omitempty"`
}

type ShopCatalog struct {
	ShopType ShopType      `json:"shop_type"`
	Title    string        `json:"title"`
	NPCName  string        `json:"npc_name"`
	Items    []CatalogItem `json:"items"`
}

type NPCInspectResult struct {
	ShopType       ShopType `json:"shop_type"`
	NPCName        string   `json:"npc_name"`
	Dialogue       string   `json:"dialogue"`
	SecretShopHint string   `json:"secret_shop_hint,omitempty"`
}

func ValidateShopType(st ShopType) bool {
	switch st {
	case ShopTypeWeapon, ShopTypeArmor, ShopTypeItem:
		return true
	default:
		return false
	}
}

// GetSalesItemIDs returns the legacy catalog item IDs for a given shop type and job level.
// Corresponds 1:1 to party2/lib/weapon.cgi:18, armor.cgi:17, and item.cgi:17-22.
func GetSalesItemIDs(shopType ShopType, jobLv int) ([]string, error) {
	switch shopType {
	case ShopTypeWeapon:
		return getWeaponSalesItemIDs(jobLv), nil
	case ShopTypeArmor:
		return getArmorSalesItemIDs(jobLv), nil
	case ShopTypeItem:
		return getItemSalesItemIDs(jobLv), nil
	default:
		return nil, ErrInvalidShopType
	}
}

func getWeaponSalesItemIDs(jobLv int) []string {
	if jobLv > 11 {
		return []string{
			"weapon-01", "weapon-02", "weapon-03", "weapon-04", "weapon-05", "weapon-43",
			"weapon-06", "weapon-07", "weapon-08", "weapon-09", "weapon-10",
			"weapon-11", "weapon-12", "weapon-13", "weapon-14", "weapon-15", "weapon-16",
		}
	}
	if jobLv < 0 {
		jobLv = 0
	}
	return []string{
		"weapon-01", "weapon-02", "weapon-03", "weapon-04", "weapon-05", "weapon-43",
		fmt.Sprintf("weapon-%02d", 6+jobLv),
	}
}

func getArmorSalesItemIDs(jobLv int) []string {
	maxIdx := 5 + jobLv
	if jobLv > 11 || maxIdx > 16 {
		maxIdx = 16
	}
	if maxIdx < 1 {
		maxIdx = 1
	}
	res := make([]string, 0, maxIdx)
	for i := 1; i <= maxIdx; i++ {
		res = append(res, fmt.Sprintf("armor-%02d", i))
	}
	return res
}

func getItemSalesItemIDs(jobLv int) []string {
	var ids []int
	switch {
	case jobLv >= 7:
		ids = []int{1, 2, 3, 7, 8, 9, 11, 76, 14, 79, 102, 41, 42, 101, 127}
	case jobLv >= 5:
		ids = []int{1, 2, 3, 7, 8, 9, 11, 76, 14, 79, 41, 42, 101, 127}
	case jobLv >= 3:
		ids = []int{1, 2, 7, 8, 9, 11, 14, 79, 41, 42, 127}
	case jobLv >= 1:
		ids = []int{1, 2, 7, 8, 9, 11, 14, 79, 127}
	default:
		ids = []int{1, 7, 8, 9, 127}
	}
	res := make([]string, len(ids))
	for i, id := range ids {
		res[i] = fmt.Sprintf("item-%03d", id)
	}
	return res
}

func GetShopMeta(shopType ShopType) (title string, npcName string) {
	switch shopType {
	case ShopTypeWeapon:
		return "武器屋", "@ブッキー"
	case ShopTypeArmor:
		return "防具屋", "@アマノ"
	case ShopTypeItem:
		return "道具屋", "@アイテムコ"
	default:
		return "", ""
	}
}

func GetShopWords(shopType ShopType) []string {
	switch shopType {
	case ShopTypeWeapon:
		return []string{
			"いらっしゃい！ここは武器屋だぜ！",
			"強い武器ほど重いぜ！重いと素早さが下がるから注意しな！",
			"武器の強さ＝攻撃力だぜ！",
			"まとめて買ってくれるなら、あんたの預かり所に送ってやるぜ！",
		}
	case ShopTypeArmor:
		return []string{
			"いらっしゃいッス！ここは防具屋ッス！",
			"防御力が高ければ、敵から受けるダメージが減るッスよ！",
			"防具も重いと素早さが下がるッスから注意ッス！",
			"まとめて買ってくれるなら、預かり所に送っておくッスよ！",
		}
	case ShopTypeItem:
		return []string{
			"いらしゃいませぇ〜ここは道具屋ニャ",
			"冒険に出る前に道具があると便利ニャ",
			"道具は戦闘中に使うニャ！",
			"この世界のどこかに秘密の店というあやしいお店があるらしいですよぉ",
		}
	default:
		return nil
	}
}

func GetInspectDialogue(shopType ShopType) (dialogue string, hint string) {
	switch shopType {
	case ShopTypeWeapon:
		return "おいおい、俺は武器じゃねぇぜ", ""
	case ShopTypeArmor:
		return "な、な、何を見ているッスか！？！", ""
	case ShopTypeItem:
		return "ほえ？なんでしょうかぁ？", SecretShopHint
	default:
		return "", ""
	}
}

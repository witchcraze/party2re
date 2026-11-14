package http

import (
	"context"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
)

type ShopQuantityBounds struct {
	Minimum int `json:"minimum"`
	Maximum int `json:"maximum"`
}

type ShopPurchaseParams struct {
	ItemDefinitionID string `json:"item_definition_id"`
}

type ShopProduct struct {
	shop.CatalogItem
	SelectParams   playercontext.Subject `json:"select_params"`
	PurchaseParams ShopPurchaseParams    `json:"purchase_params"`
}

// ShopNPCObservation keeps NPC facts; dialogue comes from npc_talk/npc_inspect.
type ShopNPCObservation struct {
	shop.NPCInspectResult
}

type ShopSceneInfo struct {
	Parent   string             `json:"parent"`
	Title    string             `json:"title"`
	ShopType shop.ShopType      `json:"shop_type"`
	NPC      ShopNPCObservation `json:"npc"`
	Quantity ShopQuantityBounds `json:"quantity"`
	// SaleOffer is the item in this shop's hand; null when that hand is empty.
	SaleOffer *shop.SaleOffer `json:"sale_offer"`
	// Recipes are the accessory shop's synthesis recipes.
	Recipes []shop.SynthesisRecipe `json:"recipes,omitempty"`
}

type ShopCatalogSceneData struct {
	ShopSceneInfo
	Items []ShopProduct `json:"items"`
	Page  ScenePage     `json:"page"`
}

type ShopProductSceneData struct {
	ShopSceneInfo
	Product ShopProduct `json:"product"`
}

func (h *Handler) shopSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.shops == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	kind := shop.ShopType(strings.TrimPrefix(n.Destination, "shop_"))
	catalog, err := h.shops.GetCatalog(ctx, kind, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	inspect, err := h.shops.InspectNPC(ctx, kind, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	offer, err := h.shops.GetSaleOffer(ctx, kind, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	info := ShopSceneInfo{Parent: "town", Title: catalog.Title, ShopType: catalog.ShopType,
		NPC:      ShopNPCObservation{NPCInspectResult: inspect},
		Quantity: ShopQuantityBounds{Minimum: 1, Maximum: shop.MaxTransactionQuantity}, SaleOffer: offer}
	if kind == shop.ShopTypeAccessory {
		info.Recipes = shop.AllSynthesisRecipes()
	}
	products := make([]ShopProduct, 0, len(catalog.Items))
	for _, p := range catalog.Items {
		product := ShopProduct{CatalogItem: p, SelectParams: playercontext.Subject{Kind: "item", ID: p.ID}, PurchaseParams: ShopPurchaseParams{ItemDefinitionID: p.ID}}
		if n.Subject != (playercontext.Subject{}) && p.ID == n.Subject.ID {
			return ShopProductSceneData{ShopSceneInfo: info, Product: product}, nil
		}
		products = append(products, product)
	}
	if n.Subject != (playercontext.Subject{}) {
		// A catalog may change between the selector read and this projection.
		return nil, playercontext.ErrSelectionNotFound
	}
	slices.SortFunc(products, func(a, b ShopProduct) int { return strings.Compare(a.ID, b.ID) })
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start := min(n.Offset, len(products))
	end := min(start+limit, len(products))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(products) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	return ShopCatalogSceneData{ShopSceneInfo: info, Items: append([]ShopProduct{}, products[start:end]...), Page: page}, nil
}

// shopSaleOffer offers ＠うる only for the observed sale candidate.
func shopSaleOffer(info ShopSceneInfo) scenePresentation {
	presentation := scenePresentation{title: &info.Title}
	if info.SaleOffer == nil {
		presentation.hide = []string{"shop_sell"}
	} else {
		presentation.templates = map[string]map[string]any{"shop_sell": {"item_instance_id": info.SaleOffer.ItemInstanceID}}
	}
	return presentation
}

// shop.cgi files' @words and shiraberu_npc; an empty inspect uses the default.
var shopNPCWords = map[shop.ShopType]struct {
	words   []string
	inspect string
}{
	shop.ShopTypeWeapon: {[]string{
		"いらっしゃい！ここは武器屋だぜ！",
		"強い武器ほど重いぜ！重いと素早さが下がるから注意しな！",
		"武器の強さ＝攻撃力だぜ！",
		"まとめて買ってくれるなら、あんたの預かり所に送ってやるぜ！",
	}, "おいおい、俺は武器じゃねぇぜ"},
	shop.ShopTypeArmor: {[]string{
		"いらっしゃいッス！ここは防具屋ッス！",
		"防御力が高ければ、敵から受けるダメージが減るッスよ！",
		"防具も重いと素早さが下がるッスから注意ッス！",
		"まとめて買ってくれるなら、預かり所に送っておくッスよ！",
	}, "な、な、何を見ているッスか！？"},
	shop.ShopTypeItem: {[]string{
		"いらしゃいませぇ〜ここは道具屋ニャ",
		"冒険に出る前に道具があると便利ニャ",
		"道具は戦闘中に使うニャ！",
		"この世界のどこかに秘密の店というあやしいお店があるらしいですよぉ",
	}, "ほえ？なんでしょうかぁ？"},
	shop.ShopTypeAccessory: {[]string{
		"ここはアクセサリー屋。ここでしか手に入らないアイテムばっかりよ",
		"おすすめのアクセサリーを買うと良いと思うわ！",
		"独占禁止法？　ぼったくり？　細かいことはいいの！",
	}, ""},
}

var shopTitles = map[shop.ShopType]string{shop.ShopTypeWeapon: "武器店", shop.ShopTypeArmor: "防具店", shop.ShopTypeItem: "道具店", shop.ShopTypeAccessory: "装飾品店"}

func shopFacility(kind shop.ShopType) facility {
	commands := []string{"shop_purchase", "shop_batch_purchase", "shop_sell"}
	if kind == shop.ShopTypeAccessory {
		commands = []string{"shop_purchase", "shop_sell", "shop_synthesize"}
	}
	_, name := shop.GetShopMeta(kind)
	npc := &npcProfile{name: name, words: shopNPCWords[kind].words}
	if inspect := shopNPCWords[kind].inspect; inspect != "" {
		npc.inspect = name + "「" + inspect + "」"
	}
	bind := func(n playercontext.Selection, _ string) map[string]map[string]any {
		bound := make(map[string]map[string]any)
		for _, id := range []string{"shop_purchase", "shop_batch_purchase", "shop_sell"} {
			bound[id] = map[string]any{"shop_type": string(kind)}
		}
		if n.Subject != (playercontext.Subject{}) {
			bound["shop_purchase"]["item_definition_id"] = n.Subject.ID
		}
		return bound
	}
	return facility{title: shopTitles[kind], commands: commands, bind: bind, read: (*Handler).shopSceneData, placeLog: true, npc: npc,
		present: func(_ *Handler, _ playercontext.Result, data any) scenePresentation {
			switch data := data.(type) {
			case ShopCatalogSceneData:
				presentation := shopSaleOffer(data.ShopSceneInfo)
				presentation.next = data.Page.Next
				return presentation
			case ShopProductSceneData:
				return shopSaleOffer(data.ShopSceneInfo)
			}
			return scenePresentation{}
		}}
}

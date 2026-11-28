package http

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
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
	// Quantity bounds one shop_batch_purchase line.
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

// shopInspects are the shop.cgi files' shiraberu_npc; an empty one uses the default.
var shopInspects = map[shop.ShopType]string{
	shop.ShopTypeWeapon: "おいおい、俺は武器じゃねぇぜ",
	shop.ShopTypeArmor:  "な、な、何を見ているッスか！？",
	shop.ShopTypeItem:   "ほえ？なんでしょうかぁ？",
}

// shopWords builds a shop's legacy @words. Lines naming a random product pick
// from the actor's whole sales tier, as legacy $sales[int(rand(@sales))] does.
func shopWords(kind shop.ShopType) func(*Handler, context.Context, corecharacter.Character) ([]string, error) {
	return func(_ *Handler, _ context.Context, c corecharacter.Character) ([]string, error) {
		ids, err := shop.GetSalesItemIDs(kind, c.JobLevel)
		if err != nil {
			return nil, err
		}
		catalog, err := coreitem.DefaultCatalog()
		if err != nil {
			return nil, err
		}
		def, err := catalog.FindByID(ids[random.Intn(len(ids))])
		if err != nil {
			return nil, err
		}
		title, _ := shop.GetShopMeta(kind)
		n, pick := c.Name, def.Name
		switch kind {
		case shop.ShopTypeWeapon:
			return []string{
				"ここは武器屋だ！戦いに武器は必須だぜ！",
				n + "には" + pick + "なんか良いんじゃねぇか？",
				n + "には" + pick + "がオススメだ！",
				"銅の剣はどぉの剣？",
				"よぉ！何か買っていくのか？",
				"素早さが高いと会心の一撃や回避率が上がるぞ！",
				"攻撃力が高い分、重さも重くなり素早さが下がる。つまり、自分に合った装備をしろってことだ！",
				"この世界のどこかに、自分の強さにより武器の強さも変わる武器があるらしいぜ！",
				"モンスターにやられたとしてもお金が半分になることはないぜ！",
				fmt.Sprintf("%sの攻撃力は%dか…。Lv.%dにしてはなかなかだな！", n, c.Stats.Attack, c.Level),
				fmt.Sprintf("%sの転職回数は%d回か！転職回数が多ければ熟練者と見なし、もっと強い武器を売ってやるぜ！", n, c.JobLevel),
			}, nil
		case shop.ShopTypeArmor:
			return []string{
				"ここは防具屋ッス！防具を装備すればダメージを減らすことができるッス！",
				"素早さがないと攻撃をかわすことができないッス！",
				"素早さに自信がない場合は、ステテコパンツがオススメッス！",
				"強さや重さは１回の戦闘ごとで変わるッス！",
				n + "さんは" + pick + "なんて似合いそうッスね！",
				"重い鎧でガチガチに固めるか、ヒラヒラの服で回避率を上げるのか、どちらが好きッスか？",
				"いつかあっしもあぶない水着を着るのが夢ッス",
				fmt.Sprintf("%sさんの転職回数は%d回ッスね！転職回数が多いと熟練者と見なし売れる物が増えるッス！", n, c.JobLevel),
				fmt.Sprintf("%sさんの守備力は%dッスね！。なかなかの固さッスね！", n, c.Stats.Defense),
			}, nil
		case shop.ShopTypeItem:
			return []string{
				"いらしゃいませぇ〜ここは" + title + "ニャ",
				"冒険に出る前に道具があると便利ニャ",
				"初心者たんには薬草をオススメしてますぅ",
				"守りの石は相手からのだめぇじを軽減することができるらしいニャ",
				n + "たんは顔色が悪そうなのでぇ、毒消し草を食べるといいニャ",
				"この草おいしいニャ。モグモグ…。あぅっ！またお店の品を食べてしまったニャ…",
				"天使の鈴は頭が悪い人に使うといいらしぃニャ。あたしのことぢゃないニャ",
				n + "たんの今日の夕食は薬草料理がオススメニャ",
				n + "たんはべじたりあんですかぁ？",
				"魔法使いたんは魔法の聖水を持っていくとよいですよぉ",
				"この世界のどこかに秘密の店というあやしいお店があるらしいですよぉ",
				"今日のオススメ商品はコレニャ！じゃじゃ〜ん " + pick + " ニャ！",
			}, nil
		default:
			return []string{
				"ここは" + title + "。ここでしか手に入らないアイテムばっかりよ",
				n + "は " + pick + " を買うと良いと思うわ！",
				"独占禁止法？　ぼったくり？　細かいことはいいの！",
			}, nil
		}
	}
}

func shopFacility(kind shop.ShopType) facility {
	commands := []string{"shop_purchase", "shop_batch_purchase", "shop_sell"}
	if kind == shop.ShopTypeItem {
		commands = append(commands, "shop_secretshop")
	}
	if kind == shop.ShopTypeAccessory {
		commands = []string{"shop_purchase", "shop_sell", "shop_synthesize"}
	}
	title, name := shop.GetShopMeta(kind)
	npc := &npcProfile{name: name, wordsOf: shopWords(kind)}
	if inspect := shopInspects[kind]; inspect != "" {
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
	return facility{title: title, commands: commands, bind: bind, read: (*Handler).shopSceneData, placeLog: true, npc: npc,
		present: func(h *Handler, ctx context.Context, result playercontext.Result, data any) scenePresentation {
			var presentation scenePresentation
			switch data := data.(type) {
			case ShopCatalogSceneData:
				presentation = shopSaleOffer(data.ShopSceneInfo)
				presentation.next = data.Page.Next
			case ShopProductSceneData:
				presentation = shopSaleOffer(data.ShopSceneInfo)
			}
			// ＠ひみつのみせ answers only characters the secret shop admits (item.cgi himitsunomise).
			if kind == shop.ShopTypeItem && h.playerContext.Enterable(ctx, result, "secretshop") != nil {
				presentation.hide = append(presentation.hide, "shop_secretshop")
			}
			return presentation
		}}
}

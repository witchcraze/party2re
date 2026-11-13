package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/tavern"
)

// TavernService is the Tavern surface used by its scene and commands.
type TavernService interface {
	GetMenu() []tavern.MenuItem
	GetStatus(ctx context.Context, characterID string) (tavern.TavernStatus, error)
	OrderMeal(ctx context.Context, characterID string, itemID string) (tavern.OrderResult, error)
	ReserveDelivery(ctx context.Context, characterID string, itemID string) (tavern.DeliveryReservation, error)
	CancelDelivery(ctx context.Context, characterID string) error
}

type tavernItemRequest struct {
	ItemID string `json:"item_id"`
}

// TavernMenuEntry is a menu item; order_name is the name ＠ちゅうもん uses,
// which is the profile like_food for the full course (bar.cgi:86-94).
type TavernMenuEntry struct {
	tavern.MenuItem
	OrderName string `json:"order_name"`
}

// TavernSceneData is the actor's menu, fullness and standing delivery order.
type TavernSceneData struct {
	Parent   string                      `json:"parent"`
	Menu     []TavernMenuEntry           `json:"menu"`
	IsFull   bool                        `json:"is_full"`
	Delivery *tavern.DeliveryReservation `json:"delivery"`
}

// TavernOrderResult is a ＠ちゅうもん outcome. full reports the legacy refusal
// while already full, which changes nothing.
type TavernOrderResult struct {
	Full           bool   `json:"full"`
	ItemID         string `json:"item_id,omitempty"`
	ItemName       string `json:"item_name,omitempty"`
	HPHealed       int    `json:"hp_healed"`
	MPHealed       int    `json:"mp_healed"`
	CurrentHP      int    `json:"current_hp"`
	CurrentMP      int    `json:"current_mp"`
	RemainingGold  int    `json:"remaining_gold"`
	TicketsAwarded int    `json:"tickets_awarded"`
	TotalTickets   int    `json:"total_tickets"`
	Message        string `json:"message"`
}

// TavernDeliveryResult is a ＠でりばりー outcome; delivery is null after cancellation.
type TavernDeliveryResult struct {
	Delivery *tavern.DeliveryReservation `json:"delivery"`
	Message  string                      `json:"message"`
}

// WithTavern configures the tavern scene observation and its commands.
func WithTavern(service TavernService) Option {
	return func(h *Handler) {
		h.tavern = service
		if service == nil {
			withActionCommand[tavernItemRequest]("tavern_order", nil, nil)(h)
			withActionCommand[tavernItemRequest]("tavern_delivery_reserve", nil, nil)(h)
			withActionCommand[struct{}]("tavern_delivery_cancel", nil, nil)(h)
			return
		}
		withActionCommand("tavern_order", h.tavernOrder, tavernRejection)(h)
		withActionCommand("tavern_delivery_reserve", func(ctx context.Context, actor string, p tavernItemRequest) (any, error) {
			d, err := service.ReserveDelivery(ctx, actor, p.ItemID)
			if err != nil {
				return nil, err
			}
			return TavernDeliveryResult{Delivery: &d, Message: "冒険が終わったら" + d.ItemName + "が届くようにしたわ"}, nil
		}, tavernRejection)(h)
		withActionCommand("tavern_delivery_cancel", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			if err := service.CancelDelivery(ctx, actor); err != nil {
				return nil, err
			}
			return TavernDeliveryResult{Message: "でりばりーをキャンセルしたわ"}, nil
		}, tavernRejection)(h)
	}
}

// tavernOrder is ＠ちゅうもん; the reply follows bar.cgi chuumon. The profile is
// read first, so a read failure rejects before the meal commits.
func (h *Handler) tavernOrder(ctx context.Context, actor string, p tavernItemRequest) (any, error) {
	profile, err := h.characters.GetProfile(ctx, actor)
	if err != nil {
		return nil, err
	}
	res, err := h.tavern.OrderMeal(ctx, actor, p.ItemID)
	if errors.Is(err, tavern.ErrAlreadyFull) {
		return TavernOrderResult{Full: true, Message: "そんなに飲み食いするとお腹がこわれちゃうわよ？"}, nil
	}
	if err != nil {
		return nil, err
	}
	name := res.Item.Name
	for _, m := range h.tavernMenu(profile) {
		if m.ID == res.Item.ID {
			name = m.OrderName
		}
	}
	who := profile.Character.Name
	msg := "おまたせ、" + name + "よ♪"
	if res.Item.HPHeal > 0 {
		msg += who + "のＨＰが回復した！"
	}
	if res.Item.MPHeal > 0 {
		msg += who + "のＭＰが回復した！"
	}
	msg += fmt.Sprintf("福引券を%d枚もらった！", res.Item.Tickets)
	return TavernOrderResult{ItemID: res.Item.ID, ItemName: name, HPHealed: res.HPHealed, MPHealed: res.MPHealed,
		CurrentHP: res.CurrentHP, CurrentMP: res.CurrentMP, RemainingGold: res.RemainingGold,
		TicketsAwarded: res.TicketsAwarded, TotalTickets: res.TotalTickets, Message: msg}, nil
}

// tavernMenu renames the full course (the last entry) to the actor's like_food.
func (h *Handler) tavernMenu(profile character.ProfileView) []TavernMenuEntry {
	items := h.tavern.GetMenu()
	menu := make([]TavernMenuEntry, len(items))
	for i, item := range items {
		menu[i] = TavernMenuEntry{MenuItem: item, OrderName: item.Name}
	}
	if like := profile.Profile.BioData["like_food"]; like != "" && len(menu) > 0 {
		menu[len(menu)-1].OrderName = like
	}
	return menu
}

func (h *Handler) tavernSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.tavern == nil {
		return nil, errSceneNotConfigured
	}
	actor := r.Snapshot.Character.ID
	status, err := h.tavern.GetStatus(ctx, actor)
	if err != nil {
		return nil, err
	}
	profile, err := h.characters.GetProfile(ctx, actor)
	if err != nil {
		return nil, err
	}
	return TavernSceneData{Parent: "town", Menu: h.tavernMenu(profile), IsFull: status.IsFull, Delivery: status.DeliveryReservation}, nil
}

func tavernRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, tavern.ErrCharacterNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, tavern.ErrMenuItemNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "TAVERN_ITEM_NOT_FOUND", Message: "Choose one of the observed menu items."}
	case errors.Is(err, tavern.ErrInsufficientFunds):
		return http.StatusConflict, ErrorDetail{Code: "TAVERN_INSUFFICIENT_FUNDS", Message: "お金が足りないみたいね"}
	case errors.Is(err, tavern.ErrNoActiveDelivery):
		return http.StatusConflict, ErrorDetail{Code: "TAVERN_NO_DELIVERY", Message: "No delivery order to cancel."}
	default:
		return 0, ErrorDetail{}
	}
}

// The tavern keeps its clean-room names (docs/design/tavern.md); bar.cgi @words.
var tavernFacility = facility{
	title: "冒険者の酒場", commands: []string{"tavern_order", "tavern_delivery_reserve", "tavern_delivery_cancel"},
	read: (*Handler).tavernSceneData, placeLog: true,
	npc: &npcProfile{name: tavern.NPCName, wordsOf: tavernWords},
}

// tavernDishes are the catalog names; @words names a random dish.
var tavernDishes = func() []string {
	catalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		panic(err)
	}
	names := []string{}
	for _, item := range catalog.Items() {
		names = append(names, item.Name)
	}
	return names
}()

func tavernWords(*Handler, context.Context, corecharacter.Character) ([]string, error) {
	return []string{
		"いらっしゃい。何か食べてく？",
		"{name}さんは" + tavernDishes[random.Intn(len(tavernDishes))] + "は好きかしら？",
		"食材にＭＰを回復させる魔法の聖水やＨＰを回復させる薬草がふくまれているのよ",
		"ＨＰを回復させたいならデザートやご飯物を食べていくといいわ",
		"ＭＰを回復させたいならドリンクを飲んでいくといいわ",
		"食べたり飲んだりした後は、運動しなきゃね",
		"お酒は大人になってからね",
	}, nil
}

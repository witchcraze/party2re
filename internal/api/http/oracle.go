package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
)

// OracleShopService is the Oracle shop surface used by its scene and commands.
type OracleShopService interface {
	GetOracleStatus(ctx context.Context, characterID string) (*store.OracleStatus, error)
	BuyCostumeItem(ctx context.Context, characterID string, itemNo int) (*store.CostumeBuyResult, error)
	BuyHomeWallpaper(ctx context.Context, characterID string, wallpaper string) (*store.HomeWallpaperResult, error)
}

type oracleBuyRequest struct {
	ItemNo int `json:"item_no"`
}

type oracleWallpaperRequest struct {
	Wallpaper string `json:"wallpaper"`
}

// OracleSceneData is the actor's costume catalog, the wallpapers and the worn costume.
type OracleSceneData struct {
	Parent string `json:"parent"`
	store.OracleStatus
}

// OracleBuyResult is a ＠かう outcome.
type OracleBuyResult struct {
	store.CostumeBuyResult
	Message string `json:"message"`
}

// OracleWallpaperResult is a ＠かべがみ outcome.
type OracleWallpaperResult struct {
	Wallpaper string `json:"wallpaper"`
	Message   string `json:"message"`
}

// OracleBlackMarketResult is the ＠やみいちば move into the black market.
type OracleBlackMarketResult struct {
	Selection playercontext.Selection `json:"selection"`
	Message   string                  `json:"message"`
}

// oracleGoldShort carries the actor's name for goods.cgi's "$mよ…ゴールドが足らん".
type oracleGoldShort struct{ name string }

func (e oracleGoldShort) Error() string { return store.ErrInsufficientFunds.Error() }
func (e oracleGoldShort) Unwrap() error { return store.ErrInsufficientFunds }

// WithOracleShop configures the Oracle shop scene observation and its commands.
func WithOracleShop(service OracleShopService) Option {
	return func(h *Handler) {
		h.oracle = service
		if service == nil {
			withActionCommand[oracleBuyRequest]("oracle_buy", nil, nil)(h)
			withActionCommand[oracleWallpaperRequest]("oracle_wallpaper", nil, nil)(h)
			return
		}
		withActionCommand("oracle_buy", func(ctx context.Context, actor string, p oracleBuyRequest) (any, error) {
			name, err := h.actorName(ctx, actor)
			if err != nil {
				return nil, err
			}
			res, err := service.BuyCostumeItem(ctx, actor, p.ItemNo)
			if errors.Is(err, store.ErrInsufficientFunds) {
				return nil, oracleGoldShort{name}
			}
			if err != nil {
				return nil, err
			}
			msg := res.ItemName + "は" + name + "の預かり所に送っておいたよん"
			if res.DeliveredTo == string(depot.DeliveredToInventory) {
				msg = res.ItemName + "だな。ほい、どうぞ"
			}
			return OracleBuyResult{CostumeBuyResult: *res, Message: msg}, nil
		}, oracleRejection)(h)
		withActionCommand("oracle_wallpaper", func(ctx context.Context, actor string, p oracleWallpaperRequest) (any, error) {
			name, err := h.actorName(ctx, actor)
			if err != nil {
				return nil, err
			}
			res, err := service.BuyHomeWallpaper(ctx, actor, p.Wallpaper)
			if errors.Is(err, store.ErrInsufficientFunds) {
				return nil, oracleGoldShort{name}
			}
			if err != nil {
				return nil, err
			}
			return OracleWallpaperResult{Wallpaper: res.Wallpaper,
				Message: name + "の家の壁紙を " + strings.TrimSuffix(res.Wallpaper, ".gif") + " に、張り替えておいたよん"}, nil
		}, oracleRejection)(h)
	}
}

// actorName is read before a purchase so a failure rejects before it commits.
func (h *Handler) actorName(ctx context.Context, actor string) (string, error) {
	char, err := h.characters.Get(ctx, actor)
	if err != nil {
		return "", err
	}
	return char.Name, nil
}

// registerOracleBlackMarket connects ＠やみいちば: the hidden goods.cgi action
// that moves a qualified character into the black market.
func (h *Handler) registerOracleBlackMarket(service *playercontext.Service) {
	if service == nil || !service.NavigationConfigured() {
		return
	}
	navigationCommand(h, "oracle_blackmarket", func(ctx context.Context, owner, actor string, _ struct{}) (OracleBlackMarketResult, error) {
		selection, err := service.Enter(ctx, owner, actor, "blackmarket")
		if err != nil {
			return OracleBlackMarketResult{}, err
		}
		h.leavePlace(ctx, actor)
		return OracleBlackMarketResult{Selection: selection, Message: "闇市場を見つけました！"}, nil
	})
}

func (h *Handler) oracleSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.oracle == nil {
		return nil, errSceneNotConfigured
	}
	status, err := h.oracle.GetOracleStatus(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return OracleSceneData{Parent: "town", OracleStatus: *status}, nil
}

func oracleRejection(err error) (int, ErrorDetail) {
	var short oracleGoldShort
	switch {
	case errors.As(err, &short):
		return http.StatusConflict, ErrorDetail{Code: "ORACLE_INSUFFICIENT_GOLD", Message: short.name + "よ…ゴールドが足らん"}
	case errors.Is(err, store.ErrCostumeNotAvailable), errors.Is(err, store.ErrItemUnavailableInHelperQuest):
		return http.StatusBadRequest, ErrorDetail{Code: "ORACLE_COSTUME_UNAVAILABLE", Message: "Choose one of the observed costumes."}
	case errors.Is(err, store.ErrInvalidWallpaper):
		return http.StatusBadRequest, ErrorDetail{Code: "ORACLE_INVALID_WALLPAPER", Message: "Choose one of the observed wallpapers."}
	default:
		return 0, ErrorDetail{}
	}
}

// goods.cgi @words; inspection hints at the hidden ＠やみいちば.
var oracleFacility = facility{
	title: "オラクル屋", commands: []string{"oracle_buy", "oracle_wallpaper", "oracle_blackmarket"},
	read: (*Handler).oracleSceneData, placeLog: true,
	npc: &npcProfile{name: "@ラクル", words: []string{
		"おっ！久しぶりのお客だ！よく来たよく来た！ここはオラクル屋だよ",
		"＠かべがみでそなたの家をオシャレにすることができるよん",
		"壁紙は買ってしばらくしたら、こちらで勝手に張り替えておくよん。すぐ確認したい人は自分の家で更新ボタンを押すといいよ",
		"衣装は着た時からその日限りのレンタルだよ、次の日には返してもらうよ",
		"衣装は転職するときにも返してもらうよ",
	}, inspect: "@ラクル「おっ？なんじゃなんじゃ？わしゃ何も知らんよ」"},
}

package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// LotteryService is the raffle hall and takarakuji shop surface used by their scenes and commands.
type LotteryService interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	PlayRaffle(ctx context.Context, characterID string) (lottery.RaffleResult, int, corecharacter.Character, error)
	GetTakarakujiStatus(ctx context.Context) (lottery.TakarakujiStatus, error)
	BuyTakarakujiTicket(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error)
	GetCharacterTakarakujiTicket(ctx context.Context, characterID string) (*lottery.TakarakujiTicket, []lottery.TakarakujiTicket, error)
}

var jst = time.FixedZone("JST", 9*60*60)

// RafflePrizeRow is one ＠しょうひん row (lot.cgi shouhin).
type RafflePrizeRow struct {
	Rank      string `json:"rank"`
	Color     string `json:"color"`
	ColorName string `json:"color_name"`
	Name      string `json:"name"`
}

// LotterySceneData is the actor's coupons and the prize table of the raffle
// they would play: 300 or more coupons always play the special raffle.
type LotterySceneData struct {
	Parent  string           `json:"parent"`
	Tickets int              `json:"tickets"`
	Special bool             `json:"special"`
	Cost    int              `json:"cost"`
	Prizes  []RafflePrizeRow `json:"prizes"`
}

// TakarakujiSceneData is the current round and whether the actor holds its ticket.
type TakarakujiSceneData struct {
	Parent string `json:"parent"`
	lottery.TakarakujiStatus
	DrawDateLabel string `json:"draw_date_label"`
	Purchased     bool   `json:"purchased"`
}

// LotteryRaffleResult is a ＠ふくびき outcome.
type LotteryRaffleResult struct {
	lottery.RaffleResult
	RemainingTickets int    `json:"remaining_tickets"`
	Message          string `json:"message"`
}

// TakarakujiBuyResult is a ＠かう outcome. purchased is false for the legacy
// sold-out and one-per-person replies, which change nothing.
type TakarakujiBuyResult struct {
	Purchased     bool                      `json:"purchased"`
	Ticket        *lottery.TakarakujiTicket `json:"ticket"`
	RemainingGold int                       `json:"remaining_gold,omitempty"`
	Message       string                    `json:"message"`
}

// WithLottery configures the raffle hall and takarakuji shop.
func WithLottery(service LotteryService) Option {
	return func(h *Handler) {
		h.lottery = service
		if service == nil {
			withActionCommand[struct{}]("lottery_raffle", nil, nil)(h)
			withActionCommand[struct{}]("takarakuji_buy", nil, nil)(h)
			return
		}
		withActionCommand("lottery_raffle", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			res, remaining, char, err := service.PlayRaffle(ctx, actor)
			if err != nil {
				return nil, err
			}
			return LotteryRaffleResult{RaffleResult: res, RemainingTickets: remaining, Message: raffleMessage(res, remaining, char.Name)}, nil
		}, lotteryRejection)(h)
		withActionCommand("takarakuji_buy", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			res, err := service.BuyTakarakujiTicket(ctx, actor)
			switch {
			case errors.Is(err, lottery.ErrSoldOut):
				return TakarakujiBuyResult{Message: "今回の宝くじは完売したよー"}, nil
			case errors.Is(err, lottery.ErrAlreadyPurchased):
				return TakarakujiBuyResult{Message: "おひとりさまおひとつ！"}, nil
			case err != nil:
				return nil, err
			}
			return TakarakujiBuyResult{Purchased: true, Ticket: &res.Ticket, RemainingGold: res.RemainingGold,
				Message: "ありがとー。当たってたら " + drawDateLabel(res.DrawDate) + " に賞品が届くからね"}, nil
		}, lotteryRejection)(h)
	}
}

// drawDateLabel is the legacy unpadded JST year/month/day.
func drawDateLabel(t time.Time) string {
	d := t.In(jst)
	return fmt.Sprintf("%d/%d/%d", d.Year(), int(d.Month()), d.Day())
}

func raffleCost(tickets int) int {
	if tickets >= lottery.SpecialRaffleCost {
		return lottery.SpecialRaffleCost
	}
	return lottery.StandardRaffleCost
}

func (h *Handler) lotterySceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.lottery == nil {
		return nil, errSceneNotConfigured
	}
	tickets, err := h.lottery.GetRaffleTickets(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	special := tickets >= lottery.SpecialRaffleCost
	return LotterySceneData{Parent: "town", Tickets: tickets, Special: special, Cost: raffleCost(tickets),
		Prizes: rafflePrizeRows(special, int(time.Now().In(jst).Weekday()))}, nil
}

// rafflePrizeRows mirrors lot.cgi shouhin: the 4th tier's seeds share one row.
func rafflePrizeRows(special bool, wday int) []RafflePrizeRow {
	if special {
		return []RafflePrizeRow{{Rank: "１等", Color: "gold", Name: "オーブ"}}
	}
	ranks := []string{"特賞", "１等", "２等", "３等", "４等", "５等", "６等"}
	rows := []RafflePrizeRow{}
	for i, roll := range []int{0, 1, 4, 8, 14, 45, 55} {
		p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, roll, wday)
		name := p.Name
		if i == 4 {
			name = "種系"
		}
		rows = append(rows, RafflePrizeRow{Rank: ranks[i], Color: p.Color, ColorName: p.ColorName, Name: name})
	}
	return rows
}

func (h *Handler) takarakujiSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.lottery == nil {
		return nil, errSceneNotConfigured
	}
	// Reading the current round provisions it when absent (the legacy shop page did too).
	status, err := h.lottery.GetTakarakujiStatus(ctx)
	if err != nil {
		return nil, err
	}
	current, _, err := h.lottery.GetCharacterTakarakujiTicket(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return TakarakujiSceneData{Parent: "town", TakarakujiStatus: status, DrawDateLabel: drawDateLabel(status.DrawDate), Purchased: current != nil}, nil
}

// raffleMessage follows lot.cgi fukubiki.
func raffleMessage(res lottery.RaffleResult, remaining int, actorName string) string {
	special := res.RaffleType == lottery.RaffleSpecial
	head := "プニョプニョプニョ…コロコロコロ…...,,,● 【" + res.Prize.ColorName + "スライム】"
	if special {
		head = "ガラガラガラ…コロコロコロ…...,,,● 【" + res.Prize.ColorName + "オーブ】"
	}
	comment := raffleComment(special, res.Prize)
	if res.Prize.Tier == lottery.PrizeTierMiss {
		if count := remaining / res.TicketsUsed; count > 0 {
			return fmt.Sprintf("%s %sあと%d回まわせるよ", head, comment, count)
		}
		return head + " " + comment + "また挑戦してね"
	}
	if res.TransferredToDepot {
		return head + comment + res.Prize.Name + "は、" + actorName + "さんの預かり所に送っておきますね"
	}
	return head + comment + "はい。どうぞ！"
}

func raffleComment(special bool, p lottery.RafflePrize) string {
	if special {
		switch p.Tier {
		case lottery.PrizeTierGrand:
			return "！！！えっ！？あれっ！？なんだこれは？！え〜と…ハズレです！………な、なんですか！？…わかりましたよ。他の人には内緒ですよ。"
		case lottery.PrizeTierMiss:
			return "おおっ！ホワイトオーブが出ました〜！って…それはハズレです…"
		default:
			return "おおっ！" + p.Name + "が出ました〜！おめでとうございま〜す！こちらが" + p.Name + "になります！"
		}
	}
	switch p.Tier {
	case lottery.PrizeTierGrand:
		return "！！！えっ！？えっ！？お、大当たり〜！大当たり〜！あれ？出ないはずなのにな…ゴニョゴニョ。お、おめでとうございます！特賞です！特賞が出ました〜！どうぞ！こちらが特賞の" + p.Name + "です！お受け取りください！"
	case lottery.PrizeTier1st:
		return "！！おっ！大当たり〜！大当たり〜！おめでとうございます！１等が出ました〜！こちらが１等の" + p.Name + "です！"
	case lottery.PrizeTier2nd:
		return "！！おっ！大当たり〜！大当たり〜！おめでとうございます！２等が出ました〜！こちらが２等の" + p.Name + "です！"
	case lottery.PrizeTier3rd:
		return "大当たり〜！大当たり〜！おめでとうございます！３等が出ました〜！こちらが３等の" + p.Name + "です！"
	case lottery.PrizeTier4th:
		return "おおっ、当たりで〜す！おめでとうございま〜す！４等が出ました〜！こちらが４等の" + p.Name + "です！"
	case lottery.PrizeTier5th:
		return "当ったり〜！おめでとうございます！５等の" + p.Name + "です！"
	case lottery.PrizeTier6th:
		return "おめでとう。６等の" + p.Name + "で〜す！"
	default:
		return "ハズレです…"
	}
}

func lotteryRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, lottery.ErrInsufficientTickets):
		return http.StatusConflict, ErrorDetail{Code: "LOTTERY_INSUFFICIENT_TICKETS", Message: "福引券を 3 枚以上、持っていないようですねぇ…"}
	case errors.Is(err, lottery.ErrInsufficientGold):
		return http.StatusConflict, ErrorDetail{Code: "TAKARAKUJI_INSUFFICIENT_GOLD", Message: "お金が足りないよ"}
	case errors.Is(err, lottery.ErrAlreadyDrawn):
		return http.StatusConflict, ErrorDetail{Code: "TAKARAKUJI_ROUND_CLOSED", Message: "The round is being drawn."}
	default:
		return 0, ErrorDetail{}
	}
}

// lot.cgi and takarakuzi.cgi @words; the tavern keeps its clean-room name.
var (
	lotteryFacility = facility{title: "福引所", commands: []string{"lottery_raffle"}, read: (*Handler).lotterySceneData, placeLog: true,
		npc: &npcProfile{name: "@フクスケ", wordsOf: lotteryWords, inspect: "なんと、{name}は福引券を見つけた！\n@フクスケ「ダメだよ！あげないよ！」"}}
	takarakujiFacility = facility{title: "宝くじ屋", commands: []string{"takarakuji_buy"}, read: (*Handler).takarakujiSceneData, placeLog: true,
		npc: &npcProfile{name: lottery.TakarakujiNPCName, wordsOf: takarakujiWords}}
)

func lotteryWords(h *Handler, ctx context.Context, char corecharacter.Character) ([]string, error) {
	if h.lottery == nil {
		return nil, nil
	}
	tickets, err := h.lottery.GetRaffleTickets(ctx, char.ID)
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("福引券%d枚で１回まわすことができるよ", raffleCost(tickets)),
		"福引の賞品は「＠しょうひん」で確認してね",
		"特賞は曜日によって変わるよ",
		"福引券は冒険者の酒場で食事するともらえるよ",
		fmt.Sprintf("%d枚以上福引券を持っている人は、特別な福引ができるよ", lottery.SpecialRaffleCost),
	}, nil
}

func takarakujiWords(h *Handler, ctx context.Context, _ corecharacter.Character) ([]string, error) {
	if h.lottery == nil {
		return nil, nil
	}
	status, err := h.lottery.GetTakarakujiStatus(ctx)
	if err != nil {
		return nil, err
	}
	return []string{
		"宝くじの三要素！　夢！運！げんじつ！",
		"宝くじを当てたいなら、当たるまで買うといいよ！",
		"スライムは眼中にありません！",
		"大体10日くらいで賞品は変わるよ",
		fmt.Sprintf("あと %d 人分の宝くじがあるよ", status.RemainingTickets),
	}, nil
}

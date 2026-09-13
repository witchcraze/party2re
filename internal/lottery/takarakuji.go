package lottery

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

const (
	TakarakujiCostGold   = 30000
	TakarakujiMaxTickets = 20

	TakarakujiTitle   = "宝くじ屋"
	TakarakujiNPCName = "@クラゲ"
)

var (
	ErrSoldOut          = errors.New("今回の宝くじは完売したよー")
	ErrAlreadyPurchased = errors.New("おひとりさまおひとつ！")
	ErrRoundNotFound    = errors.New("takarakuji round not found")
	ErrAlreadyDrawn     = errors.New("takarakuji round already drawn")
	ErrNotReadyToDraw   = errors.New("takarakuji round not yet ready to draw")
)

// Legacy prize candidates from party2/lib/takarakuzi.cgi
var (
	IttoPrizeCandidates = []string{
		"item-129", // 神の錬金レシピ
		"item-266", // 奇跡の錬金レシピ
		"item-265", // 聖なる秘石
		"item-255", // 黄金の林檎
	}

	NitoPrizeCandidates = []string{
		"item-168",
		"item-264",
		"item-150",
		"item-173",
		"item-207",
		"armor-40",
		"weapon-40",
		"item-265",
	}

	SantoPrizeCandidates = []string{
		"item-126",
		"item-244",
		"item-243",
		"item-199",
		"item-253",
		"item-254",
		"item-263",
		"item-264",
	}

	TalkPhrases = []string{
		"宝くじの三要素！　夢！運！げんじつ！",
		"宝くじを当てたいなら、当たるまで買うといいよ！",
		"スライムは眼中にありません！",
		"大体10日くらいで賞品は変わるよ",
	}
)

type TakarakujiPrizeItem struct {
	Rank         int    `json:"rank"`
	ItemID       string `json:"item_id"`
	ItemName     string `json:"item_name"`
	WinnersCount int    `json:"winners_count"`
}

type TakarakujiRound struct {
	RoundID      int        `json:"round_id"`
	DrawDate     time.Time  `json:"draw_date"`
	IsDrawn      bool       `json:"is_drawn"`
	DrawnAt      *time.Time `json:"drawn_at,omitempty"`
	Prize1ItemID string     `json:"prize_1_item_id"`
	Prize1Amount int        `json:"prize_1_amount"`
	Prize2ItemID string     `json:"prize_2_item_id"`
	Prize2Amount int        `json:"prize_2_amount"`
	Prize3ItemID string     `json:"prize_3_item_id"`
	Prize3Amount int        `json:"prize_3_amount"`
	CreatedAt    time.Time  `json:"created_at"`
}

type TakarakujiTicket struct {
	ID          string    `json:"id"`
	RoundID     int       `json:"round_id"`
	CharacterID string    `json:"character_id"`
	PurchasedAt time.Time `json:"purchased_at"`
	WonRank     int       `json:"won_rank"`
	WonItemID   *string   `json:"won_item_id,omitempty"`
}

type TakarakujiStatus struct {
	RoundID          int                   `json:"round_id"`
	Title            string                `json:"title"`
	NPCName          string                `json:"npc_name"`
	TicketPrice      int                   `json:"ticket_price"`
	MaxTickets       int                   `json:"max_tickets"`
	SoldCount        int                   `json:"sold_count"`
	RemainingTickets int                   `json:"remaining_tickets"`
	IsSoldOut        bool                  `json:"is_sold_out"`
	DrawDate         time.Time             `json:"draw_date"`
	Prizes           []TakarakujiPrizeItem `json:"prizes"`
	TalkPhrases      []string              `json:"talk_phrases"`
}

type TakarakujiPurchaseResult struct {
	Ticket        TakarakujiTicket `json:"ticket"`
	RemainingGold int              `json:"remaining_gold"`
	NPCMessage    string           `json:"npc_message"`
}

type TakarakujiWinner struct {
	Rank        int    `json:"rank"`
	CharacterID string `json:"character_id"`
	ItemID      string `json:"item_id"`
	ItemName    string `json:"item_name"`
	IsDummy     bool   `json:"is_dummy"`
}

type TakarakujiDrawResult struct {
	RoundID   int                `json:"round_id"`
	DrawnAt   time.Time          `json:"drawn_at"`
	Winners   []TakarakujiWinner `json:"winners"`
	NextRound TakarakujiRound    `json:"next_round"`
}

// NextDrawDateJST calculates the next drawing date in Japan Standard Time (UTC+9).
// In authentic party2 (lib/takarakuzi.cgi), drawings occur on the 1st, 11th, and 21st of each month at 00:00:00 JST.
func NextDrawDateJST(t time.Time) time.Time {
	jst := time.FixedZone("JST", 9*60*60)
	inJST := t.In(jst)
	year, month, day := inJST.Date()

	if day >= 1 && day < 11 {
		return time.Date(year, month, 11, 0, 0, 0, 0, jst)
	} else if day >= 11 && day < 21 {
		return time.Date(year, month, 21, 0, 0, 0, 0, jst)
	} else {
		firstOfNextMonth := time.Date(year, month, 1, 0, 0, 0, 0, jst).AddDate(0, 1, 0)
		return time.Date(firstOfNextMonth.Year(), firstOfNextMonth.Month(), 1, 0, 0, 0, 0, jst)
	}
}

// RollNewRoundPrizes generates random prizes and quantities for a new round matching lib/takarakuzi.cgi.
func RollNewRoundPrizes() (prize1 string, amount1 int, prize2 string, amount2 int, prize3 string, amount3 int, err error) {
	idx1, err := rand.Int(rand.Reader, big.NewInt(int64(len(IttoPrizeCandidates))))
	if err != nil {
		return "", 0, "", 0, "", 0, err
	}
	prize1 = IttoPrizeCandidates[idx1.Int64()]
	amount1 = 1

	idx2, err := rand.Int(rand.Reader, big.NewInt(int64(len(NitoPrizeCandidates))))
	if err != nil {
		return "", 0, "", 0, "", 0, err
	}
	prize2 = NitoPrizeCandidates[idx2.Int64()]
	amt2, err := rand.Int(rand.Reader, big.NewInt(2))
	if err != nil {
		return "", 0, "", 0, "", 0, err
	}
	amount2 = int(amt2.Int64()) + 1

	idx3, err := rand.Int(rand.Reader, big.NewInt(int64(len(SantoPrizeCandidates))))
	if err != nil {
		return "", 0, "", 0, "", 0, err
	}
	prize3 = SantoPrizeCandidates[idx3.Int64()]
	amt3, err := rand.Int(rand.Reader, big.NewInt(3))
	if err != nil {
		return "", 0, "", 0, "", 0, err
	}
	amount3 = int(amt3.Int64()) + 2

	return prize1, amount1, prize2, amount2, prize3, amount3, nil
}

func (s *Service) resolveItemName(itemID string) string {
	if s.itemDefProvider != nil {
		def, err := s.itemDefProvider.FindByID(itemID)
		if err == nil && def.Name != "" {
			return def.Name
		}
	}
	return itemID
}

func (s *Service) getOrCreateActiveRound(ctx context.Context, now time.Time) (TakarakujiRound, error) {
	round, err := s.repo.GetActiveTakarakujiRound(ctx)
	if err == nil {
		return round, nil
	}
	if !errors.Is(err, ErrRoundNotFound) {
		return TakarakujiRound{}, err
	}

	p1, a1, p2, a2, p3, a3, err := RollNewRoundPrizes()
	if err != nil {
		return TakarakujiRound{}, err
	}

	drawDate := NextDrawDateJST(now)
	newRound := TakarakujiRound{
		DrawDate:     drawDate,
		IsDrawn:      false,
		Prize1ItemID: p1,
		Prize1Amount: a1,
		Prize2ItemID: p2,
		Prize2Amount: a2,
		Prize3ItemID: p3,
		Prize3Amount: a3,
		CreatedAt:    now,
	}

	return s.repo.CreateTakarakujiRound(ctx, newRound)
}

func (s *Service) GetTakarakujiStatus(ctx context.Context) (TakarakujiStatus, error) {
	now := s.now()
	round, err := s.getOrCreateActiveRound(ctx, now)
	if err != nil {
		return TakarakujiStatus{}, err
	}

	soldCount, err := s.repo.CountTakarakujiTickets(ctx, round.RoundID)
	if err != nil {
		return TakarakujiStatus{}, err
	}

	remaining := TakarakujiMaxTickets - soldCount
	if remaining < 0 {
		remaining = 0
	}

	prizes := []TakarakujiPrizeItem{
		{
			Rank:         1,
			ItemID:       round.Prize1ItemID,
			ItemName:     s.resolveItemName(round.Prize1ItemID),
			WinnersCount: round.Prize1Amount,
		},
		{
			Rank:         2,
			ItemID:       round.Prize2ItemID,
			ItemName:     s.resolveItemName(round.Prize2ItemID),
			WinnersCount: round.Prize2Amount,
		},
		{
			Rank:         3,
			ItemID:       round.Prize3ItemID,
			ItemName:     s.resolveItemName(round.Prize3ItemID),
			WinnersCount: round.Prize3Amount,
		},
	}

	phrases := make([]string, len(TalkPhrases)+1)
	copy(phrases, TalkPhrases)
	phrases[len(phrases)-1] = fmt.Sprintf("あと %d 人分の宝くじがあるよ", remaining)

	return TakarakujiStatus{
		RoundID:          round.RoundID,
		Title:            TakarakujiTitle,
		NPCName:          TakarakujiNPCName,
		TicketPrice:      TakarakujiCostGold,
		MaxTickets:       TakarakujiMaxTickets,
		SoldCount:        soldCount,
		RemainingTickets: remaining,
		IsSoldOut:        soldCount >= TakarakujiMaxTickets,
		DrawDate:         round.DrawDate,
		Prizes:           prizes,
		TalkPhrases:      phrases,
	}, nil
}

func (s *Service) BuyTakarakujiTicket(ctx context.Context, characterID string) (TakarakujiPurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return TakarakujiPurchaseResult{}, corecharacter.ErrNotFound
	}
	characterID = strings.TrimSpace(characterID)

	now := s.now()
	round, err := s.getOrCreateActiveRound(ctx, now)
	if err != nil {
		return TakarakujiPurchaseResult{}, err
	}

	ticket, updatedChar, err := s.repo.PurchaseTakarakujiTicket(ctx, round.RoundID, characterID, TakarakujiCostGold)
	if err != nil {
		return TakarakujiPurchaseResult{}, err
	}

	jst := time.FixedZone("JST", 9*60*60)
	drawDateJST := round.DrawDate.In(jst)
	npcMessage := fmt.Sprintf("ありがとー。当たってたら %04d/%02d/%02d に賞品が届くからね", drawDateJST.Year(), drawDateJST.Month(), drawDateJST.Day())

	return TakarakujiPurchaseResult{
		Ticket:        ticket,
		RemainingGold: updatedChar.Money,
		NPCMessage:    npcMessage,
	}, nil
}

func (s *Service) GetCharacterTakarakujiTicket(ctx context.Context, characterID string) (*TakarakujiTicket, []TakarakujiTicket, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, nil, corecharacter.ErrNotFound
	}
	characterID = strings.TrimSpace(characterID)

	now := s.now()
	round, err := s.getOrCreateActiveRound(ctx, now)
	if err != nil {
		return nil, nil, err
	}

	var currentTicket *TakarakujiTicket
	t, err := s.repo.GetCharacterTakarakujiTicket(ctx, round.RoundID, characterID)
	if err == nil {
		currentTicket = &t
	}

	history, err := s.repo.ListCharacterTakarakujiTickets(ctx, characterID)
	if err != nil {
		return nil, nil, err
	}

	return currentTicket, history, nil
}

func (s *Service) DrawTakarakuji(ctx context.Context, now time.Time) (TakarakujiDrawResult, error) {
	round, err := s.repo.GetActiveTakarakujiRound(ctx)
	if err != nil {
		return TakarakujiDrawResult{}, err
	}
	if round.IsDrawn {
		return TakarakujiDrawResult{}, ErrAlreadyDrawn
	}

	tickets, err := s.repo.ListRoundTakarakujiTickets(ctx, round.RoundID)
	if err != nil {
		return TakarakujiDrawResult{}, err
	}

	pool := make([]string, 0, TakarakujiMaxTickets)
	for _, t := range tickets {
		pool = append(pool, t.CharacterID)
	}
	for len(pool) < TakarakujiMaxTickets {
		pool = append(pool, fmt.Sprintf("<dummy_%d>", len(pool)))
	}

	type prizeSpec struct {
		rank   int
		itemID string
		amount int
	}
	prizes := []prizeSpec{
		{rank: 1, itemID: round.Prize1ItemID, amount: round.Prize1Amount},
		{rank: 2, itemID: round.Prize2ItemID, amount: round.Prize2Amount},
		{rank: 3, itemID: round.Prize3ItemID, amount: round.Prize3Amount},
	}

	var winners []TakarakujiWinner
	winningTicketsMap := make(map[string]TakarakujiTicket)

	for _, p := range prizes {
		for i := 0; i < p.amount && len(pool) > 0; i++ {
			idxBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(pool))))
			if err != nil {
				return TakarakujiDrawResult{}, err
			}
			idx := int(idxBig.Int64())
			winnerID := pool[idx]

			pool = append(pool[:idx], pool[idx+1:]...)

			isDummy := strings.HasPrefix(winnerID, "<dummy_")
			winner := TakarakujiWinner{
				Rank:        p.rank,
				CharacterID: winnerID,
				ItemID:      p.itemID,
				ItemName:    s.resolveItemName(p.itemID),
				IsDummy:     isDummy,
			}
			winners = append(winners, winner)

			if !isDummy {
				for _, t := range tickets {
					if t.CharacterID == winnerID {
						itemCopy := p.itemID
						t.WonRank = p.rank
						t.WonItemID = &itemCopy
						winningTicketsMap[t.ID] = t
						break
					}
				}

				_ = s.deliverPrizeToDepot(ctx, winnerID, p.itemID)
			}
		}
	}

	var winningTickets []TakarakujiTicket
	for _, t := range winningTicketsMap {
		winningTickets = append(winningTickets, t)
	}

	if err := s.repo.SettleTakarakujiRound(ctx, round.RoundID, now, winningTickets); err != nil {
		return TakarakujiDrawResult{}, err
	}

	p1, a1, p2, a2, p3, a3, err := RollNewRoundPrizes()
	if err != nil {
		return TakarakujiDrawResult{}, err
	}
	nextDrawDate := NextDrawDateJST(now)
	nextRound, err := s.repo.CreateTakarakujiRound(ctx, TakarakujiRound{
		DrawDate:     nextDrawDate,
		IsDrawn:      false,
		Prize1ItemID: p1,
		Prize1Amount: a1,
		Prize2ItemID: p2,
		Prize2Amount: a2,
		Prize3ItemID: p3,
		Prize3Amount: a3,
		CreatedAt:    now,
	})
	if err != nil {
		return TakarakujiDrawResult{}, err
	}

	return TakarakujiDrawResult{
		RoundID:   round.RoundID,
		DrawnAt:   now,
		Winners:   winners,
		NextRound: nextRound,
	}, nil
}

func (s *Service) deliverPrizeToDepot(ctx context.Context, characterID, itemID string) error {
	if s.depotRepo == nil {
		return nil
	}

	var char corecharacter.Character
	if s.charRepo != nil {
		c, err := s.charRepo.FindByID(ctx, characterID)
		if err == nil {
			char = c
		}
	}

	dep, err := s.depotRepo.FindByCharacterIDForUpdate(ctx, characterID)
	if errors.Is(err, depot.ErrNotFound) {
		dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	dep.RefreshCapacity(char.JobLevel, char.OverDepot)

	inst, err := coreitem.NewInstance(itemID, 1)
	if err != nil {
		return err
	}

	if err := dep.AddItem(inst); err != nil {
		return err
	}

	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return err
	}

	if s.collectionRecorder != nil {
		itemName := s.resolveItemName(itemID)
		_ = s.collectionRecorder.RecordItemDiscovered(ctx, characterID, itemID, itemName, "takarakuji")
	}

	return nil
}

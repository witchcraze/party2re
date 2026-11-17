package contest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// PrizeMedalItemID is the medal item delivered to winners and champion voters.
const PrizeMedalItemID = "item-023"

// Prize defines rewards awarded to top contest winners. Medals are the
// delivered 9/5/2 items, not the 10/6/3 prize table (photo-contest.md#prizes).
type Prize struct {
	Gold        int
	Medals      int
	GuildPoints int
}

var (
	PrizeFirst  = Prize{Gold: 15000, Medals: 9, GuildPoints: 700}
	PrizeSecond = Prize{Gold: 7000, Medals: 5, GuildPoints: 300}
	PrizeThird  = Prize{Gold: 3000, Medals: 2, GuildPoints: 100}
)

// PrizeDelivery persists awards inside the settlement transaction while the
// recipient's character lock is held.
type PrizeDelivery interface {
	// HoldGold holds gold for Depot collection; award names it, e.g. 第1回フォトコンテスト1位.
	HoldGold(ctx context.Context, characterID string, amount int, award string) error
	// DeliverMedals adds medal items to the Depot regardless of capacity; the
	// notified count of them each get their own delivery notice.
	DeliverMedals(ctx context.Context, recipient corecharacter.Character, quantity, notified int) error
}

// WithPrizeDelivery persists winner and champion-voter awards; settlement
// with awards fails without it.
func WithPrizeDelivery(d PrizeDelivery) Option {
	return func(s *Service) {
		s.prizes = d
	}
}

var errPrizeDeliveryUnconfigured = errors.New("contest prize delivery is not configured")

// awardPrizes delivers the placements of ranked entries and the champion-voter
// medals, returning how many voters were rewarded. Recipients are locked in ID
// order before any Depot (Rank 2 → 5), and guilds last in ID order (Rank 7).
func (s *Service) awardPrizes(ctx context.Context, round int, entries []ContestEntry, now time.Time) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	if s.prizes == nil {
		return 0, errPrizeDeliveryUnconfigured
	}
	prizes := []Prize{PrizeFirst, PrizeSecond, PrizeThird}
	placed := entries[:min(len(entries), len(prizes))]
	votes, err := s.contests.ListVotesByEntryID(ctx, entries[0].ID)
	if err != nil {
		return 0, err
	}

	rank := make(map[string]int, len(placed))
	voted := make(map[string]bool, len(votes))
	var ids []string
	for i, e := range placed {
		rank[e.CharacterID] = i + 1
		ids = append(ids, e.CharacterID)
	}
	for _, v := range votes {
		if !voted[v.VoterCharacterID] {
			voted[v.VoterCharacterID] = true
			ids = append(ids, v.VoterCharacterID)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	recipients := make(map[string]corecharacter.Character, len(ids))
	for _, id := range ids {
		char, err := s.characters.FindByIDForUpdate(ctx, id)
		if errors.Is(err, corecharacter.ErrNotFound) {
			continue // deleted after ranking; its rows cascade away
		}
		if err != nil {
			return 0, err
		}
		recipients[id] = char
	}

	votersRewarded := 0
	for _, id := range ids {
		char, ok := recipients[id]
		if !ok {
			continue
		}
		medals, notified := 0, 0
		if r, ok := rank[id]; ok {
			if err := s.prizes.HoldGold(ctx, id, prizes[r-1].Gold, fmt.Sprintf("第%d回フォトコンテスト%d位", round, r)); err != nil {
				return 0, err
			}
			medals = prizes[r-1].Medals
		}
		if voted[id] {
			medals, notified = medals+1, 1
			votersRewarded++
		}
		if err := s.prizes.DeliverMedals(ctx, char, medals, notified); err != nil {
			return 0, err
		}
	}

	guildPoints := map[string]int64{}
	for i, e := range placed {
		if _, ok := recipients[e.CharacterID]; ok && e.GuildID != "" {
			guildPoints[e.GuildID] += int64(prizes[i].GuildPoints)
		}
	}
	guildIDs := make([]string, 0, len(guildPoints))
	for guildID := range guildPoints {
		guildIDs = append(guildIDs, guildID)
	}
	slices.Sort(guildIDs)
	if s.guilds != nil {
		for _, guildID := range guildIDs {
			if err := s.guilds.AddPoints(ctx, guildID, guildPoints[guildID]); err != nil {
				return 0, err
			}
		}
	}

	for i, e := range placed {
		if _, ok := recipients[e.CharacterID]; !ok {
			continue
		}
		if err := s.memories.Record(ctx, e.CharacterID, corecharacter.MemoryContestPrize,
			fmt.Sprintf("★第%d回フォトコンテスト%d位入賞★", round, i+1)); err != nil {
			return 0, err
		}
		if s.news != nil {
			content := fmt.Sprintf("★第%d回フォトコンテスト%d位 %s★", round, i+1, e.CharacterName)
			//lint:ignore error-swallow best-effort informational news does not invalidate contest prizes
			_ = s.news.PublishNews(ctx, "contest", "フォトコンテスト結果発表", content, "ワコール", now)
		}
	}
	return votersRewarded, nil
}

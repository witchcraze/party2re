package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestService_CasinoWinsAndAlchemy(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	repo.casinoWinsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c-gambler", CharacterName: "Lucky", Score: 50},
	}
	repo.casinoWinsTotal = 1

	repo.alchemyRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c-alchemist", CharacterName: "Brewer", Score: 120},
	}
	repo.alchemyTotal = 1

	ctx := context.Background()

	// 1. Direct service methods
	casPage, err := svc.GetCasinoWinsRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetCasinoWinsRanking failed: %v", err)
	}
	if casPage.Total != 1 || casPage.Entries[0].CharacterID != "c-gambler" {
		t.Fatalf("unexpected casino wins ranking: %+v", casPage)
	}

	alcPage, err := svc.GetAlchemyRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetAlchemyRanking failed: %v", err)
	}
	if alcPage.Total != 1 || alcPage.Entries[0].CharacterID != "c-alchemist" {
		t.Fatalf("unexpected alchemy ranking: %+v", alcPage)
	}

	// 2. Dynamic GetRankingByType with canonical and legacy aliases
	dynCas, err := svc.GetRankingByType(ctx, ranking.RankingTypeCasinoWins, 10, 0, false)
	if err != nil {
		t.Fatalf("GetRankingByType(casino_wins) failed: %v", err)
	}
	if p, ok := dynCas.(ranking.RankingPage[ranking.CharacterRankingEntry]); !ok || p.Total != 1 {
		t.Fatalf("unexpected dynamic casino ranking: %+v", dynCas)
	}

	dynCasAlias, err := svc.GetRankingByType(ctx, "cas_c", 10, 0, false)
	if err != nil {
		t.Fatalf("GetRankingByType(cas_c) failed: %v", err)
	}
	if p, ok := dynCasAlias.(ranking.RankingPage[ranking.CharacterRankingEntry]); !ok || p.Total != 1 {
		t.Fatalf("unexpected dynamic cas_c ranking: %+v", dynCasAlias)
	}

	dynAlc, err := svc.GetRankingByType(ctx, ranking.RankingTypeAlchemy, 10, 0, false)
	if err != nil {
		t.Fatalf("GetRankingByType(alchemy) failed: %v", err)
	}
	if p, ok := dynAlc.(ranking.RankingPage[ranking.CharacterRankingEntry]); !ok || p.Total != 1 {
		t.Fatalf("unexpected dynamic alchemy ranking: %+v", dynAlc)
	}

	dynAlcAlias, err := svc.GetRankingByType(ctx, "alc_c", 10, 0, false)
	if err != nil {
		t.Fatalf("GetRankingByType(alc_c) failed: %v", err)
	}
	if p, ok := dynAlcAlias.(ranking.RankingPage[ranking.CharacterRankingEntry]); !ok || p.Total != 1 {
		t.Fatalf("unexpected dynamic alc_c ranking: %+v", dynAlcAlias)
	}

	// 3. Snapshot refreshes
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeCasinoWins); err != nil {
		t.Fatalf("RefreshSnapshot(casino_wins) failed: %v", err)
	}
	if err := svc.RefreshSnapshot(ctx, "cas_c"); err != nil {
		t.Fatalf("RefreshSnapshot(cas_c) failed: %v", err)
	}
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeAlchemy); err != nil {
		t.Fatalf("RefreshSnapshot(alchemy) failed: %v", err)
	}
	if err := svc.RefreshSnapshot(ctx, "alc_c"); err != nil {
		t.Fatalf("RefreshSnapshot(alc_c) failed: %v", err)
	}
}

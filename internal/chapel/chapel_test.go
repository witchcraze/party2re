package chapel_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
)

type mockChapelRepo struct {
	blessing chapel.CharacterBlessing
}

func (m *mockChapelRepo) GetBlessing(_ context.Context, charID string) (chapel.CharacterBlessing, error) {
	if m.blessing.CharacterID == "" {
		return chapel.CharacterBlessing{
			CharacterID:       charID,
			ActiveBlessing:    chapel.BlessingNone,
			DonationGoldTotal: 0,
			PrayedAt:          time.Now().UTC(),
		}, nil
	}
	return m.blessing, nil
}

func (m *mockChapelRepo) SelectBlessing(_ context.Context, charID string, b chapel.BlessingType) (chapel.CharacterBlessing, error) {
	m.blessing.CharacterID = charID
	m.blessing.ActiveBlessing = b
	m.blessing.PrayedAt = time.Now().UTC()
	return m.blessing, nil
}

func (m *mockChapelRepo) Donate(_ context.Context, charID string, gold int) (chapel.CharacterBlessing, error) {
	m.blessing.CharacterID = charID
	m.blessing.DonationGoldTotal += int64(gold)
	return m.blessing, nil
}

func TestComputeRewardModifiers(t *testing.T) {
	// 1. EXP Blessing with roll < 0.25 -> 1.5x EXP
	mods := chapel.ComputeRewardModifiers(chapel.BlessingExp, 0.10)
	if mods.ExpMultiplier != 1.5 || mods.GoldMultiplier != 1.0 {
		t.Errorf("Exp Blessing lucky roll: exp=%f, gold=%f", mods.ExpMultiplier, mods.GoldMultiplier)
	}

	// 2. EXP Blessing with roll >= 0.25 -> 1.0x EXP
	mods = chapel.ComputeRewardModifiers(chapel.BlessingExp, 0.50)
	if mods.ExpMultiplier != 1.0 {
		t.Errorf("Exp Blessing unlucky roll: exp=%f", mods.ExpMultiplier)
	}

	// 3. Gold Blessing with roll < 0.25 -> 1.5x Gold
	mods = chapel.ComputeRewardModifiers(chapel.BlessingGold, 0.20)
	if mods.GoldMultiplier != 1.5 || mods.ExpMultiplier != 1.0 {
		t.Errorf("Gold Blessing lucky roll: gold=%f, exp=%f", mods.GoldMultiplier, mods.ExpMultiplier)
	}

	// 4. Drop Blessing -> +10% drop bonus rate
	mods = chapel.ComputeRewardModifiers(chapel.BlessingDrop, 0.0)
	if mods.DropBonusRate != 0.10 {
		t.Errorf("Drop Blessing: rate=%f", mods.DropBonusRate)
	}
}

func TestChapelService(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Select Blessing
	b, err := svc.SelectBlessing(ctx, "char1", chapel.BlessingExp)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingExp {
		t.Errorf("active blessing = %v, want EXP", b.ActiveBlessing)
	}

	// 2. Invalid Blessing
	if _, err := svc.SelectBlessing(ctx, "char1", "INVALID"); err != chapel.ErrInvalidBlessing {
		t.Errorf("expected ErrInvalidBlessing, got %v", err)
	}

	// 3. Donate
	b, err = svc.Donate(ctx, "char1", 1000)
	if err != nil {
		t.Fatalf("Donate failed: %v", err)
	}
	if b.DonationGoldTotal != 1000 {
		t.Errorf("donation total = %d, want 1000", b.DonationGoldTotal)
	}
}

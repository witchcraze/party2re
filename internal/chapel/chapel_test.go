package chapel_test

import (
	"context"
	"errors"
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
			CharacterID:    charID,
			ActiveBlessing: chapel.BlessingNone,
			PrayedAt:       time.Now().UTC(),
		}, nil
	}
	return m.blessing, nil
}

func (m *mockChapelRepo) SelectBlessing(_ context.Context, charID string, b chapel.BlessingType) (chapel.CharacterBlessing, error) {
	if m.blessing.ActiveBlessing != chapel.BlessingNone && m.blessing.ActiveBlessing != "" {
		return chapel.CharacterBlessing{}, chapel.ErrAlreadyPrayed
	}
	m.blessing.CharacterID = charID
	m.blessing.ActiveBlessing = b
	m.blessing.PrayedAt = time.Now().UTC()
	return m.blessing, nil
}

func (m *mockChapelRepo) ClearBlessing(_ context.Context, _ string) error {
	m.blessing.ActiveBlessing = chapel.BlessingNone
	return nil
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

	// 5. Monster Blessing -> +50% monster recruit bonus rate
	mods = chapel.ComputeRewardModifiers(chapel.BlessingMonster, 0.0)
	if mods.MonsterRecruitBonusRate != 0.50 {
		t.Errorf("Monster Blessing: recruit rate=%f, want 0.50", mods.MonsterRecruitBonusRate)
	}
}

func TestParseBlessingType(t *testing.T) {
	cases := []struct {
		input   string
		want    chapel.BlessingType
		wantErr bool
	}{
		{"GOLD", chapel.BlessingGold, false},
		{"gold", chapel.BlessingGold, false},
		{"お金がほしい", chapel.BlessingGold, false},
		{"1", chapel.BlessingGold, false},
		{"EXP", chapel.BlessingExp, false},
		{"強くなりたい", chapel.BlessingExp, false},
		{"2", chapel.BlessingExp, false},
		{"MONSTER", chapel.BlessingMonster, false},
		{"モンスターと仲良くしたい", chapel.BlessingMonster, false},
		{"3", chapel.BlessingMonster, false},
		{"DROP", chapel.BlessingDrop, false},
		{"宝箱がほしい", chapel.BlessingDrop, false},
		{"4", chapel.BlessingDrop, false},
		{"CASINO", chapel.BlessingCasino, false},
		{"コインがほしい", chapel.BlessingCasino, false},
		{"5", chapel.BlessingCasino, false},
		{"NONE", chapel.BlessingNone, false},
		{"invalid", "", true},
		{"999", "", true},
	}

	for _, tc := range cases {
		got, err := chapel.ParseBlessingType(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("ParseBlessingType(%q) expected error, got nil", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ParseBlessingType(%q) unexpected error: %v", tc.input, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseBlessingType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestChapelService(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Initial State & Status
	status, err := svc.GetStatus(ctx, "char1", "勇者アルス")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.LocationName != "礼拝堂" || status.NPCName != "@シスター" || status.BackgroundImage != "bgimg/chapel.gif" {
		t.Errorf("unexpected status facility info: %+v", status)
	}
	if status.HasActiveBlessing {
		t.Errorf("expected no active blessing initially")
	}
	if len(status.AvailableBlessings) != 5 {
		t.Errorf("expected 5 available blessings, got %d", len(status.AvailableBlessings))
	}
	if len(status.Dialogues) != 3 || status.Dialogues[0] != "勇者アルスに神のご加護があらんことを" {
		t.Errorf("unexpected dialogues: %v", status.Dialogues)
	}

	// 2. Select Monster Blessing
	b, err := svc.SelectBlessing(ctx, "char1", chapel.BlessingMonster)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("active blessing = %v, want MONSTER", b.ActiveBlessing)
	}

	// 3. Single active wish constraint: second prayer must be rejected with ErrAlreadyPrayed
	_, err = svc.SelectBlessing(ctx, "char1", chapel.BlessingExp)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed, got %v", err)
	}

	// 4. Status reflects active blessing
	status, err = svc.GetStatus(ctx, "char1", "勇者アルス")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.HasActiveBlessing || status.ActiveBlessing != chapel.BlessingMonster || status.ActiveBlessingName != "モンスターと仲良くしたい" {
		t.Errorf("unexpected active blessing in status: %+v", status)
	}

	// 5. Clear blessing -> allows new prayer
	if err := svc.ClearBlessing(ctx, "char1"); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}
	b, err = svc.Pray(ctx, "char1", chapel.BlessingGold)
	if err != nil {
		t.Fatalf("Pray after clear failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingGold {
		t.Errorf("active blessing = %v, want GOLD", b.ActiveBlessing)
	}

	// 6. Validation errors
	if _, err := svc.SelectBlessing(ctx, "", chapel.BlessingExp); !errors.Is(err, chapel.ErrInvalidCharacterID) {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
	if _, err := svc.SelectBlessing(ctx, "char2", "INVALID"); !errors.Is(err, chapel.ErrInvalidBlessing) {
		t.Errorf("expected ErrInvalidBlessing, got %v", err)
	}
	if _, err := svc.SelectBlessing(ctx, "char2", chapel.BlessingNone); !errors.Is(err, chapel.ErrInvalidBlessing) {
		t.Errorf("expected ErrInvalidBlessing for BlessingNone, got %v", err)
	}
}

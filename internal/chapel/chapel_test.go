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
	nowFunc  func() time.Time
}

func (m *mockChapelRepo) now() time.Time {
	if m.nowFunc != nil {
		return m.nowFunc()
	}
	return time.Now()
}

func (m *mockChapelRepo) GetBlessing(_ context.Context, charID string) (chapel.CharacterBlessing, error) {
	if m.blessing.CharacterID == "" {
		return chapel.CharacterBlessing{
			CharacterID:    charID,
			ActiveBlessing: chapel.BlessingNone,
			PrayedAt:       time.Time{},
		}, nil
	}
	return m.blessing, nil
}

func (m *mockChapelRepo) SelectBlessing(_ context.Context, charID string, b chapel.BlessingType) (chapel.CharacterBlessing, error) {
	if (m.blessing.ActiveBlessing != chapel.BlessingNone && m.blessing.ActiveBlessing != "") || !m.blessing.PrayedAt.IsZero() {
		return chapel.CharacterBlessing{}, chapel.ErrAlreadyPrayed
	}
	m.blessing.CharacterID = charID
	m.blessing.ActiveBlessing = b
	m.blessing.PrayedAt = m.now().UTC()
	return m.blessing, nil
}

func (m *mockChapelRepo) ClearBlessing(_ context.Context, _ string) error {
	now := m.now().UTC()
	m.blessing.ActiveBlessing = chapel.BlessingNone
	if !m.blessing.PrayedAt.IsZero() && !chapel.IsSameJSTDay(m.blessing.PrayedAt, now) {
		m.blessing.PrayedAt = time.Time{}
		m.blessing.CharacterID = ""
	}
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

	// 4. Drop Blessing with roll < 0.20 -> +1 extra chest
	mods = chapel.ComputeRewardModifiers(chapel.BlessingDrop, 0.15)
	if mods.ExtraChestBonus != 1 {
		t.Errorf("Drop Blessing lucky roll: extra=%d, want 1", mods.ExtraChestBonus)
	}

	// 5. Drop Blessing with roll >= 0.20 -> 0 extra chests
	mods = chapel.ComputeRewardModifiers(chapel.BlessingDrop, 0.50)
	if mods.ExtraChestBonus != 0 {
		t.Errorf("Drop Blessing unlucky roll: extra=%d, want 0", mods.ExtraChestBonus)
	}

	// 6. Monster Blessing -> +0.25% flat monster recruit bonus rate (+0.5 out of 200)
	mods = chapel.ComputeRewardModifiers(chapel.BlessingMonster, 0.0)
	if mods.MonsterRecruitBonusRate != 0.0025 {
		t.Errorf("Monster Blessing: recruit rate=%f, want 0.0025", mods.MonsterRecruitBonusRate)
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

	// 5. Validation errors
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

func TestService_SameDaySleep_RejectsRePrayer(t *testing.T) {
	ctx := context.Background()
	currentTime := time.Date(2026, 10, 9, 10, 0, 0, 0, chapel.JST)
	nowFunc := func() time.Time { return currentTime }

	repo := &mockChapelRepo{nowFunc: nowFunc}
	svc, err := chapel.NewService(repo, chapel.WithNowFunc(nowFunc))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Pray at 10:00 JST
	b, err := svc.SelectBlessing(ctx, "char1", chapel.BlessingGold)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingGold {
		t.Fatalf("expected BlessingGold, got %v", b.ActiveBlessing)
	}

	// 2. Sleep at 14:00 JST (same day)
	currentTime = time.Date(2026, 10, 9, 14, 0, 0, 0, chapel.JST)
	if err := svc.ClearBlessing(ctx, "char1"); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}

	// Active blessing must be cleared (NONE), but prayer quota record remains
	status, err := svc.GetStatus(ctx, "char1", "アルス")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.HasActiveBlessing {
		t.Errorf("expected HasActiveBlessing = false after sleep, got true")
	}
	if status.PrayedAt == nil {
		t.Errorf("expected PrayedAt to be retained after same-day sleep")
	}

	// 3. Attempt re-prayer at 15:00 JST (same day) -> must be REJECTED
	currentTime = time.Date(2026, 10, 9, 15, 0, 0, 0, chapel.JST)
	_, err = svc.SelectBlessing(ctx, "char1", chapel.BlessingExp)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed on same-day re-prayer after sleep, got %v", err)
	}
}

func TestService_MidnightRolloverWithoutSleep_PreservesBlessingAndBlocksRePrayer(t *testing.T) {
	ctx := context.Background()
	// Day 1 20:00 JST
	currentTime := time.Date(2026, 10, 9, 20, 0, 0, 0, chapel.JST)
	nowFunc := func() time.Time { return currentTime }

	repo := &mockChapelRepo{nowFunc: nowFunc}
	svc, err := chapel.NewService(repo, chapel.WithNowFunc(nowFunc))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Pray on Day 1
	b, err := svc.SelectBlessing(ctx, "char1", chapel.BlessingExp)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingExp {
		t.Fatalf("expected BlessingExp, got %v", b.ActiveBlessing)
	}

	// 2. Midnight passes to Day 2 01:00 JST without sleep
	currentTime = time.Date(2026, 10, 10, 1, 0, 0, 0, chapel.JST)

	// Blessing must REMAIN active! Date rollover does not wipe active blessing without sleep.
	status, err := svc.GetStatus(ctx, "char1", "アルス")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.HasActiveBlessing || status.ActiveBlessing != chapel.BlessingExp {
		t.Errorf("expected blessing to remain active across midnight, got %+v", status)
	}

	// Cannot re-pray without having slept
	_, err = svc.SelectBlessing(ctx, "char1", chapel.BlessingGold)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed without sleep on next day, got %v", err)
	}
}

func TestService_MidnightRolloverWithSleep_ClearsBlessingAndAllowsRePrayer(t *testing.T) {
	ctx := context.Background()
	// Day 1 20:00 JST
	currentTime := time.Date(2026, 10, 9, 20, 0, 0, 0, chapel.JST)
	nowFunc := func() time.Time { return currentTime }

	repo := &mockChapelRepo{nowFunc: nowFunc}
	svc, err := chapel.NewService(repo, chapel.WithNowFunc(nowFunc))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Pray on Day 1
	_, err = svc.SelectBlessing(ctx, "char1", chapel.BlessingDrop)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}

	// 2. Midnight passes to Day 2 08:00 JST, then sleeps
	currentTime = time.Date(2026, 10, 10, 8, 0, 0, 0, chapel.JST)
	if err := svc.ClearBlessing(ctx, "char1"); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}

	// Both active blessing and quota record must be cleared
	status, err := svc.GetStatus(ctx, "char1", "アルス")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.HasActiveBlessing {
		t.Errorf("expected no active blessing after next-day sleep")
	}
	if status.PrayedAt != nil {
		t.Errorf("expected PrayedAt to be cleared after next-day sleep, got %v", status.PrayedAt)
	}

	// 3. Re-prayer at 09:00 JST on Day 2 succeeds!
	currentTime = time.Date(2026, 10, 10, 9, 0, 0, 0, chapel.JST)
	b, err := svc.SelectBlessing(ctx, "char1", chapel.BlessingCasino)
	if err != nil {
		t.Fatalf("SelectBlessing on Day 2 after sleep failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingCasino {
		t.Errorf("expected BlessingCasino on Day 2, got %v", b.ActiveBlessing)
	}
}

func TestService_NeverPrayed_CanPray(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	status, err := svc.GetStatus(ctx, "char-new", "新人")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.HasActiveBlessing {
		t.Errorf("expected HasActiveBlessing = false for unprayed character")
	}
	if status.PrayedAt != nil {
		t.Errorf("expected PrayedAt = nil for unprayed character, got %v", status.PrayedAt)
	}

	b, err := svc.SelectBlessing(ctx, "char-new", chapel.BlessingExp)
	if err != nil {
		t.Fatalf("SelectBlessing failed for new character: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingExp {
		t.Errorf("expected BlessingExp, got %v", b.ActiveBlessing)
	}
}

func TestIsSameJSTDay(t *testing.T) {
	jst := chapel.JST
	t1 := time.Date(2026, 10, 9, 23, 59, 59, 0, jst)
	t2 := time.Date(2026, 10, 9, 0, 0, 1, 0, jst)
	t3 := time.Date(2026, 10, 10, 0, 0, 1, 0, jst)

	if !chapel.IsSameJSTDay(t1, t2) {
		t.Errorf("expected t1 and t2 to be same JST day")
	}
	if chapel.IsSameJSTDay(t1, t3) {
		t.Errorf("expected t1 and t3 to NOT be same JST day")
	}
	if chapel.IsSameJSTDay(time.Time{}, t1) {
		t.Errorf("expected zero time to return false")
	}
}

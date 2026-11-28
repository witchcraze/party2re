package party

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func setupPartyAdventureTestService(t *testing.T, opts ...Option) (*Service, *mockPartyRepository, *mockCharacterRepository) {
	t.Helper()
	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(partyRepo, charRepo, stages, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return svc, partyRepo, charRepo
}

func TestCreateParty_ModeValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, charRepo := setupPartyAdventureTestService(t)

	leader := corecharacter.Character{
		ID:    "mode-val-lead",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	charRepo.chars[leader.ID] = leader

	t.Run("empty mode defaults to normal", func(t *testing.T) {
		detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Speed:   18,
			Name:    "PartyEmptyMode",
			StageID: "stage-01",
			Mode:    "",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if detail.Party.Mode != adventure.CrawlModeNormal {
			t.Errorf("expected Mode %q, got %q", adventure.CrawlModeNormal, detail.Party.Mode)
		}
		_, _ = svc.LeaveParty(ctx, detail.Party.ID, leader.ID)
	})

	t.Run("normal mode accepted case-insensitively", func(t *testing.T) {
		detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Speed:   18,
			Name:    "PartyNormalMode",
			StageID: "stage-01",
			Mode:    "NORMAL",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if detail.Party.Mode != adventure.CrawlModeNormal {
			t.Errorf("expected Mode %q, got %q", adventure.CrawlModeNormal, detail.Party.Mode)
		}
		_, _ = svc.LeaveParty(ctx, detail.Party.ID, leader.ID)
	})

	t.Run("unimplemented modes return ErrUnsupportedMode", func(t *testing.T) {
		for _, unsupported := range []string{"calm", "try", "hard", "CALM"} {
			_, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
				Speed:   18,
				Name:    "PartyUnsupported",
				StageID: "stage-01",
				Mode:    unsupported,
			})
			if !errors.Is(err, ErrUnsupportedMode) {
				t.Errorf("mode %q: expected ErrUnsupportedMode, got %v", unsupported, err)
			}
		}
	})

	t.Run("unrecognized mode returns ErrInvalidMode", func(t *testing.T) {
		_, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
			Speed:   18,
			Name:    "PartyInvalid",
			StageID: "stage-01",
			Mode:    "not_a_valid_mode",
		})
		if !errors.Is(err, ErrInvalidMode) {
			t.Errorf("expected ErrInvalidMode, got %v", err)
		}
	})
}

func TestPartyAdmission_MatchesStageCatalogAndAvailableStages(t *testing.T) {
	ctx := context.Background()
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	partyRepo := newMockPartyRepository()
	charRepo := newMockCharacterRepository()
	svc, err := NewService(partyRepo, charRepo, stages)
	if err != nil {
		t.Fatal(err)
	}

	jst := time.FixedZone("JST", 9*60*60)

	testCases := []struct {
		name     string
		level    int
		jobLevel int
		time     time.Time
		stageID  string
	}{
		{"stage-02 with Level 1 JobLevel 0 (accepted)", 1, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-02"},
		{"stage-02 with JobLevel 0", 20, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-02"},
		{"stage-03 with JobLevel 0 (rejected)", 20, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-03"},
		{"stage-03 with JobLevel 1 (accepted)", 20, 1, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-03"},
		{"stage-23 outside winter (rejected)", 99, 10, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-23"},
		{"stage-23 in winter (accepted)", 99, 10, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-23"},
		{"stage-25 in winter with JobLevel 9 (rejected)", 99, 9, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-25"},
		{"stage-25 in winter with JobLevel 10 (accepted)", 99, 10, time.Date(2026, 12, 15, 12, 0, 0, 0, jst), "stage-25"},
		{"stage-27 outside halloween (rejected)", 99, 0, time.Date(2026, 5, 1, 12, 0, 0, 0, jst), "stage-27"},
		{"stage-27 in halloween (accepted)", 99, 0, time.Date(2026, 10, 15, 12, 0, 0, 0, jst), "stage-27"},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			charID := fmt.Sprintf("char-%d-%s", i, tc.stageID)
			char := corecharacter.Character{
				ID:       charID,
				Name:     "Leader",
				Level:    tc.level,
				JobLevel: tc.jobLevel,
				Stats: corecharacter.Stats{
					HP:    100,
					MaxHP: 100,
				},
			}
			charRepo.chars[char.ID] = char

			stages.SetNowFunc(func() time.Time { return tc.time })

			available := stages.AvailableStagesAt(char, tc.time)
			isAvailable := false
			for _, st := range available {
				if st.ID == tc.stageID {
					isAvailable = true
					break
				}
			}

			accessErr := stages.CanAccessStageAt(char, tc.stageID, tc.time)
			if (accessErr == nil) != isAvailable {
				t.Fatalf("AvailableStages and CanAccessStageAt mismatch: available=%v, accessErr=%v", isAvailable, accessErr)
			}

			_, createErr := svc.CreateParty(ctx, char.ID, CreatePartyRequest{
				Speed:      18,
				Name:       fmt.Sprintf("Party-%d-%s", i, tc.stageID),
				StageID:    tc.stageID,
				MaxMembers: 4,
			})

			if isAvailable {
				if createErr != nil {
					t.Fatalf("CreateParty unexpectedly failed for available stage %s: %v", tc.stageID, createErr)
				}
			} else {
				if !errors.Is(createErr, accessErr) {
					t.Fatalf("CreateParty error %v does not match expected access error %v", createErr, accessErr)
				}
			}
		})
	}
}

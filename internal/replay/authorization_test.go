package replay_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/replay"
)

type replayIdentity struct{}

func (replayIdentity) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	if token == "" || token == "invalid" {
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}
	return coreplayer.Player{ID: token}, nil
}

func (replayIdentity) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	return corecharacter.Character{ID: id, PlayerID: "owner"}, nil
}

func TestReplayAuthorization(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	svc, err := replay.NewService(repo, replayIdentity{}, replayIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	repo.replays["personal"] = replay.BattleReplay{ID: "personal", Provenance: replay.ProvenancePersonal, ArchiveRecipientID: "recipient", InitiatorID: "participant"}
	repo.replays["unknown"] = replay.BattleReplay{ID: "unknown", ArchiveRecipientID: "recipient"}
	repo.replays["public-flag"] = replay.BattleReplay{ID: "public-flag", Provenance: "global", ArchiveRecipientID: "recipient"}
	for _, tc := range []struct {
		token, id string
		want      error
	}{
		{"owner", "personal", nil},
		{"", "personal", replay.ErrReplayNotFound},
		{"participant", "personal", replay.ErrReplayNotFound},
		{"spectator", "personal", replay.ErrReplayNotFound},
		{"owner", "unknown", replay.ErrReplayNotFound},
		{"owner", "public-flag", replay.ErrReplayNotFound},
		{"owner", "missing", replay.ErrReplayNotFound},
		{"invalid", "missing", coreplayer.ErrAuthentication},
		{"invalid", "personal", coreplayer.ErrAuthentication},
	} {
		t.Run(tc.token+"/"+tc.id, func(t *testing.T) {
			_, err := svc.GetReplay(ctx, tc.token, tc.id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := svc.GetCharacterHistory(ctx, "participant", "recipient", "", 10); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatal(err)
	}
	if _, err := svc.GetCharacterHistoryByCursor(ctx, "participant", "recipient", "", 10, ""); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatal(err)
	}
}

func TestRecordReplayProvenanceValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	svc, err := replay.NewService(repo, replayIdentity{}, replayIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		provenance, recipient string
		valid                 bool
	}{
		{"", "", true},
		{replay.ProvenancePersonal, "recipient", true},
		{replay.ProvenancePersonal, "", false},
		{replay.ProvenancePersonal, "  ", false},
		{"", "recipient", false},
		{"global", "recipient", false},
		{"global", "", false},
	} {
		replayID, err := svc.RecordBattle(ctx, replay.SaveReplayRequest{
			Provenance: tc.provenance, ArchiveRecipientID: tc.recipient, CombatType: replay.CombatTypeAdventure,
			Initiator: replay.ParticipantSnapshot{ID: "initiator"}, Opponent: replay.ParticipantSnapshot{ID: "opponent"},
		})
		if tc.valid {
			if err != nil {
				t.Fatal(err)
			}
			stored, err := repo.FindByID(ctx, replayID)
			if err != nil || stored.Provenance != tc.provenance || stored.ArchiveRecipientID != tc.recipient {
				t.Fatalf("metadata lost: %+v %v", stored, err)
			}
		} else if !errors.Is(err, replay.ErrInvalidReplay) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

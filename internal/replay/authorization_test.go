package replay_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
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
	repo.replays["second"] = replay.BattleReplay{ID: "second", Provenance: replay.ProvenancePersonal, ArchiveRecipientID: "second-char", InitiatorID: "second-char"}
	repo.replays["unknown"] = replay.BattleReplay{ID: "unknown", ArchiveRecipientID: "recipient"}
	repo.replays["public-flag"] = replay.BattleReplay{ID: "public-flag", Provenance: "global", ArchiveRecipientID: "recipient"}
	for _, tc := range []struct {
		token, id string
		want      error
	}{
		{"owner", "personal", nil},
		{"owner", "second", nil},
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

type failFindByIDRepo struct {
	*mockReplayRepo
}

func (r failFindByIDRepo) FindByID(context.Context, string) (*replay.BattleReplay, error) {
	return nil, errors.New("decode failure must not leak before authorization")
}

func TestReplayAuthorization_HistoryPaginationAndPayloadNonDisclosure(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	now := time.Now().UTC().Truncate(time.Second)

	var visible []string
	for _, suffix := range []string{"1", "2", "3", "4", "5"} {
		rep := replay.BattleReplay{
			ID:                 "rep-" + suffix,
			Provenance:         replay.ProvenancePersonal,
			ArchiveRecipientID: "recipient",
			CombatType:         replay.CombatTypeAdventure,
			CreatedAt:          now,
		}
		visible = append([]string{rep.ID}, visible...)
		repo.replays[rep.ID] = rep
	}
	repo.replays["rep-other"] = replay.BattleReplay{ID: "rep-other", Provenance: replay.ProvenancePersonal, ArchiveRecipientID: "other-char", CombatType: replay.CombatTypeAdventure, CreatedAt: now}
	repo.replays["rep-unknown"] = replay.BattleReplay{ID: "rep-unknown", ArchiveRecipientID: "recipient", CombatType: replay.CombatTypeAdventure, CreatedAt: now}

	// Non-owner does not reach FindByID when payload is malformed.
	chars := replayScopedChars{owners: map[string]string{"recipient": "owner", "other-char": "other"}}
	svc, err := replay.NewService(failFindByIDRepo{repo}, replayIdentity{}, chars)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetReplay(ctx, "owner", "rep-other"); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatalf("malformed non-owned replay must return ErrReplayNotFound before decode, got %v", err)
	}

	svc, err = replay.NewService(repo, replayIdentity{}, chars)
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.GetCharacterHistory(ctx, "owner", "recipient", replay.CombatTypePvP, 20)
	if err != nil || len(history) != 0 {
		t.Fatalf("unexpected pvp history: %+v %v", history, err)
	}
	history, err = svc.GetCharacterHistory(ctx, "owner", "recipient", replay.CombatTypeAdventure, 20)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range history {
		got = append(got, h.ID)
	}
	if !reflect.DeepEqual(got, visible) {
		t.Fatalf("history: %v want %v", got, visible)
	}

	var paged []string
	cursor := ""
	for range 4 {
		page, err := svc.GetCharacterHistoryByCursor(ctx, "owner", "recipient", "", 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range page.Items {
			paged = append(paged, h.ID)
		}
		if page.NextCursor != "" {
			_, nextID := pagination.DecodeCursorParts(page.NextCursor)
			if nextID != page.Items[len(page.Items)-1].ID {
				t.Fatalf("cursor %q points to wrong id %q", page.NextCursor, nextID)
			}
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if !reflect.DeepEqual(paged, visible) {
		t.Fatalf("paged: %v want %v", paged, visible)
	}

	empty, err := svc.GetCharacterHistoryByCursor(ctx, "owner", "recipient", "", 2, pagination.EncodeCursor(now, visible[len(visible)-1]))
	if err != nil || len(empty.Items) != 0 || empty.HasMore || empty.NextCursor != "" {
		t.Fatalf("empty terminal page mismatch: %+v %v", empty, err)
	}
}

type replayScopedChars struct {
	owners map[string]string
}

func (c replayScopedChars) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	owner, ok := c.owners[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return corecharacter.Character{ID: id, PlayerID: owner}, nil
}

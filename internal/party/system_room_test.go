package party

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

func sealingRoomFixture(t *testing.T, opts ...Option) (*Service, *mockPartyRepository, *mockCharacterRepository) {
	t.Helper()
	svc, repo, chars, _ := setupTestService(t, opts...)
	repo.parties["seal"] = Party{ID: "seal", Kind: KindSealing, LeaderName: "@破壊神", StageID: "king1", MaxMembers: 2, NeedJoin: "hp_400_o", Speed: 12, Status: StatusRecruiting}
	return svc, repo, chars
}

func joiner(id string, mutate func(*corecharacter.Character)) corecharacter.Character {
	c := corecharacter.Character{ID: id, Name: id, Level: 1, Stats: corecharacter.Stats{HP: 500, MaxHP: 500}, Tired: 10}
	if mutate != nil {
		mutate(&c)
	}
	return c
}

func TestJoinSealingRoom_RejectionsChargeNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		char  corecharacter.Character
		setup func(*mockPartyRepository)
		want  error
	}{
		"need_join":   {joiner("weak", func(c *corecharacter.Character) { c.Stats.MaxHP = 399 }), nil, ErrNeedJoinNotMet},
		"unconscious": {joiner("fallen", func(c *corecharacter.Character) { c.Stats.HP = 0 }), nil, ErrCharacterUnconscious},
		"exhausted":   {joiner("tired", func(c *corecharacter.Character) { c.Tired = 100 }), nil, ErrCharacterExhausted},
		"other room": {joiner("busy", nil), func(r *mockPartyRepository) {
			r.parties["p2"] = Party{ID: "p2", Status: StatusRecruiting}
			r.memberParty["busy"] = "p2"
		}, ErrAlreadyInParty},
		"full": {joiner("late", nil), func(r *mockPartyRepository) { r.members["seal"] = map[string]Member{"a": {}, "b": {}} }, ErrPartyFull},
		"battle running": {joiner("mid", nil), func(r *mockPartyRepository) {
			p := r.parties["seal"]
			p.Status = StatusInProgress
			r.parties["seal"] = p
		}, ErrPartyNotRecruiting},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, chars := sealingRoomFixture(t)
			chars.chars[tc.char.ID] = tc.char
			if tc.setup != nil {
				tc.setup(repo)
			}
			if _, err := svc.JoinParty(context.Background(), "seal", tc.char.ID, ""); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if got := chars.chars[tc.char.ID]; got.Tired != tc.char.Tired {
				t.Fatalf("rejected join charged fatigue: %d -> %d", tc.char.Tired, got.Tired)
			}
			if _, ok := repo.members["seal"][tc.char.ID]; ok {
				t.Fatal("rejected join added a member")
			}
		})
	}
}

func TestJoinSealingRoom_ChargesTwentyTiredOnceIgnoringLobbyGates(t *testing.T) {
	svc, repo, chars := sealingRoomFixture(t)
	chars.chars["hero"] = joiner("hero", nil)
	// No password, level or ordinary stage access applies; king1 is not an Adventure stage.
	detail, err := svc.JoinParty(context.Background(), "seal", "hero", "wrong")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 1 || chars.chars["hero"].Tired != 30 {
		t.Fatalf("join: members=%d Tired=%d", len(detail.Members), chars.chars["hero"].Tired)
	}
	if _, err := svc.JoinParty(context.Background(), "seal", "hero", ""); !errors.Is(err, ErrAlreadyInParty) || chars.chars["hero"].Tired != 30 {
		t.Fatalf("repeat join: err=%v Tired=%d", err, chars.chars["hero"].Tired)
	}
	if _, ok := repo.members["seal"]["hero"]; !ok {
		t.Fatal("member missing")
	}
}

type rollbackAfterCallback struct{}

func (rollbackAfterCallback) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return economy.WithRollbackScope(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return errors.New("commit failed")
	})
}

func TestJoinSealingRoom_RolledBackChargeRemovesMembership(t *testing.T) {
	svc, repo, chars := sealingRoomFixture(t, WithTransactionProvider(rollbackAfterCallback{}))
	chars.chars["hero"] = joiner("hero", nil)
	if _, err := svc.JoinParty(context.Background(), "seal", "hero", ""); err == nil {
		t.Fatal("rolled-back join succeeded")
	}
	if _, ok := repo.members["seal"]["hero"]; ok || repo.memberParty["hero"] != "" {
		t.Fatal("rolled-back join kept its lobby membership")
	}
}

func TestCreateParty_CannotOpenSealingStages(t *testing.T) {
	svc, _, chars, _ := setupTestService(t)
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	svc.stages = stages
	chars.chars["hero"] = joiner("hero", nil)
	if _, err := svc.CreateParty(context.Background(), "hero", CreatePartyRequest{Name: "seal", StageID: "king1"}); !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("ordinary creation opened a king stage: %v", err)
	}
}

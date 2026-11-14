package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type contestSceneCharacters struct {
	contest.CharacterRepository
	f *gatewayFixture
}

func (r contestSceneCharacters) FindByID(ctx context.Context, actor string) (corecharacter.Character, error) {
	return r.f.FindByID(ctx, actor)
}

type contestSceneRepository struct {
	contest.ContestRepository
	photos     []contest.Photo
	entries    []contest.ContestEntry
	votes      []contest.ContestVote
	readErr    error
	writeErr   error
	afterWrite func()
	writes     int
}

func (r *contestSceneRepository) ListPhotosByCharacterID(_ context.Context, actor string) ([]contest.Photo, error) {
	return slices.DeleteFunc(slices.Clone(r.photos), func(p contest.Photo) bool { return p.CharacterID != actor }), r.readErr
}

func (r *contestSceneRepository) FindPhotoByID(_ context.Context, id string) (contest.Photo, error) {
	for _, p := range r.photos {
		if p.ID == id {
			return p, nil
		}
	}
	return contest.Photo{}, contest.ErrPhotoNotFound
}

func (r *contestSceneRepository) DeletePhoto(_ context.Context, id string) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.photos = slices.DeleteFunc(r.photos, func(p contest.Photo) bool { return p.ID == id })
	r.wrote()
	return nil
}

func (r *contestSceneRepository) GetActiveRound(context.Context) (contest.ContestRound, error) {
	return contest.ContestRound{Round: 3, Status: contest.StatusActive}, nil
}

func (r *contestSceneRepository) GetPreparingRound(context.Context) (contest.ContestRound, error) {
	return contest.ContestRound{}, contest.ErrContestNotFound
}

func (r *contestSceneRepository) ListEntriesByRound(context.Context, int) ([]contest.ContestEntry, error) {
	return slices.Clone(r.entries), r.readErr
}

func (r *contestSceneRepository) FindEntryByIDForUpdate(_ context.Context, id string) (contest.ContestEntry, error) {
	for _, e := range r.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return contest.ContestEntry{}, contest.ErrEntryNotFound
}

func (r *contestSceneRepository) HasVotedInRound(_ context.Context, round int, actor string) (bool, error) {
	return slices.ContainsFunc(r.votes, func(v contest.ContestVote) bool {
		return v.Round == round && v.VoterCharacterID == actor
	}), nil
}

func (r *contestSceneRepository) SaveVote(_ context.Context, vote contest.ContestVote) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.votes = append(r.votes, vote)
	return nil
}

func (r *contestSceneRepository) IncrementEntryVotes(_ context.Context, id string) error {
	for i := range r.entries {
		if r.entries[i].ID == id {
			r.entries[i].Votes++
			r.wrote()
			return nil
		}
	}
	return contest.ErrEntryNotFound
}

func (r *contestSceneRepository) wrote() {
	r.writes++
	if r.afterWrite != nil {
		r.afterWrite()
	}
}

func newContestSceneRepository(f *contestFixture) *contestSceneRepository {
	r := &contestSceneRepository{photos: slices.Clone(f.photos), entries: slices.Clone(f.entries)}
	for i := range r.photos {
		r.photos[i].CharacterID = "hero"
	}
	return r
}

func TestContestNormalizedRepliesRealService(t *testing.T) {
	for _, action := range []string{"contest_photo_delete", "contest_vote"} {
		for _, padded := range []bool{false, true} {
			for _, refreshFails := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/padded=%t/refreshFails=%t", action, padded, refreshFails), func(t *testing.T) {
					f := newContestFixture(t)
					r := newContestSceneRepository(f)
					service, err := contest.NewService(contestSceneCharacters{f: f.gatewayFixture}, r)
					if err != nil {
						t.Fatal(err)
					}
					f.service = service
					_, router := f.router(t, "contest")
					if refreshFails {
						r.afterWrite = func() { r.readErr = errors.New("private refresh failure") }
					}
					key, id, want := "photo_id", "p-old", "2枚目のスクリーンショットを削除しましたわよ"
					if action == "contest_vote" {
						key, id, want = "entry_id", "e9", "夕焼けに投票しました"
					}
					if padded {
						id = " \t\u3000" + id + "\n "
					}
					params, err := json.Marshal(map[string]string{key: id})
					if err != nil {
						t.Fatal(err)
					}
					status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(action, string(params)), nil)
					got := tavernResult[ContestResult](t, raw)
					if status != 200 || string(raw["success"]) != "true" || got.Message != want || r.writes != 1 {
						t.Fatalf("reply: %d %s, writes=%d", status, raw, r.writes)
					}
					if action == "contest_photo_delete" {
						if len(r.photos) != 1 || r.photos[0].ID != "p-new" {
							t.Fatalf("wrong remaining photos: %+v", r.photos)
						}
					} else if len(r.votes) != 1 || r.votes[0].EntryID != "e9" || r.votes[0].VoterCharacterID != "hero" || r.entries[0].Votes != 1 || got.Vote == nil || got.Vote.EntryID != "e9" {
						t.Fatalf("wrong vote: %+v / %+v / %+v", r.votes, r.entries, got.Vote)
					}
					if refreshFails {
						var detail ErrorDetail
						if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" || string(raw["context"]) != "null" {
							t.Fatalf("lost refresh failure: %s (%v)", raw, err)
						}
					}
					r.readErr = nil
					status, observation := navigationGET(t, router)
					data := decodeShopScene[ContestSceneData](t, observation)
					if status != 200 || r.writes != 1 || len(data.Photos) != len(r.photos) || data.ActiveEntries[0].Votes != r.entries[0].Votes {
						t.Fatalf("GET recovery: %d %+v, writes=%d", status, data, r.writes)
					}
				})
			}
		}
	}
}

func TestContestNormalizedRepliesRejectFailures(t *testing.T) {
	for _, tc := range []struct {
		name, action, params, code string
		status                     int
	}{
		{"foreign photo", "contest_photo_delete", `{"photo_id":" p-other "}`, "CONTEST_PHOTO_NOT_FOUND", 400},
		{"missing photo", "contest_photo_delete", `{"photo_id":" missing "}`, "CONTEST_PHOTO_NOT_FOUND", 400},
		{"missing entry", "contest_vote", `{"entry_id":" missing "}`, "CONTEST_ENTRY_NOT_FOUND", 400},
		{"photo read", "contest_photo_delete", `{"photo_id":" p-old "}`, "EXECUTION_FAILED", 500},
		{"entry read", "contest_vote", `{"entry_id":" e9 "}`, "EXECUTION_FAILED", 500},
		{"photo write", "contest_photo_delete", `{"photo_id":" p-old "}`, "EXECUTION_FAILED", 500},
		{"vote write", "contest_vote", `{"entry_id":" e9 "}`, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContestFixture(t)
			r := newContestSceneRepository(f)
			r.photos = append(r.photos, contest.Photo{ID: "p-other", CharacterID: "other"})
			if tc.name == "photo read" || tc.name == "entry read" {
				r.readErr = errors.New("private read failure")
			}
			if tc.name == "photo write" || tc.name == "vote write" {
				r.writeErr = errors.New("private write failure")
			}
			service, err := contest.NewService(contestSceneCharacters{f: f.gatewayFixture}, r)
			if err != nil {
				t.Fatal(err)
			}
			f.service = service
			_, router := f.router(t, "contest")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, raw, tc.code)
			if status != tc.status || r.writes != 0 || len(r.photos) != 3 || len(r.votes) != 0 || r.entries[0].Votes != 0 {
				t.Fatalf("failed command mutated state: %d %s %+v", status, raw, r)
			}
		})
	}
}

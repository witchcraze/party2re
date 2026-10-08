package home

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type viewRepository struct {
	Repository
	err          error
	fail         string
	privateReads int
}

func (r *viewRepository) GetUnreadLetterCount(context.Context, string) (int, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "unread" {
		return 1, r.err
	}
	return 1, nil
}
func (r *viewRepository) ListDeliveryNotices(context.Context, string, bool) ([]DeliveryNotice, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "notices" {
		return nil, r.err
	}
	return []DeliveryNotice{{Message: "private"}}, nil
}
func (r *viewRepository) ListCompanionPhrases(context.Context, string) ([]CompanionPhrase, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "phrases" {
		return nil, r.err
	}
	return []CompanionPhrase{{Phrase: "private"}}, nil
}

func TestHomeViewPrivacyAndRequiredReads(t *testing.T) {
	chars := &mockCharReader{chars: map[string]corecharacter.Character{
		"owner":   {ID: "owner", PlayerID: "player", Name: "Owner", Money: 123456789},
		"visitor": {ID: "visitor", PlayerID: "other"},
	}}
	repo := &viewRepository{Repository: newMockHomeRepo(chars.chars), err: errors.New("private read failure")}
	svc, err := NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{"", "visitor"} {
		player := ""
		if viewer != "" {
			player = "other"
		}
		view, err := svc.GetHomeView(context.Background(), "owner", viewer, player)
		if err != nil || view.IsOwner || repo.privateReads != 0 {
			t.Fatalf("public: %+v %v reads=%d", view, err, repo.privateReads)
		}
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"player_id", "money", "unread_letter_count", "companion_phrase_count", "recent_delivery_count"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("leaked %s: %s", forbidden, raw)
			}
		}
	}
	if _, err := svc.GetHomeView(context.Background(), "owner", "owner", "other"); !errors.Is(err, ErrForbidden) || repo.privateReads != 0 {
		t.Fatalf("spoof: %v", err)
	}
	for _, failure := range []string{"unread", "phrases", "notices"} {
		repo.fail = failure
		if _, err := svc.GetHomeView(context.Background(), "owner", "owner", "player"); !errors.Is(err, repo.err) {
			t.Fatalf("required %s enrichment: %v", failure, err)
		}
	}
	repo.err = nil
	view, err := svc.GetHomeView(context.Background(), "owner", "owner", "player")
	if err != nil || !view.IsOwner || view.Private == nil || view.Private.UnreadLetterCount != 1 {
		t.Fatalf("owned: %+v %v", view, err)
	}
	if _, err := svc.GetHomeView(context.Background(), "missing", "", ""); !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if len(repo.Repository.(*mockHomeRepo).homes) != 0 {
		t.Fatal("observation created home")
	}
}

package home

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pagination"
)

type noticePageRepository struct {
	Repository
	read          bool
	fail          error
	limit, offset int
	before        time.Time
	beforeID      string
}

func (r *noticePageRepository) ListDeliveryNoticesPage(_ context.Context, actor string, limit, offset int, before time.Time, beforeID string) ([]DeliveryNotice, error) {
	r.read, r.limit, r.offset, r.before, r.beforeID = true, limit, offset, before, beforeID
	return []DeliveryNotice{{ID: "b", CreatedAt: time.Unix(1, 0)}, {ID: "a", CreatedAt: time.Unix(1, 0)}}, r.fail
}
func TestDeliveryNoticePageBoundary(t *testing.T) {
	repo := &noticePageRepository{}
	chars := &mockCharReader{chars: map[string]corecharacter.Character{"owner": {ID: "owner", PlayerID: "player"}}}
	svc, err := NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, player := range []string{"", "other"} {
		if _, err := svc.ListDeliveryNoticesPage(ctx, "owner", player, 1, 0, ""); !errors.Is(err, ErrForbidden) || repo.read {
			t.Fatalf("unauthorized: %v read=%v", err, repo.read)
		}
	}
	if _, err := svc.ListDeliveryNoticesPage(ctx, "missing", "player", 1, 0, ""); err == nil || repo.read {
		t.Fatal("missing character read notices")
	}
	cursor := pagination.EncodeCursor(time.Unix(2, 0), "c")
	page, err := svc.ListDeliveryNoticesPage(ctx, "owner", "player", 1, 4, cursor)
	if err != nil || !page.HasMore || len(page.Items) != 1 || page.NextCursor != pagination.EncodeCursor(time.Unix(1, 0), "b") || repo.limit != 2 || repo.offset != 0 || !repo.before.Equal(time.Unix(2, 0)) || repo.beforeID != "c" {
		t.Fatalf("page=%+v repo=%+v err=%v", page, repo, err)
	}
	if _, err := svc.ListDeliveryNoticesPage(ctx, "owner", "player", 1000, -1, ""); err != nil || repo.limit != 101 || repo.offset != 0 {
		t.Fatalf("bound: %+v %v", repo, err)
	}
	if _, err := svc.ListDeliveryNoticesPage(ctx, "owner", "player", 1, 3, ""); err != nil || repo.offset != 3 {
		t.Fatalf("offset: %+v %v", repo, err)
	}
	repo.fail = errors.New("read failed")
	if _, err := svc.ListDeliveryNoticesPage(ctx, "owner", "player", 1, 0, ""); !errors.Is(err, repo.fail) {
		t.Fatalf("read failure: %v", err)
	}
}

func (m *mockHomeRepo) ListDeliveryNoticesPage(_ context.Context, actor string, limit, offset int, before time.Time, beforeID string) ([]DeliveryNotice, error) {
	var notices []DeliveryNotice
	for _, n := range m.notices[actor] {
		if !n.IsCleared && (before.IsZero() || n.CreatedAt.Before(before) || (n.CreatedAt.Equal(before) && n.ID < beforeID)) {
			notices = append(notices, n)
		}
	}
	sort.Slice(notices, func(i, j int) bool {
		if notices[i].CreatedAt.Equal(notices[j].CreatedAt) {
			return notices[i].ID > notices[j].ID
		}
		return notices[i].CreatedAt.After(notices[j].CreatedAt)
	})
	return pagination.SlicePage(notices, limit, offset).Items, nil
}

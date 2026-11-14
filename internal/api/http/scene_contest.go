package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type contestEnterRequest struct {
	PhotoID string `json:"photo_id"`
	Title   string `json:"title"`
}

type contestVoteRequest struct {
	EntryID string `json:"entry_id"`
	Comment string `json:"comment"`
}

type contestPhotoRequest struct {
	PhotoID string `json:"photo_id"`
}

// ContestSceneData is the round state, the entries open for voting and the
// actor's own photos, newest first (legacy screenshot order).
type ContestSceneData struct {
	Parent string `json:"parent"`
	contest.ContestOverview
	Photos []contest.Photo `json:"photos"`
}

// ContestResult is a contest hall command outcome with the host's reply.
// Warning reports a committed entry whose settlement scheduling failed.
type ContestResult struct {
	Entry   *contest.ContestEntry `json:"entry,omitempty"`
	Vote    *contest.ContestVote  `json:"vote,omitempty"`
	Message string                `json:"message"`
	Warning string                `json:"warning,omitempty"`
}

// contestRefusal carries photo.cgi's refusal line for errors whose text needs request facts.
type contestRefusal struct {
	message string
	err     error
}

func (e contestRefusal) Error() string { return e.err.Error() }
func (e contestRefusal) Unwrap() error { return e.err }

func (h *Handler) registerContestCommands() {
	service := h.contest
	if service == nil {
		withActionCommand[contestEnterRequest]("contest_enter", nil, nil)(h)
		withActionCommand[contestVoteRequest]("contest_vote", nil, nil)(h)
		withActionCommand[contestPhotoRequest]("contest_photo_delete", nil, nil)(h)
		return
	}
	withActionCommand("contest_enter", func(ctx context.Context, actor string, p contestEnterRequest) (any, error) {
		entry, err := service.EnterContest(ctx, actor, p.PhotoID, p.Title)
		switch {
		case errors.Is(err, contest.ErrScheduleFailed):
			return ContestResult{Entry: &entry, Message: entryMessage(entry), Warning: "entry saved, but settlement scheduling failed"}, nil
		case errors.Is(err, contest.ErrInvalidTitle), errors.Is(err, contest.ErrTitleTooLong):
			return nil, contestRefusal{entryTitleMessage(p.Title), err}
		case err != nil:
			return nil, err
		}
		return ContestResult{Entry: &entry, Message: entryMessage(entry)}, nil
	}, contestRejection)(h)
	withActionCommand("contest_vote", func(ctx context.Context, actor string, p contestVoteRequest) (any, error) {
		p.EntryID = strings.TrimSpace(p.EntryID)
		// The reply names the entry and round, so read them before voting.
		entries, err := service.GetCurrentEntries(ctx)
		if err != nil {
			return nil, err
		}
		vote, err := service.Vote(ctx, actor, p.EntryID, p.Comment)
		if errors.Is(err, contest.ErrAlreadyVoted) && len(entries) > 0 {
			return nil, contestRefusal{fmt.Sprintf("第%d回のフォトコンテストには、すでに投票済みです", entries[0].Round), err}
		}
		if err != nil {
			return nil, err
		}
		title := p.EntryID
		if i := slices.IndexFunc(entries, func(e contest.ContestEntry) bool { return e.ID == p.EntryID }); i >= 0 {
			title = entries[i].Title
		}
		return ContestResult{Vote: &vote, Message: title + "に投票しました"}, nil
	}, contestRejection)(h)
	withActionCommand("contest_photo_delete", func(ctx context.Context, actor string, p contestPhotoRequest) (any, error) {
		p.PhotoID = strings.TrimSpace(p.PhotoID)
		photos, err := service.ListPhotos(ctx, actor)
		if err != nil {
			return nil, err
		}
		if err := service.DeletePhoto(ctx, actor, p.PhotoID); err != nil {
			return nil, err
		}
		n := slices.IndexFunc(photos, func(ph contest.Photo) bool { return ph.ID == p.PhotoID }) + 1
		return ContestResult{Message: fmt.Sprintf("%d枚目のスクリーンショットを削除しましたわよ", n)}, nil
	}, contestRejection)(h)
}

func entryMessage(e contest.ContestEntry) string {
	return fmt.Sprintf("第%d回フォトコンテストに『%s』という題名でエントリーしました", e.Round, e.Title)
}

// entryTitleMessage picks photo.cgi entori's title refusal; the last matching
// check wins there, so emptiness outranks length, which outranks characters.
func entryTitleMessage(title string) string {
	switch {
	case title == "":
		return "題名を記入してください"
	case utf8.RuneCountInString(title) > contest.MaxTitleLength:
		return "題名は40文字までです"
	case strings.Contains(title, "＠"):
		return "題名に不正な文字( ＠ )が含まれています"
	case strings.ContainsAny(title, ",;\"'&<>@"):
		return "題名に不正な文字( ,;\"'&<>@ )が含まれています"
	case strings.ContainsFunc(title, unicode.IsSpace):
		return "題名に不正な空白が含まれています"
	default:
		return "題名に不正な文字が含まれています"
	}
}

func (h *Handler) contestSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.contest == nil {
		return nil, errSceneNotConfigured
	}
	overview, err := h.contest.GetOverview(ctx)
	if err != nil {
		return nil, err
	}
	photos, err := h.contest.ListPhotos(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	if photos == nil {
		photos = []contest.Photo{}
	}
	return ContestSceneData{Parent: "town", ContestOverview: overview, Photos: photos}, nil
}

func contestRejection(err error) (int, ErrorDetail) {
	var refusal contestRefusal
	var entered contest.AlreadyEnteredError
	switch {
	case errors.As(err, &refusal):
		return http.StatusConflict, ErrorDetail{Code: "CONTEST_REFUSED", Message: refusal.message}
	case errors.As(err, &entered):
		return http.StatusConflict, ErrorDetail{Code: "CONTEST_ALREADY_ENTERED", Message: "【" + entered.Title + "】という作品で、すでにエントリー済みです"}
	case errors.Is(err, contest.ErrDuplicateTitle):
		return http.StatusConflict, ErrorDetail{Code: "CONTEST_DUPLICATE_TITLE", Message: "同じタイトルの作品がすでにエントリーされています"}
	case errors.Is(err, contest.ErrConsecutiveEntryDisallowed):
		return http.StatusConflict, ErrorDetail{Code: "CONTEST_CONSECUTIVE_ENTRY", Message: "連続でコンテストにエントリーすることはできません\n次回のコンテストまでお待ちください"}
	case errors.Is(err, contest.ErrPhotoNotFound), errors.Is(err, contest.ErrForbidden):
		return http.StatusBadRequest, ErrorDetail{Code: "CONTEST_PHOTO_NOT_FOUND", Message: "Choose one of your observed photos."}
	case errors.Is(err, contest.ErrEntryNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "CONTEST_ENTRY_NOT_FOUND", Message: "Choose one of the observed entries."}
	case errors.Is(err, contest.ErrContestNotActive):
		return http.StatusConflict, ErrorDetail{Code: "CONTEST_NOT_ACTIVE", Message: "No contest is open for voting."}
	case errors.Is(err, contest.ErrCommentTooLong):
		return http.StatusBadRequest, ErrorDetail{Code: "CONTEST_COMMENT_TOO_LONG", Message: "The comment is too long."}
	case errors.Is(err, contest.ErrCharacterNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}

// photo.cgi @words; {name} is the actor ($m).
var contestFacility = facility{
	title: "フォトコン会場", commands: []string{"contest_enter", "contest_vote", "contest_photo_delete"},
	read: (*Handler).contestSceneData, placeLog: true,
	npc: &npcProfile{name: "@ワコール", words: []string{
		"フォトコンテストの会場、略してフォトコン会場へようこそ。私が主催者のワコールざます",
		"ここでは、{name}が撮ったスクリーンショットを消したり、コンテストに応募したりできるざます",
		"コンテスト上位入賞者には、ゴールドと賞品が授与されるざます",
		"コンテスト１位の作品に投票した参加者にも小さなメダルが配られるざます",
		"フォトコンで重要なのは、何が写っているかはもちろん。タイトルやコメントなども重要なポイントざます",
		"自分で撮ったスクリーンショットを見たり消すことができるざます",
		"ただ撮るだけではなく、コスプレしたり色々と工夫することが大事ざます",
		fmt.Sprintf("スクリーンショットは最大%d枚まで所持することができるざます。それ以上は、＠けす必要があるざます", contest.MaxPhotosPerCharacter),
	}},
}

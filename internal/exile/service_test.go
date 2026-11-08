package exile_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/exile"
)

type memRepo struct {
	updated   time.Time
	petitions []exile.Petition
	touchErr  error
	sentences []exile.Sentence
	events    []string
}

func (m *memRepo) SaveSentence(_ context.Context, s exile.Sentence) error {
	m.sentences = append(slices.DeleteFunc(m.sentences, func(o exile.Sentence) bool { return o.CharacterID == s.CharacterID }), s)
	return nil
}
func (m *memRepo) ListSentences(context.Context) ([]exile.Sentence, error) {
	m.events = append(m.events, "list")
	return slices.Clone(m.sentences), nil
}
func (m *memRepo) DeleteSentence(_ context.Context, s exile.Sentence) error {
	m.sentences = slices.DeleteFunc(m.sentences, func(o exile.Sentence) bool { return o == s })
	return nil
}

func (m *memRepo) LockBoard(context.Context) (time.Time, error) {
	m.events = append(m.events, "lock")
	return m.updated, nil
}
func (m *memRepo) TouchBoard(_ context.Context, at time.Time) error {
	if m.touchErr != nil {
		return m.touchErr
	}
	m.updated = at
	return nil
}
func (m *memRepo) ListPetitions(context.Context) ([]exile.Petition, error) {
	out := make([]exile.Petition, len(m.petitions))
	for i, p := range m.petitions {
		p.Votes = slices.Clone(p.Votes)
		out[i] = p
	}
	return out, nil
}
func (m *memRepo) CreatePetition(_ context.Context, p exile.Petition) error {
	m.petitions = append(m.petitions, p)
	return nil
}
func (m *memRepo) AddVote(_ context.Context, id string, v exile.Vote) error {
	for i := range m.petitions {
		if m.petitions[i].ID == id {
			m.petitions[i].Votes = append([]exile.Vote{v}, m.petitions[i].Votes...)
		}
	}
	return nil
}
func (m *memRepo) DeletePetition(_ context.Context, id string) error {
	m.petitions = slices.DeleteFunc(m.petitions, func(p exile.Petition) bool { return p.ID == id })
	return nil
}
func (m *memRepo) IsInvolved(_ context.Context, c string) (bool, error) {
	for _, p := range m.petitions {
		if p.ApplicantID == c || p.TargetID == c || slices.ContainsFunc(p.Votes, func(v exile.Vote) bool { return v.VoterID == c }) {
			return true, nil
		}
	}
	return false, nil
}

// memTx restores the repository on failure, standing in for a rollback.
// commitErr fails the commit after fn succeeded.
type memTx struct {
	repo      *memRepo
	commitErr *error
}

func (t memTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	t.repo.events = append(t.repo.events, "begin")
	defer func() { t.repo.events = append(t.repo.events, "end") }()
	saved := memRepo{updated: t.repo.updated, touchErr: t.repo.touchErr, sentences: slices.Clone(t.repo.sentences), events: t.repo.events}
	saved.petitions, _ = t.repo.ListPetitions(ctx)
	err := fn(ctx)
	if err == nil && t.commitErr != nil {
		err = *t.commitErr
	}
	if err != nil {
		*t.repo = saved
		return err
	}
	return nil
}

type chars map[string]corecharacter.Character

func (c chars) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return c.FindByID(ctx, id)
}

func (c chars) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if v, ok := c[id]; ok {
		return v, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}
func (c chars) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	for _, v := range c {
		if v.Name == name {
			return v, nil
		}
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

// sleeps records each deadline as the remaining sleep at the fixture time.
type sleeps struct {
	set map[string]time.Duration
	now time.Time
	err error
}

func (s *sleeps) SetLockUntil(_ context.Context, category, id string, until time.Time) error {
	if category != timer.CategorySleep {
		return fmt.Errorf("unexpected category %s", category)
	}
	if s.err != nil {
		return s.err
	}
	s.set[id] = until.Sub(s.now)
	return nil
}

type fixture struct {
	commit error
	repo   *memRepo
	chars  chars
	sleeps *sleeps
	news   []string
	svc    *exile.Service
	now    time.Time
}

func newFixture(t *testing.T, voters int) *fixture {
	t.Helper()
	f := &fixture{repo: &memRepo{}, chars: chars{}, sleeps: &sleeps{set: map[string]time.Duration{}}, now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	add := func(id string, level int) { f.chars[id] = corecharacter.Character{ID: id, Name: id, JobLevel: level} }
	add("app", 50)
	add("bad", 0)
	add("low", 49)
	for i := range voters {
		add(fmt.Sprintf("v%02d", i), 50)
	}
	news := exile.NewsPublisherFunc(func(_ context.Context, category, title, _, author string, _ time.Time) error {
		if category != "exile" || author != "@追放騎士" {
			t.Fatalf("bad news %s %s", category, author)
		}
		f.news = append(f.news, title)
		return nil
	})
	f.sleeps.now = f.now
	f.svc = exile.NewService(f.repo, f.chars, f.sleeps, memTx{f.repo, &f.commit}, news)
	return f
}

func (f *fixture) petition(t *testing.T, actor, target string) {
	t.Helper()
	if _, err := f.svc.Petition(context.Background(), actor, target, "暴言", f.now); err != nil {
		t.Fatalf("petition %s->%s: %v", actor, target, err)
	}
}

func TestPetitionGuards(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	cases := []struct {
		actor, target, reason string
		want                  error
	}{
		{"app", "", "x", exile.ErrInvalidTarget},
		{"app", "bad", " ", exile.ErrInvalidTarget},
		{"low", "bad", "x", exile.ErrJobLevelTooLow},
		{"app", "app", "x", exile.ErrSelfPetition},
		{"app", "nobody", "x", exile.ErrTargetNotFound},
		{"app", "bad", string(slices.Repeat([]rune("あ"), exile.MaxCommandLength-17-len("bad")+1)), exile.ErrInvalidReason},
	}
	for _, c := range cases {
		if _, err := f.svc.Petition(ctx, c.actor, c.target, c.reason, f.now); !errors.Is(err, c.want) {
			t.Fatalf("%+v: got %v", c, err)
		}
	}
	p, err := f.svc.Petition(ctx, "app", "bad", "暴言", f.now)
	if err != nil || len(p.Votes) != 1 || !p.Votes[0].Approve || p.Votes[0].VoterID != "app" || f.repo.updated != f.now || len(f.news) != 1 {
		t.Fatalf("petition not recorded with applicant approval: %+v %v", p, err)
	}
	if _, err := f.svc.Petition(ctx, "v00", "bad", "x", f.now); err == nil {
		// v00 does not exist in this fixture.
		t.Fatal("unknown actor accepted")
	}
	f.chars["v00"] = corecharacter.Character{ID: "v00", Name: "v00", JobLevel: 50}
	if _, err := f.svc.Petition(ctx, "v00", "bad", "x", f.now); !errors.Is(err, exile.ErrAlreadyPetitioned) {
		t.Fatalf("duplicate target: %v", err)
	}
}

// Legacy counts rows before appending, so four petitions are admitted and any
// applicant holding four blocks every new petition.
func TestPetitionLimitFollowsLegacyScan(t *testing.T) {
	f := newFixture(t, 8)
	for i := range 4 {
		f.petition(t, "app", fmt.Sprintf("v%02d", i))
	}
	if _, err := f.svc.Petition(context.Background(), "app", "v04", "x", f.now); !errors.Is(err, exile.ErrPetitionLimit) {
		t.Fatalf("fifth petition: %v", err)
	}
	if _, err := f.svc.Petition(context.Background(), "v05", "v06", "x", f.now); !errors.Is(err, exile.ErrPetitionLimit) {
		t.Fatalf("other applicant not blocked: %v", err)
	}
	// A duplicate target on an earlier row is reported before the count.
	if _, err := f.svc.Petition(context.Background(), "v05", "v00", "x", f.now); !errors.Is(err, exile.ErrAlreadyPetitioned) {
		t.Fatalf("duplicate check order: %v", err)
	}
}

func TestVoteGuiltyAfterTenApprovals(t *testing.T) {
	f := newFixture(t, 10)
	f.chars["bad"] = corecharacter.Character{ID: "bad", Name: "bad", JobLevel: 50}
	f.petition(t, "app", "bad")
	ctx := context.Background()
	if _, err := f.svc.Vote(ctx, "low", "bad", true, f.now); !errors.Is(err, exile.ErrJobLevelTooLow) {
		t.Fatalf("low voter: %v", err)
	}
	if _, err := f.svc.Vote(ctx, "bad", "bad", true, f.now); !errors.Is(err, exile.ErrTargetCannotVote) {
		t.Fatalf("target vote: %v", err)
	}
	if _, err := f.svc.Vote(ctx, "app", "bad", true, f.now); !errors.Is(err, exile.ErrAlreadyVoted) {
		t.Fatalf("applicant approval: %v", err)
	}
	if _, err := f.svc.Vote(ctx, "v00", "nobody", true, f.now); !errors.Is(err, exile.ErrPetitionNotFound) {
		t.Fatalf("unknown petition: %v", err)
	}
	for i := range 8 {
		r, err := f.svc.Vote(ctx, fmt.Sprintf("v%02d", i), "bad", true, f.now)
		if err != nil || r.Verdict != "" || r.Approvals != i+2 {
			t.Fatalf("vote %d: %+v %v", i, r, err)
		}
	}
	if _, err := f.svc.Vote(ctx, "v00", "bad", false, f.now); !errors.Is(err, exile.ErrAlreadyVoted) {
		t.Fatalf("double vote: %v", err)
	}
	r, err := f.svc.Vote(ctx, "v08", "bad", true, f.now)
	if err != nil || r.Verdict != exile.VerdictGuilty || r.Approvals != 10 || len(f.repo.petitions) != 0 || f.sleeps.set["bad"] != time.Hour || len(f.sleeps.set) != 1 || len(f.repo.sentences) != 0 {
		t.Fatalf("guilty verdict: %+v %v %v", r, err, f.sleeps.set)
	}
}

func TestVoteAcquittalSentencesApplicant(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		f := newFixture(t, 10)
		f.petition(t, "app", "bad")
		if deleted {
			delete(f.chars, "app")
		}
		var r exile.VoteResult
		var err error
		for i := range 10 {
			r, err = f.svc.Vote(context.Background(), fmt.Sprintf("v%02d", i), "bad", false, f.now)
			if err != nil {
				t.Fatal(err)
			}
		}
		if r.Verdict != exile.VerdictAcquitted || r.Oppositions != 10 || r.Approvals != 1 || len(f.repo.petitions) != 0 {
			t.Fatalf("acquittal: %+v", r)
		}
		if want := map[string]time.Duration{"app": 5 * 24 * time.Hour}; deleted {
			if len(f.sleeps.set) != 0 {
				t.Fatalf("deleted applicant sentenced: %v", f.sleeps.set)
			}
		} else if f.sleeps.set["app"] != want["app"] || len(f.sleeps.set) != 1 {
			t.Fatalf("applicant sleep: %v", f.sleeps.set)
		}
	}
}

// SQL is authoritative: a timer failure (or a stop before projection) keeps
// the committed verdict and its sentence row, and the next board command
// projects the same deadline. Expired sentences are dropped unprojected.
func TestVerdictSurvivesTimerFailureAndIsProjectedLater(t *testing.T) {
	f := newFixture(t, 9)
	f.approveUntilLast(t)
	f.sleeps.err = errors.New("valkey down")
	r, err := f.svc.Vote(context.Background(), "v08", "bad", true, f.now)
	if err != nil || r.Verdict != exile.VerdictGuilty || len(f.repo.petitions) != 0 || len(f.repo.sentences) != 1 || len(f.sleeps.set) != 0 {
		t.Fatalf("committed verdict lost: %+v %v %+v", r, err, f.repo.sentences)
	}
	f.sleeps.err = nil
	f.chars["v09"] = corecharacter.Character{ID: "v09", Name: "v09", JobLevel: 50}
	f.chars["other"] = corecharacter.Character{ID: "other", Name: "other"}
	f.now = f.now.Add(10 * time.Minute)
	f.petition(t, "v09", "other")
	if f.sleeps.set["bad"] != time.Hour || len(f.repo.sentences) != 0 {
		t.Fatalf("sentence not projected on next command: %v %+v", f.sleeps.set, f.repo.sentences)
	}
	f.repo.sentences = []exile.Sentence{{CharacterID: "app", SleepUntil: f.now.Add(-time.Second)}}
	if _, err := f.svc.Vote(context.Background(), "v00", "other", true, f.now); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.sleeps.set["app"]; ok || len(f.repo.sentences) != 0 {
		t.Fatalf("expired sentence projected or kept: %v %+v", f.sleeps.set, f.repo.sentences)
	}
}

// The lock uses the last write to the whole board, and an applicant's
// opposition withdraws every own petition even when voting on another.
func TestWithdrawalUsesBoardWriteTime(t *testing.T) {
	f := newFixture(t, 4)
	ctx := context.Background()
	f.petition(t, "app", "bad")
	f.petition(t, "app", "v00")
	f.petition(t, "v01", "v02")
	late := f.now.Add(3 * time.Hour)
	// Another character's vote on an unrelated petition extends the lock.
	if _, err := f.svc.Vote(ctx, "v03", "v02", true, f.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Vote(ctx, "app", "v02", false, late); !errors.Is(err, exile.ErrCancelLocked) || len(f.repo.petitions) != 3 {
		t.Fatalf("lock ignored board write: %v", err)
	}
	r, err := f.svc.Vote(ctx, "app", "v02", false, f.now.Add(5*time.Hour))
	if err != nil || len(r.Withdrawn) != 2 || r.Petition == nil || r.Oppositions != 1 || len(f.repo.petitions) != 1 || f.repo.updated != f.now.Add(5*time.Hour) {
		t.Fatalf("withdrawal with vote: %+v %v %+v", r, err, f.repo.petitions)
	}
	if involved, err := f.svc.IsInvolved(ctx, "app"); err != nil || !involved {
		t.Fatalf("voter is not involved: %v %v", involved, err)
	}
	if involved, err := f.svc.IsInvolved(ctx, "bad"); err != nil || involved {
		t.Fatalf("withdrawn target still involved: %v %v", involved, err)
	}
}

func TestBoardVisibleOnlyToEligible(t *testing.T) {
	f := newFixture(t, 0)
	f.petition(t, "app", "bad")
	if b, err := f.svc.GetBoard(context.Background(), "low"); err != nil || b.Eligible || len(b.Petitions) != 0 {
		t.Fatalf("ineligible board: %+v %v", b, err)
	}
	if b, err := f.svc.GetBoard(context.Background(), "app"); err != nil || !b.Eligible || len(b.Petitions) != 1 {
		t.Fatalf("eligible board: %+v %v", b, err)
	}
}

func (f *fixture) approveUntilLast(t *testing.T) {
	t.Helper()
	f.petition(t, "app", "bad")
	for i := range 8 {
		if _, err := f.svc.Vote(context.Background(), fmt.Sprintf("v%02d", i), "bad", true, f.now); err != nil {
			t.Fatal(err)
		}
	}
}

// A verdict whose board write or commit fails never touches the timer and
// leaves no sentence, so the sentenced character's sleep stays as it was.
func TestVerdictFailureAfterTimerKeepsPriorSleep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prior time.Duration
		fail  func(*fixture)
	}{
		{"commit fails, no prior sleep", 0, func(f *fixture) { f.commit = errors.New("commit failed") }},
		{"commit fails, longer prior sleep", 2 * time.Hour, func(f *fixture) { f.commit = errors.New("commit failed") }},
		{"board write fails", 0, func(f *fixture) { f.repo.touchErr = errors.New("touch failed") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, 9)
			f.approveUntilLast(t)
			if tc.prior > 0 {
				f.sleeps.set["bad"] = tc.prior
			}
			tc.fail(f)
			if _, err := f.svc.Vote(context.Background(), "v08", "bad", true, f.now); err == nil {
				t.Fatal("failed verdict reported success")
			}
			if got, ok := f.sleeps.set["bad"]; got != tc.prior || ok != (tc.prior > 0) || len(f.repo.sentences) != 0 {
				t.Fatalf("sleep after rollback = %v (set=%v), want %v", got, ok, tc.prior)
			}
			if len(f.repo.petitions) != 1 || len(f.repo.petitions[0].Votes) != 9 {
				t.Fatalf("verdict not rolled back: %+v", f.repo.petitions)
			}
		})
	}
}

// renamingChars renames the target right after the unlocked name lookup,
// as a concurrent ChangeName committing first would.
type renamingChars struct{ chars }

func (c renamingChars) FindByName(ctx context.Context, name string) (corecharacter.Character, error) {
	found, err := c.chars.FindByName(ctx, name)
	if err == nil {
		renamed := found
		renamed.Name = "renamed"
		c.chars[found.ID] = renamed
	}
	return found, err
}

func TestPetitionRejectsTargetRenamedBeforeLock(t *testing.T) {
	f := newFixture(t, 0)
	svc := exile.NewService(f.repo, renamingChars{f.chars}, f.sleeps, memTx{f.repo, &f.commit}, nil)
	if _, err := svc.Petition(context.Background(), "app", "bad", "x", f.now); !errors.Is(err, exile.ErrTargetNotFound) || len(f.repo.petitions) != 0 {
		t.Fatalf("petition kept a stale name: %v %+v", err, f.repo.petitions)
	}
}

// Projection reads pending sentences only under the board lock, so it is
// ordered after every committed verdict and cannot write an older deadline.
func TestSentenceProjectionHoldsBoardLock(t *testing.T) {
	f := newFixture(t, 10)
	f.approveUntilLast(t)
	f.repo.events = nil
	if _, err := f.svc.Vote(context.Background(), "v08", "bad", true, f.now); err != nil {
		t.Fatal(err)
	}
	locked := false
	for _, e := range f.repo.events {
		switch e {
		case "begin", "end":
			locked = false
		case "lock":
			locked = true
		case "list":
			if !locked {
				t.Fatalf("sentences read outside the board lock: %v", f.repo.events)
			}
		}
	}
	if !slices.Contains(f.repo.events, "list") || f.sleeps.set["bad"] != time.Hour {
		t.Fatalf("verdict not projected: %v %v", f.repo.events, f.sleeps.set)
	}
}

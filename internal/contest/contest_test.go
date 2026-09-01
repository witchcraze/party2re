package contest_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Mock repositories

type mockCharRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	char, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return char, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[char.ID] = char
	return nil
}

type mockContestRepo struct {
	mu      sync.Mutex
	photos  map[string]contest.Photo
	rounds  map[int]contest.ContestRound
	entries map[string]contest.ContestEntry
	votes   map[string]contest.ContestVote
	legends []contest.ContestLegend
}

func newMockContestRepo() *mockContestRepo {
	return &mockContestRepo{
		photos:  make(map[string]contest.Photo),
		rounds:  make(map[int]contest.ContestRound),
		entries: make(map[string]contest.ContestEntry),
		votes:   make(map[string]contest.ContestVote),
	}
}

func (m *mockContestRepo) SavePhoto(ctx context.Context, photo contest.Photo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.photos[photo.ID] = photo
	return nil
}

func (m *mockContestRepo) FindPhotoByID(ctx context.Context, id string) (contest.Photo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.photos[id]
	if !ok {
		return contest.Photo{}, contest.ErrPhotoNotFound
	}
	return p, nil
}

func (m *mockContestRepo) FindPhotoByIDForUpdate(ctx context.Context, id string) (contest.Photo, error) {
	return m.FindPhotoByID(ctx, id)
}

func (m *mockContestRepo) ListPhotosByCharacterID(ctx context.Context, charID string) ([]contest.Photo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []contest.Photo
	for _, p := range m.photos {
		if p.CharacterID == charID {
			res = append(res, p)
		}
	}
	return res, nil
}

func (m *mockContestRepo) CountPhotosByCharacterID(ctx context.Context, charID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, p := range m.photos {
		if p.CharacterID == charID {
			count++
		}
	}
	return count, nil
}

func (m *mockContestRepo) DeletePhoto(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.photos, id)
	return nil
}

func (m *mockContestRepo) GetRoundByNumber(ctx context.Context, round int) (contest.ContestRound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rounds[round]
	if !ok {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return r, nil
}

func (m *mockContestRepo) GetRoundByNumberForUpdate(ctx context.Context, round int) (contest.ContestRound, error) {
	return m.GetRoundByNumber(ctx, round)
}

func (m *mockContestRepo) GetActiveRound(ctx context.Context) (contest.ContestRound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rounds {
		if r.Status == contest.StatusActive {
			return r, nil
		}
	}
	return contest.ContestRound{}, contest.ErrContestNotFound
}

func (m *mockContestRepo) GetActiveRoundForUpdate(ctx context.Context) (contest.ContestRound, error) {
	return m.GetActiveRound(ctx)
}

func (m *mockContestRepo) GetPreparingRound(ctx context.Context) (contest.ContestRound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rounds {
		if r.Status == contest.StatusPreparing {
			return r, nil
		}
	}
	return contest.ContestRound{}, contest.ErrContestNotFound
}

func (m *mockContestRepo) GetPreparingRoundForUpdate(ctx context.Context) (contest.ContestRound, error) {
	return m.GetPreparingRound(ctx)
}

func (m *mockContestRepo) GetLatestSettledRound(ctx context.Context) (contest.ContestRound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var latest contest.ContestRound
	found := false
	for _, r := range m.rounds {
		if r.Status == contest.StatusSettled {
			if !found || r.Round > latest.Round {
				latest = r
				found = true
			}
		}
	}
	if !found {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return latest, nil
}

func (m *mockContestRepo) SaveRound(ctx context.Context, round contest.ContestRound) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rounds[round.Round] = round
	return nil
}

func (m *mockContestRepo) SaveEntry(ctx context.Context, entry contest.ContestEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[entry.ID] = entry
	return nil
}

func (m *mockContestRepo) FindEntryByID(ctx context.Context, id string) (contest.ContestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return contest.ContestEntry{}, contest.ErrEntryNotFound
	}
	return e, nil
}

func (m *mockContestRepo) FindEntryByIDForUpdate(ctx context.Context, id string) (contest.ContestEntry, error) {
	return m.FindEntryByID(ctx, id)
}

func (m *mockContestRepo) FindEntryByRoundAndCharacter(ctx context.Context, round int, charID string) (contest.ContestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Round == round && e.CharacterID == charID {
			return e, nil
		}
	}
	return contest.ContestEntry{}, contest.ErrEntryNotFound
}

func (m *mockContestRepo) FindEntryByRoundAndTitle(ctx context.Context, round int, title string) (contest.ContestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Round == round && e.Title == title {
			return e, nil
		}
	}
	return contest.ContestEntry{}, contest.ErrEntryNotFound
}

func (m *mockContestRepo) ListEntriesByRound(ctx context.Context, round int) ([]contest.ContestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []contest.ContestEntry
	for _, e := range m.entries {
		if e.Round == round {
			res = append(res, e)
		}
	}
	return res, nil
}

func (m *mockContestRepo) CountEntriesByRound(ctx context.Context, round int) (int, error) {
	entries, err := m.ListEntriesByRound(ctx, round)
	return len(entries), err
}

func (m *mockContestRepo) SaveVote(ctx context.Context, vote contest.ContestVote) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.votes[vote.ID] = vote
	return nil
}

func (m *mockContestRepo) HasVotedInRound(ctx context.Context, round int, voterCharID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.votes {
		if v.Round == round && v.VoterCharacterID == voterCharID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockContestRepo) ListVotesByRound(ctx context.Context, round int) ([]contest.ContestVote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []contest.ContestVote
	for _, v := range m.votes {
		if v.Round == round {
			res = append(res, v)
		}
	}
	return res, nil
}

func (m *mockContestRepo) ListVotesByEntryID(ctx context.Context, entryID string) ([]contest.ContestVote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []contest.ContestVote
	for _, v := range m.votes {
		if v.EntryID == entryID {
			res = append(res, v)
		}
	}
	return res, nil
}

func (m *mockContestRepo) IncrementEntryVotes(ctx context.Context, entryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[entryID]
	if !ok {
		return contest.ErrEntryNotFound
	}
	e.Votes++
	m.entries[entryID] = e
	return nil
}

func (m *mockContestRepo) SaveLegend(ctx context.Context, legend contest.ContestLegend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.legends = append(m.legends, legend)
	return nil
}

func (m *mockContestRepo) ListLegends(ctx context.Context, limit, offset int) ([]contest.ContestLegend, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := len(m.legends)
	if offset >= len(m.legends) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(m.legends) {
		end = len(m.legends)
	}
	return m.legends[offset:end], total, nil
}

type mockGuildService struct {
	mu     sync.Mutex
	points map[string]int64
}

func newMockGuildService() *mockGuildService {
	return &mockGuildService{points: make(map[string]int64)}
}

func (m *mockGuildService) AddGuildExp(ctx context.Context, characterID string, exp int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.points[characterID] += exp
	return nil
}

type mockNewsPublisher struct {
	mu   sync.Mutex
	news []string
}

func newMockNewsPublisher() *mockNewsPublisher {
	return &mockNewsPublisher{}
}

func (m *mockNewsPublisher) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.news = append(m.news, content)
	return nil
}

// Tests

func TestDialogueAndOverview(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	svc, err := contest.NewService(charRepo, contestRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	dialogue := svc.GetDialogue()
	if dialogue.NPCName != "@ワコール" {
		t.Errorf("expected NPCName @ワコール, got %s", dialogue.NPCName)
	}
	if len(dialogue.Phrases) == 0 {
		t.Error("expected dialogue phrases, got empty")
	}

	ctx := context.Background()
	overview, err := svc.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview error: %v", err)
	}
	if overview.MinEntries != contest.MinEntriesForContest {
		t.Errorf("expected MinEntries %d, got %d", contest.MinEntriesForContest, overview.MinEntries)
	}
}

func TestSaveAndListPhotos(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	char := corecharacter.Character{ID: "char-1", Name: "Hero", Money: 1000}
	charRepo.Update(context.Background(), char)

	svc, err := contest.NewService(charRepo, contestRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Title validation tests
	invalidTitles := []string{"", "   ", "Title,with,comma", "Title;semi", "Title\"quote", "Title<tag>", "Title@at", "Title＠fullat"}
	for _, it := range invalidTitles {
		_, err := svc.SavePhoto(ctx, char.ID, it, "Town", "http://image.png", "Caption", "")
		if !errors.Is(err, contest.ErrInvalidTitle) {
			t.Errorf("expected ErrInvalidTitle for title '%s', got %v", it, err)
		}
	}

	// Successful save
	photo, err := svc.SavePhoto(ctx, char.ID, "A Beautiful Sunset", "Forest Stage", "http://image.png", "Sun going down", "")
	if err != nil {
		t.Fatalf("SavePhoto failed: %v", err)
	}
	if photo.ID == "" || photo.CharacterID != char.ID || photo.Title != "A Beautiful Sunset" {
		t.Errorf("unexpected photo: %+v", photo)
	}

	// List photos
	photos, err := svc.ListPhotos(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListPhotos failed: %v", err)
	}
	if len(photos) != 1 || photos[0].ID != photo.ID {
		t.Errorf("unexpected photos: %+v", photos)
	}

	// Capacity limit (max 20)
	for i := 2; i <= contest.MaxPhotosPerCharacter; i++ {
		_, err := svc.SavePhoto(ctx, char.ID, fmt.Sprintf("Photo %d", i), "Town", "", "", "")
		if err != nil {
			t.Fatalf("failed to save photo %d: %v", i, err)
		}
	}

	// 21st photo should fail
	_, err = svc.SavePhoto(ctx, char.ID, "Photo 21", "Town", "", "", "")
	if !errors.Is(err, contest.ErrMaxPhotosReached) {
		t.Errorf("expected ErrMaxPhotosReached, got %v", err)
	}

	// Delete photo
	err = svc.DeletePhoto(ctx, char.ID, photo.ID)
	if err != nil {
		t.Fatalf("DeletePhoto failed: %v", err)
	}

	// Non-owner delete
	err = svc.DeletePhoto(ctx, "char-other", "non-existent")
	if !errors.Is(err, contest.ErrPhotoNotFound) {
		t.Errorf("expected ErrPhotoNotFound, got %v", err)
	}
}

func TestContestEntryAndConsecutiveRules(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	char1 := corecharacter.Character{ID: "char-1", Name: "Hero", Money: 1000}
	char2 := corecharacter.Character{ID: "char-2", Name: "Mage", Money: 1000}
	charRepo.Update(context.Background(), char1)
	charRepo.Update(context.Background(), char2)

	svc, err := contest.NewService(charRepo, contestRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	photo1, _ := svc.SavePhoto(ctx, char1.ID, "Photo One", "Town", "url1", "caption1", "")
	photo2, _ := svc.SavePhoto(ctx, char2.ID, "Photo Two", "Town", "url2", "caption2", "")

	// 1. Submit entry for char1 into Round 1 (auto-created preparing round)
	entry1, err := svc.EnterContest(ctx, char1.ID, photo1.ID, "My First Contest Entry")
	if err != nil {
		t.Fatalf("EnterContest failed: %v", err)
	}
	if entry1.Round != 1 || entry1.CharacterID != char1.ID || entry1.Title != "My First Contest Entry" {
		t.Errorf("unexpected entry1: %+v", entry1)
	}

	// 2. Duplicate entry by char1 in same preparing round rejected
	_, err = svc.EnterContest(ctx, char1.ID, photo1.ID, "Another Title")
	if !errors.Is(err, contest.ErrAlreadyEntered) {
		t.Errorf("expected ErrAlreadyEntered, got %v", err)
	}

	// 3. Duplicate title by char2 in same preparing round rejected
	_, err = svc.EnterContest(ctx, char2.ID, photo2.ID, "My First Contest Entry")
	if !errors.Is(err, contest.ErrDuplicateTitle) {
		t.Errorf("expected ErrDuplicateTitle, got %v", err)
	}

	// 4. Submit unique entry for char2
	entry2, err := svc.EnterContest(ctx, char2.ID, photo2.ID, "My Second Contest Entry")
	if err != nil {
		t.Fatalf("EnterContest for char2 failed: %v", err)
	}
	if entry2.CharacterID != char2.ID {
		t.Errorf("unexpected entry2: %+v", entry2)
	}

	// 5. Activate Round 1 and prepare Round 2 to test consecutive entry restriction
	round1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	round1.Status = contest.StatusActive
	contestRepo.SaveRound(ctx, round1)

	round2 := contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: round1.EndTime,
		EndTime:   round1.EndTime.Add(contest.ContestCycleDays * 24 * time.Hour),
	}
	contestRepo.SaveRound(ctx, round2)

	// char1 has an entry in active Round 1 -> attempting to enter Round 2 (preparing) should fail!
	photo1b, _ := svc.SavePhoto(ctx, char1.ID, "Photo 1B", "Town", "url1b", "", "")
	_, err = svc.EnterContest(ctx, char1.ID, photo1b.ID, "Consecutive Attempt")
	if !errors.Is(err, contest.ErrConsecutiveEntryDisallowed) {
		t.Errorf("expected ErrConsecutiveEntryDisallowed, got %v", err)
	}
}

func TestVotingRules(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	author := corecharacter.Character{ID: "author", Name: "Author", Money: 1000}
	voter1 := corecharacter.Character{ID: "voter1", Name: "Voter One", Money: 1000}
	voter2 := corecharacter.Character{ID: "voter2", Name: "Voter Two", Money: 1000}
	charRepo.Update(context.Background(), author)
	charRepo.Update(context.Background(), voter1)
	charRepo.Update(context.Background(), voter2)

	svc, err := contest.NewService(charRepo, contestRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	photo, _ := svc.SavePhoto(ctx, author.ID, "Cool Shot", "Cave", "url", "", "")
	entry, _ := svc.EnterContest(ctx, author.ID, photo.ID, "Epic Battle Scene")

	// 1. Voting when contest is preparing (not active) should fail
	_, err = svc.Vote(ctx, voter1.ID, entry.ID, "Great photo!")
	if !errors.Is(err, contest.ErrContestNotActive) {
		t.Errorf("expected ErrContestNotActive, got %v", err)
	}

	// 2. Activate contest
	round, _ := contestRepo.GetRoundByNumber(ctx, 1)
	round.Status = contest.StatusActive
	contestRepo.SaveRound(ctx, round)

	// 3. Self-voting disallowed
	_, err = svc.Vote(ctx, author.ID, entry.ID, "I vote for me")
	if !errors.Is(err, contest.ErrSelfVoteDisallowed) {
		t.Errorf("expected ErrSelfVoteDisallowed, got %v", err)
	}

	// 4. Voter 1 votes successfully
	vote1, err := svc.Vote(ctx, voter1.ID, entry.ID, "Amazing composition!")
	if err != nil {
		t.Fatalf("Vote failed: %v", err)
	}
	if vote1.VoterCharacterID != voter1.ID || vote1.EntryID != entry.ID {
		t.Errorf("unexpected vote: %+v", vote1)
	}

	// 5. Voter 1 cannot vote again in same round
	_, err = svc.Vote(ctx, voter1.ID, entry.ID, "Vote again")
	if !errors.Is(err, contest.ErrAlreadyVoted) {
		t.Errorf("expected ErrAlreadyVoted, got %v", err)
	}

	// 6. Voter 2 votes successfully
	vote2, err := svc.Vote(ctx, voter2.ID, entry.ID, "Nice color!")
	if err != nil {
		t.Fatalf("Vote 2 failed: %v", err)
	}
	if vote2.VoterCharacterID != voter2.ID {
		t.Errorf("unexpected vote2: %+v", vote2)
	}

	// Check entry votes counter
	updatedEntry, _ := contestRepo.FindEntryByID(ctx, entry.ID)
	if updatedEntry.Votes != 2 {
		t.Errorf("expected 2 votes, got %d", updatedEntry.Votes)
	}
}

func TestSettlementPostponementWhenUnderMinEntries(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := contest.NewService(charRepo, contestRepo, contest.WithNowFunc(func() time.Time { return now }))

	ctx := context.Background()

	// Create active round with only 2 entries (< 5)
	activeRound := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour), // Expired
	}
	contestRepo.SaveRound(ctx, activeRound)

	for i := 1; i <= 2; i++ {
		char := corecharacter.Character{ID: fmt.Sprintf("char-%d", i), Name: fmt.Sprintf("Hero%d", i)}
		charRepo.Update(ctx, char)
		entry := contest.ContestEntry{
			ID:          fmt.Sprintf("entry-%d", i),
			Round:       1,
			CharacterID: char.ID,
			Title:       fmt.Sprintf("Title %d", i),
		}
		contestRepo.SaveEntry(ctx, entry)
	}

	res, err := svc.SettleContest(ctx, false)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}
	if !res.Postponed {
		t.Errorf("expected contest to be postponed, got %+v", res)
	}
	if res.PrizesDistributed {
		t.Error("expected prizes NOT to be distributed on postponement")
	}

	// Verify round end time was extended
	updatedRound, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if !updatedRound.EndTime.After(now) {
		t.Errorf("expected updated round end time in future, got %v", updatedRound.EndTime)
	}
}

func TestSettlementAndPrizeDistribution(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	guildSvc := newMockGuildService()
	newsPub := newMockNewsPublisher()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithGuildService(guildSvc),
		contest.WithNewsPublisher(newsPub),
		contest.WithNowFunc(func() time.Time { return now }),
	)

	ctx := context.Background()

	// Setup 5 characters and entries
	activeRound := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	}
	contestRepo.SaveRound(ctx, activeRound)

	prepRound := contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: activeRound.EndTime,
		EndTime:   activeRound.EndTime.Add(contest.ContestCycleDays * 24 * time.Hour),
	}
	contestRepo.SaveRound(ctx, prepRound)

	chars := make([]corecharacter.Character, 5)
	entries := make([]contest.ContestEntry, 5)
	voteCounts := []int{25, 18, 12, 5, 2} // 1st, 2nd, 3rd, 4th, 5th

	for i := 0; i < 5; i++ {
		chars[i] = corecharacter.Character{
			ID:          fmt.Sprintf("char-%d", i+1),
			Name:        fmt.Sprintf("Player%d", i+1),
			Money:       1000,
			SmallMedals: 5,
		}
		charRepo.Update(ctx, chars[i])

		entries[i] = contest.ContestEntry{
			ID:            fmt.Sprintf("entry-%d", i+1),
			Round:         1,
			CharacterID:   chars[i].ID,
			CharacterName: chars[i].Name,
			Title:         fmt.Sprintf("Masterpiece %d", i+1),
			Votes:         voteCounts[i],
			CreatedAt:     now.Add(time.Duration(-i) * time.Hour),
		}
		contestRepo.SaveEntry(ctx, entries[i])
	}

	// Add 3 voters who voted for 1st place (entry-1)
	voters := make([]corecharacter.Character, 3)
	for i := 0; i < 3; i++ {
		voters[i] = corecharacter.Character{
			ID:          fmt.Sprintf("voter-%d", i+1),
			Name:        fmt.Sprintf("Voter%d", i+1),
			SmallMedals: 0,
		}
		charRepo.Update(ctx, voters[i])

		vote := contest.ContestVote{
			ID:                 fmt.Sprintf("vote-%d", i+1),
			Round:              1,
			EntryID:            entries[0].ID,
			VoterCharacterID:   voters[i].ID,
			VoterCharacterName: voters[i].Name,
			Comment:            "Awesome!",
		}
		contestRepo.SaveVote(ctx, vote)
	}

	// Settle contest
	res, err := svc.SettleContest(ctx, false)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}

	if res.Postponed || !res.PrizesDistributed {
		t.Errorf("unexpected settlement result: %+v", res)
	}
	if res.VotersRewarded != 3 {
		t.Errorf("expected 3 voters rewarded, got %d", res.VotersRewarded)
	}

	// Verify 1st place prizes: +15000 G, +10 Medals, +700 GP
	firstChar, _ := charRepo.FindByID(ctx, chars[0].ID)
	if firstChar.Money != 1000+15000 {
		t.Errorf("expected 16000 G, got %d", firstChar.Money)
	}
	if firstChar.SmallMedals != 5+10 {
		t.Errorf("expected 15 medals, got %d", firstChar.SmallMedals)
	}
	if guildSvc.points[chars[0].ID] != 700 {
		t.Errorf("expected 700 guild points, got %d", guildSvc.points[chars[0].ID])
	}

	// Verify 2nd place prizes: +7000 G, +6 Medals, +300 GP
	secondChar, _ := charRepo.FindByID(ctx, chars[1].ID)
	if secondChar.Money != 1000+7000 {
		t.Errorf("expected 8000 G, got %d", secondChar.Money)
	}
	if secondChar.SmallMedals != 5+6 {
		t.Errorf("expected 11 medals, got %d", secondChar.SmallMedals)
	}

	// Verify 3rd place prizes: +3000 G, +3 Medals, +100 GP
	thirdChar, _ := charRepo.FindByID(ctx, chars[2].ID)
	if thirdChar.Money != 1000+3000 {
		t.Errorf("expected 4000 G, got %d", thirdChar.Money)
	}
	if thirdChar.SmallMedals != 5+3 {
		t.Errorf("expected 8 medals, got %d", thirdChar.SmallMedals)
	}

	// Verify voters received 1 medal each
	for i := 0; i < 3; i++ {
		v, _ := charRepo.FindByID(ctx, voters[i].ID)
		if v.SmallMedals != 1 {
			t.Errorf("expected voter %d to receive 1 medal, got %d", i+1, v.SmallMedals)
		}
	}

	// Verify Hall of Fame legend recorded
	legends, err := svc.GetLegends(ctx, 10, 0)
	if err != nil || len(legends.Items) != 1 || legends.Total != 1 {
		t.Fatalf("expected 1 legend, got %d (err: %v)", len(legends.Items), err)
	}
	if legends.Items[0].Round != 1 || legends.Items[0].Title != entries[0].Title {
		t.Errorf("unexpected legend: %+v", legends.Items[0])
	}

	// Verify Round 1 is settled, Round 2 is now active, Round 3 is preparing
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected Round 1 settled, got %s", r1.Status)
	}

	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected Round 2 active, got %s", r2.Status)
	}

	r3, _ := contestRepo.GetRoundByNumber(ctx, 3)
	if r3.Status != contest.StatusPreparing {
		t.Errorf("expected Round 3 preparing, got %s", r3.Status)
	}

	// Verify news announcement
	if len(newsPub.news) == 0 {
		t.Error("expected news announcement to be published")
	}
}

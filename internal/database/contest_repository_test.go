package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

func TestContestRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "ContestChar")
	if err != nil {
		t.Fatal(err)
	}

	voter, err := database.CreateTestCharacter(ctx, db, "ContestVoter")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewContestRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Photo CRUD
	photoID := id.New()
	photo := contest.Photo{
		ID:          photoID,
		CharacterID: char.ID,
		Title:       "Scenic Mountain Peak",
		Location:    "Mountain Stage 3",
		ImageURL:    "http://example.com/photo.png",
		Caption:     "A lovely vista",
		Metadata:    `{"filter":"sepia"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := repo.SavePhoto(ctx, photo); err != nil {
		t.Fatalf("SavePhoto failed: %v", err)
	}

	foundPhoto, err := repo.FindPhotoByID(ctx, photoID)
	if err != nil {
		t.Fatalf("FindPhotoByID failed: %v", err)
	}
	if foundPhoto.Title != photo.Title || foundPhoto.CharacterID != char.ID {
		t.Errorf("unexpected foundPhoto: %+v", foundPhoto)
	}

	photos, err := repo.ListPhotosByCharacterID(ctx, char.ID)
	if err != nil || len(photos) == 0 {
		t.Fatalf("ListPhotosByCharacterID failed: %v", err)
	}

	count, err := repo.CountPhotosByCharacterID(ctx, char.ID)
	if err != nil || count != len(photos) {
		t.Fatalf("CountPhotosByCharacterID failed: count=%d, expected=%d", count, len(photos))
	}

	// 2. Contest Rounds
	roundNum := int(now.Unix()%100000) + 10000
	round := contest.ContestRound{
		Round:     roundNum,
		Status:    contest.StatusPreparing,
		StartTime: now,
		EndTime:   now.Add(10 * 24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.SaveRound(ctx, round); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	foundRound, err := repo.GetRoundByNumber(ctx, roundNum)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	if foundRound.Status != contest.StatusPreparing {
		t.Errorf("expected status preparing, got %s", foundRound.Status)
	}

	// 3. Contest Entry
	entryID := id.New()
	entry := contest.ContestEntry{
		ID:            entryID,
		Round:         roundNum,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		Title:         "Epic Sunrise Over Peaks",
		PhotoID:       photoID,
		ImageURL:      photo.ImageURL,
		Caption:       photo.Caption,
		Votes:         0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := repo.SaveEntry(ctx, entry); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	foundEntry, err := repo.FindEntryByID(ctx, entryID)
	if err != nil {
		t.Fatalf("FindEntryByID failed: %v", err)
	}
	if foundEntry.Title != entry.Title {
		t.Errorf("unexpected foundEntry: %+v", foundEntry)
	}

	byChar, err := repo.FindEntryByRoundAndCharacter(ctx, roundNum, char.ID)
	if err != nil || byChar.ID != entryID {
		t.Fatalf("FindEntryByRoundAndCharacter failed: %v", err)
	}

	byTitle, err := repo.FindEntryByRoundAndTitle(ctx, roundNum, entry.Title)
	if err != nil || byTitle.ID != entryID {
		t.Fatalf("FindEntryByRoundAndTitle failed: %v", err)
	}

	entries, err := repo.ListEntriesByRound(ctx, roundNum)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListEntriesByRound failed: %v", err)
	}

	// 4. Contest Vote & Increment
	voteID := id.New()
	vote := contest.ContestVote{
		ID:                 voteID,
		Round:              roundNum,
		EntryID:            entryID,
		VoterCharacterID:   voter.ID,
		VoterCharacterName: voter.Name,
		Comment:            "Gorgeous colors!",
		CreatedAt:          now,
	}

	if err := repo.SaveVote(ctx, vote); err != nil {
		t.Fatalf("SaveVote failed: %v", err)
	}

	hasVoted, err := repo.HasVotedInRound(ctx, roundNum, voter.ID)
	if err != nil || !hasVoted {
		t.Fatalf("HasVotedInRound failed: %v, hasVoted=%v", err, hasVoted)
	}

	if err := repo.IncrementEntryVotes(ctx, entryID); err != nil {
		t.Fatalf("IncrementEntryVotes failed: %v", err)
	}

	updatedEntry, _ := repo.FindEntryByID(ctx, entryID)
	if updatedEntry.Votes != 1 {
		t.Errorf("expected 1 vote, got %d", updatedEntry.Votes)
	}

	votesByEntry, err := repo.ListVotesByEntryID(ctx, entryID)
	if err != nil || len(votesByEntry) != 1 {
		t.Fatalf("ListVotesByEntryID failed: %v", err)
	}

	// 5. Contest Legend
	legend := contest.ContestLegend{
		Round:         roundNum,
		EntryID:       entryID,
		Title:         entry.Title,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		Votes:         1,
		ImageURL:      entry.ImageURL,
		Caption:       entry.Caption,
		SettledAt:     now,
	}

	if err := repo.SaveLegend(ctx, legend); err != nil {
		t.Fatalf("SaveLegend failed: %v", err)
	}

	legends, total, err := repo.ListLegends(ctx, 10, 0)
	if err != nil || len(legends) == 0 || total < 1 {
		t.Fatalf("ListLegends failed: err=%v, len=%d, total=%d", err, len(legends), total)
	}

	// 6. Delete Photo
	if err := repo.DeletePhoto(ctx, photoID); err != nil {
		t.Fatalf("DeletePhoto failed: %v", err)
	}

	_, err = repo.FindPhotoByID(ctx, photoID)
	if !errors.Is(err, contest.ErrPhotoNotFound) {
		t.Errorf("expected ErrPhotoNotFound, got %v", err)
	}
}

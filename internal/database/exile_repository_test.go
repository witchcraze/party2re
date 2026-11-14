package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/exile"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/testutil"
)

func openExileTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close exile test db: %v", err)
		}
	})
	return db
}

func TestExileBoard_Integration(t *testing.T) {
	ctx := context.Background()
	db := openExileTestDB(t)
	repo, err := database.NewExileRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	var cs []corecharacter.Character
	for i := range 12 {
		c, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Ex%d", i))
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, c)
		if _, err := db.ExecContext(ctx, "UPDATE characters SET job_level = ? WHERE id = ?", exile.MinJobLevel, c.ID); err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c)
	}
	sleeps := timer.NewService(nil)
	svc := exile.NewService(repo, charRepo, sleeps, database.NewTransactionProvider(db), nil)
	applicant, target, voters := cs[0], cs[1], cs[2:]
	now := time.Now().UTC().Truncate(time.Microsecond)

	p, err := svc.Petition(ctx, applicant.ID, target.Name, "暴言", now)
	if err != nil {
		t.Fatal(err)
	}
	board, err := svc.GetBoard(ctx, voters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var stored *exile.Petition
	for i := range board.Petitions {
		if board.Petitions[i].ID == p.ID {
			stored = &board.Petitions[i]
		}
	}
	if stored == nil || stored.TargetName != target.Name || stored.Reason != "暴言" || len(stored.Votes) != 1 || stored.Votes[0].VoterID != applicant.ID || !stored.Votes[0].Approve {
		t.Fatalf("petition not persisted: %+v", stored)
	}
	if updated, err := repo.LockBoard(ctx); err != nil || updated.Before(now) {
		t.Fatalf("board time not written: %v %v", updated, err)
	}
	if involved, err := repo.IsInvolved(ctx, target.ID); err != nil || !involved {
		t.Fatalf("target not involved: %v %v", involved, err)
	}

	// Ten concurrent approvals: nine are needed, so exactly one reaches the verdict.
	var verdicts, rejected atomic.Int32
	testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: len(voters), OpsPerWorker: 1}, func(worker, _ int) error {
		r, err := svc.Vote(ctx, voters[worker].ID, target.Name, true, now)
		switch {
		case errors.Is(err, exile.ErrPetitionNotFound):
			rejected.Add(1)
			return nil
		case err != nil:
			return err
		case r.Verdict == exile.VerdictGuilty:
			verdicts.Add(1)
		}
		return nil
	})
	if verdicts.Load() != 1 || rejected.Load() != 1 {
		t.Fatalf("verdicts=%d rejected=%d", verdicts.Load(), rejected.Load())
	}
	if remaining, err := sleeps.GetRemainingLock(ctx, timer.CategorySleep, target.ID); err != nil || remaining <= 59*time.Minute {
		t.Fatalf("guilty sleep: %v %v", remaining, err)
	}
	var pending int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM exile_sentences WHERE character_id = ?", target.ID).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("projected sentence row kept: %d %v", pending, err)
	}
	for _, c := range []corecharacter.Character{applicant, target, voters[0]} {
		if involved, err := repo.IsInvolved(ctx, c.ID); err != nil || involved {
			t.Fatalf("resolved petition still involves %s: %v", c.Name, err)
		}
	}
}

// A rename and a petition naming the same character serialize on the character
// row: exactly one succeeds, so no petition keeps a stale target name.
func TestExileRenameAndPetitionRace_Integration(t *testing.T) {
	ctx := context.Background()
	db := openExileTestDB(t)
	repo, err := database.NewExileRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	exiles := exile.NewService(repo, charRepo, timer.NewService(nil), tx, nil)
	names, err := character.NewService(charRepo, character.WithTransactionProvider(tx), character.WithExileChecker(repo))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		applicant, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("ExA%d", i))
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, applicant)
		target, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("ExT%d", i), character.NameChangeCost)
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, target)
		if _, err := db.ExecContext(ctx, "UPDATE characters SET job_level = ? WHERE id = ?", exile.MinJobLevel, applicant.ID); err != nil {
			t.Fatal(err)
		}
		newName := id8(target.ID)
		petitionErr, renameErr := testutil.RunRace2(
			func() error {
				_, err := exiles.Petition(ctx, applicant.ID, target.Name, "race", time.Now().UTC())
				return err
			},
			func() error { _, err := names.ChangeName(ctx, target.ID, newName); return err },
		)
		switch {
		case petitionErr == nil && errors.Is(renameErr, character.ErrExileInvolvedDisallowed):
		case renameErr == nil && errors.Is(petitionErr, exile.ErrTargetNotFound):
		default:
			t.Fatalf("round %d: petition=%v rename=%v", i, petitionErr, renameErr)
		}
	}
}

func id8(id string) string { return "R" + id[:7] }

type exileLookupHook struct {
	exile.CharacterReader
	beforeLookup func(context.Context) error
}

func (r exileLookupHook) FindByName(ctx context.Context, name string) (corecharacter.Character, error) {
	if err := r.beforeLookup(ctx); err != nil {
		return corecharacter.Character{}, err
	}
	return r.CharacterReader.FindByName(ctx, name)
}

func TestExileSelfPetitionAfterApplicantRename_Integration(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items",
		"exile_board", "exile_petitions", "exile_votes", "exile_sentences", "news_articles")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	boardTime := now.Add(-24 * time.Hour)
	if _, err := db.ExecContext(ctx, "INSERT INTO exile_board (id, updated_at) VALUES ('global', ?)", boardTime); err != nil {
		t.Fatal(err)
	}
	applicant, err := database.CreateTestCharacterWithFunds(ctx, db, "ExSelf", character.NameChangeCost+123)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET job_level = ? WHERE id = ?", exile.MinJobLevel, applicant.ID); err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewExileRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	newsRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	newsService, err := notification.NewService(newsRepo, newsRepo)
	if err != nil {
		t.Fatal(err)
	}
	news := exile.NewsPublisherFunc(func(ctx context.Context, category, title, content, author string, at time.Time) error {
		_, err := newsService.PublishNews(ctx, category, title, content, author, at)
		return err
	})
	tx := database.NewTransactionProvider(db)
	names, err := character.NewService(charRepo, character.WithTransactionProvider(tx), character.WithExileChecker(repo), character.WithNewsPublisher(news))
	if err != nil {
		t.Fatal(err)
	}
	newName := id8(applicant.ID)
	reader := exileLookupHook{CharacterReader: charRepo, beforeLookup: func(ctx context.Context) error {
		// Commit the independent rename after the early self-name check, before target lookup.
		_, err := names.ChangeName(ctx, applicant.ID, newName)
		return err
	}}
	svc := exile.NewService(repo, reader, timer.NewService(nil), tx, news)
	if _, err := svc.Petition(ctx, applicant.ID, newName, "self rename race", now); !errors.Is(err, exile.ErrSelfPetition) {
		t.Errorf("petition error = %v, want ErrSelfPetition", err)
	}
	stored, err := charRepo.FindByID(ctx, applicant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != newName || stored.Money != 123 {
		t.Errorf("independently committed rename/fee lost: name=%s money=%d", stored.Name, stored.Money)
	}
	for _, table := range []string{"exile_petitions", "exile_votes", "exile_sentences"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s contains %d rows after rejection", table, count)
		}
	}
	var updated time.Time
	if err := db.QueryRowContext(ctx, "SELECT updated_at FROM exile_board WHERE id = 'global'").Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Equal(boardTime) {
		t.Errorf("board time changed: got %v want %v", updated, boardTime)
	}
	articles, total, err := newsRepo.ListNews(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(articles) != 1 || articles[0].Category != "character" {
		t.Errorf("only the committed rename should publish news: total=%d articles=%+v", total, articles)
	}
}

func TestExileFixtureCleanup(t *testing.T) {
	observer := openExileTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	obsExileRepo, err := database.NewExileRepository(observer)
	if err != nil {
		t.Fatalf("NewExileRepository observer: %v", err)
	}

	unrelatedApp, err := database.CreateTestCharacter(ctx, observer, "UnExA")
	if err != nil {
		t.Fatalf("CreateTestCharacter unrelatedApp: %v", err)
	}
	database.RegisterCharacterCleanup(t, observer, unrelatedApp)
	unrelatedTarget, err := database.CreateTestCharacter(ctx, observer, "UnExT")
	if err != nil {
		t.Fatalf("CreateTestCharacter unrelatedTarget: %v", err)
	}
	database.RegisterCharacterCleanup(t, observer, unrelatedTarget)

	unrelatedPetition := exile.Petition{
		ID:            id.New(),
		ApplicantID:   unrelatedApp.ID,
		ApplicantName: unrelatedApp.Name,
		TargetID:      unrelatedTarget.ID,
		TargetName:    unrelatedTarget.Name,
		Reason:        "unrelated",
		CreatedAt:     now,
		Votes: []exile.Vote{{
			VoterID:   unrelatedApp.ID,
			VoterName: unrelatedApp.Name,
			Approve:   true,
			VotedAt:   now,
		}},
	}
	if err := obsExileRepo.CreatePetition(ctx, unrelatedPetition); err != nil {
		t.Fatalf("CreatePetition unrelated: %v", err)
	}
	if err := obsExileRepo.SaveSentence(ctx, exile.Sentence{CharacterID: unrelatedTarget.ID, SleepUntil: now.Add(time.Hour)}); err != nil {
		t.Fatalf("SaveSentence unrelated: %v", err)
	}

	seed := func(t *testing.T, db *sql.DB, prefix string) (corecharacter.Character, corecharacter.Character, corecharacter.Character, string) {
		t.Helper()
		app, err := database.CreateTestCharacter(ctx, db, prefix+"A")
		if err != nil {
			t.Fatalf("CreateTestCharacter applicant: %v", err)
		}
		database.RegisterCharacterCleanup(t, db, app)
		target, err := database.CreateTestCharacter(ctx, db, prefix+"T")
		if err != nil {
			t.Fatalf("CreateTestCharacter target: %v", err)
		}
		database.RegisterCharacterCleanup(t, db, target)
		voter, err := database.CreateTestCharacter(ctx, db, prefix+"V")
		if err != nil {
			t.Fatalf("CreateTestCharacter voter: %v", err)
		}
		database.RegisterCharacterCleanup(t, db, voter)

		exRepo, err := database.NewExileRepository(db)
		if err != nil {
			t.Fatalf("NewExileRepository: %v", err)
		}
		petitionID := id.New()
		if err := exRepo.CreatePetition(ctx, exile.Petition{
			ID:            petitionID,
			ApplicantID:   app.ID,
			ApplicantName: app.Name,
			TargetID:      target.ID,
			TargetName:    target.Name,
			Reason:        "owned",
			CreatedAt:     now,
			Votes: []exile.Vote{
				{VoterID: app.ID, VoterName: app.Name, Approve: true, VotedAt: now},
				{VoterID: voter.ID, VoterName: voter.Name, Approve: true, VotedAt: now},
			},
		}); err != nil {
			t.Fatalf("CreatePetition owned: %v", err)
		}
		if err := exRepo.SaveSentence(ctx, exile.Sentence{CharacterID: target.ID, SleepUntil: now.Add(time.Hour)}); err != nil {
			t.Fatalf("SaveSentence owned: %v", err)
		}
		return app, target, voter, petitionID
	}

	type exileRun struct {
		app, target, voter corecharacter.Character
		petitionID         string
	}
	var normalRun, earlyRun exileRun
	t.Run("normal scope end", func(t *testing.T) {
		db := openExileTestDB(t)
		normalRun.app, normalRun.target, normalRun.voter, normalRun.petitionID = seed(t, db, "ExNm")
	})
	t.Run("early return path", func(t *testing.T) {
		db := openExileTestDB(t)
		earlyRun.app, earlyRun.target, earlyRun.voter, earlyRun.petitionID = seed(t, db, "ExEr")
		return
	})

	assertRowCount := func(query, arg string, want int) {
		t.Helper()
		var got int
		if err := observer.QueryRowContext(ctx, query, arg).Scan(&got); err != nil {
			t.Fatalf("query %q (%s): %v", query, arg, err)
		}
		if got != want {
			t.Fatalf("query %q (%s) = %d, want %d", query, arg, got, want)
		}
	}

	for _, run := range []exileRun{normalRun, earlyRun} {
		for _, c := range []corecharacter.Character{run.app, run.target, run.voter} {
			assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", c.ID, 0)
			assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", c.ID, 0)
			assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", c.PlayerID, 0)
		}
		assertRowCount("SELECT COUNT(*) FROM exile_petitions WHERE id = ?", run.petitionID, 0)
		assertRowCount("SELECT COUNT(*) FROM exile_votes WHERE petition_id = ?", run.petitionID, 0)
		assertRowCount("SELECT COUNT(*) FROM exile_sentences WHERE character_id = ?", run.target.ID, 0)
	}

	for _, c := range []corecharacter.Character{unrelatedApp, unrelatedTarget} {
		assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", c.ID, 1)
		assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", c.ID, 1)
		assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", c.PlayerID, 1)
	}
	assertRowCount("SELECT COUNT(*) FROM exile_petitions WHERE id = ?", unrelatedPetition.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM exile_votes WHERE petition_id = ?", unrelatedPetition.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM exile_sentences WHERE character_id = ?", unrelatedTarget.ID, 1)
}

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
)

func openMonsterTestDB(t *testing.T) (*sql.DB, *CharacterRepository, *MonsterRepository, *monster.Service) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment: %v", err)
	}
	t.Cleanup(func() {
		//lint:ignore error-swallow test cleanup close
		_ = db.Close()
	})

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatalf("NewCharacterRepository: %v", err)
	}
	monRepo, err := NewMonsterRepository(db)
	if err != nil {
		t.Fatalf("NewMonsterRepository: %v", err)
	}
	svc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(NewTransactionProvider(db)))
	return db, charRepo, monRepo, svc
}

func registerCharacterCleanup(t *testing.T, db *sql.DB, charIDs ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, cid := range charIDs {
			//lint:ignore error-swallow best-effort test row cleanup
			_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", cid)
		}
	})
}

func TestMonsterService_SQL_LocationAndOwnershipMatrix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, _, monRepo, svc := openMonsterTestDB(t)

	// Ensure deterministic ID ordering for sender/recipient lock order coverage
	c1, err := CreateTestCharacter(ctx, db, "MonM1")
	if err != nil {
		t.Fatalf("CreateTestCharacter c1: %v", err)
	}
	c2, err := CreateTestCharacter(ctx, db, "MonM2")
	if err != nil {
		t.Fatalf("CreateTestCharacter c2: %v", err)
	}
	registerCharacterCleanup(t, db, c1.ID, c2.ID)

	// 1. Tame and bring a pet home for c1, then fill c1's box to capacity (50)
	homePet, err := svc.TameMonster(ctx, c1.ID, "slime", "家スラ")
	if err != nil {
		t.Fatalf("TameMonster homePet: %v", err)
	}
	if _, err := svc.BringToHome(ctx, c1.ID, homePet.ID); err != nil {
		t.Fatalf("BringToHome: %v", err)
	}

	now := time.Now().UTC()
	var c1BoxMonster monster.MonsterInstance
	for i := 0; i < monster.BaseBoxCapacity; i++ {
		inst := monster.MonsterInstance{
			ID:          id.New(),
			CharacterID: c1.ID,
			MonsterID:   "slime",
			CustomName:  fmt.Sprintf("箱%d", i),
			Location:    monster.LocationBox,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := monRepo.Save(ctx, inst); err != nil {
			t.Fatalf("Save c1 box monster %d: %v", i, err)
		}
		if i == 0 {
			c1BoxMonster = inst
		}
	}

	// DepositToBox must fail when box is full
	if _, err := svc.DepositToBox(ctx, c1.ID, homePet.ID); !errors.Is(err, monster.ErrBoxFull) {
		t.Fatalf("DepositToBox on full box: got %v, want %v", err, monster.ErrBoxFull)
	}

	// 2. Home pet must be rejected by Rename, SendMonster, and ReleaseMonster (cannot bypass full box)
	if _, err := svc.Rename(ctx, c1.ID, homePet.ID, "改名不可"); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("Rename home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}
	if err := svc.SendMonster(ctx, c1.ID, c2.ID, homePet.ID); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("SendMonster home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}
	if err := svc.ReleaseMonster(ctx, c1.ID, homePet.ID); !errors.Is(err, monster.ErrInvalidLocation) {
		t.Fatalf("ReleaseMonster home pet: got %v, want %v", err, monster.ErrInvalidLocation)
	}

	storedHome, err := monRepo.FindByID(ctx, homePet.ID)
	if err != nil {
		t.Fatalf("FindByID homePet: %v", err)
	}
	if storedHome.CharacterID != c1.ID || storedHome.Location != monster.LocationHome || storedHome.CustomName != "家スラ" {
		t.Fatalf("homePet mutated in SQL after rejected ops: %+v", storedHome)
	}

	// 3. Unowned monsters (both home and box) must be rejected with ErrForbidden
	if _, err := svc.Rename(ctx, c2.ID, homePet.ID, "他家"); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("Rename unowned home pet: got %v, want %v", err, monster.ErrForbidden)
	}
	if _, err := svc.Rename(ctx, c2.ID, c1BoxMonster.ID, "他箱"); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("Rename unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}
	if err := svc.SendMonster(ctx, c2.ID, c1.ID, c1BoxMonster.ID); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("SendMonster unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}
	if err := svc.ReleaseMonster(ctx, c2.ID, c1BoxMonster.ID); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("ReleaseMonster unowned box monster: got %v, want %v", err, monster.ErrForbidden)
	}

	// 4. Boxed monster succeeds for Rename, SendMonster (c1 -> c2 and c2 -> c1 after freeing a slot), and ReleaseMonster
	renamed, err := svc.Rename(ctx, c1.ID, c1BoxMonster.ID, "改名成功")
	if err != nil {
		t.Fatalf("Rename box monster: %v", err)
	}
	if renamed.CustomName != "改名成功" {
		t.Fatalf("Rename custom_name = %q, want %q", renamed.CustomName, "改名成功")
	}

	// Send c1 -> c2 (now c1 box has 49, c2 box has 1)
	if err := svc.SendMonster(ctx, c1.ID, c2.ID, c1BoxMonster.ID); err != nil {
		t.Fatalf("SendMonster c1 -> c2: %v", err)
	}
	// Send back c2 -> c1 (covers reverse character lock order)
	if err := svc.SendMonster(ctx, c2.ID, c1.ID, c1BoxMonster.ID); err != nil {
		t.Fatalf("SendMonster c2 -> c1: %v", err)
	}
	// Release from c1 box
	if err := svc.ReleaseMonster(ctx, c1.ID, c1BoxMonster.ID); err != nil {
		t.Fatalf("ReleaseMonster box monster: %v", err)
	}
	if _, err := monRepo.FindByID(ctx, c1BoxMonster.ID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("FindByID after release: got %v, want %v", err, monster.ErrMonsterNotFound)
	}
}

func TestMonsterService_SQL_ConcurrencyStress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, charRepo, monRepo, svc := openMonsterTestDB(t)
	cfg := GetStressConfig()

	createHighCapChar := func(t *testing.T, prefix string) corecharacter.Character {
		t.Helper()
		c, err := CreateTestCharacter(ctx, db, prefix)
		if err != nil {
			t.Fatalf("CreateTestCharacter %s: %v", prefix, err)
		}
		c.JobLevel = 100
		c.OverMonster = 5
		if err := charRepo.Update(ctx, c); err != nil {
			t.Fatalf("Update high-cap character %s: %v", prefix, err)
		}
		return c
	}

	t.Run("SendVsSend", func(t *testing.T) {
		chars := []corecharacter.Character{
			createHighCapChar(t, "SvS1"),
			createHighCapChar(t, "SvS2"),
			createHighCapChar(t, "SvS3"),
		}
		registerCharacterCleanup(t, db, chars[0].ID, chars[1].ID, chars[2].ID)

		// Order characters so c2.ID < c1.ID < c3.ID, ensuring c1->c2 (sender > recipient)
		// and c1->c3 (sender < recipient) exercise both ID lock ordering directions.
		if chars[0].ID > chars[1].ID {
			chars[0], chars[1] = chars[1], chars[0]
		}
		if chars[1].ID > chars[2].ID {
			chars[1], chars[2] = chars[2], chars[1]
		}
		if chars[0].ID > chars[1].ID {
			chars[0], chars[1] = chars[1], chars[0]
		}
		c2, c1, c3 := chars[0], chars[1], chars[2]

		totalMonsters := cfg.Workers / 2
		if totalMonsters < 1 {
			totalMonsters = 1
		}

		now := time.Now().UTC()
		monsters := make([]monster.MonsterInstance, totalMonsters)
		for i := range monsters {
			inst := monster.MonsterInstance{
				ID:          id.New(),
				CharacterID: c1.ID,
				MonsterID:   "slime",
				CustomName:  fmt.Sprintf("送%d", i),
				Location:    monster.LocationBox,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := monRepo.Save(ctx, inst); err != nil {
				t.Fatalf("Save monster %d: %v", i, err)
			}
			monsters[i] = inst
		}

		sentToC2 := make([]atomic.Int32, totalMonsters)
		sentToC3 := make([]atomic.Int32, totalMonsters)
		var forbiddenCount atomic.Int32

		res := RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
			// Pair workers (2k, 2k+1) on the same monster index while alternating target recipients
			idx := (workerID/2 + op) % totalMonsters
			instID := monsters[idx].ID

			if (workerID+op)%2 == 0 {
				err := svc.SendMonster(ctx, c1.ID, c2.ID, instID)
				if err == nil {
					sentToC2[idx].Add(1)
					return nil
				}
				if errors.Is(err, monster.ErrForbidden) {
					forbiddenCount.Add(1)
					return nil
				}
				return err
			}

			err := svc.SendMonster(ctx, c1.ID, c3.ID, instID)
			if err == nil {
				sentToC3[idx].Add(1)
				return nil
			}
			if errors.Is(err, monster.ErrForbidden) {
				forbiddenCount.Add(1)
				return nil
			}
			return err
		})

		if res.Failures != 0 {
			t.Fatalf("SendVsSend unexpected failures: %+v", res)
		}
		if int64(forbiddenCount.Load()) != res.TotalOps-int64(totalMonsters) {
			t.Fatalf("SendVsSend forbidden rejections = %d, want %d", forbiddenCount.Load(), res.TotalOps-int64(totalMonsters))
		}

		wantC2Count := 0
		wantC3Count := 0
		for i, m := range monsters {
			toC2 := sentToC2[i].Load()
			toC3 := sentToC3[i].Load()
			if toC2+toC3 != 1 {
				t.Fatalf("monster %d (%s) send success count: toC2=%d, toC3=%d, want exactly 1 total", i, m.ID, toC2, toC3)
			}
			wantOwner := c2.ID
			if toC3 == 1 {
				wantOwner = c3.ID
				wantC3Count++
			} else {
				wantC2Count++
			}

			stored, err := monRepo.FindByID(ctx, m.ID)
			if err != nil {
				t.Fatalf("FindByID monster %s: %v", m.ID, err)
			}
			if stored.CharacterID != wantOwner || stored.Location != monster.LocationBox {
				t.Fatalf("monster %s owner/location = (%s, %s), want (%s, box)", m.ID, stored.CharacterID, stored.Location, wantOwner)
			}
		}

		c1Count, err := monRepo.CountByLocation(ctx, c1.ID, monster.LocationBox)
		if err != nil {
			t.Fatalf("CountByLocation c1: %v", err)
		}
		c2Count, err := monRepo.CountByLocation(ctx, c2.ID, monster.LocationBox)
		if err != nil {
			t.Fatalf("CountByLocation c2: %v", err)
		}
		c3Count, err := monRepo.CountByLocation(ctx, c3.ID, monster.LocationBox)
		if err != nil {
			t.Fatalf("CountByLocation c3: %v", err)
		}
		if c1Count != 0 || c2Count != wantC2Count || c3Count != wantC3Count {
			t.Fatalf("final box counts (c1=%d, c2=%d, c3=%d), want (0, %d, %d)", c1Count, c2Count, c3Count, wantC2Count, wantC3Count)
		}
		t.Logf("SendVsSend completed: %d ops in %v (sentToC2=%d, sentToC3=%d, forbidden=%d, 0 deadlocks)",
			res.TotalOps, res.Duration, wantC2Count, wantC3Count, forbiddenCount.Load())
	})

	t.Run("SendVsRelease", func(t *testing.T) {
		sender := createHighCapChar(t, "SvR1")
		recipient := createHighCapChar(t, "SvR2")
		registerCharacterCleanup(t, db, sender.ID, recipient.ID)

		totalMonsters := cfg.Workers / 2
		if totalMonsters < 1 {
			totalMonsters = 1
		}

		now := time.Now().UTC()
		monsters := make([]monster.MonsterInstance, totalMonsters)
		for i := range monsters {
			inst := monster.MonsterInstance{
				ID:          id.New(),
				CharacterID: sender.ID,
				MonsterID:   "dragon",
				CustomName:  fmt.Sprintf("競%d", i),
				Location:    monster.LocationBox,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := monRepo.Save(ctx, inst); err != nil {
				t.Fatalf("Save monster %d: %v", i, err)
			}
			monsters[i] = inst
		}

		sentFlags := make([]atomic.Int32, totalMonsters)
		releasedFlags := make([]atomic.Int32, totalMonsters)

		res := RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
			idx := (workerID/2 + op) % totalMonsters
			instID := monsters[idx].ID

			if (workerID+op)%2 == 0 {
				err := svc.SendMonster(ctx, sender.ID, recipient.ID, instID)
				if err == nil {
					sentFlags[idx].Add(1)
					return nil
				}
				if errors.Is(err, monster.ErrForbidden) || errors.Is(err, monster.ErrMonsterNotFound) {
					return nil
				}
				return err
			}

			err := svc.ReleaseMonster(ctx, sender.ID, instID)
			if err == nil {
				releasedFlags[idx].Add(1)
				return nil
			}
			if errors.Is(err, monster.ErrForbidden) || errors.Is(err, monster.ErrMonsterNotFound) {
				return nil
			}
			return err
		})

		if res.Failures != 0 {
			t.Fatalf("SendVsRelease unexpected failures: %+v", res)
		}

		expectedRecipientCount := 0
		for i, m := range monsters {
			sent := sentFlags[i].Load()
			released := releasedFlags[i].Load()
			if sent+released != 1 {
				t.Fatalf("monster %d (%s) non-exclusive terminal outcome: sent=%d, released=%d", i, m.ID, sent, released)
			}
			stored, err := monRepo.FindByID(ctx, m.ID)
			if sent == 1 {
				expectedRecipientCount++
				if err != nil {
					t.Fatalf("sent monster %s missing in SQL: %v", m.ID, err)
				}
				if stored.CharacterID != recipient.ID || stored.Location != monster.LocationBox {
					t.Fatalf("sent monster %s unexpected state: %+v", m.ID, stored)
				}
			} else {
				if !errors.Is(err, monster.ErrMonsterNotFound) {
					t.Fatalf("released monster %s still present in SQL: %+v (err=%v)", m.ID, stored, err)
				}
			}
		}

		recipientBoxCount, err := monRepo.CountByLocation(ctx, recipient.ID, monster.LocationBox)
		if err != nil {
			t.Fatalf("CountByLocation recipient: %v", err)
		}
		if recipientBoxCount != expectedRecipientCount {
			t.Fatalf("recipient box count = %d, want %d", recipientBoxCount, expectedRecipientCount)
		}
		t.Logf("SendVsRelease completed: %d ops in %v (sent=%d, released=%d)", res.TotalOps, res.Duration, expectedRecipientCount, totalMonsters-expectedRecipientCount)
	})

	t.Run("LastSlotCapacityRace", func(t *testing.T) {
		recipient, err := CreateTestCharacter(ctx, db, "CapRec")
		if err != nil {
			t.Fatalf("CreateTestCharacter recipient: %v", err)
		}
		senders := make([]corecharacter.Character, cfg.Workers)
		cleanupIDs := []string{recipient.ID}
		for w := 0; w < cfg.Workers; w++ {
			senders[w] = createHighCapChar(t, fmt.Sprintf("CapS%d", w))
			cleanupIDs = append(cleanupIDs, senders[w].ID)
		}
		registerCharacterCleanup(t, db, cleanupIDs...)

		now := time.Now().UTC()
		// Pre-fill recipient box to 49/50 (leaving 1 slot) plus 1 home pet
		for i := 0; i < monster.BaseBoxCapacity-1; i++ {
			if err := monRepo.Save(ctx, monster.MonsterInstance{
				ID:          id.New(),
				CharacterID: recipient.ID,
				MonsterID:   "slime",
				CustomName:  fmt.Sprintf("先%d", i),
				Location:    monster.LocationBox,
				CreatedAt:   now,
				UpdatedAt:   now,
			}); err != nil {
				t.Fatalf("Save recipient prefill %d: %v", i, err)
			}
		}
		if err := monRepo.Save(ctx, monster.MonsterInstance{
			ID:          id.New(),
			CharacterID: recipient.ID,
			MonsterID:   "slime",
			CustomName:  "家ペット",
			Location:    monster.LocationHome,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("Save recipient home pet: %v", err)
		}

		contenderMonsters := make([][]monster.MonsterInstance, cfg.Workers)
		for w := 0; w < cfg.Workers; w++ {
			contenderMonsters[w] = make([]monster.MonsterInstance, cfg.OpsPerWorker)
			for op := 0; op < cfg.OpsPerWorker; op++ {
				inst := monster.MonsterInstance{
					ID:          id.New(),
					CharacterID: senders[w].ID,
					MonsterID:   "goblin",
					CustomName:  fmt.Sprintf("枠%d_%d", w, op),
					Location:    monster.LocationBox,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := monRepo.Save(ctx, inst); err != nil {
					t.Fatalf("Save contender [%d][%d]: %v", w, op, err)
				}
				contenderMonsters[w][op] = inst
			}
		}

		var accepted atomic.Int32
		var rejectedFull atomic.Int32

		res := RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
			err := svc.SendMonster(ctx, senders[workerID].ID, recipient.ID, contenderMonsters[workerID][op].ID)
			if err == nil {
				accepted.Add(1)
				return nil
			}
			if errors.Is(err, monster.ErrRecipientBoxFull) {
				rejectedFull.Add(1)
				return nil
			}
			return err
		})

		if res.Failures != 0 {
			t.Fatalf("LastSlotCapacityRace unexpected failures: %+v", res)
		}
		if accepted.Load() != 1 {
			t.Fatalf("accepted into final slot = %d, want 1", accepted.Load())
		}
		if int64(rejectedFull.Load()) != res.TotalOps-1 {
			t.Fatalf("rejectedFull = %d, want %d", rejectedFull.Load(), res.TotalOps-1)
		}

		finalRecipientBox, err := monRepo.CountByLocation(ctx, recipient.ID, monster.LocationBox)
		if err != nil {
			t.Fatalf("CountByLocation recipient: %v", err)
		}
		if finalRecipientBox != monster.BaseBoxCapacity {
			t.Fatalf("recipient final box count = %d, want %d", finalRecipientBox, monster.BaseBoxCapacity)
		}

		totalSenderRemaining := 0
		for w := 0; w < cfg.Workers; w++ {
			c, err := monRepo.CountByLocation(ctx, senders[w].ID, monster.LocationBox)
			if err != nil {
				t.Fatalf("CountByLocation sender %d: %v", w, err)
			}
			totalSenderRemaining += c
		}
		if int64(totalSenderRemaining) != res.TotalOps-1 {
			t.Fatalf("remaining sender monsters = %d, want %d", totalSenderRemaining, res.TotalOps-1)
		}
		t.Logf("LastSlotCapacityRace completed: %d contenders, 1 accepted, %d rejected with ErrRecipientBoxFull in %v",
			res.TotalOps, rejectedFull.Load(), res.Duration)
	})
}

type failingDeleteCompanionStore struct {
	monster.HomeCompanionStore
	err error
}

func (f *failingDeleteCompanionStore) DeleteHomeCompanion(_ context.Context, _, _ string) error {
	return f.err
}

func TestMonsterService_SQL_GodHomeCompanionsAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, charRepo, monRepo, _ := openMonsterTestDB(t)
	homeMemberRepo, err := NewHomeMemberRepository(db)
	if err != nil {
		t.Fatalf("NewHomeMemberRepository: %v", err)
	}

	c1, err := CreateTestCharacter(ctx, db, "GodMon1")
	if err != nil {
		t.Fatalf("CreateTestCharacter c1: %v", err)
	}
	c2, err := CreateTestCharacter(ctx, db, "GodMon2")
	if err != nil {
		t.Fatalf("CreateTestCharacter c2: %v", err)
	}
	registerCharacterCleanup(t, db, c1.ID, c2.ID)

	txProvider := NewTransactionProvider(db)
	svc := monster.NewService(
		charRepo,
		monRepo,
		monster.WithTransactionProvider(txProvider),
		monster.WithHomeCompanionStore(homeMemberRepo),
	)

	now := time.Now().UTC().Truncate(time.Second)
	ortegaID := id.New()
	if err := homeMemberRepo.AddMember(ctx, god.HomeMember{
		ID:          ortegaID,
		CharacterID: c1.ID,
		IsNPC:       true,
		Name:        "オルテガ",
		Icon:        "chr/029.gif",
		Color:       "#ffffff",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("AddMember ortega: %v", err)
	}
	nonNPCID := id.New()
	if err := homeMemberRepo.AddMember(ctx, god.HomeMember{
		ID:          nonNPCID,
		CharacterID: c1.ID,
		IsNPC:       false,
		Name:        "同居人",
		Icon:        "chr/001.gif",
		Color:       "#ffffff",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("AddMember nonNPC: %v", err)
	}

	// 1. Only is_npc=true members are counted as Home pets or depositable
	summary, err := svc.GetSummary(ctx, c1.ID, "")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.HomeCount != 1 || len(summary.Monsters) != 1 || summary.Monsters[0].ID != ortegaID {
		t.Fatalf("expected only ortega in summary, got %+v", summary)
	}
	if _, err := svc.DepositToBox(ctx, c1.ID, nonNPCID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("DepositToBox non-NPC member: got %v, want %v", err, monster.ErrMonsterNotFound)
	}
	if _, err := svc.DepositToBox(ctx, c2.ID, ortegaID); !errors.Is(err, monster.ErrForbidden) {
		t.Fatalf("DepositToBox unowned ortega: got %v, want %v", err, monster.ErrForbidden)
	}

	// Verify repository-level DeleteHomeCompanion scopes by character_id and is_npc=TRUE
	if err := homeMemberRepo.DeleteHomeCompanion(ctx, c2.ID, ortegaID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("DeleteHomeCompanion with wrong characterID: got %v, want %v", err, monster.ErrMonsterNotFound)
	}
	if err := homeMemberRepo.DeleteHomeCompanion(ctx, c1.ID, nonNPCID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("DeleteHomeCompanion on is_npc=false resident: got %v, want %v", err, monster.ErrMonsterNotFound)
	}
	membersAfterRejectedDelete, err := homeMemberRepo.FindByCharacterID(ctx, c1.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after rejected deletes: %v", err)
	}
	if len(membersAfterRejectedDelete) != 2 {
		t.Fatalf("expected both ortega and nonNPC resident intact, got %+v", membersAfterRejectedDelete)
	}

	// 2. Duplicate name collision with Ortega in home_members
	dupBox, err := svc.TameMonster(ctx, c1.ID, "slime", "オルテガ")
	if err != nil {
		t.Fatalf("TameMonster dupBox: %v", err)
	}
	if _, err := svc.BringToHome(ctx, c1.ID, dupBox.ID); !errors.Is(err, monster.ErrDuplicatePetNameAtHome) {
		t.Fatalf("BringToHome colliding with Ortega: got %v, want %v", err, monster.ErrDuplicatePetNameAtHome)
	}

	// 3. Mixed capacity: 1 God companion + 7 character_monsters = 8 -> 9th rejected with ErrHomeFull
	for i := 1; i <= 7; i++ {
		m, err := svc.TameMonster(ctx, c1.ID, "slime", fmt.Sprintf("家ペット%d", i))
		if err != nil {
			t.Fatalf("TameMonster %d: %v", i, err)
		}
		if _, err := svc.BringToHome(ctx, c1.ID, m.ID); err != nil {
			t.Fatalf("BringToHome %d: %v", i, err)
		}
	}
	ninthPet, err := svc.TameMonster(ctx, c1.ID, "slime", "家ペット8")
	if err != nil {
		t.Fatalf("TameMonster ninthPet: %v", err)
	}
	if _, err := svc.BringToHome(ctx, c1.ID, ninthPet.ID); !errors.Is(err, monster.ErrHomeFull) {
		t.Fatalf("BringToHome 9th pet: got %v, want %v", err, monster.ErrHomeFull)
	}

	// 4. Full box (50 monsters in box) rejects DepositToBox on Ortega without mutating home_members or character_monsters
	// c1 currently has 2 boxed monsters (dupBox, ninthPet); add 48 more to reach BaseBoxCapacity (50)
	var extraBoxIDs []string
	for i := 1; i <= monster.BaseBoxCapacity-2; i++ {
		bid := id.New()
		extraBoxIDs = append(extraBoxIDs, bid)
		if err := monRepo.Save(ctx, monster.MonsterInstance{
			ID:          bid,
			CharacterID: c1.ID,
			MonsterID:   "slime",
			CustomName:  fmt.Sprintf("箱満%d", i),
			Location:    monster.LocationBox,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("Save extra box monster %d: %v", i, err)
		}
	}
	if _, err := svc.DepositToBox(ctx, c1.ID, ortegaID); !errors.Is(err, monster.ErrBoxFull) {
		t.Fatalf("DepositToBox ortega on full box: got %v, want %v", err, monster.ErrBoxFull)
	}
	if _, err := monRepo.FindByID(ctx, ortegaID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("expected ortega not in character_monsters after ErrBoxFull, got err=%v", err)
	}
	companionsAfterBoxFull, err := homeMemberRepo.ListHomeCompanions(ctx, c1.ID)
	if err != nil {
		t.Fatalf("ListHomeCompanions after ErrBoxFull: %v", err)
	}
	if len(companionsAfterBoxFull) != 1 || companionsAfterBoxFull[0].ID != ortegaID {
		t.Fatalf("expected ortega still in home_members after ErrBoxFull, got %+v", companionsAfterBoxFull)
	}
	if err := svc.ReleaseMonster(ctx, c1.ID, extraBoxIDs[0]); err != nil {
		t.Fatalf("ReleaseMonster to free box slot: %v", err)
	}

	// 5. Mid-transaction storage error after character_monsters.Save rolls back both stores
	simulatedErr := errors.New("simulated home_members delete failure")
	failingSvc := monster.NewService(
		charRepo,
		monRepo,
		monster.WithTransactionProvider(txProvider),
		monster.WithHomeCompanionStore(&failingDeleteCompanionStore{
			HomeCompanionStore: homeMemberRepo,
			err:                simulatedErr,
		}),
	)
	if _, err := failingSvc.DepositToBox(ctx, c1.ID, ortegaID); !errors.Is(err, simulatedErr) {
		t.Fatalf("DepositToBox with failing delete: got %v, want %v", err, simulatedErr)
	}
	if _, err := monRepo.FindByID(ctx, ortegaID); !errors.Is(err, monster.ErrMonsterNotFound) {
		t.Fatalf("expected character_monsters insert to roll back, got err=%v", err)
	}
	companionsAfterRollback, err := homeMemberRepo.ListHomeCompanions(ctx, c1.ID)
	if err != nil {
		t.Fatalf("ListHomeCompanions: %v", err)
	}
	if len(companionsAfterRollback) != 1 || companionsAfterRollback[0].ID != ortegaID {
		t.Fatalf("expected ortega still in home_members after rollback, got %+v", companionsAfterRollback)
	}

	// 5. Successful DepositToBox moves Ortega from home_members to character_monsters (location=box),
	// and BringToHome brings Ortega back to location=home preserving ID, icon, and custom name.
	deposited, err := svc.DepositToBox(ctx, c1.ID, ortegaID)
	if err != nil {
		t.Fatalf("DepositToBox ortega: %v", err)
	}
	if deposited.ID != ortegaID || deposited.CharacterID != c1.ID || deposited.MonsterID != "chr/029.gif" || deposited.CustomName != "オルテガ" || deposited.Location != monster.LocationBox {
		t.Fatalf("unexpected deposited ortega: %+v", deposited)
	}
	companionsAfterDeposit, err := homeMemberRepo.ListHomeCompanions(ctx, c1.ID)
	if err != nil {
		t.Fatalf("ListHomeCompanions after deposit: %v", err)
	}
	if len(companionsAfterDeposit) != 0 {
		t.Fatalf("expected 0 companions in home_members after deposit, got %+v", companionsAfterDeposit)
	}
	remainingMembers, err := homeMemberRepo.FindByCharacterID(ctx, c1.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after deposit: %v", err)
	}
	if len(remainingMembers) != 1 || remainingMembers[0].ID != nonNPCID || remainingMembers[0].IsNPC {
		t.Fatalf("expected non-NPC resident to remain in home_members, got %+v", remainingMembers)
	}

	broughtHome, err := svc.BringToHome(ctx, c1.ID, ortegaID)
	if err != nil {
		t.Fatalf("BringToHome ortega: %v", err)
	}
	if broughtHome.ID != ortegaID || broughtHome.CharacterID != c1.ID || broughtHome.MonsterID != "chr/029.gif" || broughtHome.CustomName != "オルテガ" || broughtHome.Location != monster.LocationHome {
		t.Fatalf("unexpected broughtHome ortega: %+v", broughtHome)
	}
}

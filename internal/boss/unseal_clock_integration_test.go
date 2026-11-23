package boss_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/runhost"
)

type controllableClock struct{ nanos atomic.Int64 }

func (c *controllableClock) Set(t time.Time) { c.nanos.Store(t.UTC().UnixNano()) }
func (c *controllableClock) Now() time.Time  { return time.Unix(0, c.nanos.Load()).UTC() }

type fixedZeroRNG struct{}

func (fixedZeroRNG) Intn(int) int     { return 0 }
func (fixedZeroRNG) IntN(int) int     { return 0 }
func (fixedZeroRNG) Float64() float64 { return 0 }

type stubNormalPenalty struct{}

func (stubNormalPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

type failingAdventureRepo struct {
	adventure.Repository
	fail atomic.Bool
}

func (r *failingAdventureRepo) Save(ctx context.Context, v adventure.Adventure) error {
	if err := r.Repository.Save(ctx, v); err != nil {
		return err
	}
	if r.fail.Load() {
		return errors.New("injected adventure save failure")
	}
	return nil
}

type failingPartyLogRepo struct {
	party.AdventureLogRepository
	fail atomic.Bool
}

func (r *failingPartyLogRepo) SaveAdventureLog(ctx context.Context, log party.PartyAdventureLog) error {
	if err := r.AdventureLogRepository.SaveAdventureLog(ctx, log); err != nil {
		return err
	}
	if r.fail.Load() {
		return errors.New("injected party log save failure")
	}
	return nil
}

type failingNormalStore struct {
	runhost.Store[adventure.NormalRun]
	failReceipt atomic.Bool
}

func (s *failingNormalStore) SaveRunReceipt(ctx context.Context, receipt runhost.Receipt) error {
	if err := s.Store.SaveRunReceipt(ctx, receipt); err != nil {
		return err
	}
	if s.failReceipt.Load() {
		return errors.New("injected receipt write failure")
	}
	return nil
}

type alwaysWinBattleEngine struct{}

func (alwaysWinBattleEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	remHP := make(map[string]int, len(req.Participants))
	remMP := make(map[string]int, len(req.Participants))
	for _, p := range req.Participants {
		remHP[p.ID] = p.HP
		remMP[p.ID] = p.MP
	}
	var winnerID string
	if len(req.Participants) > 0 {
		winnerID = req.Participants[0].ID
	}
	return corebattle.Result{
		Outcome:     corebattle.OutcomeWin,
		WinnerID:    winnerID,
		Turns:       1,
		Reward:      req.VictoryReward,
		RemainingHP: remHP,
		RemainingMP: remMP,
	}, nil
}

func (alwaysWinBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	remHP := make(map[string]int, len(req.Allies))
	remMP := make(map[string]int, len(req.Allies))
	for _, a := range req.Allies {
		remHP[a.ID] = a.HP
		remMP[a.ID] = a.MP
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		Turns:       1,
		TotalReward: req.VictoryReward,
		RemainingHP: remHP,
		RemainingMP: remMP,
	}, nil
}

func TestUnsealClock_Stage19AllPathsAndBossCrystals_SQLIntegration(t *testing.T) {
	db := database.CreateTestDatabase(t,
		"players", "characters", "inventory_items", "equipment_slots",
		"character_depots", "depot_items", "character_jobs", "character_job_history", "character_job_masteries",
		"adventures", "party_adventure_logs",
		"runs", "run_enrollments",
		"run_recipients", "run_receipts",
		"character_boss_records", "boss_challenge_history",
		"boss_room_settlements", "boss_unseal_clock",
	)

	ctx := context.Background()
	clock := &controllableClock{}
	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	clock.Set(t0)

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advBaseRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advRepo := &failingAdventureRepo{Repository: advBaseRepo}
	partyDBRepo, err := database.NewPartyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	partyLogRepo := &failingPartyLogRepo{AdventureLogRepository: partyDBRepo}
	normalBaseStore, err := database.NewRunRepository[adventure.NormalRun](db)
	if err != nil {
		t.Fatal(err)
	}
	normalStore := &failingNormalStore{Store: normalBaseStore}
	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}

	adapter := battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips),
		battle.WithDepotRepository(depots),
		battle.WithJobHistoryRepository(jobs),
		battle.WithTransactionProvider(tx),
		battle.WithTransactionRunner(runner),
		battle.WithUnsealClockWriter(bossRepo),
		battle.WithNowFunc(clock.Now),
	)

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	lobbyRepo := party.NewValkeyRepository(nil, party.WithDurableLogRepository(partyLogRepo))
	partySvc, err := party.NewService(
		lobbyRepo, chars, invs, stages, monsters, alwaysWinBattleEngine{},
		party.WithParticipantBuilder(adapter),
		party.WithPostBattleSettler(adapter),
		party.WithTransactionProvider(tx),
		party.WithRNG(func(int) int { return 0 }),
	)
	if err != nil {
		t.Fatal(err)
	}

	soloAdvSvc, err := adventure.NewServiceWithCatalogs(
		advRepo, chars, invs, stages, monsters, alwaysWinBattleEngine{}, nil, nil, clock,
		adventure.WithParticipantBuilder(adapter),
		adventure.WithPostBattleSettler(adapter),
		adventure.WithTransactionProvider(tx),
	)
	if err != nil {
		t.Fatal(err)
	}

	normalSvc, err := adventure.NewNormalRunService(adventure.NormalRunConfig{
		Store: normalStore, Characters: chars, Inventories: invs,
		Builder: adapter, Settler: adapter, History: advRepo,
		Stages: stages, Monsters: monsters,
		Penalty: stubNormalPenalty{}, Clock: clock, RNG: fixedZeroRNG{},
		ExitEffects: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	bossSvc, err := boss.NewService(bossRepo, chars, alwaysWinBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	bossSvc.Configure(
		boss.WithPartyRepository(lobbyRepo),
		boss.WithTransactionProvider(tx),
		boss.WithNowFunc(clock.Now),
	)

	newHero := func(prefix string) corecharacter.Character {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, prefix, 1000)
		if err != nil {
			t.Fatal(err)
		}
		c.Level = 99
		c.JobLevel = 20
		c.Stats = corecharacter.Stats{HP: 5000, MaxHP: 5000, MP: 500, MaxMP: 500, Attack: 2000, Defense: 2000, Agility: 2000}
		if err := chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	sealKing1 := func(partyID string, hero corecharacter.Character, partyCreatedAt time.Time) int {
		t.Helper()
		hero.Tired = 0
		hero.Stats.HP = hero.Stats.MaxHP
		if err := chars.Update(ctx, hero); err != nil {
			t.Fatal(err)
		}
		if err := lobbyRepo.SaveParty(ctx, party.Party{
			ID: partyID, LeaderCharacterID: hero.ID, StageID: "king1",
			Status: party.StatusRecruiting, MaxMembers: 6, CreatedAt: partyCreatedAt,
		}); err != nil {
			t.Fatal(err)
		}
		if err := lobbyRepo.AddMember(ctx, party.Member{
			PartyID: partyID, CharacterID: hero.ID, CharacterName: hero.Name, ReadyState: true, IsLeader: true,
		}); err != nil {
			t.Fatal(err)
		}
		res, err := bossSvc.StartSealingBattle(ctx, partyID, hero.ID)
		if err != nil {
			t.Fatalf("StartSealingBattle(%s): %v", partyID, err)
		}
		return res.RewardCrystals
	}

	heroBoss := newHero("BossHero")

	// 1. Before any unseal: GetUnsealedAt is nil and boss awards 1.0x (70), ignoring party.CreatedAt
	if got, err := bossRepo.GetUnsealedAt(ctx); err != nil || got != nil {
		t.Fatalf("initial GetUnsealedAt = %v (err=%v), want nil", got, err)
	}
	if crystals := sealKing1("p-before-unseal", heroBoss, clock.Now().Add(-5*time.Second)); crystals != 70 {
		t.Fatalf("pre-unseal crystals = %d, want 70 (1.0x)", crystals)
	}

	// Prepare an already-existing boss sealing room to verify re-unseal still overwrites unsealed_at
	if err := lobbyRepo.SaveParty(ctx, party.Party{
		ID: "existing-king1", LeaderCharacterID: "system", StageID: "king1",
		Status: party.StatusRecruiting, MaxMembers: 6, CreatedAt: t0.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Normal Run stage-19 floor-11 treasure: rollback, commit with MaoCount, replay/chest/exit non-trigger
	heroNormal := newHero("Norm")
	entry, err := normalSvc.EnterSolo(ctx, heroNormal.PlayerID, heroNormal.ID, id.New(), "stage-19", adventure.CrawlModeNormal, 3)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	if err := normalStore.WithRunForUpdate(ctx, runID, false, func(txCtx context.Context) error {
		r, err := normalStore.FindRun(txCtx, runID)
		if err != nil {
			return err
		}
		r.Round = 10
		r.EncounterFloor = 10
		for i := range r.Battle.Actors {
			if slices.Contains(r.Enemies, r.Battle.Actors[i].ID) {
				r.Battle.Actors[i].HP = 0
			}
		}
		r.Version++
		return normalStore.SaveRun(txCtx, *r, r.Version-1)
	}); err != nil {
		t.Fatal(err)
	}

	t1 := t0.Add(15 * time.Minute)
	clock.Set(t1)
	beforeRun, err := normalStore.FindRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	advCmd := runhost.Command{
		RunID: runID, ID: "normal-unseal-cmd", PlayerID: heroNormal.PlayerID,
		ActorID: heroNormal.ID, Version: beforeRun.Version, Action: "advance",
	}

	// 2a. Settlement rollback leaves clock nil and MaoCount 0
	normalStore.failReceipt.Store(true)
	if _, err := normalSvc.Command(ctx, advCmd); err == nil {
		t.Fatal("expected injected receipt failure")
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts != nil {
		t.Fatalf("rolled-back normal unseal leaked clock: %v (err=%v)", ts, err)
	}
	if c, err := chars.FindByID(ctx, heroNormal.ID); err != nil || c.MaoCount != 0 {
		t.Fatalf("rolled-back normal unseal leaked MaoCount: %d (err=%v)", c.MaoCount, err)
	}

	// 2b. Successful floor-11 treasure creation commits MaoCount=1 and unsealed_at=t1 together
	normalStore.failReceipt.Store(false)
	receipt1, err := normalSvc.Command(ctx, advCmd)
	if err != nil {
		t.Fatalf("normal advance to stage-19 floor-11 failed: %v", err)
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t1) {
		t.Fatalf("after normal unseal GetUnsealedAt = %v (err=%v), want %v", ts, err, t1)
	}
	if c, err := chars.FindByID(ctx, heroNormal.ID); err != nil || c.MaoCount != 1 {
		t.Fatalf("after normal unseal MaoCount = %d (err=%v), want 1", c.MaoCount, err)
	}

	// 2c. Receipt replay, chest, and exit MUST NOT overwrite unsealed_at
	clock.Set(t1.Add(30 * time.Minute))
	if replayReceipt, err := normalSvc.Command(ctx, advCmd); err != nil || replayReceipt.Version != receipt1.Version {
		t.Fatalf("receipt replay failed or drifted: %+v err=%v", replayReceipt, err)
	}
	rAfter, err := normalStore.FindRun(ctx, runID)
	if err != nil || len(rAfter.Boxes) == 0 {
		t.Fatalf("expected floor 11 boxes, run=%+v err=%v", rAfter, err)
	}
	for _, cmd := range []runhost.Command{
		{RunID: runID, ID: id.New(), PlayerID: heroNormal.PlayerID, ActorID: heroNormal.ID, Version: rAfter.Version, Action: "chest", TargetID: rAfter.Boxes[0].ID},
	} {
		clock.Set(clock.Now().Add(5 * time.Second))
		if _, err := normalSvc.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	rAfterChest, err := normalStore.FindRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(clock.Now().Add(5 * time.Second))
	if _, err := normalSvc.Command(ctx, runhost.Command{
		RunID: runID, ID: id.New(), PlayerID: heroNormal.PlayerID, ActorID: heroNormal.ID, Version: rAfterChest.Version, Action: "exit",
	}); err != nil {
		t.Fatal(err)
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t1) {
		t.Fatalf("replay/chest/exit overwrote unsealed_at: got %v (err=%v), want %v", ts, err, t1)
	}

	// 3. Solo Crawl stage-19: rollback preserves t1, success overwrites clock to t2 + increments MaoCount
	heroSolo := newHero("SoloC")
	t2 := t1.Add(90 * time.Minute)
	clock.Set(t2)
	advRepo.fail.Store(true)
	if _, err := soloAdvSvc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{heroSolo.ID}, StageID: "stage-19", Mode: adventure.CrawlModeNormal, Rng: func(int) int { return 0 },
	}); err == nil {
		t.Fatal("expected injected solo adventure failure")
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t1) {
		t.Fatalf("rolled-back solo crawl leaked clock: got %v, want %v", ts, t1)
	}
	if c, err := chars.FindByID(ctx, heroSolo.ID); err != nil || c.MaoCount != 0 {
		t.Fatalf("rolled-back solo crawl leaked MaoCount: %d", c.MaoCount)
	}

	advRepo.fail.Store(false)
	soloRes, err := soloAdvSvc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{heroSolo.ID}, StageID: "stage-19", Mode: adventure.CrawlModeNormal, Rng: func(int) int { return 0 },
	})
	if err != nil || !soloRes.UnsealDemonKing {
		t.Fatalf("solo stage-19 crawl failed: res=%+v err=%v", soloRes, err)
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t2) {
		t.Fatalf("solo crawl did not overwrite clock: got %v, want %v", ts, t2)
	}
	if c, err := chars.FindByID(ctx, heroSolo.ID); err != nil || c.MaoCount != 1 {
		t.Fatalf("solo crawl MaoCount = %d, want 1", c.MaoCount)
	}

	// 4. Party Crawl stage-19: rollback preserves t2, success overwrites clock to t3 + increments MaoCount
	p1, p2 := newHero("PC1"), newHero("PC2")
	partyCrawlID := "party-stage19-unseal"
	if err := lobbyRepo.SaveParty(ctx, party.Party{
		ID: partyCrawlID, LeaderCharacterID: p1.ID, StageID: "stage-19",
		Mode: adventure.CrawlModeNormal, Status: party.StatusRecruiting, MaxMembers: 6,
	}); err != nil {
		t.Fatal(err)
	}
	for i, c := range []corecharacter.Character{p1, p2} {
		if err := lobbyRepo.AddMember(ctx, party.Member{PartyID: partyCrawlID, CharacterID: c.ID, IsLeader: i == 0, ReadyState: true}); err != nil {
			t.Fatal(err)
		}
	}

	t3 := t2.Add(90 * time.Minute)
	clock.Set(t3)
	partyLogRepo.fail.Store(true)
	if _, err := partySvc.StartPartyAdventure(ctx, partyCrawlID, p1.ID); err == nil {
		t.Fatal("expected injected party crawl log failure")
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t2) {
		t.Fatalf("rolled-back party crawl leaked clock: got %v, want %v", ts, t2)
	}
	for _, memberID := range []string{p1.ID, p2.ID} {
		if c, err := chars.FindByID(ctx, memberID); err != nil || c.MaoCount != 0 {
			t.Fatalf("rolled-back party crawl leaked MaoCount for %s: %d", memberID, c.MaoCount)
		}
	}

	// Ready members again after deferred cleanup restored recruiting state
	for _, memberID := range []string{p1.ID, p2.ID} {
		if err := lobbyRepo.UpdateMemberReady(ctx, partyCrawlID, memberID, true); err != nil {
			t.Fatal(err)
		}
	}
	partyLogRepo.fail.Store(false)
	if _, err := partySvc.StartPartyAdventure(ctx, partyCrawlID, p1.ID); err != nil {
		t.Fatalf("party stage-19 crawl failed: %v", err)
	}
	if ts, err := bossRepo.GetUnsealedAt(ctx); err != nil || ts == nil || !ts.Equal(t3) {
		t.Fatalf("party crawl did not overwrite clock: got %v, want %v", ts, t3)
	}
	for _, memberID := range []string{p1.ID, p2.ID} {
		if c, err := chars.FindByID(ctx, memberID); err != nil || c.MaoCount != 1 {
			t.Fatalf("party crawl MaoCount for %s = %d, want 1", memberID, c.MaoCount)
		}
	}

	// 5. Boss settlement reads persisted unsealed_at=t3 across exact 600/601, 1800/1801, 3600/3601 boundaries
	boundaries := []struct {
		name      string
		elapsed   time.Duration
		createdAt time.Time
		want      int
	}{
		{"600s (3x)", 600 * time.Second, t3.Add(-2 * time.Hour), 210},
		{"601s (2x)", 601 * time.Second, t3.Add(600 * time.Second), 140},
		{"1800s (2x)", 1800 * time.Second, t3.Add(-2 * time.Hour), 140},
		{"1801s (1.5x per enemy)", 1801 * time.Second, t3.Add(1800 * time.Second), 102},
		{"3600s (1.5x per enemy)", 3600 * time.Second, t3.Add(-2 * time.Hour), 102},
		{"3601s (1x)", 3601 * time.Second, t3.Add(3600 * time.Second), 70},
	}
	for i, tc := range boundaries {
		clock.Set(t3.Add(tc.elapsed))
		if got := sealKing1("boundary-"+string(rune('a'+i)), heroBoss, tc.createdAt); got != tc.want {
			t.Errorf("%s: got %d crystals, want %d", tc.name, got, tc.want)
		}
	}
}

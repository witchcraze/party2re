package challenge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func setupSessionTestService(t *testing.T) (*challenge.Service, *mockChallengeRepo, *mockCharRepo, *mockActiveStore) {
	t.Helper()
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": {
				ID:         "hero",
				Level:      30,
				Experience: 10000,
				Stats: corecharacter.Stats{
					HP:      350,
					MaxHP:   350,
					Attack:  500,
					Defense: 300,
					Agility: 250,
				},
			},
			"weakling": {
				ID:         "weakling",
				Level:      1,
				Experience: 0,
				Stats: corecharacter.Stats{
					HP:      10,
					MaxHP:   10,
					Attack:  5,
					Defense: 0,
				},
			},
		},
	}
	activeStore := newMockActiveStore(nil)
	svc, err := challenge.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		challenge.WithActiveSessionStore(activeStore),
	)
	if err != nil {
		t.Fatalf("failed to create challenge service: %v", err)
	}
	return svc, repo, charRepo, activeStore
}

func TestAdvanceRound_PersistenceErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty character ID returns ErrCharacterNotFound", func(t *testing.T) {
		svc, _, _, _ := setupSessionTestService(t)
		_, _, err := svc.AdvanceRound(ctx, "", "sess-1", 1)
		if !errors.Is(err, challenge.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("activeStore GetActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		activeStore.getActiveSessionErr = errors.New("valkey connection reset")
		_, err := svc.GetActiveSession(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "valkey connection reset") {
			t.Errorf("expected valkey error, got %v", err)
		}
	})

	t.Run("charRepo FindByID error propagates", func(t *testing.T) {
		svc, _, charRepo, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		charRepo.findByIDErr = errors.New("character db read failed")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err == nil || !strings.Contains(err.Error(), "character db read failed") {
			t.Errorf("expected charRepo error, got %v", err)
		}
	})

	t.Run("repo GetHallOfFame error propagates with context", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.getHofErr = errors.New("db connection refused")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err == nil || !strings.Contains(err.Error(), "getting hall of fame: db connection refused") {
			t.Errorf("expected wrapped hall of fame error, got %v", err)
		}
	})

	t.Run("repo SaveHallOfFame error propagates with context", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.saveHofErr = errors.New("hall of fame write lock failed")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err == nil || !strings.Contains(err.Error(), "saving hall of fame: hall of fame write lock failed") {
			t.Errorf("expected wrapped save hall of fame error, got %v", err)
		}
	})

	t.Run("activeStore AdvanceRound error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		activeStore.advanceRoundErr = errors.New("lua script timeout")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err == nil || !strings.Contains(err.Error(), "lua script timeout") {
			t.Errorf("expected advance round error, got %v", err)
		}
	})

	t.Run("successful round does not require a second SaveActiveSession", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		activeStore.saveActiveSessionErr = errors.New("valkey oom")

		_, updated, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil || updated == nil || updated.CurrentRound != 2 {
			t.Errorf("expected an atomic successful round, got %+v, err=%v", updated, err)
		}
	})

	t.Run("repo FinalizeSession on defeat error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "weakling", "0")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.finalizeSessionErr = errors.New("mariadb deadlocked")

		_, _, err = svc.AdvanceRound(ctx, "weakling", sess.ID, sess.Revision)
		if err == nil || !strings.Contains(err.Error(), "mariadb deadlocked") {
			t.Errorf("expected finalize session error, got %v", err)
		}
	})
}

func TestExecuteRound_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo FindSessionByID error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findSessionByIDErr = errors.New("db session query failed")

		_, _, err := svc.AdvanceRound(ctx, "hero", "sess-1", 1)
		if err == nil || !strings.Contains(err.Error(), "db session query failed") {
			t.Errorf("expected repo find error, got %v", err)
		}
	})
}

func TestGetSession_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo FindSessionByID error propagates when not in active store", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findSessionByIDErr = errors.New("durable query timeout")

		_, err := svc.GetSession(ctx, "hero", "non-existent-sess")
		if err == nil || !strings.Contains(err.Error(), "durable query timeout") {
			t.Errorf("expected repo error, got %v", err)
		}
	})
}

func TestGetCharacterRecords_ValidationAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty character ID returns error", func(t *testing.T) {
		svc, _, _, _ := setupSessionTestService(t)
		_, err := svc.GetCharacterRecords(ctx, "")
		if err == nil || !strings.Contains(err.Error(), "character id is required") {
			t.Errorf("expected character id required error, got %v", err)
		}
	})

	t.Run("repo FindRecordsByCharacter error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findRecordsByCharErr = errors.New("records db timeout")

		_, err := svc.GetCharacterRecords(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "records db timeout") {
			t.Errorf("expected repo records error, got %v", err)
		}
	})
}

type scriptedRNG struct {
	ints   []int
	floats []float64
	intIdx int
	fltIdx int
}

func (r *scriptedRNG) Intn(n int) int {
	if r.intIdx >= len(r.ints) {
		return 0
	}
	v := r.ints[r.intIdx]
	r.intIdx++
	if n <= 0 {
		return 0
	}
	return v % n
}

func (r *scriptedRNG) IntN(n int) int { return r.Intn(n) }

func (r *scriptedRNG) Float64() float64 {
	if r.fltIdx >= len(r.floats) {
		return 0
	}
	v := r.floats[r.fltIdx]
	r.fltIdx++
	return v
}

type capturePartyEngine struct {
	calls   int
	enemies []corebattle.Participant
}

func (e *capturePartyEngine) Resolve(corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{}, errors.New("expected PartyBattleResolver")
}

func (e *capturePartyEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls++
	e.enemies = append([]corebattle.Participant(nil), req.Enemies...)
	rem := make(map[string]int, len(req.Allies))
	for _, a := range req.Allies {
		rem[a.ID] = a.HP
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  "allies",
		RemainingHP: rem,
	}, nil
}

func TestTierZero_TreasureBranchReplacesCombatAndThresholds(t *testing.T) {
	ctx := context.Background()

	newTierZeroFixture := func(t *testing.T, round int, rng *scriptedRNG) (*challenge.Service, *capturePartyEngine, *challenge.ChallengeSession) {
		t.Helper()
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"hero": {
					ID:    "hero",
					Level: 10,
					Stats: corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 200, Defense: 100},
				},
			},
		}
		store := challenge.NewMemorySessionRepository()
		engine := &capturePartyEngine{}
		svc, err := challenge.NewService(repo, charRepo, engine, challenge.WithActiveSessionStore(store), challenge.WithRNG(rng))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		sess.CurrentRound = round
		saveRunFixture(t, ctx, repo, store, sess)
		return svc, engine, sess
	}

	t.Run("threshold 10 boundary: round 9 fights monster, round 10 with <0.10 gets treasure without combat", func(t *testing.T) {
		// Round 9: Intn(11)->0 (threshold=10); round 9 < 10 so monster spawns (Float64()->0.0 for count=1, 4x 0.0 for stats)
		// Round 10: Intn(11)->0 (threshold=10), Float64()->0.0999 (<0.10), Intn(18)->17 ("item-103")
		// Round 11: already has treasure, so even with Intn(11)->0 and Float64()->0.0, fights monster
		rng := &scriptedRNG{
			ints:   []int{0, 0, 17, 0},
			floats: []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0999, 0.0, 0.0, 0.0, 0.0, 0.0},
		}
		svc, engine, sess := newTierZeroFixture(t, 9, rng)

		res9, sess, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 9: %v", err)
		}
		if engine.calls != 1 || res9.AwardedItem != "" || res9.RoundExp != 20 || res9.RoundGold != 5 {
			t.Fatalf("round 9 expected combat reward (20/5) and no treasure, got calls=%d res=%+v", engine.calls, res9)
		}

		res10, sess, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 10: %v", err)
		}
		if engine.calls != 1 {
			t.Fatalf("round 10 treasure branch must not invoke combat engine, calls=%d", engine.calls)
		}
		if !res10.Won || res10.RoundExp != 0 || res10.RoundGold != 0 || res10.AwardedItem != "item-103" {
			t.Fatalf("round 10 unexpected treasure result: %+v", res10)
		}
		if sess.AccumulatedExp != 20 || sess.AccumulatedGold != 5 || len(sess.AccumulatedItems) != 1 || sess.AccumulatedItems[0] != "item-103" {
			t.Fatalf("round 10 unexpected session state: %+v", sess)
		}

		res11, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 11: %v", err)
		}
		if engine.calls != 2 || res11.AwardedItem != "" {
			t.Fatalf("round 11 must fight monster after treasure already obtained, calls=%d res=%+v", engine.calls, res11)
		}
	})

	t.Run("threshold 20 boundary: round 19 fights monster, round 20 triggers treasure", func(t *testing.T) {
		// Round 19: Intn(11)->10 (threshold=20); round 19 < 20 -> combat (1 count draw + 4 stat draws)
		// Round 20: Intn(11)->10 (threshold=20), Float64()->0.0 (<0.10), Intn(18)->0 ("item-004") -> treasure
		rng := &scriptedRNG{
			ints:   []int{10, 10, 0},
			floats: []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		}
		svc, engine, sess := newTierZeroFixture(t, 19, rng)

		res19, sess, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 19: %v", err)
		}
		if engine.calls != 1 || res19.AwardedItem != "" {
			t.Fatalf("round 19 expected combat, got calls=%d res=%+v", engine.calls, res19)
		}

		res20, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 20: %v", err)
		}
		if engine.calls != 1 || res20.AwardedItem != "item-004" || res20.RoundExp != 0 || res20.RoundGold != 0 {
			t.Fatalf("round 20 expected treasure item-004 without combat, got calls=%d res=%+v", engine.calls, res20)
		}
	})

	t.Run("10% probability boundary: 0.10 does not trigger treasure", func(t *testing.T) {
		// Round 10: Intn(11)->0 (threshold=10), Float64()->0.10 (not < 0.10) -> falls through to monster combat
		rng := &scriptedRNG{
			ints:   []int{0},
			floats: []float64{0.10, 0.0, 0.0, 0.0, 0.0},
		}
		svc, engine, sess := newTierZeroFixture(t, 10, rng)

		res, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("round 10 with 0.10 roll: %v", err)
		}
		if engine.calls != 1 || res.AwardedItem != "" || res.RoundExp != 20 || res.RoundGold != 5 {
			t.Fatalf("expected 0.10 roll to fight monster, got calls=%d res=%+v", engine.calls, res)
		}
	})
}

func TestChallenge_IntTruncationUnscaledRewardsAndMonsterPool(t *testing.T) {
	ctx := context.Background()

	t.Run("stat scaling truncates via int instead of math.Round and does not scale EXP/Gold", func(t *testing.T) {
		// Party size = 1, Round 1 (scale = 1.1).
		// With variance Float64() = 0.5: (0.9 + 0.15 - 0.1) * 1.1 = 0.95 * 1.1 = 1.045.
		// Slime BaseHP=150 -> 150 * 1.045 = 156.75 -> int truncates to 156 (math.Round would be 157).
		// Slime BaseAttack=100 -> 100 * 1.045 = 104.5 -> int truncates to 104 (math.Round would be 105).
		// Slime BaseDefense=50 -> 50 * 1.045 = 52.25 -> int truncates to 52.
		// Slime BaseAgility=200 with variance Float64() = 0.42: (0.9 + 0.126 - 0.1) * 1.1 = 1.0186
		//   -> 200 * 1.0186 = 203.72 -> int truncates to 203 (math.Round would be 204).
		// Rewards remain unscaled: Exp=20, Gold=5 (not 22 and 6).
		rng := &scriptedRNG{
			ints:   []int{0},
			floats: []float64{0.0, 0.5, 0.5, 0.5, 0.42},
		}
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"hero": {ID: "hero", Level: 10, Stats: corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 200, Defense: 100}},
			},
		}
		engine := &capturePartyEngine{}
		svc, err := challenge.NewService(repo, charRepo, engine, challenge.WithRNG(rng))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		res, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("AdvanceRound: %v", err)
		}
		if len(engine.enemies) != 1 {
			t.Fatalf("expected 1 enemy, got %d", len(engine.enemies))
		}
		got := engine.enemies[0]
		if got.HP != 156 || got.Attack != 104 || got.Defense != 52 || got.Agility != 203 {
			t.Fatalf("expected truncated stats HP=156 Attack=104 Defense=52 Agility=203, got HP=%d Attack=%d Defense=%d Agility=%d", got.HP, got.Attack, got.Defense, got.Agility)
		}
		if res.RoundExp != 20 || res.RoundGold != 5 {
			t.Fatalf("expected unscaled rewards Exp=20 Gold=5, got Exp=%d Gold=%d", res.RoundExp, res.RoundGold)
		}
	})

	t.Run("monster count formula and random candidate pool selection", func(t *testing.T) {
		// Party size = 1 -> 2.5 + 1*0.6 = 3.1.
		// Float64() = 0.99 -> c = int(0.99 * 3.1) = int(3.069) = 3 -> monsterCount = 4.
		// Pool of 3 candidates selected via Intn(3) with sequence [2, 0, 1, 2].
		rng := &scriptedRNG{
			ints:   []int{2, 0, 1, 2},
			floats: []float64{0.99},
		}
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"hero": {ID: "hero", Level: 10, Stats: corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 200, Defense: 100}},
			},
		}
		customTiers := map[string]challenge.ChallengeTier{
			"pool-test": {
				ID:              "pool-test",
				Name:            "Pool Test",
				MinLevel:        1,
				MaxParticipants: 1,
				TreasureRound:   10,
				MonsterPool: []challenge.ChallengeMonster{
					{Name: "Mon0", BaseHP: 100, BaseAttack: 50, BaseDefense: 20, BaseExp: 10, BaseGold: 3},
					{Name: "Mon1", BaseHP: 120, BaseAttack: 60, BaseDefense: 30, BaseExp: 15, BaseGold: 7},
					{Name: "Mon2", BaseHP: 140, BaseAttack: 70, BaseDefense: 40, BaseExp: 25, BaseGold: 11},
				},
			},
		}
		engine := &capturePartyEngine{}
		svc, err := challenge.NewService(repo, charRepo, engine, challenge.WithRNG(rng), challenge.WithCustomTiers(customTiers))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		sess, err := svc.StartSession(ctx, "hero", "pool-test")
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		res, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("AdvanceRound: %v", err)
		}
		if len(engine.enemies) != 4 {
			t.Fatalf("expected 4 enemies (c=3 -> c+1=4), got %d", len(engine.enemies))
		}
		wantNames := []string{"Mon2A", "Mon0B", "Mon1C", "Mon2D"}
		for i, want := range wantNames {
			if engine.enemies[i].Name != want {
				t.Fatalf("enemy[%d].Name = %q, want %q", i, engine.enemies[i].Name, want)
			}
		}
		// Total unscaled rewards: 25 + 10 + 15 + 25 = 75 Exp, 11 + 3 + 7 + 11 = 32 Gold
		if res.RoundExp != 75 || res.RoundGold != 32 {
			t.Fatalf("expected total unscaled rewards Exp=75 Gold=32, got Exp=%d Gold=%d", res.RoundExp, res.RoundGold)
		}
	})

	t.Run("tier 0 slime agility propagates to real battle engine turn order", func(t *testing.T) {
		rng := &scriptedRNG{
			ints:   []int{0},
			floats: []float64{0.0},
		}
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"hero": {ID: "hero", Level: 10, Stats: corecharacter.Stats{HP: 350, MaxHP: 350, Attack: 500, Defense: 300, Agility: 50}},
			},
		}
		svc, err := challenge.NewService(repo, charRepo, corebattle.Engine{}, challenge.WithRNG(rng))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		sess, err := svc.StartSession(ctx, "hero", "0")
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		res, _, err := svc.AdvanceRound(ctx, "hero", sess.ID, sess.Revision)
		if err != nil {
			t.Fatalf("AdvanceRound: %v", err)
		}
		if len(res.BattleResult.Logs) == 0 || res.BattleResult.Logs[0].ActorID != "スライム-1-A" {
			t.Fatalf("expected faster Slime (base_agility=200 -> scaled 176 > hero agility 50) to act first, got logs=%+v", res.BattleResult.Logs)
		}
	})
}

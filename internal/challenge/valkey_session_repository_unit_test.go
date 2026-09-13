package challenge_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeySessionRepository_NewAndFallback(t *testing.T) {
	ctx := context.Background()

	// When client is nil, it seamlessly falls back to in-memory store
	repo, err := challenge.NewValkeySessionRepository(nil, challenge.WithSessionTTL(15*time.Minute))
	if err != nil || repo == nil {
		t.Fatalf("expected non-nil repo, got repo=%v err=%v", repo, err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	sess := challenge.ChallengeSession{
		ID:          "chal-fallback-1",
		CharacterID: "char-fallback-1",
		PartyID:     "party-fallback-1",
		PartyName:   "Fallback Party",
		PartyColor:  "#00FF00",
		Members: []challenge.ChallengeMember{
			{
				CharacterID:        "char-fallback-1",
				CharacterName:      "LeaderHero",
				Level:              20,
				MaxHP:              150,
				CharacterCurrentHP: 150,
			},
		},
		TierID:             "novice",
		CurrentRound:       1,
		CharacterCurrentHP: 150,
		AccumulatedExp:     10,
		AccumulatedGold:    20,
		AccumulatedItems:   []string{"herb"},
		Status:             challenge.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 1. SaveActiveSession fallback
	if err := repo.SaveActiveSession(ctx, sess); err != nil {
		t.Fatalf("fallback SaveActiveSession error: %v", err)
	}

	// 2. GetActiveSession fallback
	retrieved, err := repo.GetActiveSession(ctx, "char-fallback-1")
	if err != nil || retrieved == nil {
		t.Fatalf("expected retrieved session, got %v, err=%v", retrieved, err)
	}
	if retrieved.ID != "chal-fallback-1" || retrieved.PartyID != "party-fallback-1" || len(retrieved.Members) != 1 {
		t.Errorf("unexpected retrieved session: %+v", retrieved)
	}

	// 3. AdvanceRound fallback
	advanceOutcome, err := repo.AdvanceRound(ctx, "char-fallback-1", challenge.AdvanceRoundParams{
		ExpectedSessionID: "chal-fallback-1",
		SurvivingHP:       140,
		ExpDelta:          25,
		GoldDelta:         50,
		RewardItemID:      "potion",
		Now:               time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("fallback AdvanceRound error: %v", err)
	}
	if advanceOutcome.Session.CurrentRound != 2 || advanceOutcome.Session.CharacterCurrentHP != 140 {
		t.Errorf("unexpected fallback AdvanceRound result: %+v", advanceOutcome)
	}

	// 4. DeleteActiveSession fallback
	if err := repo.DeleteActiveSession(ctx, "char-fallback-1"); err != nil {
		t.Fatalf("fallback DeleteActiveSession error: %v", err)
	}
	afterDelete, err := repo.GetActiveSession(ctx, "char-fallback-1")
	if err != nil || afterDelete != nil {
		t.Fatalf("expected nil after delete, got %v, err=%v", afterDelete, err)
	}
}

func TestValkeySessionRepository_SaveActiveSession(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	sess := challenge.ChallengeSession{
		ID:          "chal-save-1",
		CharacterID: "char-100",
		PartyID:     "party-100",
		PartyName:   "Alpha Challengers",
		PartyColor:  "#FF0000",
		Members: []challenge.ChallengeMember{
			{
				CharacterID:        "char-100",
				CharacterName:      "LeaderHero",
				Icon:               "chr/001.gif",
				JobID:              "warrior",
				OldJobID:           "novice",
				Level:              25,
				MaxHP:              180,
				MaxMP:              50,
				CharacterCurrentHP: 180,
				Attack:             70,
				Defense:            40,
				Agility:            30,
			},
			{
				CharacterID:        "char-101",
				CharacterName:      "MageAlly",
				Icon:               "chr/002.gif",
				JobID:              "mage",
				Level:              24,
				MaxHP:              110,
				MaxMP:              120,
				CharacterCurrentHP: 110,
				Attack:             35,
				Defense:            25,
				Agility:            45,
			},
		},
		TierID:             "veteran",
		CurrentRound:       3,
		CharacterCurrentHP: 160,
		AccumulatedExp:     250,
		AccumulatedGold:    500,
		AccumulatedItems:   []string{"sword", "shield"},
		Status:             challenge.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 1. Success case: DoMulti executes HSET (session), HSET (rewards), EXPIRE (session), EXPIRE (rewards)
	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		res := make([]valkeygo.ValkeyResult, len(multi))
		for i := range multi {
			res[i] = valkeytest.MakeOKResult()
		}
		return res
	}))

	repo, _ := challenge.NewValkeySessionRepository(client, challenge.WithSessionTTL(1*time.Hour))
	if err := repo.SaveActiveSession(ctx, sess); err != nil {
		t.Fatalf("unexpected SaveActiveSession error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 recorded commands, got %d: %v", len(cmds), cmds)
	}

	// Command 0: HSET sessionKey
	if cmds[0][0] != "HSET" || !strings.Contains(cmds[0][1], "char:char-100}:session") {
		t.Errorf("unexpected HSET session command: %v", cmds[0])
	}
	// Verify members are properly encoded in HSET
	hasMembersField := false
	hasPartyIDField := false
	for i := 2; i < len(cmds[0])-1; i += 2 {
		if cmds[0][i] == "party_id" && cmds[0][i+1] == "party-100" {
			hasPartyIDField = true
		}
		if cmds[0][i] == "members" {
			hasMembersField = true
			decodedMembers, err := challenge.DecodeJSON[[]challenge.ChallengeMember](cmds[0][i+1])
			if err != nil || len(decodedMembers) != 2 || decodedMembers[0].CharacterName != "LeaderHero" {
				t.Errorf("unexpected members JSON encoding: %v, err=%v", cmds[0][i+1], err)
			}
		}
	}
	if !hasMembersField || !hasPartyIDField {
		t.Errorf("expected party_id and members in HSET, got %v", cmds[0])
	}

	// Command 1: HSET rewardsKey
	if cmds[1][0] != "HSET" || !strings.Contains(cmds[1][1], "char:char-100}:rewards") {
		t.Errorf("unexpected HSET rewards command: %v", cmds[1])
	}

	// Command 2 & 3: EXPIRE with 3600s
	if cmds[2][0] != "EXPIRE" || cmds[2][2] != "3600" {
		t.Errorf("unexpected EXPIRE session command: %v", cmds[2])
	}
	if cmds[3][0] != "EXPIRE" || cmds[3][2] != "3600" {
		t.Errorf("unexpected EXPIRE rewards command: %v", cmds[3])
	}

	// 2. Error case: DoMulti command returns error
	errDoMulti := errors.New("valkey domulti error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeErrorResult(errDoMulti),
			valkeytest.MakeOKResult(),
			valkeytest.MakeOKResult(),
			valkeytest.MakeOKResult(),
		}
	}))
	repoErr, _ := challenge.NewValkeySessionRepository(clientErr)
	if err := repoErr.SaveActiveSession(ctx, sess); !errors.Is(err, errDoMulti) {
		t.Fatalf("expected errDoMulti, got %v", err)
	}
}

func TestValkeySessionRepository_GetActiveSession(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	nowStr := now.Format(time.RFC3339Nano)
	membersJSON := challenge.EncodeJSON([]challenge.ChallengeMember{
		{
			CharacterID:        "char-200",
			CharacterName:      "SoloHero",
			Level:              30,
			MaxHP:              220,
			CharacterCurrentHP: 200,
		},
	})
	itemsJSON := challenge.EncodeJSON([]string{"gem-ruby", "herb-super"})

	sessionMap := map[string]string{
		"session_id":           "chal-get-1",
		"character_id":         "char-200",
		"party_id":             "party-solo",
		"party_name":           "Solo Runner",
		"party_color":          "#0000FF",
		"members":              membersJSON,
		"tier_id":              "expert",
		"current_round":        "5",
		"character_current_hp": "200",
		"status":               string(challenge.StatusActive),
		"created_at":           nowStr,
		"updated_at":           nowStr,
	}

	rewardsMap := map[string]string{
		"exp":   "800",
		"gold":  "1500",
		"items": itemsJSON,
	}

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeStringMapResult(sessionMap),
			valkeytest.MakeStringMapResult(rewardsMap),
		}
	}))
	repo, _ := challenge.NewValkeySessionRepository(client)

	active, err := repo.GetActiveSession(ctx, "char-200")
	if err != nil {
		t.Fatalf("unexpected GetActiveSession error: %v", err)
	}
	if active == nil {
		t.Fatalf("expected active session, got nil")
	}

	if active.ID != "chal-get-1" || active.PartyID != "party-solo" || active.PartyColor != "#0000FF" {
		t.Errorf("unexpected identity fields: %+v", active)
	}
	if active.CurrentRound != 5 || active.CharacterCurrentHP != 200 || active.TierID != "expert" {
		t.Errorf("unexpected progress fields: %+v", active)
	}
	if active.AccumulatedExp != 800 || active.AccumulatedGold != 1500 {
		t.Errorf("unexpected rewards: exp=%d gold=%d", active.AccumulatedExp, active.AccumulatedGold)
	}
	if len(active.AccumulatedItems) != 2 || active.AccumulatedItems[0] != "gem-ruby" {
		t.Errorf("unexpected items: %+v", active.AccumulatedItems)
	}
	if len(active.Members) != 1 || active.Members[0].CharacterName != "SoloHero" {
		t.Errorf("unexpected members: %+v", active.Members)
	}

	// 2. Not found: empty sessionMap
	clientEmpty := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeStringMapResult(map[string]string{}),
			valkeytest.MakeStringMapResult(map[string]string{}),
		}
	}))
	repoEmpty, _ := challenge.NewValkeySessionRepository(clientEmpty)
	emptyActive, err := repoEmpty.GetActiveSession(ctx, "char-200")
	if err != nil || emptyActive != nil {
		t.Fatalf("expected nil active session on empty map, got %v, err=%v", emptyActive, err)
	}

	// 3. Not found: nil result from Valkey
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeNilResult(),
			valkeytest.MakeNilResult(),
		}
	}))
	repoNil, _ := challenge.NewValkeySessionRepository(clientNil)
	nilActive, err := repoNil.GetActiveSession(ctx, "char-200")
	if err != nil || nilActive != nil {
		t.Fatalf("expected nil active session on nil result, got %v, err=%v", nilActive, err)
	}

	// 4. Error on session fetch
	errGet := errors.New("hgetall session error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeErrorResult(errGet),
			valkeytest.MakeNilResult(),
		}
	}))
	repoErr, _ := challenge.NewValkeySessionRepository(clientErr)
	if _, err := repoErr.GetActiveSession(ctx, "char-200"); !errors.Is(err, errGet) {
		t.Fatalf("expected errGet, got %v", err)
	}

	// 5. Default empty items slice when items JSON is missing
	rewardsNoItems := map[string]string{
		"exp":  "100",
		"gold": "50",
	}
	clientNoItems := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeStringMapResult(sessionMap),
			valkeytest.MakeStringMapResult(rewardsNoItems),
		}
	}))
	repoNoItems, _ := challenge.NewValkeySessionRepository(clientNoItems)
	sessNoItems, err := repoNoItems.GetActiveSession(ctx, "char-200")
	if err != nil || sessNoItems == nil {
		t.Fatalf("unexpected error on missing items: %v", err)
	}
	if sessNoItems.AccumulatedItems == nil || len(sessNoItems.AccumulatedItems) != 0 {
		t.Errorf("expected non-nil empty slice for items, got %v", sessNoItems.AccumulatedItems)
	}
}

func TestValkeySessionRepository_DeleteActiveSession(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeOKResult(),
			valkeytest.MakeOKResult(),
		}
	}))
	repo, _ := challenge.NewValkeySessionRepository(client)

	if err := repo.DeleteActiveSession(ctx, "char-del-1"); err != nil {
		t.Fatalf("unexpected DeleteActiveSession error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 delete commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0][0] != "DEL" || !strings.Contains(cmds[0][1], "char:char-del-1}:session") {
		t.Errorf("unexpected DEL session command: %v", cmds[0])
	}
	if cmds[1][0] != "DEL" || !strings.Contains(cmds[1][1], "char:char-del-1}:rewards") {
		t.Errorf("unexpected DEL rewards command: %v", cmds[1])
	}
}

func TestValkeySessionRepository_AdvanceRound(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	itemsJSON := challenge.EncodeJSON([]string{"reward-sword"})

	validAdvanceVals := []string{
		"2",                          // [0] new_round
		"175",                        // [1] surviving_hp
		"150",                        // [2] total_exp
		"300",                        // [3] total_gold
		itemsJSON,                    // [4] items
		now.Format(time.RFC3339Nano), // [5] updated_at
		"veteran",                    // [6] tier_id
		now.Format(time.RFC3339Nano), // [7] created_at
	}

	params := challenge.AdvanceRoundParams{
		ExpectedSessionID: "chal-advance-1",
		SurvivingHP:       175,
		ExpDelta:          50,
		GoldDelta:         100,
		RewardItemID:      "reward-sword",
		Now:               now,
	}

	// 1. Success case via EVALSHA
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeStringSliceResult(validAdvanceVals)
	}))
	repo, _ := challenge.NewValkeySessionRepository(client)

	outcome, err := repo.AdvanceRound(ctx, "char-advance-1", params)
	if err != nil {
		t.Fatalf("unexpected AdvanceRound error: %v", err)
	}

	if outcome.Status != challenge.StatusActive {
		t.Errorf("expected StatusActive, got %v", outcome.Status)
	}
	if outcome.Session.CurrentRound != 2 || outcome.Session.CharacterCurrentHP != 175 {
		t.Errorf("unexpected round/hp: %+v", outcome.Session)
	}
	if outcome.Session.AccumulatedExp != 150 || outcome.Session.AccumulatedGold != 300 {
		t.Errorf("unexpected rewards: %+v", outcome.Session)
	}
	if len(outcome.Session.AccumulatedItems) != 1 || outcome.Session.AccumulatedItems[0] != "reward-sword" {
		t.Errorf("unexpected items: %+v", outcome.Session.AccumulatedItems)
	}
	if outcome.Session.TierID != "veteran" {
		t.Errorf("unexpected tier: %s", outcome.Session.TierID)
	}

	// 2. Error mapping: ERR_SESSION_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_SESSION_NOT_FOUND"))
	}))
	repoNotFound, _ := challenge.NewValkeySessionRepository(clientNotFound)
	if _, err := repoNotFound.AdvanceRound(ctx, "char-advance-1", params); !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}

	// 3. Error mapping: ERR_SESSION_NOT_ACTIVE
	clientNotActive := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_SESSION_NOT_ACTIVE"))
	}))
	repoNotActive, _ := challenge.NewValkeySessionRepository(clientNotActive)
	if _, err := repoNotActive.AdvanceRound(ctx, "char-advance-1", params); !errors.Is(err, challenge.ErrSessionNotActive) {
		t.Fatalf("expected ErrSessionNotActive, got %v", err)
	}

	// 4. Error mapping: ERR_SESSION_ID_MISMATCH
	clientMismatch := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_SESSION_ID_MISMATCH"))
	}))
	repoMismatch, _ := challenge.NewValkeySessionRepository(clientMismatch)
	if _, err := repoMismatch.AdvanceRound(ctx, "char-advance-1", params); !errors.Is(err, challenge.ErrSessionIDMismatch) {
		t.Fatalf("expected ErrSessionIDMismatch, got %v", err)
	}

	// 5. Generic error
	genericErr := errors.New("lua execution error")
	clientGeneric := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(genericErr)
	}))
	repoGeneric, _ := challenge.NewValkeySessionRepository(clientGeneric)
	if _, err := repoGeneric.AdvanceRound(ctx, "char-advance-1", params); !errors.Is(err, genericErr) {
		t.Fatalf("expected genericErr, got %v", err)
	}

	// 6. Malformed response format (< 8 elements)
	clientMalformed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeStringSliceResult([]string{"2", "175"}) // only 2 elements
	}))
	repoMalformed, _ := challenge.NewValkeySessionRepository(clientMalformed)
	if _, err := repoMalformed.AdvanceRound(ctx, "char-advance-1", params); err == nil || !strings.Contains(err.Error(), "unexpected lua advance round response format") {
		t.Fatalf("expected malformed format error, got %v", err)
	}
}

func TestChallengeService_OfflineServiceQueriesAndOptions(t *testing.T) {
	ctx := context.Background()
	client := valkeytest.NewMockClient()
	valkeyStore, _ := challenge.NewValkeySessionRepository(client)

	baseRepo := newMockChallengeRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 50, Defense: 30}},
		},
	}
	engine := corebattle.Engine{}

	tierMap := map[string]challenge.ChallengeTier{
		"novice": {ID: "novice", Name: "Novice Tier", MinLevel: 1},
	}

	service, err := challenge.NewService(
		baseRepo,
		charRepo,
		engine,
		challenge.WithActiveSessionStore(valkeyStore),
		challenge.WithCustomTiers(tierMap),
		challenge.WithParticipantBuilder(nil),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Test GetCharacterRecords
	records, err := service.GetCharacterRecords(ctx, "char-1")
	if err != nil {
		t.Fatalf("GetCharacterRecords failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
	_, err = service.GetCharacterRecords(ctx, "")
	if err == nil {
		t.Errorf("expected error for empty characterID in GetCharacterRecords")
	}

	// Test GetLeaderboard
	lb, err := service.GetLeaderboard(ctx, "novice", 10)
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	if len(lb) != 0 {
		t.Errorf("expected 0 leaderboard entries, got %d", len(lb))
	}

	// Test ListHallOfFame
	hofList, err := service.ListHallOfFame(ctx)
	if err != nil {
		t.Fatalf("ListHallOfFame failed: %v", err)
	}
	if len(hofList) != 0 {
		t.Errorf("expected 0 hof entries, got %d", len(hofList))
	}

	// Test GetActiveSession with empty charID
	_, err = service.GetActiveSession(ctx, "")
	if err == nil {
		t.Errorf("expected error for empty charID in GetActiveSession")
	}

	// Test RetireSession / Cashout error branches
	_, err = service.RetireSession(ctx, "", "sess-1")
	if err == nil {
		t.Errorf("expected ErrCharacterNotFound for empty characterID")
	}
	_, err = service.Cashout(ctx, "nonexistent-sess")
	if !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound for nonexistent session cashout, got %v", err)
	}
}

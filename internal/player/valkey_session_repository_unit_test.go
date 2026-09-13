package player

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeySessionRepository_Options(t *testing.T) {
	client := valkeytest.NewMockClient()

	repoDef := NewValkeySessionRepository(client)
	if repoDef.sessionPrefix != DefaultSessionKeyPrefix || repoDef.playerPrefix != DefaultPlayerSessionsKeyPrefix {
		t.Errorf("expected default prefixes, got %q, %q", repoDef.sessionPrefix, repoDef.playerPrefix)
	}

	repoCustom := NewValkeySessionRepository(
		client,
		WithSessionKeyPrefix("custom:sess:"),
		WithPlayerSessionsKeyPrefix("custom:player:"),
		nil, // Nil option safely ignored
	)
	if repoCustom.sessionPrefix != "custom:sess:" || repoCustom.playerPrefix != "custom:player:" {
		t.Errorf("expected custom prefixes, got %q, %q", repoCustom.sessionPrefix, repoCustom.playerPrefix)
	}

	repoEmpty := NewValkeySessionRepository(client, WithSessionKeyPrefix(""), WithPlayerSessionsKeyPrefix(""))
	if repoEmpty.sessionPrefix != DefaultSessionKeyPrefix || repoEmpty.playerPrefix != DefaultPlayerSessionsKeyPrefix {
		t.Errorf("expected default prefixes on empty string, got %q, %q", repoEmpty.sessionPrefix, repoEmpty.playerPrefix)
	}
}

func TestValkeySessionRepository_KeyHelpers(t *testing.T) {
	repo := NewValkeySessionRepository(nil)
	if got := repo.sessionKey("token123"); got != DefaultSessionKeyPrefix+"token123" {
		t.Errorf("unexpected sessionKey: %q", got)
	}
	if got := repo.playerSessionsKey("player456"); got != DefaultPlayerSessionsKeyPrefix+"player456" {
		t.Errorf("unexpected playerSessionsKey: %q", got)
	}
}

func TestValkeySessionRepository_Save_Validations(t *testing.T) {
	ctx := context.Background()
	repo := NewValkeySessionRepository(nil)
	testCases := []coreplayer.Session{
		{ID: "", PlayerID: "p1", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "   \t", PlayerID: "p1", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "tok1", PlayerID: "", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "tok1", PlayerID: "  \n  ", ExpiresAt: time.Now().Add(time.Hour)},
	}
	for i, sess := range testCases {
		if err := repo.Save(ctx, sess); !errors.Is(err, coreplayer.ErrInvalidSession) {
			t.Errorf("case %d: expected ErrInvalidSession, got %v", i, err)
		}
	}
}

func TestValkeySessionRepository_Save_MockClient(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := NewValkeySessionRepository(client)
	sess := coreplayer.Session{
		ID:        "tok-success",
		PlayerID:  "player-success",
		CreatedAt: now,
		ExpiresAt: now.Add(2 * time.Hour),
	}

	if err := repo.Save(ctx, sess); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands (SET, ZADD, ZREMRANGEBYSCORE, EXPIRE), got %d: %v", len(cmds), cmds)
	}
	if cmds[0][0] != "SET" || cmds[0][1] != DefaultSessionKeyPrefix+"tok-success" {
		t.Errorf("unexpected SET command: %v", cmds[0])
	}
	if cmds[1][0] != "ZADD" || cmds[1][1] != DefaultPlayerSessionsKeyPrefix+"player-success" {
		t.Errorf("unexpected ZADD command: %v", cmds[1])
	}
	if cmds[2][0] != "ZREMRANGEBYSCORE" || cmds[2][1] != DefaultPlayerSessionsKeyPrefix+"player-success" {
		t.Errorf("unexpected ZREMRANGEBYSCORE command: %v", cmds[2])
	}
	if cmds[3][0] != "EXPIRE" || cmds[3][1] != DefaultPlayerSessionsKeyPrefix+"player-success" {
		t.Errorf("unexpected EXPIRE command: %v", cmds[3])
	}
	if repo.MemorySessionCount() != 0 {
		t.Errorf("expected 0 sessions in memory store, got %d", repo.MemorySessionCount())
	}
}

func TestValkeySessionRepository_Save_TTLEdgeCases(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Case A: ttl <= 0 -> resets to SessionDuration (604800s)
	clExp := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoExp := NewValkeySessionRepository(clExp)
	if err := repoExp.Save(ctx, coreplayer.Session{ID: "tok-past", PlayerID: "p-past", ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatalf("Save(expired) failed: %v", err)
	}
	cmdsExp := clExp.RecordedCommandStrings()
	if len(cmdsExp) > 0 && (cmdsExp[0][3] != "EX" || cmdsExp[0][4] != "604800") {
		t.Errorf("expected EX 604800 for expired session fallback, got %v", cmdsExp[0])
	}

	// Case B: sub-second ttl -> clamped to 1s
	clShort := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoShort := NewValkeySessionRepository(clShort)
	if err := repoShort.Save(ctx, coreplayer.Session{ID: "tok-short", PlayerID: "p-short", ExpiresAt: now.Add(200 * time.Millisecond)}); err != nil {
		t.Fatalf("Save(short) failed: %v", err)
	}
	cmdsShort := clShort.RecordedCommandStrings()
	if len(cmdsShort) > 0 && (cmdsShort[0][3] != "EX" || cmdsShort[0][4] != "1") {
		t.Errorf("expected EX 1 for sub-second TTL, got %v", cmdsShort[0])
	}
}

func TestValkeySessionRepository_Save_ErrorsAndWrongTypeRecovery(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	sess := coreplayer.Session{ID: "tok-err", PlayerID: "player-err", ExpiresAt: now.Add(time.Hour)}

	// 1. SET error
	clSetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("connection reset"))
	}))
	if err := NewValkeySessionRepository(clSetErr).Save(ctx, sess); err == nil || !strings.Contains(err.Error(), "save session to valkey") {
		t.Fatalf("expected 'save session to valkey' error, got %v", err)
	}

	// 2. WRONGTYPE on ZADD triggers DEL and retry ZADD
	zaddCount := 0
	clWrongType := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if len(c) > 0 && c[0] == "ZADD" {
			zaddCount++
			if zaddCount == 1 {
				return valkeytest.MakeErrorResult(errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"))
			}
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clWrongType).Save(ctx, sess); err != nil {
		t.Fatalf("expected successful WRONGTYPE recovery, got %v", err)
	}

	var hasDel bool
	for _, c := range clWrongType.RecordedCommandStrings() {
		if len(c) > 0 && c[0] == "DEL" && c[1] == DefaultPlayerSessionsKeyPrefix+"player-err" {
			hasDel = true
		}
	}
	if !hasDel || zaddCount != 2 {
		t.Errorf("expected DEL and retry ZADD on WRONGTYPE, hasDel=%v, zaddCount=%d", hasDel, zaddCount)
	}
}

func TestValkeySessionRepository_FindByID_Validations(t *testing.T) {
	ctx := context.Background()
	repo := NewValkeySessionRepository(nil)
	if _, err := repo.FindByID(ctx, ""); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for empty id, got %v", err)
	}
	if _, err := repo.FindByID(ctx, "   \t\n  "); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for whitespace id, got %v", err)
	}
}

func TestValkeySessionRepository_FindByID_MockClient(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Successful JSON session retrieval
	validSess := coreplayer.Session{ID: "tok-find-json", PlayerID: "player-json", ExpiresAt: now.Add(time.Hour)}
	sessJSON, _ := json.Marshal(validSess)
	clJSON := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "GET" {
			return valkeytest.MakeStringResult(string(sessJSON))
		}
		return valkeytest.MakeOKResult()
	}))
	got, err := NewValkeySessionRepository(clJSON).FindByID(ctx, validSess.ID)
	if err != nil || got.ID != validSess.ID || got.PlayerID != validSess.PlayerID {
		t.Fatalf("FindByID failed: got %+v, err: %v", got, err)
	}

	// 2. Legacy string-only token mapping fallback
	clPlain := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "GET" {
			return valkeytest.MakeStringResult("legacy-player-777")
		}
		return valkeytest.MakeOKResult()
	}))
	gotPlain, err := NewValkeySessionRepository(clPlain).FindByID(ctx, "tok-plain")
	if err != nil || gotPlain.PlayerID != "legacy-player-777" || gotPlain.ID != "tok-plain" {
		t.Fatalf("FindByID(plain) failed: got %+v, err: %v", gotPlain, err)
	}

	// 3. Valkey Nil
	clNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	if _, err := NewValkeySessionRepository(clNil).FindByID(ctx, "nonexistent"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession on Valkey nil, got %v", err)
	}

	// 4. Corrupted JSON returned
	clCorrupt := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("{corrupt-json")
	}))
	if _, err := NewValkeySessionRepository(clCorrupt).FindByID(ctx, "corrupt-tok"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession on corrupt JSON, got %v", err)
	}

	// 5. Expired session JSON
	expJSON, _ := json.Marshal(coreplayer.Session{ID: "tok-exp", PlayerID: "p-exp", ExpiresAt: now.Add(-time.Hour)})
	clExp := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult(string(expJSON))
	}))
	if _, err := NewValkeySessionRepository(clExp).FindByID(ctx, "tok-exp"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession on expired session, got %v", err)
	}

	// 6. Transient network error with in-memory store fallback populated
	clTrans := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("valkey cluster down"))
	}))
	repoTrans := NewValkeySessionRepository(clTrans)
	memSess := coreplayer.Session{ID: "tok-mem", PlayerID: "p-mem", ExpiresAt: now.Add(time.Hour)}
	repoTrans.memorySessions[memSess.ID] = memSess

	if gotMem, err := repoTrans.FindByID(ctx, memSess.ID); err != nil || gotMem.PlayerID != "p-mem" {
		t.Fatalf("expected in-memory fallback, got %+v, err: %v", gotMem, err)
	}
	if _, err := repoTrans.FindByID(ctx, "tok-not-in-mem"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession when session is not in memory, got %v", err)
	}
}

func TestValkeySessionRepository_Revoke_MockClient(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repoEmpty := NewValkeySessionRepository(nil)
	if err := repoEmpty.Revoke(ctx, "   \t", now); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for whitespace id, got %v", err)
	}

	// 1. Normal JSON session revoked from Valkey
	validSess := coreplayer.Session{ID: "tok-rev-1", PlayerID: "p-rev-1", ExpiresAt: now.Add(time.Hour)}
	sessJSON, _ := json.Marshal(validSess)
	clJSON := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "GETDEL" {
			return valkeytest.MakeStringResult(string(sessJSON))
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clJSON).Revoke(ctx, validSess.ID, now); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// 2. Legacy string session revoked from Valkey
	clLegacy := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "GETDEL" {
			return valkeytest.MakeStringResult("legacy-player-revoke")
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clLegacy).Revoke(ctx, "tok-legacy", now); err != nil {
		t.Fatalf("Revoke legacy failed: %v", err)
	}

	// 3. GETDEL returns nil, but session is present in memory
	clNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoMem := NewValkeySessionRepository(clNil)
	memSess := coreplayer.Session{ID: "tok-mem-rev", PlayerID: "p-mem", ExpiresAt: now.Add(time.Hour)}
	repoMem.memorySessions[memSess.ID] = memSess

	if err := repoMem.Revoke(ctx, memSess.ID, now); err != nil {
		t.Fatalf("Revoke from in-memory fallback failed: %v", err)
	}
	if repoMem.MemorySessionCount() != 0 {
		t.Errorf("expected 0 in-memory sessions, got %d", repoMem.MemorySessionCount())
	}
	if err := repoMem.Revoke(ctx, "unknown-id", now); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession, got %v", err)
	}
}

func TestValkeySessionRepository_DeleteByPlayerID_MockClient(t *testing.T) {
	ctx := context.Background()

	if err := NewValkeySessionRepository(nil).DeleteByPlayerID(ctx, "   "); err != nil {
		t.Fatalf("expected nil for empty playerID, got %v", err)
	}

	// 1. Normal flow: ZRANGE returns 2 members
	clNormal := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "ZRANGE" {
			return valkeytest.MakeStringSliceResult([]string{"tok-a", "tok-b"})
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clNormal).DeleteByPlayerID(ctx, "target-p1"); err != nil {
		t.Fatalf("DeleteByPlayerID failed: %v", err)
	}

	// 2. Empty ZRANGE result
	clEmpty := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 && c[0] == "ZRANGE" {
			return valkeytest.MakeStringSliceResult([]string{})
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clEmpty).DeleteByPlayerID(ctx, "target-empty"); err != nil {
		t.Fatalf("DeleteByPlayerID(empty) failed: %v", err)
	}

	// 3. WRONGTYPE recovery with SMEMBERS fallback
	clWrong := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if c := cmd.Commands(); len(c) > 0 {
			if c[0] == "ZRANGE" {
				return valkeytest.MakeErrorResult(errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"))
			}
			if c[0] == "SMEMBERS" {
				return valkeytest.MakeStringSliceResult([]string{"legacy-tok"})
			}
		}
		return valkeytest.MakeOKResult()
	}))
	if err := NewValkeySessionRepository(clWrong).DeleteByPlayerID(ctx, "target-legacy"); err != nil {
		t.Fatalf("DeleteByPlayerID(wrongtype) failed: %v", err)
	}
}

func TestValkeySessionRepository_PurgeExpiredZSet_EdgeCases(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Client is nil: safe no-op
	NewValkeySessionRepository(nil).purgeExpiredZSet(ctx, "some-key", now)

	// PlayerKey is empty: safe no-op
	cl := valkeytest.NewMockClient()
	repo := NewValkeySessionRepository(cl)
	repo.purgeExpiredZSet(ctx, "", now)
	repo.purgeExpiredZSet(ctx, "   \t", now)
	if len(cl.RecordedCommandStrings()) != 0 {
		t.Errorf("expected no commands executed for empty playerKey, got %v", cl.RecordedCommandStrings())
	}
}

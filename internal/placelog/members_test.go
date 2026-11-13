package placelog_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestMembersReadFailure(t *testing.T) {
	broken := placelog.NewMembers(valkeytest.NewMockClient(valkeytest.WithDoHandler(func(context.Context, valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("down"))
	})), 0)
	if _, err := broken.Read(context.Background(), placelog.Place{Kind: "facility", ID: "bank"}, time.Now()); err == nil {
		t.Fatal("store failure must not read as an empty member list")
	}
}

func realMembers(t *testing.T) (*placelog.Members, valkey.Client, placelog.Place) {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(client.Close)
	p := placelog.Place{Kind: "test", ID: id.New()}
	t.Cleanup(func() {
		base := "party2:placemember:{test:" + p.ID + "}:"
		client.Do(context.Background(), client.B().Del().Key(base+"joined", base+"expiry", base+"profile", base+"token").Build())
	})
	return placelog.NewMembers(client, 300*time.Second), client, p
}

func memberIDs(t *testing.T, s *placelog.Members, p placelog.Place, now time.Time) []string {
	t.Helper()
	got, err := s.Read(context.Background(), p, now)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, m := range got {
		ids = append(ids, m.CharacterID)
	}
	return ids
}

func newMember(t *testing.T, client valkey.Client, name string) placelog.Member {
	m := placelog.Member{CharacterID: id.New(), Name: name, Color: "#FFFFFF"}
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key("party2:placemember:character:"+m.CharacterID).Build())
	})
	return m
}

func TestValkeyMembersExpiryOrderAndLeave(t *testing.T) {
	s, client, p := realMembers(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	a, b := newMember(t, client, "A"), newMember(t, client, "B")

	if got := memberIDs(t, s, p, t0); len(got) != 0 {
		t.Fatalf("empty place = %v", got)
	}
	if err := s.Refresh(ctx, p, a, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(ctx, p, b, t0.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	// A refresh keeps the arrival position and updates the appearance.
	a.Name = "A2"
	if err := s.Refresh(ctx, p, a, t0.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, p, t0.Add(20*time.Second))
	if err != nil || len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("members = %+v, %v; want A2 then B", got, err)
	}
	// B was last seen at t0+10: listed at exactly +300 s, gone one second later.
	if ids := memberIDs(t, s, p, t0.Add(310*time.Second)); len(ids) != 2 {
		t.Fatalf("at the boundary = %v; want both", ids)
	}
	if ids := memberIDs(t, s, p, t0.Add(311*time.Second)); len(ids) != 1 || ids[0] != a.CharacterID {
		t.Fatalf("after B expired = %v; want only A", ids)
	}
	// An expired member that returns is appended after the others.
	if err := s.Refresh(ctx, p, b, t0.Add(312*time.Second)); err != nil {
		t.Fatal(err)
	}
	if ids := memberIDs(t, s, p, t0.Add(312*time.Second)); len(ids) != 2 || ids[1] != b.CharacterID {
		t.Fatalf("returning member = %v; want B last", ids)
	}
	if err := s.Leave(ctx, a.CharacterID); err != nil {
		t.Fatal(err)
	}
	if err := s.Leave(ctx, a.CharacterID); err != nil {
		t.Fatalf("leaving twice: %v", err)
	}
	if ids := memberIDs(t, s, p, t0.Add(312*time.Second)); len(ids) != 1 || ids[0] != b.CharacterID {
		t.Fatalf("after A left = %v; want only B", ids)
	}
	ttl, err := client.Do(ctx, client.B().Ttl().Key("party2:placemember:{test:"+p.ID+"}:profile").Build()).AsInt64()
	if err != nil || ttl <= 0 || ttl > 301 {
		t.Fatalf("ttl = %d, %v; want refreshed within 301 s", ttl, err)
	}
}

func TestValkeyMembersPhysicalTTLBoundary(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(client.Close)

	newPlace := func() placelog.Place {
		p := placelog.Place{Kind: "test", ID: id.New()}
		t.Cleanup(func() {
			base := "party2:placemember:{test:" + p.ID + "}:"
			client.Do(context.Background(), client.B().Del().Key(base+"joined", base+"expiry", base+"profile", base+"token").Build())
		})
		return p
	}

	ctx := context.Background()
	s := placelog.NewMembers(client, time.Second)

	// 1. Single member in an idle place stays visible at the inclusive boundary
	// under real Valkey expiry, hides beyond it, and expires physically afterwards.
	p1 := newPlace()
	a := newMember(t, client, "A")
	t0 := time.Now()
	if err := s.Refresh(ctx, p1, a, t0); err != nil {
		t.Fatal(err)
	}
	if ids := memberIDs(t, s, p1, t0); !slices.Equal(ids, []string{a.CharacterID}) {
		t.Fatalf("immediate read = %v; want [%s]", ids, a.CharacterID)
	}

	time.Sleep(1100 * time.Millisecond)

	if ids := memberIDs(t, s, p1, t0.Add(time.Second)); !slices.Equal(ids, []string{a.CharacterID}) {
		t.Fatalf("inclusive TTL boundary lost member: %v", ids)
	}
	if ids := memberIDs(t, s, p1, t0.Add(2*time.Second)); len(ids) != 0 {
		t.Fatalf("beyond boundary = %v; want empty", ids)
	}
	base1 := "party2:placemember:{test:" + p1.ID + "}:"
	existsBeforeCleanup, err := client.Do(ctx, client.B().Exists().Key(base1+"expiry").Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	if existsBeforeCleanup != 1 {
		t.Fatalf("expiry key missing before physical retention elapsed: exists=%d", existsBeforeCleanup)
	}
	// Verify keys expire after physical retention without GET writes extending them.
	time.Sleep(1100 * time.Millisecond)
	for _, k := range []string{
		base1 + "joined",
		base1 + "expiry",
		base1 + "profile",
		base1 + "token",
		"party2:placemember:character:" + a.CharacterID,
	} {
		exists, err := client.Do(ctx, client.B().Exists().Key(k).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		if exists != 0 {
			t.Fatalf("key %s still exists after physical expiry", k)
		}
	}

	// 2. Character pointer remains valid at the boundary for Leave and movement.
	p2 := newPlace()
	p3 := newPlace()
	p4 := newPlace()
	b := newMember(t, client, "B")
	c := newMember(t, client, "C")
	t1 := time.Now()
	if err := s.Refresh(ctx, p2, b, t1); err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(ctx, p3, c, t1); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)

	if err := s.Leave(ctx, b.CharacterID); err != nil {
		t.Fatal(err)
	}
	if pointer := readPointerPlace(t, ctx, client, b.CharacterID); pointer != "" {
		t.Fatalf("pointer after boundary Leave = %q; want empty", pointer)
	}
	assertPlaceMembershipState(t, ctx, client, s, p2, t1.Add(time.Second), b.CharacterID, false)

	if err := s.Refresh(ctx, p4, c, t1.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if pointer := readPointerPlace(t, ctx, client, c.CharacterID); pointer != p4.Kind+":"+p4.ID {
		t.Fatalf("pointer after boundary move = %q; want %s:%s", pointer, p4.Kind, p4.ID)
	}
	assertPlaceMembershipState(t, ctx, client, s, p3, t1.Add(time.Second), c.CharacterID, false)
	assertPlaceMembershipState(t, ctx, client, s, p4, t1.Add(time.Second), c.CharacterID, true)
}

func TestValkeyMembersOnePlaceAtATime(t *testing.T) {
	s, client, p := realMembers(t)
	_, _, q := realMembers(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")
	if err := s.Refresh(ctx, p, a, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(ctx, q, a, now); err != nil {
		t.Fatal(err)
	}
	if ids := memberIDs(t, s, p, now); len(ids) != 0 {
		t.Fatalf("old place still lists %v", ids)
	}
	if ids := memberIDs(t, s, q, now); len(ids) != 1 {
		t.Fatalf("new place = %v; want A", ids)
	}
}

func TestValkeyMembersCorruptProfileFailsRead(t *testing.T) {
	s, client, p := realMembers(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")
	if err := s.Refresh(ctx, p, a, now); err != nil {
		t.Fatal(err)
	}
	client.Do(ctx, client.B().Hset().Key("party2:placemember:{test:"+p.ID+"}:profile").FieldValue().FieldValue(a.CharacterID, "{bad").Build())
	if _, err := s.Read(ctx, p, now); err == nil {
		t.Fatal("corrupt profile must fail the read")
	}
}

func TestValkeyMembersConcurrentRefreshNoDuplicates(t *testing.T) {
	s, client, p := realMembers(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	members := []placelog.Member{newMember(t, client, "A"), newMember(t, client, "B"), newMember(t, client, "C")}
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(w, op int) error {
		m := members[(w+op)%len(members)]
		m.Name = fmt.Sprintf("%s-%d", m.Name, op)
		return s.Refresh(ctx, p, m, now)
	})
	if res.Failures != 0 {
		t.Fatalf("refresh failures: %d", res.Failures)
	}
	if ids := memberIDs(t, s, p, now); len(ids) != len(members) {
		t.Fatalf("members = %v; want each character once", ids)
	}
}

func TestValkeyMembersSameSecondArrivalOrder(t *testing.T) {
	s, client, p := realMembers(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	// IDs sort opposite to arrival, so a tie broken by member ID would reverse them.
	late, early := newMember(t, client, "Z"), newMember(t, client, "A")
	late.CharacterID, early.CharacterID = "z"+late.CharacterID, "a"+early.CharacterID
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key("party2:placemember:character:"+late.CharacterID, "party2:placemember:character:"+early.CharacterID).Build())
	})
	for _, m := range []placelog.Member{late, early, late} {
		if err := s.Refresh(ctx, p, m, now); err != nil {
			t.Fatal(err)
		}
	}
	if ids := memberIDs(t, s, p, now); len(ids) != 2 || ids[0] != late.CharacterID || ids[1] != early.CharacterID {
		t.Fatalf("same-second order = %v; want arrival order %s, %s", ids, late.CharacterID, early.CharacterID)
	}
}

// Concurrent refreshes and leaves for one character always converge to matching
// place lists and pointer with no duplicates, unreachable rows, or mixed state.
func TestValkeyMembersConcurrentRefreshAndLeave(t *testing.T) {
	s, client, p := realMembers(t)
	_, _, q := realMembers(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(w, op int) error {
		switch (w + op) % 3 {
		case 0:
			return s.Refresh(ctx, p, a, now)
		case 1:
			return s.Refresh(ctx, q, a, now)
		default:
			return s.Leave(ctx, a.CharacterID)
		}
	})
	if res.Failures != 0 {
		t.Fatalf("failures: %d", res.Failures)
	}
	pointerPlace := readPointerPlace(t, ctx, client, a.CharacterID)
	for _, place := range []placelog.Place{p, q} {
		wantPresent := pointerPlace == place.Kind+":"+place.ID
		assertPlaceMembershipState(t, ctx, client, s, place, now, a.CharacterID, wantPresent)
	}
	if err := s.Leave(ctx, a.CharacterID); err != nil {
		t.Fatal(err)
	}
	if pointerPlace := readPointerPlace(t, ctx, client, a.CharacterID); pointerPlace != "" {
		t.Fatalf("pointer after final leave = %q; want empty", pointerPlace)
	}
	for _, place := range []placelog.Place{p, q} {
		assertPlaceMembershipState(t, ctx, client, s, place, now, a.CharacterID, false)
	}
}

type cleanupGateClient struct {
	valkey.Client
	targetJoinedKey string
	once            sync.Once
	arrived         chan struct{}
	release         chan struct{}
}

func newCleanupGateClient(base valkey.Client, target placelog.Place) *cleanupGateClient {
	return &cleanupGateClient{
		Client:          base,
		targetJoinedKey: "party2:placemember:{" + target.Kind + ":" + target.ID + "}:joined",
		arrived:         make(chan struct{}),
		release:         make(chan struct{}),
	}
}

func (c *cleanupGateClient) gateIfCleanup(cmds ...valkey.Completed) {
	for _, cmd := range cmds {
		args := cmd.Commands()
		if len(args) == 0 {
			continue
		}
		isOldCleanup := args[0] == "ZREM" && len(args) >= 2 && args[1] == c.targetJoinedKey
		// remove_member.lua passes 4 KEYS + 2 ARGV (9 tokens total), while refresh_member.lua passes 6 ARGV (13 tokens).
		isLuaCleanup := (args[0] == "EVALSHA" || args[0] == "EVAL") && len(args) == 9 && args[2] == "4" && args[3] == c.targetJoinedKey
		if isOldCleanup || isLuaCleanup {
			c.once.Do(func() {
				close(c.arrived)
				<-c.release
			})
			return
		}
	}
}

func (c *cleanupGateClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	c.gateIfCleanup(cmd)
	return c.Client.Do(ctx, cmd)
}

func (c *cleanupGateClient) DoMulti(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
	c.gateIfCleanup(multi...)
	return c.Client.DoMulti(ctx, multi...)
}

func readPointerPlace(t *testing.T, ctx context.Context, client valkey.Client, characterID string) string {
	t.Helper()
	raw, err := client.Do(ctx, client.B().Get().Key("party2:placemember:character:"+characterID).Build()).ToString()
	if errors.Is(err, valkey.Nil) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	if i := strings.LastIndexByte(raw, '|'); i >= 0 {
		return raw[:i]
	}
	return raw
}

func assertPlaceMembershipState(t *testing.T, ctx context.Context, client valkey.Client, s *placelog.Members, place placelog.Place, now time.Time, characterID string, wantPresent bool) {
	t.Helper()
	ids := memberIDs(t, s, place, now)
	wantCount := int64(0)
	if wantPresent {
		wantCount = 1
		if !slices.Equal(ids, []string{characterID}) {
			t.Fatalf("%s lists %v; want [%s]", place.ID, ids, characterID)
		}
	} else if len(ids) != 0 {
		t.Fatalf("%s lists %v; want empty", place.ID, ids)
	}
	base := "party2:placemember:{" + place.Kind + ":" + place.ID + "}:"
	joined, err := client.Do(ctx, client.B().Zcard().Key(base+"joined").Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	expiry, err := client.Do(ctx, client.B().Zcard().Key(base+"expiry").Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	profile, err := client.Do(ctx, client.B().Hlen().Key(base+"profile").Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.Do(ctx, client.B().Hlen().Key(base+"token").Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	if joined != wantCount || expiry != wantCount || profile != wantCount || token != wantCount {
		t.Fatalf("%s mixed state: joined=%d expiry=%d profile=%d token=%d (want %d)", place.ID, joined, expiry, profile, token, wantCount)
	}
}

func TestValkeyMembersStaleRefreshCleanupPreservesNewerMembership(t *testing.T) {
	_, client, p := realMembers(t)
	_, _, q := realMembers(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")

	gated := newCleanupGateClient(client, p)
	s := placelog.NewMembers(gated, 300*time.Second)

	if err := s.Refresh(ctx, p, a, t0); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Refresh(ctx, q, a, t0.Add(time.Second))
	}()

	select {
	case <-gated.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker A to reach cleanup boundary")
	}

	if err := s.Refresh(ctx, p, a, t0.Add(2*time.Second)); err != nil {
		close(gated.release)
		t.Fatal(err)
	}

	close(gated.release)
	if err := <-errCh; err != nil {
		t.Fatalf("worker A refresh failed: %v", err)
	}

	if pointer := readPointerPlace(t, ctx, client, a.CharacterID); pointer != p.Kind+":"+p.ID {
		t.Fatalf("pointer = %q; want %s:%s", pointer, p.Kind, p.ID)
	}
	assertPlaceMembershipState(t, ctx, client, s, p, t0.Add(2*time.Second), a.CharacterID, true)
	assertPlaceMembershipState(t, ctx, client, s, q, t0.Add(2*time.Second), a.CharacterID, false)
}

func TestValkeyMembersStaleLeaveCleanupPreservesNewerRefresh(t *testing.T) {
	_, client, p := realMembers(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")

	gated := newCleanupGateClient(client, p)
	s := placelog.NewMembers(gated, 300*time.Second)

	if err := s.Refresh(ctx, p, a, t0); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Leave(ctx, a.CharacterID)
	}()

	select {
	case <-gated.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Leave to reach cleanup boundary")
	}

	if err := s.Refresh(ctx, p, a, t0.Add(time.Second)); err != nil {
		close(gated.release)
		t.Fatal(err)
	}

	close(gated.release)
	if err := <-errCh; err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	if pointer := readPointerPlace(t, ctx, client, a.CharacterID); pointer != p.Kind+":"+p.ID {
		t.Fatalf("pointer = %q; want %s:%s", pointer, p.Kind, p.ID)
	}
	assertPlaceMembershipState(t, ctx, client, s, p, t0.Add(time.Second), a.CharacterID, true)
}

func TestValkeyMembersStaleMoveCleanupAfterLeavePreservesReturnToOrigin(t *testing.T) {
	_, client, p := realMembers(t)
	_, _, q := realMembers(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	a := newMember(t, client, "A")

	gated := newCleanupGateClient(client, p)
	s := placelog.NewMembers(gated, 300*time.Second)

	if err := s.Refresh(ctx, p, a, t0); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Refresh(ctx, q, a, t0.Add(time.Second))
	}()

	select {
	case <-gated.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for move refresh to reach cleanup boundary")
	}

	if err := s.Leave(ctx, a.CharacterID); err != nil {
		close(gated.release)
		t.Fatal(err)
	}
	if err := s.Refresh(ctx, p, a, t0.Add(2*time.Second)); err != nil {
		close(gated.release)
		t.Fatal(err)
	}

	close(gated.release)
	if err := <-errCh; err != nil {
		t.Fatalf("move refresh failed: %v", err)
	}

	if pointer := readPointerPlace(t, ctx, client, a.CharacterID); pointer != p.Kind+":"+p.ID {
		t.Fatalf("pointer = %q; want %s:%s", pointer, p.Kind, p.ID)
	}
	assertPlaceMembershipState(t, ctx, client, s, p, t0.Add(2*time.Second), a.CharacterID, true)
	assertPlaceMembershipState(t, ctx, client, s, q, t0.Add(2*time.Second), a.CharacterID, false)
}

func TestMembersCleanupFailurePropagates(t *testing.T) {
	errBoom := errors.New("cleanup boom")
	failOnCleanup := func(prevPlace string) valkey.Client {
		return valkeytest.NewMockClient(
			valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				args := cmd.Commands()
				switch args[0] {
				case "SET":
					return valkeytest.MakeStringResult(prevPlace)
				case "GETDEL":
					return valkeytest.MakeStringResult(prevPlace)
				case "EVALSHA", "EVAL":
					// remove_member.lua has 2 ARGV (len 9); refresh_member.lua has 6 ARGV (len 13).
					if len(args) == 9 {
						return valkeytest.MakeErrorResult(errBoom)
					}
					return valkeytest.MakeIntResult(1)
				default:
					return valkeytest.MakeIntResult(1)
				}
			}),
			valkeytest.WithDoMultiHandler(func(_ context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
				res := make([]valkey.ValkeyResult, len(multi))
				for i := range res {
					res[i] = valkeytest.MakeErrorResult(errBoom)
				}
				return res
			}),
		)
	}

	sRefresh := placelog.NewMembers(failOnCleanup("facility:park|tok"), 300*time.Second)
	err := sRefresh.Refresh(context.Background(), placelog.Place{Kind: placelog.KindFacility, ID: "bank"}, placelog.Member{CharacterID: "c1", Name: "A"}, time.Unix(1_800_000_000, 0))
	if !errors.Is(err, errBoom) {
		t.Fatalf("Refresh cleanup error = %v; want %v", err, errBoom)
	}

	sLeave := placelog.NewMembers(failOnCleanup("facility:park|tok"), 300*time.Second)
	err = sLeave.Leave(context.Background(), "c1")
	if !errors.Is(err, errBoom) {
		t.Fatalf("Leave cleanup error = %v; want %v", err, errBoom)
	}
}

func TestValkeyMembersBoundedExpiryCleanupAndReturningOrder(t *testing.T) {
	s, client, p := realMembers(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	base := "party2:placemember:{test:" + p.ID + "}:"

	const expiredCount = 100
	joinedCmd := client.B().Zadd().Key(base + "joined").ScoreMember()
	expiryCmd := client.B().Zadd().Key(base + "expiry").ScoreMember()
	profileCmd := client.B().Hset().Key(base + "profile").FieldValue()
	for i := 0; i < expiredCount; i++ {
		cid := fmt.Sprintf("exp-%03d-%s", i, id.New())
		joinedCmd = joinedCmd.ScoreMember(float64(i+1), cid)
		expiryCmd = expiryCmd.ScoreMember(float64(t0.Add(300*time.Second).Unix()), cid)
		profileCmd = profileCmd.FieldValue(cid, fmt.Sprintf(`{"character_id":%q,"name":"E","color":"#FFFFFF"}`, cid))
	}
	for _, r := range client.DoMulti(ctx, joinedCmd.Build(), expiryCmd.Build(), profileCmd.Build()) {
		if err := r.Error(); err != nil {
			t.Fatal(err)
		}
	}

	returning := newMember(t, client, "Returning")
	live := newMember(t, client, "Live")
	// returning expires at t0+310s (after the 100 entries at t0+300s, so outside the first batch).
	if err := s.Refresh(ctx, p, returning, t0.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(ctx, p, live, t0.Add(200*time.Second)); err != nil {
		t.Fatal(err)
	}

	now := t0.Add(400 * time.Second)
	// GET-only read hides all expired entries without mutating stored cardinality.
	if ids := memberIDs(t, s, p, now); len(ids) != 1 || ids[0] != live.CharacterID {
		t.Fatalf("before refresh = %v; want only live %s", ids, live.CharacterID)
	}
	card := func(key string) int64 {
		n, err := client.Do(ctx, client.B().Zcard().Key(key).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	hlen := func(key string) int64 {
		n, err := client.Do(ctx, client.B().Hlen().Key(key).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := card(base + "expiry"); got != expiredCount+2 {
		t.Fatalf("read mutated expiry cardinality to %d, want %d", got, expiredCount+2)
	}

	// Returning expired actor refreshes while its old row is outside the bounded purge batch.
	if err := s.Refresh(ctx, p, returning, now); err != nil {
		t.Fatal(err)
	}
	afterOne := card(base + "expiry")
	if afterOne <= 2 || afterOne >= expiredCount+2 {
		t.Fatalf("after 1 refresh expiry ZCARD = %d; want bounded partial cleanup (between 2 and %d)", afterOne, expiredCount+2)
	}
	if card(base+"joined") != afterOne || hlen(base+"profile") != afterOne {
		t.Fatalf("inconsistent sizes joined=%d expiry=%d profile=%d", card(base+"joined"), afterOne, hlen(base+"profile"))
	}
	if ids := memberIDs(t, s, p, now); len(ids) != 2 || ids[0] != live.CharacterID || ids[1] != returning.CharacterID {
		t.Fatalf("after returning refresh = %v; want [%s, %s]", ids, live.CharacterID, returning.CharacterID)
	}

	// Repeated refreshes eventually reclaim all leftover expired entries.
	steps := 0
	for card(base+"expiry") > 2 && steps < 20 {
		steps++
		if err := s.Refresh(ctx, p, live, now); err != nil {
			t.Fatal(err)
		}
	}
	if steps < 2 || card(base+"expiry") != 2 || card(base+"joined") != 2 || hlen(base+"profile") != 2 {
		t.Fatalf("reclaim did not converge cleanly in %d steps: joined=%d expiry=%d profile=%d",
			steps, card(base+"joined"), card(base+"expiry"), hlen(base+"profile"))
	}
	if ids := memberIDs(t, s, p, now); len(ids) != 2 || ids[0] != live.CharacterID || ids[1] != returning.CharacterID {
		t.Fatalf("after full reclaim = %v; want [%s, %s]", ids, live.CharacterID, returning.CharacterID)
	}
	for _, suffix := range []string{"joined", "expiry", "profile", "token"} {
		ttl, err := client.Do(ctx, client.B().Ttl().Key(base+suffix).Build()).AsInt64()
		if err != nil || ttl <= 0 || ttl > 301 {
			t.Fatalf("%s ttl = %d, %v; want positive within 301s", suffix, ttl, err)
		}
	}
}

func BenchmarkValkeyMembersRefreshExpiredCohort(b *testing.B) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		b.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		b.Fatalf("connect valkey: %v", err)
	}
	b.Cleanup(client.Close)

	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	now := t0.Add(400 * time.Second)

	for _, cohort := range []int{0, 100, 1000, 5000} {
		b.Run(fmt.Sprintf("expired=%d", cohort), func(b *testing.B) {
			p := placelog.Place{Kind: "bench", ID: id.New()}
			base := "party2:placemember:{bench:" + p.ID + "}:"
			actor := placelog.Member{CharacterID: "actor-" + id.New(), Name: "Actor", Color: "#FFFFFF"}
			b.Cleanup(func() {
				client.Do(context.Background(), client.B().Del().Key(
					base+"joined", base+"expiry", base+"profile", base+"token",
					"party2:placemember:character:"+actor.CharacterID,
				).Build())
			})

			s := placelog.NewMembers(client, 300*time.Second)
			if cohort > 0 {
				const chunkSize = 500
				for start := 0; start < cohort; start += chunkSize {
					end := start + chunkSize
					if end > cohort {
						end = cohort
					}
					joinedCmd := client.B().Zadd().Key(base + "joined").ScoreMember()
					expiryCmd := client.B().Zadd().Key(base + "expiry").ScoreMember()
					profileCmd := client.B().Hset().Key(base + "profile").FieldValue()
					for i := start; i < end; i++ {
						cid := fmt.Sprintf("exp-%05d", i)
						score := float64(t0.Add(301 * time.Second).Unix())
						if i < 32 {
							score = float64(t0.Add(300 * time.Second).Unix())
						}
						joinedCmd = joinedCmd.ScoreMember(float64(i+1), cid)
						expiryCmd = expiryCmd.ScoreMember(score, cid)
						profileCmd = profileCmd.FieldValue(cid, `{"character_id":"`+cid+`","name":"E","color":"#FFFFFF"}`)
					}
					for _, r := range client.DoMulti(ctx, joinedCmd.Build(), expiryCmd.Build(), profileCmd.Build()) {
						if err := r.Error(); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
			if err := s.Refresh(ctx, p, actor, now); err != nil {
				b.Fatal(err)
			}

			replenish := func() {
				if cohort == 0 {
					return
				}
				j := client.B().Zadd().Key(base + "joined").ScoreMember()
				e := client.B().Zadd().Key(base + "expiry").ScoreMember()
				pr := client.B().Hset().Key(base + "profile").FieldValue()
				n := min(cohort, 32)
				for i := 0; i < n; i++ {
					cid := fmt.Sprintf("exp-%05d", i)
					j = j.ScoreMember(float64(i+1), cid)
					e = e.ScoreMember(float64(t0.Add(300*time.Second).Unix()), cid)
					pr = pr.FieldValue(cid, `{"character_id":"`+cid+`","name":"E","color":"#FFFFFF"}`)
				}
				for _, r := range client.DoMulti(ctx, j.Build(), e.Build(), pr.Build()) {
					if err := r.Error(); err != nil {
						b.Fatal(err)
					}
				}
			}
			replenish()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := s.Refresh(ctx, p, actor, now); err != nil {
					b.Fatal(err)
				}
				if cohort > 0 {
					b.StopTimer()
					replenish()
					b.StartTimer()
				}
			}
		})
	}
}

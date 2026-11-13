package placelog_test

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		client.Do(context.Background(), client.B().Del().Key(base+"joined", base+"expiry", base+"profile").Build())
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
	if err != nil || ttl <= 0 || ttl > 300 {
		t.Fatalf("ttl = %d, %v; want refreshed within 300 s", ttl, err)
	}
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

// Concurrent refreshes and leaves for one character never leave an entry that
// its place pointer does not name, so a later move or logout can always remove it.
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
	pointer, err := client.Do(ctx, client.B().Get().Key("party2:placemember:character:"+a.CharacterID).Build()).ToString()
	if err != nil && !errors.Is(err, valkey.Nil) {
		t.Fatal(err)
	}
	for _, place := range []placelog.Place{p, q} {
		if ids := memberIDs(t, s, place, now); len(ids) != 0 && pointer != place.Kind+":"+place.ID {
			t.Fatalf("%s lists %v but the pointer is %q", place.ID, ids, pointer)
		}
	}
	if err := s.Leave(ctx, a.CharacterID); err != nil {
		t.Fatal(err)
	}
	for _, place := range []placelog.Place{p, q} {
		if ids := memberIDs(t, s, place, now); len(ids) != 0 {
			t.Fatalf("%s still lists %v after the final leave", place.ID, ids)
		}
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
	for _, suffix := range []string{"joined", "expiry", "profile"} {
		ttl, err := client.Do(ctx, client.B().Ttl().Key(base+suffix).Build()).AsInt64()
		if err != nil || ttl <= 0 || ttl > 300 {
			t.Fatalf("%s ttl = %d, %v; want positive within 300s", suffix, ttl, err)
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
					base+"joined", base+"expiry", base+"profile",
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

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

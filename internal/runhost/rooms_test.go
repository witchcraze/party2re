package runhost

import (
	"context"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type roomRunStore struct {
	Store[testRun]
	kind string
	runs []State
}

func (s *roomRunStore) ListRuns(_ context.Context, kind string) ([]State, error) {
	s.kind = kind
	return s.runs, nil
}

func roomActor(id, name string) corebattle.LifetimeActor {
	return corebattle.LifetimeActor{Participant: corebattle.Participant{ID: id, Name: name}}
}

func TestListQuestRoomsPublicRows(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := State{Admission: Admission{Name: "Old", Speed: 18, StageID: "stage-00", MaxMembers: 1}, LeaderID: "a", Roster: []string{"a"}, StartedAt: t0,
		Battle: corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{roomActor("a", "Ann")}}}
	// The original leader left: exit promoted Roster[0], and an enemy is not a member.
	newer := State{Admission: Admission{Name: "New", Speed: 3, StageID: "stage-01", Mode: "try", NeedJoin: "hp_100_o", MaxMembers: 4}, LeaderID: "c", Roster: []string{"c", "b"}, Round: 3, StartedAt: t0.Add(time.Minute),
		Battle: corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{roomActor("b", "Bob"), roomActor("c", "Cid"), roomActor("@m", "スライム")}}}
	store := &roomRunStore{runs: []State{
		older, newer,
		{Retained: true, Roster: []string{"x"}, StartedAt: t0.Add(time.Hour)}, // a system room (vs_king) is not listed here
		{StartedAt: t0.Add(time.Hour)},                                        // no members left
	}}
	s := &Service[testRun, *testRun]{cfg: Config[testRun]{Kind: "normal", Store: store}}
	got, err := s.ListQuestRooms(context.Background())
	want := []QuestRoom{
		{Name: "New", Speed: 3, StageID: "stage-01", Mode: "try", NeedJoin: "hp_100_o", Round: 3, LeaderName: "Cid", MemberNames: []string{"Cid", "Bob"}, MaxMembers: 4},
		{Name: "Old", Speed: 18, StageID: "stage-00", LeaderName: "Ann", MemberNames: []string{"Ann"}, MaxMembers: 1},
	}
	if err != nil || store.kind != "normal" || !reflect.DeepEqual(got, want) {
		t.Fatalf("kind %q err %v\n got %+v\nwant %+v", store.kind, err, got, want)
	}
}

package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
)

type activityFixtures struct {
	party   party.Party
	member  party.Member
	exp     *dungeon.ActiveExpedition
	session *challenge.ChallengeSession
	casino  *casino.RoomView
	err     error
}

func (f *activityFixtures) GetActiveParty(context.Context, string) (party.Party, party.Member, error) {
	if f.err != nil {
		return party.Party{}, party.Member{}, f.err
	}
	if f.party.ID == "" {
		return party.Party{}, party.Member{}, party.ErrNotFound
	}
	return f.party, f.member, nil
}
func (f *activityFixtures) GetActiveExpedition(context.Context, string) (*dungeon.ActiveExpedition, error) {
	return f.exp, f.err
}
func (f *activityFixtures) GetActiveSession(context.Context, string) (*challenge.ChallengeSession, error) {
	return f.session, f.err
}
func (f *activityFixtures) GetCharacterRoomView(_ context.Context, owner, actor string) (*casino.RoomView, error) {
	if owner != "owner" || actor != "hero" {
		return nil, casino.ErrRoomViewForbidden
	}
	return f.casino, f.err
}

type pvpActivityFixture struct {
	room pvp.RoomDetail
	err  error
}

func (f *pvpActivityFixture) GetCharacterRoom(context.Context, string) (pvp.RoomDetail, error) {
	if f.err != nil {
		return pvp.RoomDetail{}, f.err
	}
	if f.room.Room.ID == "" {
		return pvp.RoomDetail{}, pvp.ErrRoomNotFound
	}
	return f.room, nil
}

type gvgActivityFixture struct {
	room gvg.RoomDetail
	err  error
}

func (f *gvgActivityFixture) GetCharacterRoom(context.Context, string) (gvg.RoomDetail, error) {
	if f.err != nil {
		return gvg.RoomDetail{}, f.err
	}
	if f.room.Room.ID == "" {
		return gvg.RoomDetail{}, gvg.ErrRoomNotFound
	}
	return f.room, nil
}

func TestFeatureActivityComposition(t *testing.T) {
	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
	r := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
	ctx := context.Background()
	if facts, err := r.read(ctx, "owner", "hero"); err != nil || facts == nil || len(facts) != 0 {
		t.Fatalf("empty: %v %v", facts, err)
	}
	f.party, f.member = party.Party{ID: "party", Status: party.StatusRecruiting, LeaderCharacterID: "hero"}, party.Member{PartyID: "party", CharacterID: "hero", IsLeader: true}
	pv.room = pvp.RoomDetail{Room: pvp.ColosseumRoom{ID: "pvp", Status: pvp.StatusInProgress, LeaderCharacterID: "hero", Round: 2}, Members: []pvp.RoomMember{{CharacterID: "hero"}}}
	gv.room = gvg.RoomDetail{Room: gvg.GvGRoom{ID: "gvg", Status: gvg.StatusInProgress, LeaderCharacterID: "hero", Round: 3}, Members: []gvg.GvGMember{{CharacterID: "hero"}}}
	f.exp = &dungeon.ActiveExpedition{ID: "dungeon", CharacterID: "hero", Status: dungeon.StatusExploring, CurrentFloor: 4}
	f.session = &challenge.ChallengeSession{ID: "challenge", CharacterID: "hero", Status: challenge.StatusActive, CurrentRound: 5}
	f.casino = &casino.RoomView{Room: casino.RoomObservation{RoomSummary: casino.RoomSummary{ID: "casino", Status: casino.RoomStatusInProgress, GameType: casino.GameTypeIndian}, Round: 6}, Members: []casino.MemberObservation{{CharacterID: "hero", IsSpectator: true}}}
	facts, err := r.read(ctx, "owner", "hero")
	if err != nil || len(facts) != 6 {
		t.Fatalf("all facts: %+v %v", facts, err)
	}
	for i, want := range [][]string{{"party_leave", "party_ready", "party_start"}, {"pvp_advance"}, {"gvg_leave", "gvg_advance"}, {"dungeon_move", "dungeon_escape"}, {"challenge_advance"}, {"casino_room_leave"}} {
		if !slices.Equal(facts[i].Actions, want) {
			t.Fatalf("%s actions=%v want=%v", facts[i].Kind, facts[i].Actions, want)
		}
	}
	before := *f.casino
	for _, tc := range []struct {
		spectator bool
		action    string
		game      casino.GameType
		want      bool
	}{
		{true, "", casino.GameTypeIndian, false}, {false, "", casino.GameTypeIndian, true}, {false, "call", casino.GameTypeIndian, false}, {false, "high", casino.GameTypeHighLow, false}, {false, "", casino.GameTypeHighLow, true}, {false, "★", casino.GameTypeDoppel, true},
	} {
		f.casino.Members = []casino.MemberObservation{{CharacterID: "hero", IsSpectator: tc.spectator, Action: tc.action}}
		f.casino.Room.GameType = tc.game
		got, err := r.read(ctx, "owner", "hero")
		if err != nil || slices.Contains(got[5].Actions, "casino_room_action") != tc.want {
			t.Fatalf("turn %+v: %+v %v", tc, got, err)
		}
	}
	*f.casino = before
	// Current owner buffers remain observable even after the gameplay phase ends;
	// observation never replays their pending settlement.
	f.exp.Status, f.session.Status = dungeon.StatusCleared, challenge.StatusDefeated
	got, err := r.read(ctx, "owner", "hero")
	if err != nil || len(got) != 6 || len(got[3].Actions) != 0 || len(got[4].Actions) != 0 {
		t.Fatalf("terminal buffers: %+v %v", got, err)
	}
	for _, stage := range []string{"party", "pvp", "gvg", "dungeon", "challenge", "casino"} {
		t.Run(stage+" failure", func(t *testing.T) {
			f2, pv2, gv2 := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
			r2 := contextActivityReaders{party: f2, pvp: pv2, gvg: gv2, dungeon: f2, challenge: f2, casino: f2}
			failure := errors.New(stage + " failure")
			switch stage {
			case "party":
				f2.err = failure
			case "pvp":
				pv2.err = failure
			case "gvg":
				gv2.err = failure
			case "dungeon":
				r2.dungeon = &activityFixtures{err: failure}
			case "challenge":
				r2.challenge = &activityFixtures{err: failure}
			case "casino":
				r2.casino = &activityFixtures{err: failure}
			}
			if got, err := r2.read(ctx, "owner", "hero"); got != nil || !errors.Is(err, failure) {
				t.Fatalf("partial/hidden failure: %+v %v", got, err)
			}
		})
	}
	for _, stage := range []string{"party", "pvp", "gvg", "dungeon", "challenge", "casino"} {
		t.Run(stage+" identity", func(t *testing.T) {
			f2, pv2, gv2 := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
			r2 := contextActivityReaders{party: f2, pvp: pv2, gvg: gv2, dungeon: f2, challenge: f2, casino: f2}
			switch stage {
			case "party":
				f2.party = party.Party{ID: "party"}
				f2.member = party.Member{CharacterID: "other"}
			case "pvp":
				pv2.room.Room.ID = "pvp"
			case "gvg":
				gv2.room.Room.ID = "gvg"
			case "dungeon":
				f2.exp = &dungeon.ActiveExpedition{CharacterID: "other"}
			case "challenge":
				f2.session = &challenge.ChallengeSession{CharacterID: "other"}
			case "casino":
				f2.casino = &casino.RoomView{}
			}
			if got, err := r2.read(ctx, "owner", "hero"); got != nil || err == nil {
				t.Fatalf("unowned source: %+v %v", got, err)
			}
		})
	}
}

func TestActivityRoleAndPhaseControls(t *testing.T) {
	for _, tc := range []struct {
		kind, phase, role string
		round             int
		want              []string
	}{
		{"party", party.StatusRecruiting, "leader", 0, []string{"party_leave", "party_ready", "party_start"}},
		{"party", party.StatusRecruiting, "member", 0, []string{"party_leave", "party_ready"}},
		{"party", party.StatusInProgress, "member", 0, []string{"party_leave"}},
		{"pvp", pvp.StatusRecruiting, "leader", 0, []string{"pvp_leave", "pvp_team", "pvp_start"}},
		{"pvp", pvp.StatusRecruiting, "member", 0, []string{"pvp_leave", "pvp_team"}},
		{"pvp", pvp.StatusInProgress, "leader", 1, []string{"pvp_advance"}},
		{"pvp", pvp.StatusInProgress, "member", 1, []string{}},
		{"pvp", pvp.StatusCompleted, "member", 1, []string{"pvp_leave"}},
		{"gvg", gvg.StatusRecruiting, "leader", 0, []string{"gvg_leave", "gvg_start"}},
		{"gvg", gvg.StatusRecruiting, "member", 0, []string{"gvg_leave"}},
		{"gvg", gvg.StatusInProgress, "leader", 1, []string{"gvg_leave", "gvg_advance"}},
		{"gvg", gvg.StatusInProgress, "member", 1, []string{}},
		{"gvg", gvg.StatusCompleted, "member", 1, []string{"gvg_leave"}},
		{"casino", string(casino.RoomStatusWaiting), "leader", 0, []string{"casino_room_leave", "casino_room_start"}},
		{"casino", string(casino.RoomStatusWaiting), "member", 0, []string{"casino_room_leave"}},
		{"casino", string(casino.RoomStatusWaiting), "spectator", 0, []string{"casino_room_leave"}},
	} {
		t.Run(tc.kind+"/"+tc.phase+"/"+tc.role, func(t *testing.T) {
			f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
			r := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
			leader := "other"
			if tc.role == "leader" {
				leader = "hero"
			}
			switch tc.kind {
			case "party":
				f.party = party.Party{ID: "party", Status: tc.phase, LeaderCharacterID: leader}
				f.member = party.Member{PartyID: "party", CharacterID: "hero", IsLeader: tc.role == "leader"}
			case "pvp":
				pv.room = pvp.RoomDetail{Room: pvp.ColosseumRoom{ID: "pvp", Status: tc.phase, Round: tc.round, LeaderCharacterID: leader}, Members: []pvp.RoomMember{{CharacterID: "hero"}}}
			case "gvg":
				gv.room = gvg.RoomDetail{Room: gvg.GvGRoom{ID: "gvg", Status: tc.phase, Round: tc.round, LeaderCharacterID: leader}, Members: []gvg.GvGMember{{CharacterID: "hero"}}}
			case "casino":
				f.casino = &casino.RoomView{Room: casino.RoomObservation{RoomSummary: casino.RoomSummary{ID: "casino", Status: casino.RoomStatus(tc.phase), GameType: casino.GameTypeIndian}, Round: tc.round, LeaderCharacterID: leader}, Members: []casino.MemberObservation{{CharacterID: "hero", IsSpectator: tc.role == "spectator"}}}
			}
			got, err := r.read(context.Background(), "owner", "hero")
			if err != nil || len(got) != 1 || got[0].Role != tc.role || !slices.Equal(got[0].Actions, tc.want) {
				t.Fatalf("role/phase: %+v %v want=%v", got, err, tc.want)
			}
		})
	}
}

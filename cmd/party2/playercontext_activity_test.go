package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
	vk "github.com/witchcraze/party2re/internal/valkey"
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

func (f *activityFixtures) HasCharacterRoom(context.Context, string) (bool, error) {
	return f.casino != nil, f.err
}

type pvpActivityFixture struct {
	room     pvp.RoomDetail
	err      error
	claim    string
	claimErr error
}

func (f *pvpActivityFixture) GetCharacterClaim(context.Context, string) (string, error) {
	return f.claim, f.claimErr
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

func TestPvPClaimWithoutLiveRoomIsNotIdle(t *testing.T) {
	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{claim: "lost-room"}, &gvgActivityFixture{}
	r := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
	ctx := context.Background()
	facts, err := r.read(ctx, "owner", "hero")
	if err != nil || len(facts) != 1 || facts[0].Kind != "pvp" || facts[0].ID != "lost-room" || facts[0].Phase != "recovery_pending" || !slices.Equal(facts[0].Actions, []string{"pvp_leave"}) {
		t.Fatalf("unpublished claim: %+v %v", facts, err)
	}
	if kinds, err := r.ActivityKinds(ctx, "owner", "hero"); err != nil || !slices.Equal(kinds, []string{"pvp"}) {
		t.Fatalf("rescue kinds: %v %v", kinds, err)
	}
	pv.room = pvp.RoomDetail{Room: pvp.ColosseumRoom{ID: "lost-room", Status: pvp.StatusRecruiting}, Members: []pvp.RoomMember{{CharacterID: "hero"}}}
	if facts, err := r.read(ctx, "owner", "hero"); err != nil || len(facts) != 1 || facts[0].Phase != pvp.StatusRecruiting {
		t.Fatalf("published room duplicated by claim: %+v %v", facts, err)
	}
	pv.room, pv.claimErr = pvp.RoomDetail{}, errors.New("claim store down")
	if _, err := r.read(ctx, "owner", "hero"); !errors.Is(err, pv.claimErr) {
		t.Fatalf("claim failure hidden: %v", err)
	}
	if _, err := r.ActivityKinds(ctx, "owner", "hero"); !errors.Is(err, pv.claimErr) {
		t.Fatalf("rescue claim failure hidden: %v", err)
	}
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
	f.exp = &dungeon.ActiveExpedition{ID: "dungeon", CharacterID: "hero", Status: dungeon.StatusExploring, CurrentFloor: 4, TurnsRemaining: 20}
	f.session = &challenge.ChallengeSession{ID: "challenge", Revision: 1, CharacterID: "hero", Status: challenge.StatusActive, CurrentRound: 5}
	f.casino = &casino.RoomView{Room: casino.RoomObservation{RoomSummary: casino.RoomSummary{ID: "casino", Status: casino.RoomStatusInProgress, GameType: casino.GameTypeIndian}, Round: 6}, Members: []casino.MemberObservation{{CharacterID: "hero", IsSpectator: true}}}
	facts, err := r.read(ctx, "owner", "hero")
	if err != nil || len(facts) != 6 {
		t.Fatalf("all facts: %+v %v", facts, err)
	}
	for i, want := range [][]string{{"party_leave", "party_ready", "party_start"}, {"pvp_advance"}, {"gvg_leave", "gvg_advance"}, {"dungeon_move", "dungeon_escape"}, {"challenge_advance", "challenge_retire"}, {"casino_room_leave"}} {
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
	f.exp.Status, f.session.Status = dungeon.StatusEscaped, challenge.StatusDefeated
	got, err := r.read(ctx, "owner", "hero")
	if err != nil || len(got) != 6 || !slices.Equal(got[3].Actions, []string{"dungeon_fight", "dungeon_escape"}) || len(got[4].Actions) != 0 {
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
		{"casino", string(casino.RoomStatusWaiting), "leader", 0, []string{"casino_room_leave", "casino_room_start", "casino_room_kick"}},
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

func TestCombatMemberActivityHasNoLeaderControls(t *testing.T) {
	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
	f.exp = &dungeon.ActiveExpedition{ID: "run-d", CharacterID: "leader", Status: dungeon.StatusExploring, Members: []dungeon.ExpeditionMember{{CharacterID: "hero"}}}
	f.session = &challenge.ChallengeSession{ID: "run-c", CharacterID: "leader", Status: challenge.StatusActive, Members: []challenge.ChallengeMember{{CharacterID: "hero"}}}
	readers := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
	facts, err := readers.read(context.Background(), "owner", "hero")
	if err != nil || len(facts) != 2 {
		t.Fatalf("member activity missing: %+v %v", facts, err)
	}
	for _, fact := range facts {
		if fact.Role != "member" || len(fact.Actions) != 0 {
			t.Fatalf("member received leader controls: %+v", fact)
		}
	}
}

func TestDungeonActivityEncounterAndTerminalCandidates(t *testing.T) {
	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
	readers := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
	for _, tc := range []struct {
		status    dungeon.ExpeditionStatus
		encounter bool
		turns     int
		want      []string
	}{
		{dungeon.StatusExploring, false, 10, []string{"dungeon_move", "dungeon_escape"}},
		{dungeon.StatusExploring, true, 10, []string{"dungeon_fight", "dungeon_escape"}},
		{dungeon.StatusExploring, false, 0, []string{"dungeon_escape"}},
		{dungeon.StatusEscaped, false, 0, []string{"dungeon_fight", "dungeon_escape"}},
		{dungeon.StatusWipedOut, false, 0, []string{"dungeon_fight", "dungeon_escape"}},
	} {
		f.exp = &dungeon.ActiveExpedition{ID: "run", CharacterID: "hero", Revision: 8, Status: tc.status, TurnsRemaining: tc.turns}
		if tc.encounter {
			f.exp.Encounter = &dungeon.ActiveEncounter{}
		}
		facts, err := readers.read(context.Background(), "owner", "hero")
		if err != nil || len(facts) != 1 || facts[0].Revision != 8 || !slices.Equal(facts[0].Actions, tc.want) {
			t.Fatalf("%+v: %+v %v", tc, facts, err)
		}
	}
}

func TestChallengePendingActivitySuppressesProgress(t *testing.T) {
	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
	f.session = &challenge.ChallengeSession{ID: "run", Revision: 4, CurrentRound: 3, Status: challenge.StatusActive, RecoveryPending: true, Members: []challenge.ChallengeMember{{CharacterID: "hero"}}}
	readers := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
	facts, err := readers.read(context.Background(), "owner", "hero")
	if err != nil || len(facts) != 1 || facts[0].Phase != "recovery_pending" || facts[0].Revision != 4 || len(facts[0].Actions) != 0 {
		t.Fatalf("pending claim lost or progressed: %+v %v", facts, err)
	}
}

func TestChallengeExitActivityForEveryParticipant(t *testing.T) {
	for _, leader := range []string{"hero", "other"} {
		for _, round := range []int{1, 2} {
			f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
			f.session = &challenge.ChallengeSession{ID: "run", Revision: 3, CharacterID: leader, CurrentRound: round, Status: challenge.StatusActive, Members: []challenge.ChallengeMember{{CharacterID: "hero"}}}
			readers := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: f}
			facts, err := readers.read(context.Background(), "owner", "hero")
			want := []string{"challenge_retire"}
			if leader == "hero" || round > 1 {
				want = []string{"challenge_advance", "challenge_retire"}
			}
			if err != nil || len(facts) != 1 || facts[0].Revision != 3 || !slices.Equal(facts[0].Actions, want) {
				t.Fatalf("leader=%s round=%d: %+v %v", leader, round, facts, err)
			}
		}
	}
}

type casinoActivityAccountStub struct {
	casino.Repository
	accounts map[string]casino.Account
}

func (s *casinoActivityAccountStub) GetAccount(_ context.Context, charID string) (casino.Account, error) {
	return s.accounts[charID], nil
}

func (s *casinoActivityAccountStub) DeductBetAndCreditPayout(_ context.Context, charID string, bet, payout int64) (casino.Account, error) {
	acc := s.accounts[charID]
	acc.Coins = acc.Coins - bet + payout
	s.accounts[charID] = acc
	return acc, nil
}

type casinoActivityCharStub struct {
	casino.CharacterRepository
	chars map[string]corecharacter.Character
}

func (s *casinoActivityCharStub) FindByID(_ context.Context, charID string) (corecharacter.Character, error) {
	c, ok := s.chars[charID]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (s *casinoActivityCharStub) FindByIDForUpdate(ctx context.Context, charID string) (corecharacter.Character, error) {
	return s.FindByID(ctx, charID)
}

func (s *casinoActivityCharStub) Update(_ context.Context, ch corecharacter.Character) error {
	s.chars[ch.ID] = ch
	return nil
}

func TestCasinoActivityReaderPreservesDiscoveryAcrossRoomRenewal(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClientWithConfig(vk.ConfigFromEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	prefix := "act1091-" + id.New()[:8]
	leader := prefix + "-leader"
	peer := prefix + "-peer"
	spec := prefix + "-spec"

	chars := &casinoActivityCharStub{
		chars: map[string]corecharacter.Character{
			leader: {ID: leader, Name: "Leader", PlayerID: "owner-" + leader},
			peer:   {ID: peer, Name: "Peer", PlayerID: "owner-" + peer},
			spec:   {ID: spec, Name: "Spec", PlayerID: "owner-" + spec},
		},
	}
	accounts := &casinoActivityAccountStub{
		accounts: map[string]casino.Account{
			leader: {CharacterID: leader, Coins: 500},
			peer:   {CharacterID: peer, Coins: 500},
			spec:   {CharacterID: spec, Coins: 500},
		},
	}
	roomRepo, err := casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	casinoSvc, err := casino.NewService(accounts, casino.WithDepartureRepository(casino.NewMemoryDepartureRepository()), casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	created, err := casinoSvc.CreateRoom(ctx, leader, casino.CreateRoomRequest{
		Name: "Act-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := created.Room.ID
	t.Cleanup(func() {
		bg := context.Background()
		_ = client.Do(bg, client.B().Del().Key(casino.DefaultRoomKeyPrefix+roomID).Build())
		_ = client.Do(bg, client.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build())
		for _, actor := range []string{leader, peer, spec} {
			_ = client.Do(bg, client.B().Del().Key(casino.DefaultCharacterKeyPrefix+actor).Build())
		}
	})

	if _, err := casinoSvc.JoinRoom(ctx, roomID, peer, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := casinoSvc.SpectateRoom(ctx, roomID, spec, ""); err != nil {
		t.Fatal(err)
	}

	// Shorten leader and spectator mappings to 60s, then let peer drive room renewal via UpdateMember.
	for _, actor := range []string{leader, peer, spec} {
		if err := client.Do(ctx, client.B().Expire().Key(casino.DefaultCharacterKeyPrefix+actor).Seconds(60).Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}
	peerMem, err := roomRepo.GetMember(ctx, roomID, peer)
	if err != nil {
		t.Fatal(err)
	}
	peerMem.UpdatedAt = time.Now().UTC()
	if err := roomRepo.UpdateMember(ctx, *peerMem); err != nil {
		t.Fatal(err)
	}

	f, pv, gv := &activityFixtures{}, &pvpActivityFixture{}, &gvgActivityFixture{}
	readers := contextActivityReaders{party: f, pvp: pv, gvg: gv, dungeon: f, challenge: f, casino: casinoSvc}

	for _, actor := range []string{leader, peer, spec} {
		charKey := casino.DefaultCharacterKeyPrefix + actor
		ttl, err := client.Do(ctx, client.B().Ttl().Key(charKey).Build()).AsInt64()
		if err != nil || ttl <= 60 {
			t.Fatalf("expected renewed TTL > 60 for %s, got %d err=%v", actor, ttl, err)
		}
		if err := client.Do(ctx, client.B().Expire().Key(charKey).Seconds(ttl-61).Build()).Error(); err != nil {
			t.Fatal(err)
		}
		facts, err := readers.read(ctx, "owner-"+actor, actor)
		if err != nil || len(facts) != 1 || facts[0].Kind != "casino" || facts[0].ID != roomID {
			t.Fatalf("activity for %s across original deadline: %+v err=%v", actor, facts, err)
		}
	}

	// Kick spectator and disband by leaving remaining members -> activity returns empty.
	if err := casinoSvc.KickMember(ctx, roomID, leader, spec); err != nil {
		t.Fatal(err)
	}
	if facts, err := readers.read(ctx, "owner-"+spec, spec); err != nil || len(facts) != 0 {
		t.Fatalf("kicked spectator activity: %+v err=%v", facts, err)
	}
	if err := casinoSvc.LeaveRoom(ctx, roomID, peer); err != nil {
		t.Fatal(err)
	}
	if err := casinoSvc.LeaveRoom(ctx, roomID, leader); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{leader, peer} {
		if facts, err := readers.read(ctx, "owner-"+actor, actor); err != nil || len(facts) != 0 {
			t.Fatalf("post-disband activity(%s): %+v err=%v", actor, facts, err)
		}
	}

	// Unreadable blank mapping must return an error, not false idle.
	if err := client.Do(ctx, client.B().Set().Key(casino.DefaultCharacterKeyPrefix+leader).Value("").Ex(casino.DefaultLobbyTTL).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if facts, err := readers.read(ctx, "owner-"+leader, leader); facts != nil || !errors.Is(err, casino.ErrInvalidRoomMapping) {
		t.Fatalf("blank mapping should error, got facts=%+v err=%v", facts, err)
	}
}

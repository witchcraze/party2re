package main

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
)

type publicNormalActivity struct {
	active bool
	err    error
}

func (f publicNormalActivity) HasActiveRun(_ context.Context, actor string) (bool, error) {
	if actor != "hero" {
		return false, errors.New("wrong public target")
	}
	return f.active, f.err
}

func (f publicNormalActivity) GetActorRun(context.Context, string, string) (*adventure.ActorRun, error) {
	panic("public inspection must not read the owned run")
}

func TestPublicActivityReaders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*contextActivityReaders)
		active bool
	}{
		{"idle", func(*contextActivityReaders) {}, false},
		{"normal run", func(r *contextActivityReaders) { r.normal = publicNormalActivity{active: true} }, true},
		{"party lobby", func(r *contextActivityReaders) {
			r.party = &activityFixtures{party: party.Party{ID: "private", Status: party.StatusRecruiting}, member: party.Member{CharacterID: "hero", PartyID: "private"}}
		}, true},
		{"pvp room", func(r *contextActivityReaders) {
			r.pvp = &pvpActivityFixture{room: pvp.RoomDetail{Room: pvp.ColosseumRoom{ID: "private", Status: pvp.StatusInProgress}, Members: []pvp.RoomMember{{CharacterID: "hero"}}}}
		}, true},
		{"pvp claim after room loss", func(r *contextActivityReaders) { r.pvp = &pvpActivityFixture{claim: "private"} }, true},
		{"gvg room", func(r *contextActivityReaders) {
			r.gvg = &gvgActivityFixture{room: gvg.RoomDetail{Room: gvg.GvGRoom{ID: "private", Status: gvg.StatusRecruiting}, Members: []gvg.GvGMember{{CharacterID: "hero"}}}}
		}, true},
		{"dungeon", func(r *contextActivityReaders) {
			r.dungeon = &activityFixtures{exp: &dungeon.ActiveExpedition{ID: "private", CharacterID: "hero"}}
		}, true},
		{"challenge recovery", func(r *contextActivityReaders) {
			r.challenge = &activityFixtures{session: &challenge.ChallengeSession{ID: "private", RecoveryPending: true, Members: []challenge.ChallengeMember{{CharacterID: "hero"}}}}
		}, true},
		{"casino", func(r *contextActivityReaders) { r.casino = &activityFixtures{casino: &casino.RoomView{}} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &activityFixtures{}
			r := contextActivityReaders{normal: publicNormalActivity{}, party: f, pvp: &pvpActivityFixture{}, gvg: &gvgActivityFixture{}, dungeon: f, challenge: f, casino: f}
			tc.setup(&r)
			if active, err := r.publicActivity(context.Background(), "hero"); err != nil || active != tc.active {
				t.Fatalf("active=%v err=%v", active, err)
			}
		})
	}
}

func TestPublicActivitySourceFailures(t *testing.T) {
	want := errors.New("required read failed")
	for _, source := range []string{"normal", "party", "pvp", "pvp claim", "gvg", "dungeon", "challenge", "casino"} {
		t.Run(source, func(t *testing.T) {
			f := &activityFixtures{}
			r := contextActivityReaders{normal: publicNormalActivity{active: true}, party: f, pvp: &pvpActivityFixture{}, gvg: &gvgActivityFixture{}, dungeon: f, challenge: f, casino: f}
			switch source {
			case "normal":
				r.normal = publicNormalActivity{err: want}
			case "party":
				r.party = &activityFixtures{err: want}
			case "pvp":
				r.pvp = &pvpActivityFixture{err: want}
			case "pvp claim":
				r.pvp = &pvpActivityFixture{claimErr: want}
			case "gvg":
				r.gvg = &gvgActivityFixture{err: want}
			case "dungeon":
				r.dungeon = &activityFixtures{err: want}
			case "challenge":
				r.challenge = &activityFixtures{err: want}
			case "casino":
				r.casino = &activityFixtures{err: want}
			}
			if active, err := r.publicActivity(context.Background(), "hero"); active || !errors.Is(err, want) {
				t.Fatalf("active=%v err=%v", active, err)
			}
		})
	}
}

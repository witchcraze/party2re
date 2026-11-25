package boss_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/party"
)

func roomsByStage(repo *mockPartyRepo) map[string]party.Party {
	rooms := make(map[string]party.Party)
	for _, p := range repo.parties {
		if p.Kind == party.KindSealing {
			rooms[p.StageID] = p
		}
	}
	return rooms
}

func TestOpenSealingRooms_OpensEachStageOnceWithItsRoster(t *testing.T) {
	ctx := context.Background()
	repo := newMockPartyRepo()
	news := &mockNewsPublisher{}
	svc, err := boss.NewService(newMockBossRepo(), &mockCharRepo{}, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Configure(boss.WithPartyRepository(repo), boss.WithNewsPublisher(news))

	leader := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "lead", Name: "勇者", MaxHP: 300, ExperienceReward: 12, GoldReward: 5}}
	pet := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "pet", Name: "@スライム", MaxHP: 40}, NPC: true}
	if err := svc.OpenSealingRooms(ctx, "冒険団", []corebattle.LifetimeActor{leader, pet}); err != nil {
		t.Fatal(err)
	}
	rooms := roomsByStage(repo)
	if len(rooms) != 11 {
		t.Fatalf("expected 11 rooms (king1-10, king99), got %d", len(rooms))
	}
	for _, stage := range boss.DefaultBossCatalog() {
		room := rooms[stage.ID]
		wantMax := 6
		if stage.ID == "king99" {
			wantMax = 4
		}
		if room.LeaderCharacterID != "" || room.Name != stage.Name || room.Speed != 12 || room.MaxMembers != wantMax ||
			room.NeedJoin != stage.NeedJoin || room.PasswordHash != "" || room.Status != party.StatusRecruiting {
			t.Errorf("%s room mismatch: %+v", stage.ID, room)
		}
		if stage.ID != "king99" && (room.LeaderName != "@"+stage.LeaderName || len(room.Enemies) != len(stage.Bosses)) {
			t.Errorf("%s room should be led by its boss and hold its catalog roster: %+v", stage.ID, room)
		}
	}
	king99 := rooms["king99"]
	if king99.LeaderName != "@勇者" || len(king99.Enemies) != 2 || king99.Enemies[0].Name != "@勇者" || king99.Enemies[1].Name != "@@スライム" ||
		king99.Enemies[0].MaxHP != 300*50 || king99.Enemies[0].ExperienceReward != 12*30 {
		t.Fatalf("king99 room should hold the unsealing party's clones, leader first: %+v", king99)
	}
	if len(news.published) != 1 || news.published[0] != "冒険団(勇者、@スライム) によって封印されし者達の封印が解かれました！" {
		t.Fatalf("unexpected unseal news: %q", news.published)
	}

	// A later unseal by another party opens nothing and keeps the king99 clones.
	other := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "other", Name: "別人", MaxHP: 999}}
	if err := svc.OpenSealingRooms(ctx, "", []corebattle.LifetimeActor{other}); err != nil {
		t.Fatal(err)
	}
	if again := roomsByStage(repo); len(repo.parties) != 11 || again["king99"].ID != king99.ID || again["king99"].Enemies[0].Name != "@勇者" {
		t.Fatalf("second unseal replaced or duplicated rooms: %d rooms, king99 %+v", len(repo.parties), again["king99"])
	}
	if len(news.published) != 2 || news.published[1] != "(別人) によって封印されし者達の封印が解かれました！" {
		t.Fatalf("every unseal publishes its news: %q", news.published)
	}

	// Once a room is gone, the next unseal reopens only that stage.
	repo.deleted[rooms["king3"].ID] = true
	if err := svc.OpenSealingRooms(ctx, "", []corebattle.LifetimeActor{other}); err != nil {
		t.Fatal(err)
	}
	if len(repo.parties) != 12 {
		t.Fatalf("expected only king3 reopened, got %d rooms", len(repo.parties))
	}
}

type failingRoomRepo struct{ *mockPartyRepo }

func (failingRoomRepo) OpenSystemRoom(context.Context, party.Party) (bool, error) {
	return false, errInjected
}

func TestOpenSealingRooms_FailuresAreReportedButNewsStillRuns(t *testing.T) {
	news := &mockNewsPublisher{}
	svc, err := boss.NewService(newMockBossRepo(), &mockCharRepo{}, stubPartyBattleEngine{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Configure(boss.WithPartyRepository(failingRoomRepo{newMockPartyRepo()}), boss.WithNewsPublisher(news))
	err = svc.OpenSealingRooms(context.Background(), "q", []corebattle.LifetimeActor{{Participant: corebattle.Participant{Name: "a"}}})
	if !errors.Is(err, errInjected) || len(news.published) != 1 {
		t.Fatalf("room failure should be returned without skipping the news: err=%v news=%q", err, news.published)
	}
}

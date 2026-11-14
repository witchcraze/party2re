package database_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type helperGuildStore struct{ repo *database.GuildRepository }

func (s helperGuildStore) FindGuildIDByCharacterID(ctx context.Context, characterID string) (string, error) {
	g, _, err := s.repo.GetGuildByCharacter(ctx, characterID)
	if errors.Is(err, guild.ErrCharacterNotInGuild) {
		return "", nil
	}
	return g.ID, err
}
func (s helperGuildStore) AddGuildPoints(ctx context.Context, guildID string, points int) error {
	return s.repo.AddPoints(ctx, guildID, int64(points))
}

func TestHelperGuildQuest_RejectionAndConcurrentClaim_Integration(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "guilds", "guild_members", "helper_quests")
	ctx := context.Background()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	quests, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guilds, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	g, leader, err := database.CreateTestGuildWithLeader(ctx, db, "ClaimGuild", 10000)
	if err != nil {
		t.Fatal(err)
	}
	member, err := database.CreateTestCharacter(ctx, db, "Member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guilds.AddMember(ctx, guild.Member{GuildID: g.ID, CharacterID: member.ID, Role: guild.RoleMember, JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	solo, err := database.CreateTestCharacter(ctx, db, "Solo")
	if err != nil {
		t.Fatal(err)
	}
	for _, charID := range []string{leader.ID, member.ID, solo.ID} {
		mat, err := item.NewInstance("item-001", 4)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestDepot(ctx, db, charID, 5, []item.Instance{mat}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	q := helperquest.Quest{ID: id.New(), Title: "Guild claim", Kind: helperquest.KindItem, TargetID: "item-001", RequiredCount: 2, RewardItemID: "item-126", IsGuild: true, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)}
	if err := quests.Save(ctx, q); err != nil {
		t.Fatal(err)
	}
	other := q
	other.ID = id.New()
	if err := quests.Save(ctx, other); err != nil {
		t.Fatal(err)
	}
	before, err := quests.ListUncompleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc := helperquest.NewService(quests, chars, helperGuildStore{guilds}, database.NewTransactionProvider(db), helperquest.WithDepotRepository(depots), helperquest.WithMonsterLookup(testutil.MonsterNameLookup(t)))
	if _, err := svc.CompleteQuest(ctx, solo.ID, q.ID, now); !errors.Is(err, helperquest.ErrGuildRequired) {
		t.Fatalf("unaffiliated claim: %v", err)
	}
	after, err := quests.ListUncompleted(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejection changed board = %+v, %v", after, err)
	}
	unchanged, _, err := guilds.GetGuild(ctx, g.ID)
	if err != nil || unchanged.Points != 0 {
		t.Fatalf("rejected guild points = %+v, %v", unchanged, err)
	}
	stored, err := quests.FindByID(ctx, q.ID)
	if err != nil || stored.CompletedAt != nil {
		t.Fatalf("rejected quest = %+v, %v", stored, err)
	}
	dep, err := depots.FindByCharacterID(ctx, solo.ID)
	if err != nil {
		t.Fatal(err)
	}
	char, err := chars.FindByID(ctx, solo.ID)
	if err != nil || char.HelpCount != 0 || dep.Quantity("item-001") != 4 || dep.Quantity("item-126") != 0 {
		t.Fatalf("rejected assets = %+v/%+v, %v", char, dep, err)
	}
	for _, err := range testutil.RunRace(
		func() error { _, err := svc.CompleteQuest(ctx, leader.ID, q.ID, now); return err },
		func() error { _, err := svc.CompleteQuest(ctx, member.ID, q.ID, now); return err },
	) {
		if err != nil && !errors.Is(err, helperquest.ErrQuestAlreadyDone) {
			t.Fatal(err)
		}
	}
	points, _, err := guilds.GetGuild(ctx, g.ID)
	if err != nil || points.Points != 100 {
		t.Fatalf("guild = %+v, %v", points, err)
	}
	materials, rewards, help := 0, 0, 0
	for _, charID := range []string{leader.ID, member.ID} {
		dep, err := depots.FindByCharacterID(ctx, charID)
		if err != nil {
			t.Fatal(err)
		}
		char, err := chars.FindByID(ctx, charID)
		if err != nil {
			t.Fatal(err)
		}
		materials += dep.Quantity("item-001")
		rewards += dep.Quantity("item-126")
		help += char.HelpCount
	}
	if materials != 6 || rewards != 1 || help != 1 {
		t.Fatalf("assets = %d/%d/%d", materials, rewards, help)
	}
}

package main

import (
	"context"
	"database/sql"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/replay"
)

type cmbtServices struct {
	pvp         *pvp.Service
	gvg         *gvg.Service
	boss        *boss.Service
	dungeon     *dungeon.Service
	replay      *replay.Service
	challenge   *challenge.Service
	customSkill *custom_skill.Service
	party       *party.Service
	adv         *adventure.Service
}

type customSkillGemCatalog struct{ catalog *gemstore.Catalog }

func (c customSkillGemCatalog) FindGemByID(id string) (custom_skill.GemDefinition, bool) {
	gem, ok := c.catalog.FindGemByID(id)
	if !ok {
		return custom_skill.GemDefinition{}, false
	}
	return custom_skill.GemDefinition{
		ID: gem.ID, Name: gem.Name, SlotCost: gem.SlotCost, MPCost: gem.MPCost,
	}, true
}

func newCmbtServices(
	db *sql.DB,
	core *coreServices,
	soc *socServices,
	valkeyClient valkeygo.Client,
) (*cmbtServices, error) {
	battleEngine := corebattle.Engine{}

	var pvpRoomRepo pvp.RoomRepository
	if valkeyClient != nil {
		vr, err := pvp.NewValkeyRoomRepository(valkeyClient)
		if err != nil {
			return nil, err
		}
		pvpRoomRepo = vr
	} else {
		pvpRoomRepo = pvp.NewMemoryRoomRepository()
	}
	pvpService, err := pvp.NewService(pvpRoomRepo, core.charRepo, battleEngine)
	if err != nil {
		return nil, err
	}

	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		return nil, err
	}
	gvgService, err := gvg.NewService(gvgRepo, soc.guildRepo, core.charRepo, battleEngine)
	if err != nil {
		return nil, err
	}

	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		return nil, err
	}
	bossService, err := boss.NewService(bossRepo, core.charRepo, battleEngine)
	if err != nil {
		return nil, err
	}

	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		return nil, err
	}
	valkeyExpeditionStore, err := dungeon.NewValkeyExpeditionRepository(valkeyClient)
	if err != nil {
		return nil, err
	}
	dungeonService, err := dungeon.NewService(
		dungeonRepo,
		core.charRepo,
		battleEngine,
		dungeon.WithActiveExpeditionStore(valkeyExpeditionStore),
	)
	if err != nil {
		return nil, err
	}

	replayRepo, err := database.NewReplayRepository(db)
	if err != nil {
		return nil, err
	}
	replayService, err := replay.NewService(replayRepo)
	if err != nil {
		return nil, err
	}

	challengeRepo, err := database.NewChallengeRepository(db)
	if err != nil {
		return nil, err
	}
	valkeyChallengeStore, err := challenge.NewValkeySessionRepository(valkeyClient)
	if err != nil {
		return nil, err
	}
	challengeService, err := challenge.NewService(
		challengeRepo,
		core.charRepo,
		battleEngine,
		challenge.WithActiveSessionStore(valkeyChallengeStore),
	)
	if err != nil {
		return nil, err
	}

	customSkillRepo, err := database.NewCustomSkillRepository(db)
	if err != nil {
		return nil, err
	}
	customSkillService, err := custom_skill.NewService(customSkillRepo, core.charRepo)
	if err != nil {
		return nil, err
	}
	gemCatalog, err := gemstore.DefaultCatalog()
	if err != nil {
		return nil, err
	}
	customSkillService.ConfigureGemSynthesis(customSkillGemCatalog{catalog: gemCatalog}, core.invRepo, core.txProvider)

	adventureRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		return nil, err
	}
	adventureStages, err := adventure.InitialStageCatalog()
	if err != nil {
		return nil, err
	}
	adventureMonsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		return nil, err
	}

	dbPartyRepo, err := database.NewPartyRepository(db)
	if err != nil {
		return nil, err
	}
	partyRepo := party.NewValkeyRepository(valkeyClient, party.WithDurableLogRepository(dbPartyRepo))
	partyService, err := party.NewService(
		partyRepo,
		core.charRepo,
		core.invRepo,
		adventureStages,
		adventureMonsters,
		battleEngine,
		party.WithTransactionProvider(core.txProvider),
		party.WithNewsPublisher(party.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			_, err := soc.notification.PublishNews(ctx, cat, title, content, author, pubAt)
			return err
		})),
	)
	if err != nil {
		return nil, err
	}

	advService, err := adventure.NewServiceWithCatalogs(
		adventureRepo,
		core.charRepo,
		core.invRepo,
		adventureStages,
		adventureMonsters,
		battleEngine,
		soc.sched,
		nil,
		adventure.RealClock{},
	)
	if err != nil {
		return nil, err
	}

	// Wire party repository and news publisher into boss service so that
	// StartSealingBattle (パーティー封印戦) can resolve party membership and publish sealing news.
	bossService.Configure(
		boss.WithPartyRepository(partyRepo),
		boss.WithNewsPublisher(boss.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			_, err := soc.notification.PublishNews(ctx, cat, title, content, author, pubAt)
			return err
		})),
	)

	return &cmbtServices{
		pvp:         pvpService,
		gvg:         gvgService,
		boss:        bossService,
		dungeon:     dungeonService,
		replay:      replayService,
		challenge:   challengeService,
		customSkill: customSkillService,
		party:       partyService,
		adv:         advService,
	}, nil
}

package main

import (
	"context"
	"os"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/valkey"
)

func main() {
	logger := logging.NewJSON(os.Stderr)
	if err := run(); err != nil {
		logger.Error(context.Background(), "application.startup", err)
		os.Exit(1)
	}
	logger.Info(context.Background(), "application.ready")
}

func run() error {
	db, err := database.OpenFromEnvironment()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Ping(db); err != nil {
		return err
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		return err
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		return err
	}
	shopRepo, err := database.NewShopRepository(db)
	if err != nil {
		return err
	}
	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		return err
	}
	_, err = shop.NewServiceWithTransaction(charRepo, invRepo, shopRepo, itemCatalog)
	if err != nil {
		return err
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		return err
	}
	_, err = depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		return err
	}
	bsRepo, err := database.NewBlacksmithRepository(db)
	if err != nil {
		return err
	}
	_, err = blacksmith.NewServiceWithTransaction(charRepo, invRepo, bsRepo, itemCatalog, nil)
	if err != nil {
		return err
	}
	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		return err
	}
	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		return err
	}
	_, err = alchemy.NewServiceWithTransaction(charRepo, invRepo, alcRepo, recipeCatalog, itemCatalog)
	if err != nil {
		return err
	}
	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		return err
	}
	_, err = bank.NewService(bankRepo)
	if err != nil {
		return err
	}
	pvpRepo, err := database.NewPvPRepository(db)
	if err != nil {
		return err
	}
	_, err = pvp.NewService(pvpRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		return err
	}
	_, err = guild.NewService(guildRepo)
	if err != nil {
		return err
	}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		return err
	}
	_, err = gvg.NewService(gvgRepo, guildRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}
	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		return err
	}
	_, err = boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		return err
	}
	_, err = dungeon.NewService(dungeonRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	valkeyClient, err := valkey.NewClient()
	if err != nil {
		// Log warning but continue if Valkey is optional or fallback is acceptable.
		// For now, if we can't connect, we just won't have a scheduler.
	} else {
		defer valkeyClient.Close()

		// Setup repositories
		activityRepo, err := database.NewActivityRepository(db)
		if err != nil {
			return err
		}
		adventureRepo, err := database.NewAdventureRepository(db)
		if err != nil {
			return err
		}
		schedRepo := scheduling.NewValkeyRepository(valkeyClient)

		// Setup Scheduler & Worker
		schedService := scheduling.NewService(schedRepo)

		// Note: logger parameter uses nop logger for now as standard pkg logger isn't typed for it.
		// In a real app we would adapt logging.Logger to activity/adventure.Logger.
		activityService, err := activity.NewService(activityRepo, charRepo, schedService, nil)
		if err != nil {
			return err
		}

		adventureStages, err := adventure.InitialStageCatalog()
		if err != nil {
			return err
		}
		adventureMonsters, err := adventure.InitialMonsterCatalog()
		if err != nil {
			return err
		}
		adventureService, err := adventure.NewServiceWithCatalogs(adventureRepo, charRepo, invRepo, adventureStages, adventureMonsters, corebattle.Engine{}, schedService, nil, adventure.RealClock{})
		if err != nil {
			return err
		}

		worker := scheduling.NewWorker(schedRepo, 5*time.Second, logging.NewJSON(os.Stderr))
		worker.RegisterHandler(activity.ActivityActionTypeTrainingComplete, activity.NewTrainingHandler(activityService))
		worker.RegisterHandler(adventure.AdventureActionTypeComplete, adventure.NewAdventureCompletionHandler(adventureService))

		// In a real entrypoint we would run worker.Run(ctx, interval) in a goroutine.
		// For now, we just wire it up.
	}

	return nil
}

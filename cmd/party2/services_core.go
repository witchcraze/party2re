package main

import (
	"context"
	"database/sql"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/player"
)

type coreServices struct {
	txProvider         database.TransactionProvider
	charRepo           *database.CharacterRepository
	invRepo            *database.InventoryRepository
	charJobRepo        *database.CharacterJobRepository
	playerRepo         *database.PlayerRepository
	playerAPITokenRepo *database.PlayerAPITokenRepository
	sessionRepo        player.SessionRepository
	dbMaintRepo        *database.MaintenanceRepository
	itemCatalog        *coreitem.Catalog
	jobCatalog         *corejob.Catalog
	charService        *character.Service
	playerService      *player.Service
}

func newCoreServices(db *sql.DB, valkeyClient valkeygo.Client) (*coreServices, error) {
	playerRepo, err := database.NewPlayerRepository(db)
	if err != nil {
		return nil, err
	}
	playerAPITokenRepo, err := database.NewPlayerAPITokenRepository(db)
	if err != nil {
		return nil, err
	}
	sessionRepo := player.NewValkeySessionRepository(valkeyClient)
	dbMaintRepo, err := database.NewMaintenanceRepository(db)
	if err != nil {
		return nil, err
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		return nil, err
	}
	txProvider := database.NewTransactionProvider(db)

	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		return nil, err
	}
	charJobRepo, err := database.NewCharacterJobRepository(db)
	if err != nil {
		return nil, err
	}

	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		return nil, err
	}
	jobCatalog, err := corejob.InitialCatalog()
	if err != nil {
		return nil, err
	}

	return &coreServices{
		txProvider:         txProvider,
		charRepo:           charRepo,
		invRepo:            invRepo,
		charJobRepo:        charJobRepo,
		playerRepo:         playerRepo,
		playerAPITokenRepo: playerAPITokenRepo,
		sessionRepo:        sessionRepo,
		dbMaintRepo:        dbMaintRepo,
		itemCatalog:        itemCatalog,
		jobCatalog:         jobCatalog,
	}, nil
}

func (c *coreServices) initPlayerAndChar(
	guildRepo *database.GuildRepository,
	fleamarketRepo *database.FleaMarketRepository,
	notificationService *notification.Service,
	logger logging.Logger,
) error {
	charService, err := character.NewService(
		c.charRepo,
		character.WithTransactionProvider(c.txProvider),
		character.WithNewsPublisher(character.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			_, err := notificationService.PublishNews(ctx, cat, title, content, author, pubAt)
			return err
		})),
		character.WithGuildChecker(character.GuildMembershipCheckerFunc(func(ctx context.Context, characterID string) (bool, error) {
			g, _, err := guildRepo.GetGuildByCharacter(ctx, characterID)
			if err != nil {
				return false, nil
			}
			return g.ID != "", nil
		})),
		character.WithFleaMarketChecker(character.FleaMarketCheckerFunc(func(ctx context.Context, characterID string) (bool, error) {
			count, err := fleamarketRepo.CountActiveListingsBySeller(ctx, characterID)
			if err != nil {
				return false, err
			}
			return count > 0, nil
		})),
	)
	if err != nil {
		return err
	}

	playerService, err := player.NewService(
		c.playerRepo,
		c.sessionRepo,
		player.WithCharacterService(charService),
		player.WithAPITokenRepository(c.playerAPITokenRepo),
		player.WithTransactionProvider(c.txProvider),
		player.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	c.charService = charService
	c.playerService = playerService
	return nil
}

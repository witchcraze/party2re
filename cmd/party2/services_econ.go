package main

import (
	"database/sql"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
	gemstore "github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/store"
)

type econServices struct {
	shop           *shop.Service
	depot          *depot.Service
	blacksmith     *blacksmith.Service
	alchemy        *alchemy.Service
	bank           *bank.Service
	auction        *auction.Service
	fleamarket     *fleamarket.Service
	gemStore       *gemstore.Service
	store          *store.Service
	depotRepo      *database.DepotRepository
	gemBoxRepo     *database.GemBoxRepository
	fleamarketRepo *database.FleaMarketRepository
	storeRepo      *database.StoreRepository
}

func newEconServices(db *sql.DB, core *coreServices) (*econServices, error) {
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		return nil, err
	}

	gemBoxRepo, err := database.NewGemBoxRepository(db)
	if err != nil {
		return nil, err
	}

	shopService, err := shop.NewService(
		core.charRepo,
		core.invRepo,
		core.itemCatalog,
		shop.WithTransactionProvider(core.txProvider),
		shop.WithDepotRepository(depotRepo),
	)
	if err != nil {
		return nil, err
	}
	depotService, err := depot.NewService(
		depotRepo,
		core.charRepo,
		core.invRepo,
		depot.WithEconomy(core.economy),
		depot.WithTransactionProvider(core.txProvider),
		depot.WithItemDefinitionProvider(core.itemCatalog),
	)
	if err != nil {
		return nil, err
	}

	blacksmithService, err := blacksmith.NewService(
		core.charRepo,
		core.invRepo,
		core.itemCatalog,
		blacksmith.WithEconomy(core.economy),
	)
	if err != nil {
		return nil, err
	}

	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		return nil, err
	}
	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		return nil, err
	}
	alchemyService, err := alchemy.NewServiceWithTransaction(core.charRepo, core.invRepo, alcRepo, recipeCatalog, core.itemCatalog)
	if err != nil {
		return nil, err
	}

	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		return nil, err
	}
	bankService, err := bank.NewService(bankRepo)
	if err != nil {
		return nil, err
	}

	auctionRepo, err := database.NewAuctionRepository(db)
	if err != nil {
		return nil, err
	}
	auctionService, err := auction.NewService(auctionRepo)
	if err != nil {
		return nil, err
	}

	fleamarketRepo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		return nil, err
	}
	fleamarketService, err := fleamarket.NewService(
		fleamarketRepo,
		core.charRepo,
		core.invRepo,
		fleamarket.WithItemDefinitionProvider(core.itemCatalog),
		fleamarket.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	gemCatalog, err := gemstore.DefaultCatalog()
	if err != nil {
		return nil, err
	}
	gemStoreService, err := gemstore.NewService(
		gemCatalog,
		core.charRepo,
		core.invRepo,
		gemstore.WithItemDefinitionProvider(core.itemCatalog),
		gemstore.WithTransactionProvider(core.txProvider),
		gemstore.WithGemBoxRepository(gemBoxRepo),
		gemstore.WithDepotRepository(depotRepo),
	)
	if err != nil {
		return nil, err
	}

	storeRepo, err := database.NewStoreRepository(db)
	if err != nil {
		return nil, err
	}
	storeService := store.NewService(
		storeRepo,
		core.charRepo,
		depotRepo,
		core.itemCatalog,
		core.txProvider,
	)

	return &econServices{
		shop:           shopService,
		depot:          depotService,
		blacksmith:     blacksmithService,
		alchemy:        alchemyService,
		bank:           bankService,
		auction:        auctionService,
		fleamarket:     fleamarketService,
		gemStore:       gemStoreService,
		store:          storeService,
		depotRepo:      depotRepo,
		gemBoxRepo:     gemBoxRepo,
		fleamarketRepo: fleamarketRepo,
		storeRepo:      storeRepo,
	}, nil
}

func (e *econServices) initStore(core *coreServices, gp store.GuildPointsRegistrar, t store.TimerService) {
	e.store = store.NewService(
		e.storeRepo,
		core.charRepo,
		e.depotRepo,
		core.itemCatalog,
		core.txProvider,
		store.WithGuildPoints(gp),
		store.WithTimer(t),
	)
}

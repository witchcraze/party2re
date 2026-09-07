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
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/shop"
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
	depotRepo      *database.DepotRepository
	fleamarketRepo *database.FleaMarketRepository
}

func newEconServices(db *sql.DB, core *coreServices) (*econServices, error) {
	shopService, err := shop.NewService(core.charRepo, core.invRepo, core.itemCatalog, shop.WithTransactionProvider(core.txProvider))
	if err != nil {
		return nil, err
	}

	depotRepo, err := database.NewDepotRepository(db)
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
	)
	if err != nil {
		return nil, err
	}

	return &econServices{
		shop:           shopService,
		depot:          depotService,
		blacksmith:     blacksmithService,
		alchemy:        alchemyService,
		bank:           bankService,
		auction:        auctionService,
		fleamarket:     fleamarketService,
		gemStore:       gemStoreService,
		depotRepo:      depotRepo,
		fleamarketRepo: fleamarketRepo,
	}, nil
}

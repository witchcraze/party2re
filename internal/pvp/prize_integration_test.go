package pvp_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
)

// sqlPrizeRemitter exercises the port with real ambient-transaction stores.
type sqlPrizeRemitter struct {
	depot *database.DepotRepository
	home  *home.Service
}

func newSQLPrizeRemitter(t *testing.T, db *sql.DB) sqlPrizeRemitter {
	t.Helper()
	d, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	h, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	c, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	s, err := home.NewService(h, c)
	if err != nil {
		t.Fatal(err)
	}
	return sqlPrizeRemitter{d, s}
}

func (r sqlPrizeRemitter) HoldPrize(ctx context.Context, characterID, arenaName string, amount int) error {
	if err := r.depot.AddRemittance(ctx, depot.Remittance{ID: id.New(), CharacterID: characterID, Amount: amount, Source: depot.RemittancePvPPrize, SenderName: arenaName}); err != nil {
		return err
	}
	return r.home.AddDeliveryFlag(ctx, characterID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}

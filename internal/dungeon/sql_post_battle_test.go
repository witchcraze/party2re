package dungeon_test

import (
	"database/sql"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/database"
)

// sqlPostBattle is the shared post-battle owner over the test database, as the
// composition root wires it for Dungeon credits.
func sqlPostBattle(t *testing.T, db *sql.DB) *battle.Service {
	t.Helper()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips),
		battle.WithTransactionProvider(database.NewTransactionProvider(db)),
	)
}
